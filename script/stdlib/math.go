package stdlib

import (
	"math"

	"aego/core/mathx"
	"aego/script/bind"
)

type unary struct {
	name    string
	doc     string
	example string
	fn      func(float64) float64
}

func installMath(r *bind.Registry) {
	m := r.Module(ModMath)
	bind.Const(m, "Pi", float32(math.Pi), bind.Doc("Pi is the ratio of a circle's circumference to its diameter, about 3.14159."))
	bind.Const(m, "Tau", float32(2*math.Pi), bind.Doc("Tau is a full turn in radians, 2·Pi."))
	for _, u := range []unary{
		{"Floor", "Floor returns the greatest whole number less than or equal to x.", "cell := math.Floor(pos.X / 32)", math.Floor},
		{"Ceil", "Ceil returns the least whole number greater than or equal to x.", "pages := math.Ceil(float(count) / 10)", math.Ceil},
		{"Round", "Round returns x rounded to the nearest whole number; halves round away from zero.", "snapped := math.Round(x)", math.Round},
		{"Trunc", "Trunc drops the fractional part of x.", "whole := math.Trunc(-2.7)", math.Trunc},
		{"Sqrt", "Sqrt returns the square root of x. Negative x gives NaN.", "d := math.Sqrt(dx*dx + dy*dy)", math.Sqrt},
		{"Sin", "Sin returns the sine of an angle in radians.", "y := math.Sin(time.Elapsed() * 2) * Amplitude", math.Sin},
		{"Cos", "Cos returns the cosine of an angle in radians.", "x := math.Cos(angle) * Radius", math.Cos},
		{"Tan", "Tan returns the tangent of an angle in radians.", "slope := math.Tan(math.Deg2Rad(30))", math.Tan},
		{"Asin", "Asin returns the angle in radians whose sine is x, in [-Pi/2, Pi/2].", "a := math.Asin(0.5)", math.Asin},
		{"Acos", "Acos returns the angle in radians whose cosine is x, in [0, Pi].", "a := math.Acos(math.Dot(u, v))", math.Acos},
		{"Exp", "Exp returns e raised to the power x.", "decay := math.Exp(-dt * Damping)", math.Exp},
		{"Log", "Log returns the natural logarithm of x.", "octave := math.Log(freq) / math.Log(2)", math.Log},
	} {
		bind.Func1(m, u.name, func(_ *bind.Ctx, x float64) float64 { return u.fn(x) },
			bind.Doc(u.doc), bind.Params("x"), bind.Example(u.example))
	}
	bind.Func1(m, "Abs", func(_ *bind.Ctx, x float32) float32 { return mathx.Abs(x) },
		bind.Doc("Abs returns the absolute value of x."), bind.Params("x"), bind.Example("dist := math.Abs(target - x)"))
	bind.Func1(m, "Sign", func(_ *bind.Ctx, x float32) float32 { return mathx.Sign(x) },
		bind.Doc("Sign returns -1, 0 or 1 depending on the sign of x."), bind.Params("x"), bind.Example("dir := math.Sign(input.Axis(\"horizontal\"))"))
	bind.Func2(m, "Min", func(_ *bind.Ctx, a, b float32) float32 { return min(a, b) },
		bind.Doc("Min returns the smaller of a and b."), bind.Params("a", "b"), bind.Example("speed := math.Min(speed, MaxSpeed)"))
	bind.Func2(m, "Max", func(_ *bind.Ctx, a, b float32) float32 { return max(a, b) },
		bind.Doc("Max returns the larger of a and b."), bind.Params("a", "b"), bind.Example("hp := math.Max(hp - damage, 0)"))
	bind.Func3(m, "Clamp", func(_ *bind.Ctx, v, lo, hi float32) float32 { return mathx.Clamp(v, lo, hi) },
		bind.Doc("Clamp limits v to the range [lo, hi]."), bind.Params("v", "lo", "hi"), bind.Example("x = math.Clamp(x, -10, 10)"))
	bind.Func3(m, "Lerp", func(_ *bind.Ctx, a, b, t float32) float32 { return mathx.Lerp(a, b, t) },
		bind.Doc("Lerp blends from a to b. t = 0 gives a, t = 1 gives b; t is not clamped."), bind.Params("a", "b", "t"), bind.Example("zoom = math.Lerp(zoom, target, 5 * dt)"))
	bind.Func2(m, "Atan2", func(_ *bind.Ctx, y, x float32) float32 { return mathx.Atan2(y, x) },
		bind.Doc("Atan2 returns the angle in radians of the vector (x, y), in (-Pi, Pi]."), bind.Params("y", "x"), bind.Example("angle := math.Atan2(d.Y, d.X)"))
	bind.Func2(m, "Pow", func(_ *bind.Ctx, x, y float64) float64 { return math.Pow(x, y) },
		bind.Doc("Pow returns x raised to the power y."), bind.Params("x", "y"), bind.Example("ease := math.Pow(t, 3)"))
	bind.Func1(m, "Deg2Rad", func(_ *bind.Ctx, d float32) float32 { return mathx.Deg2Rad(d) },
		bind.Doc("Deg2Rad converts degrees to radians."), bind.Params("degrees"), bind.Example("node.SetRotation(math.Deg2Rad(45))"))
	bind.Func1(m, "Rad2Deg", func(_ *bind.Ctx, r float32) float32 { return mathx.Rad2Deg(r) },
		bind.Doc("Rad2Deg converts radians to degrees."), bind.Params("radians"), bind.Example("log.Info(string(math.Rad2Deg(node.Rotation())))"))
	bind.Func1(m, "AbsInt", func(_ *bind.Ctx, x int64) int64 { return mathx.Abs(x) },
		bind.Doc("AbsInt returns the absolute value of an int."), bind.Params("x"), bind.Example("steps := math.AbsInt(to - from)"))
	bind.Func2(m, "MinInt", func(_ *bind.Ctx, a, b int64) int64 { return min(a, b) },
		bind.Doc("MinInt returns the smaller of two ints."), bind.Params("a", "b"), bind.Example("ammo = math.MinInt(ammo + 10, MaxAmmo)"))
	bind.Func2(m, "MaxInt", func(_ *bind.Ctx, a, b int64) int64 { return max(a, b) },
		bind.Doc("MaxInt returns the larger of two ints."), bind.Params("a", "b"), bind.Example("lives = math.MaxInt(lives - 1, 0)"))
	bind.Func3(m, "ClampInt", func(_ *bind.Ctx, v, lo, hi int64) int64 { return mathx.Clamp(v, lo, hi) },
		bind.Doc("ClampInt limits an int to the range [lo, hi]."), bind.Params("v", "lo", "hi"), bind.Example("level = math.ClampInt(level, 1, 10)"))
	installVectors(m)
}

func installVectors(m *bind.Module) {
	bind.Func1(m, "Length", func(_ *bind.Ctx, v mathx.Vec2) float32 { return v.Len() },
		bind.Doc("Length returns the length of a Vec2."), bind.Params("v"), bind.Example("speed := math.Length(velocity)"))
	bind.Func1(m, "Normalize", func(_ *bind.Ctx, v mathx.Vec2) mathx.Vec2 { return v.Normalized() },
		bind.Doc("Normalize returns v scaled to length 1. A zero vector stays zero."), bind.Params("v"), bind.Example("dir := math.Normalize(target - pos)"))
	bind.Func2(m, "Dot", func(_ *bind.Ctx, a, b mathx.Vec2) float32 { return a.Dot(b) },
		bind.Doc("Dot returns the dot product of two Vec2."), bind.Params("a", "b"), bind.Example("facing := math.Dot(forward, toPlayer) > 0"))
	bind.Func2(m, "Cross", func(_ *bind.Ctx, a, b mathx.Vec2) float32 { return a.Cross(b) },
		bind.Doc("Cross returns the z component of the 3D cross product; its sign tells on which side b lies."), bind.Params("a", "b"), bind.Example("left := math.Cross(forward, toTarget) < 0"))
	bind.Func2(m, "Dist", func(_ *bind.Ctx, a, b mathx.Vec2) float32 { return a.Sub(b).Len() },
		bind.Doc("Dist returns the distance between two points."), bind.Params("a", "b"), bind.Example("if math.Dist(pos, goal) < 0.1 {"))
	bind.Func2(m, "Rotate", func(_ *bind.Ctx, v mathx.Vec2, rad float32) mathx.Vec2 { return v.Rotated(rad) },
		bind.Doc("Rotate turns v clockwise by an angle in radians."), bind.Params("v", "radians"), bind.Example("offset := math.Rotate(Vec2{1, 0}, node.Rotation())"))
	bind.Func1(m, "Angle", func(_ *bind.Ctx, v mathx.Vec2) float32 { return mathx.Atan2(v.Y, v.X) },
		bind.Doc("Angle returns the direction of v in radians."), bind.Params("v"), bind.Example("node.SetRotation(math.Angle(velocity))"))
	bind.Func3(m, "LerpVec2", func(_ *bind.Ctx, a, b mathx.Vec2, t float32) mathx.Vec2 { return a.Lerp(b, t) },
		bind.Doc("LerpVec2 blends two Vec2 component-wise."), bind.Params("a", "b", "t"), bind.Example("cam = math.LerpVec2(cam, target, 4 * dt)"))
	bind.Func1(m, "Length3", func(_ *bind.Ctx, v mathx.Vec3) float32 { return v.Len() },
		bind.Doc("Length3 returns the length of a Vec3."), bind.Params("v"), bind.Example("d := math.Length3(velocity)"))
	bind.Func1(m, "Normalize3", func(_ *bind.Ctx, v mathx.Vec3) mathx.Vec3 { return v.Normalized() },
		bind.Doc("Normalize3 returns v scaled to length 1."), bind.Params("v"), bind.Example("dir := math.Normalize3(target - pos)"))
	bind.Func2(m, "Dot3", func(_ *bind.Ctx, a, b mathx.Vec3) float32 { return a.Dot(b) },
		bind.Doc("Dot3 returns the dot product of two Vec3."), bind.Params("a", "b"), bind.Example("lit := math.Dot3(normal, light) > 0"))
	bind.Func2(m, "Cross3", func(_ *bind.Ctx, a, b mathx.Vec3) mathx.Vec3 { return a.Cross(b) },
		bind.Doc("Cross3 returns a vector perpendicular to a and b."), bind.Params("a", "b"), bind.Example("right := math.Cross3(forward, Vec3{0, 1, 0})"))
	bind.Func2(m, "Dist3", func(_ *bind.Ctx, a, b mathx.Vec3) float32 { return a.Sub(b).Len() },
		bind.Doc("Dist3 returns the distance between two 3D points."), bind.Params("a", "b"), bind.Example("near := math.Dist3(a, b) < 2"))
	bind.Func3(m, "LerpVec3", func(_ *bind.Ctx, a, b mathx.Vec3, t float32) mathx.Vec3 { return a.Lerp(b, t) },
		bind.Doc("LerpVec3 blends two Vec3 component-wise."), bind.Params("a", "b", "t"), bind.Example("p := math.LerpVec3(from, to, t)"))
	bind.Func3(m, "QuatEuler", func(_ *bind.Ctx, x, y, z float32) mathx.Quat { return mathx.QuatEuler(x, y, z) },
		bind.Doc("QuatEuler builds a rotation from Euler angles in radians, applied as Z·Y·X."), bind.Params("x", "y", "z"), bind.Example("node.SetRotation3(math.QuatEuler(0, math.Pi, 0))"))
	bind.Func2(m, "QuatAxisAngle", func(_ *bind.Ctx, axis mathx.Vec3, rad float32) mathx.Quat { return mathx.QuatAxisAngle(axis, rad) },
		bind.Doc("QuatAxisAngle builds a rotation of an angle in radians around an axis."), bind.Params("axis", "radians"), bind.Example("spin := math.QuatAxisAngle(Vec3{0, 1, 0}, Speed * dt)"))
	bind.Func2(m, "QuatMul", func(_ *bind.Ctx, a, b mathx.Quat) mathx.Quat { return a.Mul(b) },
		bind.Doc("QuatMul combines two rotations: b is applied first, then a."), bind.Params("a", "b"), bind.Example("node.SetRotation3(math.QuatMul(spin, node.Rotation3()))"))
	bind.Func2(m, "QuatRotate", func(_ *bind.Ctx, q mathx.Quat, v mathx.Vec3) mathx.Vec3 { return q.Rotate(v) },
		bind.Doc("QuatRotate rotates a vector by q."), bind.Params("q", "v"), bind.Example("forward := math.QuatRotate(node.Rotation3(), Vec3{0, 0, -1})"))
	bind.Func3(m, "QuatSlerp", func(_ *bind.Ctx, a, b mathx.Quat, t float32) mathx.Quat { return a.Slerp(b, t) },
		bind.Doc("QuatSlerp blends two rotations along the shortest arc."), bind.Params("a", "b", "t"), bind.Example("r := math.QuatSlerp(from, to, 0.5)"))
	bind.Func1(m, "QuatLookDir", func(_ *bind.Ctx, d mathx.Vec3) mathx.Quat { return mathx.QuatLookDir(d) },
		bind.Doc("QuatLookDir returns the rotation that points -Z along d."), bind.Params("dir"), bind.Example("node.SetRotation3(math.QuatLookDir(target - pos))"))
}
