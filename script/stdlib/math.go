package stdlib

import (
	"math"

	"aego/core/mathx"
	"aego/script/bind"
)

func installMath(r *bind.Registry) {
	m := r.Module(ModMath)
	bind.Const(m, "Pi", float32(math.Pi))
	bind.Const(m, "Tau", float32(2*math.Pi))
	for _, u := range []struct {
		name string
		fn   func(float64) float64
	}{
		{"Floor", math.Floor}, {"Ceil", math.Ceil}, {"Round", math.Round}, {"Trunc", math.Trunc},
		{"Sqrt", math.Sqrt}, {"Sin", math.Sin}, {"Cos", math.Cos}, {"Tan", math.Tan},
		{"Asin", math.Asin}, {"Acos", math.Acos}, {"Exp", math.Exp}, {"Log", math.Log},
	} {
		bind.Func1(m, u.name, func(_ *bind.Ctx, x float64) float64 { return u.fn(x) })
	}
	bind.Func1(m, "Abs", func(_ *bind.Ctx, x float32) float32 { return mathx.Abs(x) })
	bind.Func1(m, "Sign", func(_ *bind.Ctx, x float32) float32 { return mathx.Sign(x) })
	bind.Func2(m, "Min", func(_ *bind.Ctx, a, b float32) float32 { return min(a, b) })
	bind.Func2(m, "Max", func(_ *bind.Ctx, a, b float32) float32 { return max(a, b) })
	bind.Func3(m, "Clamp", func(_ *bind.Ctx, v, lo, hi float32) float32 { return mathx.Clamp(v, lo, hi) })
	bind.Func3(m, "Lerp", func(_ *bind.Ctx, a, b, t float32) float32 { return mathx.Lerp(a, b, t) })
	bind.Func2(m, "Atan2", func(_ *bind.Ctx, y, x float32) float32 { return mathx.Atan2(y, x) })
	bind.Func2(m, "Pow", func(_ *bind.Ctx, x, y float64) float64 { return math.Pow(x, y) })
	bind.Func1(m, "Deg2Rad", func(_ *bind.Ctx, d float32) float32 { return mathx.Deg2Rad(d) })
	bind.Func1(m, "Rad2Deg", func(_ *bind.Ctx, r float32) float32 { return mathx.Rad2Deg(r) })
	bind.Func1(m, "AbsInt", func(_ *bind.Ctx, x int64) int64 { return mathx.Abs(x) })
	bind.Func2(m, "MinInt", func(_ *bind.Ctx, a, b int64) int64 { return min(a, b) })
	bind.Func2(m, "MaxInt", func(_ *bind.Ctx, a, b int64) int64 { return max(a, b) })
	bind.Func3(m, "ClampInt", func(_ *bind.Ctx, v, lo, hi int64) int64 { return mathx.Clamp(v, lo, hi) })
	installVectors(m)
}

func installVectors(m *bind.Module) {
	bind.Func1(m, "Length", func(_ *bind.Ctx, v mathx.Vec2) float32 { return v.Len() })
	bind.Func1(m, "Normalize", func(_ *bind.Ctx, v mathx.Vec2) mathx.Vec2 { return v.Normalized() })
	bind.Func2(m, "Dot", func(_ *bind.Ctx, a, b mathx.Vec2) float32 { return a.Dot(b) })
	bind.Func2(m, "Cross", func(_ *bind.Ctx, a, b mathx.Vec2) float32 { return a.Cross(b) })
	bind.Func2(m, "Dist", func(_ *bind.Ctx, a, b mathx.Vec2) float32 { return a.Sub(b).Len() })
	bind.Func2(m, "Rotate", func(_ *bind.Ctx, v mathx.Vec2, rad float32) mathx.Vec2 { return v.Rotated(rad) })
	bind.Func1(m, "Angle", func(_ *bind.Ctx, v mathx.Vec2) float32 { return mathx.Atan2(v.Y, v.X) })
	bind.Func3(m, "LerpVec2", func(_ *bind.Ctx, a, b mathx.Vec2, t float32) mathx.Vec2 { return a.Lerp(b, t) })
	bind.Func1(m, "Length3", func(_ *bind.Ctx, v mathx.Vec3) float32 { return v.Len() })
	bind.Func1(m, "Normalize3", func(_ *bind.Ctx, v mathx.Vec3) mathx.Vec3 { return v.Normalized() })
	bind.Func2(m, "Dot3", func(_ *bind.Ctx, a, b mathx.Vec3) float32 { return a.Dot(b) })
	bind.Func2(m, "Cross3", func(_ *bind.Ctx, a, b mathx.Vec3) mathx.Vec3 { return a.Cross(b) })
	bind.Func2(m, "Dist3", func(_ *bind.Ctx, a, b mathx.Vec3) float32 { return a.Sub(b).Len() })
	bind.Func3(m, "LerpVec3", func(_ *bind.Ctx, a, b mathx.Vec3, t float32) mathx.Vec3 { return a.Lerp(b, t) })
	bind.Func3(m, "QuatEuler", func(_ *bind.Ctx, x, y, z float32) mathx.Quat { return mathx.QuatEuler(x, y, z) })
	bind.Func2(m, "QuatAxisAngle", func(_ *bind.Ctx, axis mathx.Vec3, rad float32) mathx.Quat { return mathx.QuatAxisAngle(axis, rad) })
	bind.Func2(m, "QuatMul", func(_ *bind.Ctx, a, b mathx.Quat) mathx.Quat { return a.Mul(b) })
	bind.Func2(m, "QuatRotate", func(_ *bind.Ctx, q mathx.Quat, v mathx.Vec3) mathx.Vec3 { return q.Rotate(v) })
	bind.Func3(m, "QuatSlerp", func(_ *bind.Ctx, a, b mathx.Quat, t float32) mathx.Quat { return a.Slerp(b, t) })
	bind.Func1(m, "QuatLookDir", func(_ *bind.Ctx, d mathx.Vec3) mathx.Quat { return mathx.QuatLookDir(d) })
}
