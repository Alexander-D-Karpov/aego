package module

import (
	"reflect"
	"unsafe"

	"aego/core/errs"
	"aego/core/log"
	"aego/engine/problems"
	"aego/schema"
)

type DataBuilder[T any] struct {
	r    *Registry
	desc *schema.TypeDescriptor
	err  error
}

type dataValidate func(unsafe.Pointer) []problems.Diagnostic

func DataType[T any](r *Registry, opts ...schema.Option) *DataBuilder[T] {
	b := &DataBuilder[T]{r: r}
	rt := reflect.TypeFor[T]()
	td, err := schema.RegisterData[T](r.Schema(), opts...)
	if err != nil {
		b.err = r.wrap(err, "data type "+rt.String()+" registration failed")
		return b
	}
	for i := range td.Fields {
		if fd := &td.Fields[i]; fd.Kind == schema.KindNodeRef || fd.Elem == schema.KindNodeRef {
			b.err = r.fail("data type " + td.FullName + " field " + fd.Name + " is a node reference; data assets live outside scenes")
			return b
		}
	}
	td.Owner, td.Builtin = r.Owner(), r.builtin()
	b.desc = td
	r.datas = append(r.datas, td)
	return b
}

func (b *DataBuilder[T]) Err() error {
	return b.err
}

func (b *DataBuilder[T]) Descriptor() *schema.TypeDescriptor {
	return b.desc
}

func (b *DataBuilder[T]) edit() *schema.TypeDescriptor {
	if b.desc == nil {
		return nil
	}
	if b.r.Schema().Frozen() {
		if b.err == nil {
			b.err = errs.New(errs.EngineFrozen, "data type "+b.desc.FullName+" is changed after the schema was frozen", log.F("module", b.r.Owner()))
		}
		return nil
	}
	return b.desc
}

func (b *DataBuilder[T]) Display(name string) *DataBuilder[T] {
	if d := b.edit(); d != nil {
		d.DisplayName = name
	}
	return b
}

func (b *DataBuilder[T]) Category(name string) *DataBuilder[T] {
	if d := b.edit(); d != nil {
		d.Category = name
	}
	return b
}

func (b *DataBuilder[T]) Default(fn func() T) *DataBuilder[T] {
	d := b.edit()
	if d == nil {
		return b
	}
	if fn == nil {
		if err := b.r.fail("data type " + d.FullName + " has a nil default"); b.err == nil {
			b.err = err
		}
		return b
	}
	d.Default = func() unsafe.Pointer {
		v := fn()
		return unsafe.Pointer(&v)
	}
	return b
}

func (b *DataBuilder[T]) Validate(fn func(*T) []problems.Diagnostic) *DataBuilder[T] {
	d := b.edit()
	if d == nil || fn == nil {
		return b
	}
	d.Validate = dataValidate(func(p unsafe.Pointer) []problems.Diagnostic { return fn((*T)(p)) })
	return b
}

func ValidateData(td *schema.TypeDescriptor, p unsafe.Pointer) []problems.Diagnostic {
	if h, ok := td.Validate.(dataValidate); ok {
		return h(p)
	}
	return nil
}

func (r *Registry) DataTypes() []*schema.TypeDescriptor {
	return r.datas
}
