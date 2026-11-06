package runtime

import (
	"errors"
	"unsafe"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/schema"
	"aego/script/bind"
	"aego/script/vm"
)

var (
	errBadComponent     = errors.New("component reference does not match the linked bindings")
	errReadOnly         = errors.New("component field is read-only")
	errUnsupportedField = errors.New("component field kind cannot be set from a script")
	faultComponentGone  = &vm.Fault{Code: vm.CodeNil, Message: "component is no longer on the node"}
	faultNodeGone       = &vm.Fault{Code: vm.CodeNil, Message: "component reference points to a destroyed node"}
)

func (r *Runtime) component(ref vm.Value, field int) (unsafe.Pointer, *schema.FieldDescriptor, error) {
	if r.scene == nil {
		return nil, nil, errNoScene
	}
	idx := int(ref.B)
	if idx >= len(r.comps) || field >= len(r.comps[idx].fields) {
		return nil, nil, errBadComponent
	}
	c := &r.comps[idx]
	h := bind.NodeOf(ref)
	if !r.scene.IsAlive(h) {
		return nil, nil, faultNodeGone
	}
	p, ok := r.scene.GetByType(h, c.typ)
	if !ok {
		return nil, nil, faultComponentGone
	}
	return p, c.fields[field], nil
}

func readInt(q unsafe.Pointer, size uintptr, unsigned bool) int64 {
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

func writeInt(q unsafe.Pointer, size uintptr, v int64) {
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

func (r *Runtime) nodeValue(id ids.NodeID) vm.Value {
	if id.IsZero() {
		return vm.Value{}
	}
	return bind.NodeValue(r.scene.Handle(id))
}

func (r *Runtime) nodeID(v vm.Value) ids.NodeID {
	h := bind.NodeOf(v)
	if h.IsZero() || !r.scene.IsAlive(h) {
		return ids.NodeID{}
	}
	return r.scene.ID(h)
}

// listValue copies a list field into a new VM slice on every read.
func (r *Runtime) listValue(q unsafe.Pointer, elem schema.Kind) vm.Value {
	buf := r.listBuf[:0]
	switch elem {
	case schema.KindVec2:
		for _, v := range *(*[]mathx.Vec2)(q) {
			buf = append(buf, bind.Vec2Value(v))
		}
	case schema.KindFloat32:
		for _, v := range *(*[]float32)(q) {
			buf = append(buf, vm.Float(v))
		}
	case schema.KindInt32:
		for _, v := range *(*[]int32)(q) {
			buf = append(buf, vm.Int(int64(v)))
		}
	case schema.KindInt64:
		for _, v := range *(*[]int64)(q) {
			buf = append(buf, vm.Int(v))
		}
	case schema.KindBool:
		for _, v := range *(*[]bool)(q) {
			buf = append(buf, vm.Bool(v))
		}
	}
	r.listBuf = buf
	if len(buf) == 0 {
		return vm.Value{}
	}
	return r.m.MakeSlice(1, 0, buf)
}

func (r *Runtime) GetField(ref vm.Value, field int, dst *vm.Value) error {
	p, fd, err := r.component(ref, field)
	if err != nil {
		return err
	}
	q := unsafe.Add(p, fd.Offset)
	switch fd.Kind {
	case schema.KindBool:
		*dst = vm.Bool(*(*bool)(q))
	case schema.KindInt32:
		*dst = vm.Int(int64(*(*int32)(q)))
	case schema.KindInt64:
		*dst = vm.Int(*(*int64)(q))
	case schema.KindFloat32:
		*dst = vm.Float(*(*float32)(q))
	case schema.KindFloat64:
		*dst = vm.Float(float32(*(*float64)(q)))
	case schema.KindVec2:
		*dst = bind.Vec2Value(*(*mathx.Vec2)(q))
	case schema.KindVec3:
		*dst = bind.Vec3Value(*(*mathx.Vec3)(q))
	case schema.KindColor:
		*dst = bind.ColorValue(*(*mathx.Color)(q))
	case schema.KindQuat:
		*dst = bind.QuatValue(*(*mathx.Quat)(q))
	case schema.KindRect:
		*dst = bind.RectValue(*(*mathx.Rect)(q))
	case schema.KindCurve:
		*dst = vm.Value{A: ref.A, B: uint64(uint32(ref.B)) | uint64(field)<<32}
	case schema.KindEnum:
		*dst = vm.Int(readInt(q, fd.Size, fd.Unsigned))
	case schema.KindString:
		*dst = r.m.HostString(*(*string)(q))
	case schema.KindList:
		*dst = r.listValue(q, fd.Elem)
	case schema.KindAssetRef:
		if !fd.Direct {
			return r.getSlow(p, fd, dst)
		}
		*dst = bind.AssetValue(*(*ids.AssetID)(q))
	case schema.KindNodeRef:
		if !fd.Direct {
			return r.getSlow(p, fd, dst)
		}
		*dst = r.nodeValue(*(*ids.NodeID)(q))
	default:
		return r.getSlow(p, fd, dst)
	}
	return nil
}

func (r *Runtime) SetField(ref vm.Value, field int, src vm.Value) error {
	p, fd, err := r.component(ref, field)
	if err != nil {
		return err
	}
	if fd.Flags.Has(schema.FlagReadOnly) || fd.Kind == schema.KindList {
		return errReadOnly
	}
	q := unsafe.Add(p, fd.Offset)
	switch fd.Kind {
	case schema.KindBool:
		*(*bool)(q) = src.Bool()
	case schema.KindInt32:
		*(*int32)(q) = int32(src.Int())
	case schema.KindInt64:
		*(*int64)(q) = src.Int()
	case schema.KindFloat32:
		*(*float32)(q) = src.Float()
	case schema.KindFloat64:
		*(*float64)(q) = float64(src.Float())
	case schema.KindVec2:
		*(*mathx.Vec2)(q) = bind.Vec2Of(src)
	case schema.KindVec3:
		*(*mathx.Vec3)(q) = bind.Vec3Of(src)
	case schema.KindColor:
		*(*mathx.Color)(q) = bind.ColorOf(src)
	case schema.KindQuat:
		*(*mathx.Quat)(q) = bind.QuatOf(src)
	case schema.KindRect:
		*(*mathx.Rect)(q) = bind.RectOf(src)
	case schema.KindEnum:
		writeInt(q, fd.Size, src.Int())
	case schema.KindString:
		s, _ := r.m.Str(src)
		*(*string)(q) = s
	case schema.KindAssetRef:
		if !fd.Direct {
			return r.setSlow(p, fd, src)
		}
		*(*ids.AssetID)(q) = bind.AssetOf(src)
	case schema.KindNodeRef:
		if !fd.Direct {
			return r.setSlow(p, fd, src)
		}
		*(*ids.NodeID)(q) = r.nodeID(src)
	default:
		return r.setSlow(p, fd, src)
	}
	return nil
}

func (r *Runtime) getSlow(p unsafe.Pointer, fd *schema.FieldDescriptor, dst *vm.Value) error {
	var v schema.Value
	fd.Get(p, &v)
	*dst = r.fromSchema(v)
	return nil
}

func (r *Runtime) setSlow(p unsafe.Pointer, fd *schema.FieldDescriptor, src vm.Value) error {
	sv, err := r.toSchema(fd.Kind, src)
	if err != nil {
		return err
	}
	return fd.Set(p, sv)
}

func (r *Runtime) fromSchema(v schema.Value) vm.Value {
	switch v.Kind {
	case schema.KindEnum, schema.KindInt32, schema.KindInt64:
		return vm.Int(v.I)
	case schema.KindString:
		return r.m.HostString(v.S)
	case schema.KindAssetRef:
		return bind.AssetValue(v.Asset)
	case schema.KindNodeRef:
		return r.nodeValue(v.Node)
	}
	return vm.Value{}
}

func (r *Runtime) toSchema(k schema.Kind, v vm.Value) (schema.Value, error) {
	switch k {
	case schema.KindEnum:
		return schema.Enum(v.Int()), nil
	case schema.KindString:
		s, _ := r.m.Str(v)
		return schema.String(s), nil
	case schema.KindAssetRef:
		return schema.Asset(bind.AssetOf(v)), nil
	case schema.KindNodeRef:
		return schema.Node(r.nodeID(v)), nil
	}
	return schema.Value{}, errUnsupportedField
}
