package schema

import (
	"reflect"
	"unsafe"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
)

func badValue(field string, got, want Kind) error {
	return errs.New(errs.SchemaBadValue, "value kind does not match the field",
		log.F("field", field), log.F("got", got.String()), log.F("want", want.String()))
}

func (r *Registry) makeAccessors(fd *FieldDescriptor, ft reflect.Type, off uintptr) error {
	name := fd.Name
	switch fd.Kind {
	case KindBool:
		fd.Get = func(p unsafe.Pointer, dst *Value) {
			*dst = Bool(*(*bool)(unsafe.Add(p, off)))
		}
		fd.Set = func(p unsafe.Pointer, v Value) error {
			if v.Kind != KindBool {
				return badValue(name, v.Kind, KindBool)
			}
			*(*bool)(unsafe.Add(p, off)) = v.I != 0
			return nil
		}
	case KindInt32:
		fd.Get = func(p unsafe.Pointer, dst *Value) {
			*dst = Int32(*(*int32)(unsafe.Add(p, off)))
		}
		fd.Set = func(p unsafe.Pointer, v Value) error {
			*(*int32)(unsafe.Add(p, off)) = int32(v.AsInt())
			return nil
		}
	case KindInt64:
		fd.Get = func(p unsafe.Pointer, dst *Value) {
			*dst = Int64(*(*int64)(unsafe.Add(p, off)))
		}
		fd.Set = func(p unsafe.Pointer, v Value) error {
			*(*int64)(unsafe.Add(p, off)) = v.AsInt()
			return nil
		}
	case KindFloat32:
		fd.Get = func(p unsafe.Pointer, dst *Value) {
			*dst = Float32(*(*float32)(unsafe.Add(p, off)))
		}
		fd.Set = func(p unsafe.Pointer, v Value) error {
			*(*float32)(unsafe.Add(p, off)) = float32(v.AsFloat())
			return nil
		}
	case KindFloat64:
		fd.Get = func(p unsafe.Pointer, dst *Value) {
			*dst = Float64(*(*float64)(unsafe.Add(p, off)))
		}
		fd.Set = func(p unsafe.Pointer, v Value) error {
			*(*float64)(unsafe.Add(p, off)) = v.AsFloat()
			return nil
		}
	case KindVec2:
		fd.Get = func(p unsafe.Pointer, dst *Value) {
			*dst = Vec2(*(*vec2)(unsafe.Add(p, off)))
		}
		fd.Set = func(p unsafe.Pointer, v Value) error {
			if v.Kind != KindVec2 {
				return badValue(name, v.Kind, KindVec2)
			}
			*(*vec2)(unsafe.Add(p, off)) = v.V
			return nil
		}
	case KindVec3:
		fd.Get = func(p unsafe.Pointer, dst *Value) {
			*dst = Vec3(*(*vec3)(unsafe.Add(p, off)))
		}
		fd.Set = func(p unsafe.Pointer, v Value) error {
			if v.Kind != KindVec3 {
				return badValue(name, v.Kind, KindVec3)
			}
			*(*vec3)(unsafe.Add(p, off)) = v.V3
			return nil
		}
	case KindVec4:
		fd.Get = func(p unsafe.Pointer, dst *Value) {
			*dst = Vec4(*(*vec4)(unsafe.Add(p, off)))
		}
		fd.Set = func(p unsafe.Pointer, v Value) error {
			if v.Kind != KindVec4 {
				return badValue(name, v.Kind, KindVec4)
			}
			*(*vec4)(unsafe.Add(p, off)) = v.V4
			return nil
		}
	case KindQuat:
		fd.Get = func(p unsafe.Pointer, dst *Value) {
			*dst = Quat(*(*quat)(unsafe.Add(p, off)))
		}
		fd.Set = func(p unsafe.Pointer, v Value) error {
			if v.Kind != KindQuat {
				return badValue(name, v.Kind, KindQuat)
			}
			*(*quat)(unsafe.Add(p, off)) = v.Q
			return nil
		}
	case KindColor:
		fd.Get = func(p unsafe.Pointer, dst *Value) {
			*dst = Color(*(*color)(unsafe.Add(p, off)))
		}
		fd.Set = func(p unsafe.Pointer, v Value) error {
			if v.Kind != KindColor {
				return badValue(name, v.Kind, KindColor)
			}
			*(*color)(unsafe.Add(p, off)) = v.C
			return nil
		}
	case KindRect:
		fd.Get = func(p unsafe.Pointer, dst *Value) {
			*dst = Rect(*(*rect)(unsafe.Add(p, off)))
		}
		fd.Set = func(p unsafe.Pointer, v Value) error {
			if v.Kind != KindRect {
				return badValue(name, v.Kind, KindRect)
			}
			*(*rect)(unsafe.Add(p, off)) = v.R
			return nil
		}
	case KindString:
		fd.Get = func(p unsafe.Pointer, dst *Value) {
			*dst = String(*(*string)(unsafe.Add(p, off)))
		}
		fd.Set = func(p unsafe.Pointer, v Value) error {
			if v.Kind != KindString {
				return badValue(name, v.Kind, KindString)
			}
			*(*string)(unsafe.Add(p, off)) = v.S
			return nil
		}
	case KindEnum:
		return makeEnumAccessors(fd, ft, off)
	case KindAssetRef:
		fd.Get = func(p unsafe.Pointer, dst *Value) {
			ref := reflect.NewAt(ft, unsafe.Add(p, off)).Interface().(AssetRef)
			*dst = Asset(ref.AssetRefID())
		}
		fd.Set = func(p unsafe.Pointer, v Value) error {
			if v.Kind != KindAssetRef {
				return badValue(name, v.Kind, KindAssetRef)
			}
			ref := reflect.NewAt(ft, unsafe.Add(p, off)).Interface().(AssetRef)
			ref.SetAssetRefID(v.Asset)
			return nil
		}
	case KindNodeRef:
		fd.Get = func(p unsafe.Pointer, dst *Value) {
			ref := reflect.NewAt(ft, unsafe.Add(p, off)).Interface().(NodeRef)
			*dst = Node(ref.NodeRefID())
		}
		fd.Set = func(p unsafe.Pointer, v Value) error {
			if v.Kind != KindNodeRef {
				return badValue(name, v.Kind, KindNodeRef)
			}
			ref := reflect.NewAt(ft, unsafe.Add(p, off)).Interface().(NodeRef)
			ref.SetNodeRefID(v.Node)
			return nil
		}
	case KindList:
		return r.makeListAccessors(fd, ft, off)
	case KindStruct:
		return r.makeStructAccessors(fd, ft, off)
	default:
		return errs.New(errs.SchemaBadField, "field kind has no accessor", log.F("field", name))
	}
	return nil
}

func makeEnumAccessors(fd *FieldDescriptor, ft reflect.Type, off uintptr) error {
	switch ft.Kind() {
	case reflect.Int8:
		fd.Get = func(p unsafe.Pointer, dst *Value) { *dst = Enum(int64(*(*int8)(unsafe.Add(p, off)))) }
		fd.Set = func(p unsafe.Pointer, v Value) error {
			*(*int8)(unsafe.Add(p, off)) = int8(v.AsInt())
			return nil
		}
	case reflect.Int16:
		fd.Get = func(p unsafe.Pointer, dst *Value) { *dst = Enum(int64(*(*int16)(unsafe.Add(p, off)))) }
		fd.Set = func(p unsafe.Pointer, v Value) error {
			*(*int16)(unsafe.Add(p, off)) = int16(v.AsInt())
			return nil
		}
	case reflect.Int32:
		fd.Get = func(p unsafe.Pointer, dst *Value) { *dst = Enum(int64(*(*int32)(unsafe.Add(p, off)))) }
		fd.Set = func(p unsafe.Pointer, v Value) error {
			*(*int32)(unsafe.Add(p, off)) = int32(v.AsInt())
			return nil
		}
	case reflect.Int64, reflect.Int:
		fd.Get = func(p unsafe.Pointer, dst *Value) { *dst = Enum(*(*int64)(unsafe.Add(p, off))) }
		fd.Set = func(p unsafe.Pointer, v Value) error {
			*(*int64)(unsafe.Add(p, off)) = v.AsInt()
			return nil
		}
	case reflect.Uint8:
		fd.Get = func(p unsafe.Pointer, dst *Value) { *dst = Enum(int64(*(*uint8)(unsafe.Add(p, off)))) }
		fd.Set = func(p unsafe.Pointer, v Value) error {
			*(*uint8)(unsafe.Add(p, off)) = uint8(v.AsInt())
			return nil
		}
	case reflect.Uint16:
		fd.Get = func(p unsafe.Pointer, dst *Value) { *dst = Enum(int64(*(*uint16)(unsafe.Add(p, off)))) }
		fd.Set = func(p unsafe.Pointer, v Value) error {
			*(*uint16)(unsafe.Add(p, off)) = uint16(v.AsInt())
			return nil
		}
	case reflect.Uint32:
		fd.Get = func(p unsafe.Pointer, dst *Value) { *dst = Enum(int64(*(*uint32)(unsafe.Add(p, off)))) }
		fd.Set = func(p unsafe.Pointer, v Value) error {
			*(*uint32)(unsafe.Add(p, off)) = uint32(v.AsInt())
			return nil
		}
	default:
		return errs.New(errs.SchemaBadField, "enum must have an integer underlying type",
			log.F("field", fd.Name), log.F("kind", ft.Kind().String()))
	}
	return nil
}

func (r *Registry) makeListAccessors(fd *FieldDescriptor, ft reflect.Type, off uintptr) error {
	elem := ft.Elem()
	elemKind := fd.Elem
	structID := fd.Struct
	fd.Get = func(p unsafe.Pointer, dst *Value) {
		rv := reflect.NewAt(ft, unsafe.Add(p, off)).Elem()
		out := Value{Kind: KindList}
		if rv.Len() == 0 {
			*dst = out
			return
		}
		out.List = make([]Value, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out.List[i] = r.valueFrom(rv.Index(i), elemKind, structID)
		}
		*dst = out
	}
	fd.Set = func(p unsafe.Pointer, v Value) error {
		if v.Kind != KindList {
			return badValue(fd.Name, v.Kind, KindList)
		}
		rv := reflect.NewAt(ft, unsafe.Add(p, off)).Elem()
		slice := reflect.MakeSlice(ft, len(v.List), len(v.List))
		for i := range v.List {
			if err := r.assignTo(slice.Index(i), v.List[i], elemKind, structID, elem); err != nil {
				return err
			}
		}
		rv.Set(slice)
		return nil
	}
	return nil
}

func (r *Registry) makeStructAccessors(fd *FieldDescriptor, ft reflect.Type, off uintptr) error {
	id := fd.Struct
	fd.Get = func(p unsafe.Pointer, dst *Value) {
		td, ok := r.ByID(id)
		if !ok {
			*dst = Value{Kind: KindStruct}
			return
		}
		inner := unsafe.Add(p, off)
		obj := &Object{Type: id, Fields: make([]Value, len(td.Fields))}
		for i := range td.Fields {
			td.Fields[i].Get(inner, &obj.Fields[i])
		}
		*dst = Value{Kind: KindStruct, Obj: obj}
	}
	fd.Set = func(p unsafe.Pointer, v Value) error {
		if v.Kind != KindStruct || v.Obj == nil {
			return badValue(fd.Name, v.Kind, KindStruct)
		}
		td, ok := r.ByID(id)
		if !ok {
			return errs.New(errs.SchemaUnknownType, "nested struct type is not registered",
				log.F("field", fd.Name))
		}
		inner := unsafe.Add(p, off)
		n := min(len(td.Fields), len(v.Obj.Fields))
		for i := 0; i < n; i++ {
			if err := td.Fields[i].Set(inner, v.Obj.Fields[i]); err != nil {
				return err
			}
		}
		return nil
	}
	return nil
}

func (r *Registry) valueFrom(rv reflect.Value, k Kind, structID TypeID) Value {
	switch k {
	case KindBool:
		return Bool(rv.Bool())
	case KindInt32:
		return Int32(int32(rv.Int()))
	case KindInt64:
		return Int64(rv.Int())
	case KindFloat32:
		return Float32(float32(rv.Float()))
	case KindFloat64:
		return Float64(rv.Float())
	case KindString:
		return String(rv.String())
	case KindEnum:
		if rv.CanInt() {
			return Enum(rv.Int())
		}
		return Enum(int64(rv.Uint()))
	case KindVec2:
		return Vec2(rv.Interface().(vec2))
	case KindVec3:
		return Vec3(rv.Interface().(vec3))
	case KindVec4:
		return Vec4(rv.Interface().(vec4))
	case KindQuat:
		return Quat(rv.Interface().(quat))
	case KindColor:
		return Color(rv.Interface().(color))
	case KindRect:
		return Rect(rv.Interface().(rect))
	case KindAssetRef:
		return Asset(rv.Addr().Interface().(AssetRef).AssetRefID())
	case KindNodeRef:
		return Node(rv.Addr().Interface().(NodeRef).NodeRefID())
	case KindStruct:
		td, ok := r.ByID(structID)
		if !ok {
			return Value{Kind: KindStruct}
		}
		p := rv.Addr().UnsafePointer()
		obj := &Object{Type: structID, Fields: make([]Value, len(td.Fields))}
		for i := range td.Fields {
			td.Fields[i].Get(p, &obj.Fields[i])
		}
		return Value{Kind: KindStruct, Obj: obj}
	}
	return Value{}
}

func (r *Registry) assignTo(rv reflect.Value, v Value, k Kind, structID TypeID, rt reflect.Type) error {
	switch k {
	case KindBool:
		rv.SetBool(v.I != 0)
	case KindInt32, KindInt64:
		rv.SetInt(v.AsInt())
	case KindFloat32, KindFloat64:
		rv.SetFloat(v.AsFloat())
	case KindString:
		rv.SetString(v.S)
	case KindEnum:
		if rv.CanInt() {
			rv.SetInt(v.AsInt())
		} else {
			rv.SetUint(uint64(v.AsInt()))
		}
	case KindVec2:
		rv.Set(reflect.ValueOf(v.V))
	case KindVec3:
		rv.Set(reflect.ValueOf(v.V3))
	case KindVec4:
		rv.Set(reflect.ValueOf(v.V4))
	case KindQuat:
		rv.Set(reflect.ValueOf(v.Q))
	case KindColor:
		rv.Set(reflect.ValueOf(v.C))
	case KindRect:
		rv.Set(reflect.ValueOf(v.R))
	case KindAssetRef:
		rv.Addr().Interface().(AssetRef).SetAssetRefID(v.Asset)
	case KindNodeRef:
		rv.Addr().Interface().(NodeRef).SetNodeRefID(v.Node)
	case KindStruct:
		td, ok := r.ByID(structID)
		if !ok || v.Obj == nil {
			return errs.New(errs.SchemaBadValue, "cannot assign struct element")
		}
		p := rv.Addr().UnsafePointer()
		n := min(len(td.Fields), len(v.Obj.Fields))
		for i := 0; i < n; i++ {
			if err := td.Fields[i].Set(p, v.Obj.Fields[i]); err != nil {
				return err
			}
		}
	default:
		return errs.New(errs.SchemaBadValue, "unsupported list element kind", log.F("kind", k.String()))
	}
	return nil
}

var _ = ids.AssetID{}
