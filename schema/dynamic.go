package schema

import (
	"sync/atomic"
	"unsafe"

	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
)

type dynamicLayer struct {
	byID   map[TypeID]*TypeDescriptor
	byName map[string]*TypeDescriptor
	order  []*TypeDescriptor
}

type dynamicSub struct {
	id uint64
	fn func()
}

type dynamic struct {
	layer   atomic.Pointer[dynamicLayer]
	subs    []dynamicSub
	nextSub uint64
}

func zeroOf(k Kind) Value {
	v := Value{Kind: k}
	if k == KindQuat {
		v.Q = mathx.QuatIdentity()
	}
	return v
}

func bagAccessors(fd *FieldDescriptor) {
	name, kind := fd.Name, fd.Kind
	def := zeroOf(kind)
	if fd.HasDefault {
		def = fd.Default
	}
	fd.Get = func(p unsafe.Pointer, dst *Value) {
		if v, ok := (*PropertyBag)(p).Get(name); ok && v.Kind == kind {
			*dst = v
			return
		}
		*dst = def
	}
	fd.Set = func(p unsafe.Pointer, v Value) error {
		if v.Kind != kind {
			return badValue(name, v.Kind, kind)
		}
		(*PropertyBag)(p).Set(name, v)
		return nil
	}
}

func NewScriptType(full string, former []string, fields []FieldDescriptor) (*TypeDescriptor, error) {
	if full == "" {
		return nil, errs.New(errs.SchemaBadField, "script type needs a name")
	}
	td := &TypeDescriptor{
		ID:       HashName(full),
		FullName: full,
		Former:   former,
		Version:  1,
		Kind:     TypeScript,
		Space:    SpaceAny,
		index:    map[string]int{},
		byID:     map[FieldID]int{},
	}
	for i := range fields {
		fd := fields[i]
		if fd.Name == "" {
			return nil, errs.New(errs.SchemaBadField, "script field needs a name", log.F("type", full))
		}
		if _, dup := td.index[fd.Name]; dup {
			return nil, errs.New(errs.SchemaDuplicate, "duplicate script field", log.F("type", full), log.F("field", fd.Name))
		}
		fd.ID = HashField(td.ID, fd.Name)
		if _, clash := td.byID[fd.ID]; clash {
			return nil, errs.New(errs.SchemaCollision, "script field name hash collides", log.F("type", full), log.F("field", fd.Name))
		}
		bagAccessors(&fd)
		td.index[fd.Name] = len(td.Fields)
		td.byID[fd.ID] = len(td.Fields)
		td.Fields = append(td.Fields, fd)
	}
	for i := range td.Fields {
		for _, alias := range td.Fields[i].Former {
			if _, dup := td.index[alias]; dup {
				return nil, errs.New(errs.SchemaDuplicate, "former field name collides with a field",
					log.F("type", full), log.F("former", alias))
			}
			td.index[alias] = i
		}
	}
	return td, nil
}

func (r *Registry) buildDynamic(types []*TypeDescriptor) (*dynamicLayer, error) {
	if !r.frozen {
		return nil, errs.New(errs.SchemaFrozen, "the native layer must be frozen before script types are set")
	}
	l := &dynamicLayer{
		byID:   make(map[TypeID]*TypeDescriptor, len(types)),
		byName: make(map[string]*TypeDescriptor, len(types)),
	}
	claim := func(td *TypeDescriptor, name string) error {
		if other, ok := r.byName[name]; ok {
			return errs.New(errs.SchemaDuplicate, "script type name collides with a native type",
				log.F("type", td.FullName), log.F("name", name), log.F("native", other.FullName))
		}
		if other, ok := l.byName[name]; ok && other != td {
			return errs.New(errs.SchemaDuplicate, "two script types use the same name",
				log.F("name", name), log.F("a", other.FullName), log.F("b", td.FullName))
		}
		id := HashName(name)
		if other, ok := r.byID[id]; ok {
			return errs.New(errs.SchemaCollision, "script type name hash collides with a native type",
				log.F("name", name), log.F("native", other.FullName))
		}
		if other, ok := l.byID[id]; ok && other != td {
			return errs.New(errs.SchemaCollision, "script type name hash collides",
				log.F("name", name), log.F("other", other.FullName))
		}
		l.byName[name] = td
		l.byID[id] = td
		return nil
	}
	for _, td := range types {
		if td == nil || td.Kind != TypeScript {
			return nil, errs.New(errs.SchemaBadValue, "only script types can be set dynamically")
		}
		if err := claim(td, td.FullName); err != nil {
			return nil, err
		}
		for _, alias := range td.Former {
			if err := claim(td, alias); err != nil {
				return nil, err
			}
		}
		l.order = append(l.order, td)
	}
	return l, nil
}

func (r *Registry) CheckDynamic(types []*TypeDescriptor) error {
	_, err := r.buildDynamic(types)
	return err
}

func (r *Registry) SetDynamic(types []*TypeDescriptor) error {
	l, err := r.buildDynamic(types)
	if err != nil {
		return err
	}
	r.dyn.layer.Store(l)
	for i := range r.dyn.subs {
		r.dyn.subs[i].fn()
	}
	return nil
}

func (r *Registry) Scripts() []*TypeDescriptor {
	if l := r.dyn.layer.Load(); l != nil {
		return l.order
	}
	return nil
}

func (r *Registry) Subscribe(fn func()) func() {
	r.dyn.nextSub++
	id := r.dyn.nextSub
	r.dyn.subs = append(r.dyn.subs, dynamicSub{id: id, fn: fn})
	return func() {
		for i := range r.dyn.subs {
			if r.dyn.subs[i].id == id {
				r.dyn.subs = append(r.dyn.subs[:i], r.dyn.subs[i+1:]...)
				return
			}
		}
	}
}
