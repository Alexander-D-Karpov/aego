package schema

import (
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
)

var kindNames = [...]string{
	"invalid", "bool", "int32", "int64", "float32", "float64",
	"vec2", "color", "rect", "string", "enum", "assetRef", "nodeRef", "list", "struct",
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
		KindVec2, KindColor, KindRect, KindEnum:
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
	C     mathx.Color
	R     mathx.Rect
	S     string
	Asset ids.AssetID
	Node  ids.NodeID
	List  []Value
	Obj   *Object
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
