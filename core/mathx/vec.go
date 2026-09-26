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
