package anim

import (
	"aego/core/ids"
	"aego/format/aetext"
	"aego/schema"
)

type Value struct {
	L [4]float32
	I int64
	A ids.AssetID
}

func Float(f float32) Value      { return Value{L: [4]float32{f}} }
func Lanes(l [4]float32) Value   { return Value{L: l} }
func Int(i int64) Value          { return Value{I: i} }
func Asset(id ids.AssetID) Value { return Value{A: id} }
func (v Value) Truth() bool      { return v.I != 0 }

func Bool(b bool) Value {
	if b {
		return Value{I: 1}
	}
	return Value{}
}

type class uint8

const (
	classNone class = iota
	classLanes
	classInt
	classDiscrete
	classQuat
	classAsset
)

func classOf(k schema.Kind) class {
	switch k {
	case schema.KindFloat32, schema.KindFloat64, schema.KindVec2, schema.KindVec3, schema.KindVec4, schema.KindColor, schema.KindRect:
		return classLanes
	case schema.KindInt32, schema.KindInt64:
		return classInt
	case schema.KindBool, schema.KindEnum:
		return classDiscrete
	case schema.KindQuat:
		return classQuat
	case schema.KindAssetRef:
		return classAsset
	}
	return classNone
}

func lanesOf(k schema.Kind) int {
	switch k {
	case schema.KindFloat32, schema.KindFloat64:
		return 1
	case schema.KindVec2:
		return 2
	case schema.KindVec3:
		return 3
	}
	return 4
}

func Supported(k schema.Kind) bool {
	return classOf(k) != classNone
}

func Interpolates(k schema.Kind) bool {
	c := classOf(k)
	return c == classLanes || c == classQuat
}

// LaneCount is the number of float lanes of a lane kind, 0 for discrete kinds.
func LaneCount(k schema.Kind) int {
	if classOf(k) != classLanes && classOf(k) != classQuat {
		return 0
	}
	return lanesOf(k)
}

// ValueText writes v the way a key stores it in .aeanim.
func ValueText(kind schema.Kind, v Value) aetext.Value {
	return writeValue(kind, v)
}
