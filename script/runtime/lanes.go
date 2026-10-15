package runtime

import (
	"aego/core/mathx"
	"aego/schema"
	"aego/script/abi"
	"aego/script/bytecode"
)

func laneDefault(k bytecode.Const, typ string) (schema.Value, bool) {
	t, ok := abi.Parse(typ)
	if !ok || k.Kind != bytecode.KRaw {
		return schema.Value{}, false
	}
	l := k.Lanes()
	switch t.Kind {
	case abi.Vec2:
		return schema.Vec2(mathx.Vec2{X: l[0], Y: l[1]}), true
	case abi.Vec3:
		return schema.Value{Kind: schema.KindVec3, V3: mathx.Vec3{X: l[0], Y: l[1], Z: l[2]}}, true
	case abi.Color:
		return schema.Value{Kind: schema.KindColor, C: mathx.Color{R: l[0], G: l[1], B: l[2], A: l[3]}}, true
	case abi.Quat:
		return schema.Value{Kind: schema.KindQuat, Q: mathx.Quat{X: l[0], Y: l[1], Z: l[2], W: l[3]}}, true
	case abi.Rect:
		return schema.Value{Kind: schema.KindRect, R: mathx.Rect{X: l[0], Y: l[1], W: l[2], H: l[3]}}, true
	}
	return schema.Value{}, false
}
