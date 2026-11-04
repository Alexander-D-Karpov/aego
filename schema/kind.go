package schema

import (
	"aego/core/curve"
	"aego/core/ids"
	"aego/core/mathx"
)

type TypeID uint64

type FieldID uint64

type Kind uint8

const (
	KindInvalid Kind = iota
	KindBool
	KindInt32
	KindInt64
	KindFloat32
	KindFloat64
	KindVec2
	KindColor
	KindRect
	KindString
	KindEnum
	KindAssetRef
	KindNodeRef
	KindList
	KindStruct
	KindVec3
	KindVec4
	KindQuat
	KindCurve
)

var kindNames = [...]string{
	"invalid", "bool", "int32", "int64", "float32", "float64",
	"vec2", "color", "rect", "string", "enum", "assetRef", "nodeRef", "list", "struct",
	"vec3", "vec4", "quat", "curve",
}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "unknown"
}

func (k Kind) IsPOD() bool {
	switch k {
	case KindBool, KindInt32, KindInt64, KindFloat32, KindFloat64,
		KindVec2, KindVec3, KindVec4, KindQuat, KindColor, KindRect, KindEnum:
		return true
	}
	return false
}

type EnumValue struct {
	Name  string
	Value int64
}

type Object struct {
	Type   TypeID
	Fields []Value
}

type Value struct {
	Kind  Kind
	I     int64
	F     float64
	V     mathx.Vec2
	V3    mathx.Vec3
	V4    mathx.Vec4
	Q     mathx.Quat
	C     mathx.Color
	R     mathx.Rect
	S     string
	Asset ids.AssetID
	Node  ids.NodeID
	List  []Value
	Obj   *Object
	Cv    curve.Curve
}

func Curve(c curve.Curve) Value {
	return Value{Kind: KindCurve, Cv: c.Clone()}
}

func Bool(v bool) Value {
	var i int64
	if v {
		i = 1
	}
	return Value{Kind: KindBool, I: i}
}

func Int32(v int32) Value {
	return Value{Kind: KindInt32, I: int64(v)}
}

func Int64(v int64) Value {
	return Value{Kind: KindInt64, I: v}
}

func Float32(v float32) Value {
	return Value{Kind: KindFloat32, F: float64(v)}
}

func Float64(v float64) Value {
	return Value{Kind: KindFloat64, F: v}
}

func Vec2(v mathx.Vec2) Value {
	return Value{Kind: KindVec2, V: v}
}

func Vec3(v mathx.Vec3) Value {
	return Value{Kind: KindVec3, V3: v}
}

func Vec4(v mathx.Vec4) Value {
	return Value{Kind: KindVec4, V4: v}
}

func Quat(v mathx.Quat) Value {
	return Value{Kind: KindQuat, Q: v}
}

func Color(v mathx.Color) Value {
	return Value{Kind: KindColor, C: v}
}

func Rect(v mathx.Rect) Value {
	return Value{Kind: KindRect, R: v}
}

func String(v string) Value {
	return Value{Kind: KindString, S: v}
}

func Enum(v int64) Value {
	return Value{Kind: KindEnum, I: v}
}

func Asset(id ids.AssetID) Value {
	return Value{Kind: KindAssetRef, Asset: id}
}

func Node(id ids.NodeID) Value {
	return Value{Kind: KindNodeRef, Node: id}
}

func List(items ...Value) Value {
	return Value{Kind: KindList, List: items}
}

func (v Value) AsBool() bool {
	return v.I != 0
}

func (v Value) AsInt() int64 {
	switch v.Kind {
	case KindFloat32, KindFloat64:
		return int64(v.F)
	}
	return v.I
}

func (v Value) AsFloat() float64 {
	switch v.Kind {
	case KindFloat32, KindFloat64:
		return v.F
	}
	return float64(v.I)
}

// Empty reports the zero semantic value for opt-in source serialization. It is
// independent of native memory layout and never inspects cached reference data.
func (v Value) Empty() bool {
	switch v.Kind {
	case KindInvalid:
		return true
	case KindBool, KindInt32, KindInt64, KindEnum:
		return v.I == 0
	case KindFloat32, KindFloat64:
		return v.F == 0
	case KindString:
		return v.S == ""
	case KindAssetRef:
		return v.Asset.IsZero()
	case KindNodeRef:
		return v.Node.IsZero()
	case KindList:
		return len(v.List) == 0
	case KindStruct:
		return v.Obj == nil
	case KindVec2:
		return v.V == (mathx.Vec2{})
	case KindVec3:
		return v.V3 == (mathx.Vec3{})
	case KindVec4:
		return v.V4 == (mathx.Vec4{})
	case KindColor:
		return v.C == (mathx.Color{})
	case KindQuat:
		return v.Q == (mathx.Quat{})
	case KindRect:
		return v.R == (mathx.Rect{})
	}
	return false
}
