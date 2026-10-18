package bind

import (
	"fmt"
	"reflect"
	"unsafe"

	"aego/script/abi"
	"aego/script/vm"
)

type StructField struct {
	Name  string
	Type  abi.TypeRef
	Slot  int
	Slots int
}

type StructDesc struct {
	Module string
	Name   string
	Fields []StructField
	Slots  int
	leaves []leaf
}

func (d *StructDesc) Qualified() string {
	return d.Module + "." + d.Name
}

type leafKind uint8

const (
	lBool leafKind = iota
	lInt
	lUint
	lF32
	lF64
	lVec2
	lVec3
	lVec4
	lNode
	lAsset
)

type leaf struct {
	kind leafKind
	size uintptr
	off  uintptr
	off2 uintptr
	slot int
}

var assetKinds = map[reflect.Type]string{tTex: "texture", tScene: "scene", tPrefab: "prefab", tMesh: "mesh"}

func fieldOff(t reflect.Type, name string) uintptr {
	f, _ := t.FieldByName(name)
	return f.Offset
}

func Struct[T any](m *Module, name string, opts ...Option) *StructDesc {
	rt := reflect.TypeFor[T]()
	switch {
	case m.method:
		m.valid(name, fmt.Errorf("structs cannot live in a method module"))
		return nil
	case rt.Kind() != reflect.Struct:
		m.valid(name, fmt.Errorf("%s is not a struct", rt))
		return nil
	}
	if _, dup := m.reg.structs[rt]; dup {
		//lint:ignore ST1005 "Go" is a proper noun
		m.valid(name, fmt.Errorf("Go type %s is already registered as a script struct", rt))
		return nil
	}
	d := &StructDesc{Module: m.path, Name: name}
	def := &abi.StructDef{Module: m.path, Name: name}
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		if !sf.IsExported() {
			m.valid(name, fmt.Errorf("field %s is not exported", sf.Name))
			return nil
		}
		ref, ls, n, err := m.reg.leafFor(sf.Type)
		if err != nil {
			m.valid(name, fmt.Errorf("field %s: %w", sf.Name, err))
			return nil
		}
		for _, l := range ls {
			l.off += sf.Offset
			l.off2 += sf.Offset
			l.slot += d.Slots
			d.leaves = append(d.leaves, l)
		}
		d.Fields = append(d.Fields, StructField{Name: sf.Name, Type: ref, Slot: d.Slots, Slots: n})
		def.Fields = append(def.Fields, abi.StructField{Name: sf.Name, Type: ref})
		d.Slots += n
	}
	def.Slots = d.Slots
	if !m.valid(name, m.reg.table.AddStruct(def)) {
		return nil
	}
	m.reg.structs[rt] = d
	m.valid(name, m.reg.describe(m.path, name, opts, -1))
	return d
}

func (r *Registry) leafFor(t reflect.Type) (abi.TypeRef, []leaf, int, error) {
	one := func(k abi.Kind, lk leafKind) (abi.TypeRef, []leaf, int, error) {
		return abi.T(k), []leaf{{kind: lk}}, 1, nil
	}
	switch t {
	case tVec2:
		return one(abi.Vec2, lVec2)
	case tVec3:
		return one(abi.Vec3, lVec3)
	case tColor:
		return one(abi.Color, lVec4)
	case tQuat:
		return one(abi.Quat, lVec4)
	case tRect:
		return one(abi.Rect, lVec4)
	case tNode:
		return abi.T(abi.NodeRef), []leaf{{kind: lNode, off: fieldOff(t, "Index"), off2: fieldOff(t, "Gen")}}, 1, nil
	}
	if kind, ok := assetKinds[t]; ok {
		id, _ := t.FieldByName("ID")
		l := leaf{kind: lAsset, off: id.Offset + fieldOff(id.Type, "Hi"), off2: id.Offset + fieldOff(id.Type, "Lo")}
		return abi.AssetOf(kind), []leaf{l}, 1, nil
	}
	if kind, ok := r.refs[t]; ok {
		l := leaf{kind: lAsset, off: fieldOff(idType, "Hi"), off2: fieldOff(idType, "Lo")}
		return abi.AssetOf(kind), []leaf{l}, 1, nil
	}
	if d, ok := r.structs[t]; ok {
		return abi.StructOf(d.Qualified()), append([]leaf(nil), d.leaves...), d.Slots, nil
	}
	switch t.Kind() {
	case reflect.Bool:
		return one(abi.Bool, lBool)
	case reflect.Float32:
		return one(abi.Float, lF32)
	case reflect.Float64:
		return one(abi.Float, lF64)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		ref, err := intRef(r, t)
		return ref, []leaf{{kind: lInt, size: t.Size()}}, 1, err
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		ref, err := intRef(r, t)
		return ref, []leaf{{kind: lUint, size: t.Size()}}, 1, err
	}
	//lint:ignore ST1005 "Go" is a proper noun
	return abi.TypeRef{}, nil, 0, fmt.Errorf("Go type %s cannot be a field of a script struct", t)
}

func loadInt(q unsafe.Pointer, size uintptr, unsigned bool) int64 {
	switch size {
	case 1:
		if unsigned {
			return int64(*(*uint8)(q))
		}
		return int64(*(*int8)(q))
	case 2:
		if unsigned {
			return int64(*(*uint16)(q))
		}
		return int64(*(*int16)(q))
	case 4:
		if unsigned {
			return int64(*(*uint32)(q))
		}
		return int64(*(*int32)(q))
	}
	return *(*int64)(q)
}

func storeInt(q unsafe.Pointer, size uintptr, v int64) {
	switch size {
	case 1:
		*(*uint8)(q) = uint8(v)
	case 2:
		*(*uint16)(q) = uint16(v)
	case 4:
		*(*uint32)(q) = uint32(v)
	default:
		*(*int64)(q) = v
	}
}

func encodeLeaves(p unsafe.Pointer, ls []leaf, w []vm.Value) {
	for i := range ls {
		l := &ls[i]
		q := unsafe.Add(p, l.off)
		var v vm.Value
		switch l.kind {
		case lBool:
			v = vm.Bool(*(*bool)(q))
		case lInt:
			v = vm.Int(loadInt(q, l.size, false))
		case lUint:
			v = vm.Int(loadInt(q, l.size, true))
		case lF32:
			v = vm.Float(*(*float32)(q))
		case lF64:
			v = vm.Float(float32(*(*float64)(q)))
		case lVec2:
			f := (*[2]float32)(q)
			v = vm.Vec(f[0], f[1], 0, 0)
		case lVec3:
			f := (*[3]float32)(q)
			v = vm.Vec(f[0], f[1], f[2], 0)
		case lVec4:
			f := (*[4]float32)(q)
			v = vm.Vec(f[0], f[1], f[2], f[3])
		case lNode:
			v = vm.Value{A: uint64(*(*uint32)(unsafe.Add(p, l.off2)))<<32 | uint64(*(*uint32)(q))}
		case lAsset:
			v = vm.Value{A: *(*uint64)(q), B: *(*uint64)(unsafe.Add(p, l.off2))}
		}
		w[l.slot] = v
	}
}

func decodeLeaves(p unsafe.Pointer, ls []leaf, w []vm.Value) {
	for i := range ls {
		l := &ls[i]
		q := unsafe.Add(p, l.off)
		v := w[l.slot]
		switch l.kind {
		case lBool:
			*(*bool)(q) = v.Bool()
		case lInt, lUint:
			storeInt(q, l.size, v.Int())
		case lF32:
			*(*float32)(q) = v.Float()
		case lF64:
			*(*float64)(q) = float64(v.Float())
		case lVec2:
			f := (*[2]float32)(q)
			f[0], f[1] = v.Lane(0), v.Lane(1)
		case lVec3:
			f := (*[3]float32)(q)
			f[0], f[1], f[2] = v.Lane(0), v.Lane(1), v.Lane(2)
		case lVec4:
			f := (*[4]float32)(q)
			f[0], f[1], f[2], f[3] = v.Lane(0), v.Lane(1), v.Lane(2), v.Lane(3)
		case lNode:
			*(*uint32)(q) = uint32(v.A)
			*(*uint32)(unsafe.Add(p, l.off2)) = uint32(v.A >> 32)
		case lAsset:
			*(*uint64)(q) = v.A
			*(*uint64)(unsafe.Add(p, l.off2)) = v.B
		}
	}
}

func structCodec[T any](c *codec[T], d *StructDesc) {
	ls := d.leaves
	c.ref, c.slots = abi.StructOf(d.Qualified()), d.Slots
	c.get = func(_ *vm.Machine, w []vm.Value) T {
		var v T
		decodeLeaves(unsafe.Pointer(&v), ls, w)
		return v
	}
	c.put = func(_ *vm.Machine, w []vm.Value, v T) {
		encodeLeaves(unsafe.Pointer(&v), ls, w)
	}
}

type Packer[T any] struct {
	c codec[T]
}

func NewPacker[T any](r *Registry) (Packer[T], error) {
	c, err := codecOf[T](r)
	if err != nil {
		return Packer[T]{}, err
	}
	if !abi.Plain(c.ref) {
		return Packer[T]{}, fmt.Errorf("%s cannot be packed without a VM heap", c.ref)
	}
	return Packer[T]{c: c}, nil
}

func (p Packer[T]) Ref() abi.TypeRef {
	return p.c.ref
}

func (p Packer[T]) Slots() int {
	return p.c.slots
}

func (p Packer[T]) Put(w []vm.Value, v T) {
	p.c.put(nil, w, v)
}

func (p Packer[T]) Get(w []vm.Value) T {
	return p.c.get(nil, w)
}
