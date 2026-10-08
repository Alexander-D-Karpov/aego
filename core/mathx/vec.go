package mathx

type Vec2 struct{ X, Y float32 }

type Vec3 struct{ X, Y, Z float32 }

func V2(x, y float32) Vec2 {
	return Vec2{X: x, Y: y}
}

func (v Vec2) Add(o Vec2) Vec2 {
	return Vec2{X: v.X + o.X, Y: v.Y + o.Y}
}

func (v Vec2) Sub(o Vec2) Vec2 {
	return Vec2{X: v.X - o.X, Y: v.Y - o.Y}
}

func (v Vec2) Scale(s float32) Vec2 {
	return Vec2{X: v.X * s, Y: v.Y * s}
}

func (v Vec2) Mul(o Vec2) Vec2 {
	return Vec2{X: v.X * o.X, Y: v.Y * o.Y}
}

func (v Vec2) Neg() Vec2 {
	return Vec2{X: -v.X, Y: -v.Y}
}

func (v Vec2) Dot(o Vec2) float32 {
	return v.X*o.X + v.Y*o.Y
}

func (v Vec2) Cross(o Vec2) float32 {
	return v.X*o.Y - v.Y*o.X
}

func (v Vec2) LenSq() float32 {
	return v.X*v.X + v.Y*v.Y
}

func (v Vec2) Len() float32 {
	return Sqrt(v.LenSq())
}

func (v Vec2) Normalized() Vec2 {
	l := v.Len()
	if l == 0 {
		return Vec2{}
	}
	return Vec2{X: v.X / l, Y: v.Y / l}
}

func (v Vec2) Rotated(rad float32) Vec2 {
	c, s := Cos(rad), Sin(rad)
	return Vec2{X: v.X*c - v.Y*s, Y: v.X*s + v.Y*c}
}

func (v Vec2) Lerp(o Vec2, t float32) Vec2 {
	return Vec2{X: Lerp(v.X, o.X, t), Y: Lerp(v.Y, o.Y, t)}
}

func (v Vec2) NearlyEqual(o Vec2, eps float32) bool {
	return NearlyEqual(v.X, o.X, eps) && NearlyEqual(v.Y, o.Y, eps)
}

func (v Vec3) Add(o Vec3) Vec3 {
	return Vec3{X: v.X + o.X, Y: v.Y + o.Y, Z: v.Z + o.Z}
}

func (v Vec3) Sub(o Vec3) Vec3 {
	return Vec3{X: v.X - o.X, Y: v.Y - o.Y, Z: v.Z - o.Z}
}

func (v Vec3) Scale(s float32) Vec3 {
	return Vec3{X: v.X * s, Y: v.Y * s, Z: v.Z * s}
}

func (v Vec3) Dot(o Vec3) float32 {
	return v.X*o.X + v.Y*o.Y + v.Z*o.Z
}

func (v Vec3) Cross(o Vec3) Vec3 {
	return Vec3{
		X: v.Y*o.Z - v.Z*o.Y,
		Y: v.Z*o.X - v.X*o.Z,
		Z: v.X*o.Y - v.Y*o.X,
	}
}

func (v Vec3) Len() float32 {
	return Sqrt(v.Dot(v))
}

func (v Vec3) Normalized() Vec3 {
	l := v.Len()
	if l == 0 {
		return Vec3{}
	}
	return v.Scale(1 / l)
}

type Vec4 struct{ X, Y, Z, W float32 }

func V3(x, y, z float32) Vec3 {
	return Vec3{X: x, Y: y, Z: z}
}

func V4(x, y, z, w float32) Vec4 {
	return Vec4{X: x, Y: y, Z: z, W: w}
}

func (v Vec3) Mul(o Vec3) Vec3 {
	return Vec3{X: v.X * o.X, Y: v.Y * o.Y, Z: v.Z * o.Z}
}

func (v Vec3) Neg() Vec3 {
	return Vec3{X: -v.X, Y: -v.Y, Z: -v.Z}
}

func (v Vec3) Abs() Vec3 {
	return Vec3{X: Abs(v.X), Y: Abs(v.Y), Z: Abs(v.Z)}
}

func (v Vec3) LenSq() float32 {
	return v.Dot(v)
}

func (v Vec3) Lerp(o Vec3, t float32) Vec3 {
	return Vec3{X: Lerp(v.X, o.X, t), Y: Lerp(v.Y, o.Y, t), Z: Lerp(v.Z, o.Z, t)}
}

func (v Vec3) NearlyEqual(o Vec3, eps float32) bool {
	return NearlyEqual(v.X, o.X, eps) && NearlyEqual(v.Y, o.Y, eps) && NearlyEqual(v.Z, o.Z, eps)
}

func (v Vec3) XY() Vec2 {
	return Vec2{X: v.X, Y: v.Y}
}

func Vec3Min(a, b Vec3) Vec3 {
	return Vec3{X: min(a.X, b.X), Y: min(a.Y, b.Y), Z: min(a.Z, b.Z)}
}

func Vec3Max(a, b Vec3) Vec3 {
	return Vec3{X: max(a.X, b.X), Y: max(a.Y, b.Y), Z: max(a.Z, b.Z)}
}

func (v Vec4) Add(o Vec4) Vec4 {
	return Vec4{X: v.X + o.X, Y: v.Y + o.Y, Z: v.Z + o.Z, W: v.W + o.W}
}

func (v Vec4) Sub(o Vec4) Vec4 {
	return Vec4{X: v.X - o.X, Y: v.Y - o.Y, Z: v.Z - o.Z, W: v.W - o.W}
}

func (v Vec4) Scale(s float32) Vec4 {
	return Vec4{X: v.X * s, Y: v.Y * s, Z: v.Z * s, W: v.W * s}
}

func (v Vec4) Dot(o Vec4) float32 {
	return v.X*o.X + v.Y*o.Y + v.Z*o.Z + v.W*o.W
}

func (v Vec4) Len() float32 {
	return Sqrt(v.Dot(v))
}

func (v Vec4) XYZ() Vec3 {
	return Vec3{X: v.X, Y: v.Y, Z: v.Z}
}

func (v Vec4) NearlyEqual(o Vec4, eps float32) bool {
	return NearlyEqual(v.X, o.X, eps) && NearlyEqual(v.Y, o.Y, eps) &&
		NearlyEqual(v.Z, o.Z, eps) && NearlyEqual(v.W, o.W, eps)
}
