package runtime

import (
	"aego/core/ids"
	"errors"
	"unsafe"

	"aego/core/mathx"
	"aego/schema"
	"aego/script/bind"
	"aego/script/vm"
)

var (
	errBadComponent     = errors.New("component reference does not match the linked bindings")
	errComponentGone    = errors.New("component is no longer on the node")
	errReadOnly         = errors.New("component field is read-only")
	errUnsupportedField = errors.New("component field kind cannot be set from a script")
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
	p, ok := r.scene.GetByType(bind.NodeOf(ref), c.typ)
	if !ok {
		return nil, nil, errComponentGone
	}
	return p, c.fields[field], nil
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
	default:
		var v schema.Value
		fd.Get(p, &v)
		*dst = r.fromSchema(v)
	}
	return nil
}

func (r *Runtime) SetField(ref vm.Value, field int, src vm.Value) error {
	p, fd, err := r.component(ref, field)
	if err != nil {
		return err
	}
	if fd.Flags.Has(schema.FlagReadOnly) {
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
	default:
		sv, err := r.toSchema(fd.Kind, src)
		if err != nil {
			return err
		}
		return fd.Set(p, sv)
	}
	return nil
}

func (r *Runtime) fromSchema(v schema.Value) vm.Value {
	switch v.Kind {
	case schema.KindEnum, schema.KindInt32, schema.KindInt64:
		return vm.Int(v.I)
	case schema.KindString:
		return r.m.NewString(v.S)
	case schema.KindAssetRef:
		return bind.AssetValue(v.Asset)
	case schema.KindNodeRef:
		if v.Node.IsZero() {
			return vm.Value{}
		}
		return bind.NodeValue(r.scene.Handle(v.Node))
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
		h := bind.NodeOf(v)
		if h.IsZero() || !r.scene.IsAlive(h) {
			return schema.Node(ids.NodeID{}), nil
		}
		return schema.Node(r.scene.ID(h)), nil
	}
	return schema.Value{}, errUnsupportedField
}
