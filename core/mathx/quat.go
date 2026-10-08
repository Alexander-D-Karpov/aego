package mathx

type Quat struct{ X, Y, Z, W float32 }

func QuatIdentity() Quat {
	return Quat{W: 1}
}

func QuatAxisAngle(axis Vec3, rad float32) Quat {
	a := axis.Normalized()
	if a == (Vec3{}) {
		return QuatIdentity()
	}
	s, c := Sin(rad/2), Cos(rad/2)
	return Quat{X: a.X * s, Y: a.Y * s, Z: a.Z * s, W: c}
}

func QuatEuler(x, y, z float32) Quat {
	qx := QuatAxisAngle(Vec3{X: 1}, x)
	qy := QuatAxisAngle(Vec3{Y: 1}, y)
	qz := QuatAxisAngle(Vec3{Z: 1}, z)
	return qz.Mul(qy).Mul(qx)
}

func QuatLookAt(forward, up Vec3) Quat {
	f := forward.Normalized()
	if f == (Vec3{}) {
		return QuatIdentity()
	}
	z := f.Neg()
	x := up.Cross(z)
	if x.LenSq() < 1e-12 {
		alt := Vec3{Z: 1}
		if Abs(z.Z) > 0.9 {
			alt = Vec3{X: 1}
		}
		x = alt.Cross(z)
	}
	x = x.Normalized()
	y := z.Cross(x)
	return quatFromAxes(x, y, z)
}

func quatFromAxes(x, y, z Vec3) Quat {
	r00, r01, r02 := x.X, y.X, z.X
	r10, r11, r12 := x.Y, y.Y, z.Y
	r20, r21, r22 := x.Z, y.Z, z.Z
	var q Quat
	switch tr := r00 + r11 + r22; {
	case tr > 0:
		s := Sqrt(tr+1) * 2
		q = Quat{W: 0.25 * s, X: (r21 - r12) / s, Y: (r02 - r20) / s, Z: (r10 - r01) / s}
	case r00 > r11 && r00 > r22:
		s := Sqrt(1+r00-r11-r22) * 2
		q = Quat{W: (r21 - r12) / s, X: 0.25 * s, Y: (r01 + r10) / s, Z: (r02 + r20) / s}
	case r11 > r22:
		s := Sqrt(1+r11-r00-r22) * 2
		q = Quat{W: (r02 - r20) / s, X: (r01 + r10) / s, Y: 0.25 * s, Z: (r12 + r21) / s}
	default:
		s := Sqrt(1+r22-r00-r11) * 2
		q = Quat{W: (r10 - r01) / s, X: (r02 + r20) / s, Y: (r12 + r21) / s, Z: 0.25 * s}
	}
	return q.Normalized()
}

func (q Quat) IsZero() bool {
	return q == Quat{}
}

func (q Quat) Dot(o Quat) float32 {
	return q.X*o.X + q.Y*o.Y + q.Z*o.Z + q.W*o.W
}

func (q Quat) Len() float32 {
	return Sqrt(q.Dot(q))
}

func (q Quat) Neg() Quat {
	return Quat{X: -q.X, Y: -q.Y, Z: -q.Z, W: -q.W}
}

func (q Quat) Normalized() Quat {
	l := q.Len()
	if l == 0 {
		return QuatIdentity()
	}
	return Quat{X: q.X / l, Y: q.Y / l, Z: q.Z / l, W: q.W / l}
}

func (q Quat) Conjugate() Quat {
	return Quat{X: -q.X, Y: -q.Y, Z: -q.Z, W: q.W}
}

func (q Quat) Inverse() Quat {
	n := q.Dot(q)
	if n == 0 {
		return QuatIdentity()
	}
	c := q.Conjugate()
	return Quat{X: c.X / n, Y: c.Y / n, Z: c.Z / n, W: c.W / n}
}

func (q Quat) Mul(o Quat) Quat {
	return Quat{
		X: q.W*o.X + q.X*o.W + q.Y*o.Z - q.Z*o.Y,
		Y: q.W*o.Y - q.X*o.Z + q.Y*o.W + q.Z*o.X,
		Z: q.W*o.Z + q.X*o.Y - q.Y*o.X + q.Z*o.W,
		W: q.W*o.W - q.X*o.X - q.Y*o.Y - q.Z*o.Z,
	}
}

func (q Quat) Rotate(v Vec3) Vec3 {
	u := Vec3{X: q.X, Y: q.Y, Z: q.Z}
	t := u.Cross(v).Scale(2)
	return v.Add(t.Scale(q.W)).Add(u.Cross(t))
}

func (q Quat) axes() (Vec3, Vec3, Vec3) {
	xx, yy, zz := q.X*q.X, q.Y*q.Y, q.Z*q.Z
	xy, xz, yz := q.X*q.Y, q.X*q.Z, q.Y*q.Z
	wx, wy, wz := q.W*q.X, q.W*q.Y, q.W*q.Z
	return Vec3{X: 1 - 2*(yy+zz), Y: 2 * (xy + wz), Z: 2 * (xz - wy)},
		Vec3{X: 2 * (xy - wz), Y: 1 - 2*(xx+zz), Z: 2 * (yz + wx)},
		Vec3{X: 2 * (xz + wy), Y: 2 * (yz - wx), Z: 1 - 2*(xx+yy)}
}

func (q Quat) Right() Vec3 {
	x, _, _ := q.Normalized().axes()
	return x
}

func (q Quat) Up() Vec3 {
	_, y, _ := q.Normalized().axes()
	return y
}

func (q Quat) Forward() Vec3 {
	_, _, z := q.Normalized().axes()
	return z.Neg()
}

func (q Quat) Euler() (x, y, z float32) {
	c0, c1, c2 := q.Normalized().axes()
	r20 := c0.Z
	switch {
	case r20 <= -0.99999:
		return Atan2(c1.X, c1.Y), Pi / 2, 0
	case r20 >= 0.99999:
		return Atan2(-c1.X, c1.Y), -Pi / 2, 0
	}
	return Atan2(c1.Z, c2.Z), Asin(-r20), Atan2(c0.Y, c0.X)
}

func (q Quat) Slerp(o Quat, t float32) Quat {
	d := q.Dot(o)
	if d < 0 {
		o = o.Neg()
		d = -d
	}
	if d > 0.9995 {
		return Quat{
			X: Lerp(q.X, o.X, t),
			Y: Lerp(q.Y, o.Y, t),
			Z: Lerp(q.Z, o.Z, t),
			W: Lerp(q.W, o.W, t),
		}.Normalized()
	}
	theta := Acos(d)
	s := Sin(theta)
	a := Sin((1-t)*theta) / s
	b := Sin(t*theta) / s
	return Quat{
		X: q.X*a + o.X*b,
		Y: q.Y*a + o.Y*b,
		Z: q.Z*a + o.Z*b,
		W: q.W*a + o.W*b,
	}
}

func (q Quat) Mat4() Mat4 {
	return Mat4TRS(Vec3{}, q, Vec3{X: 1, Y: 1, Z: 1})
}

func (q Quat) NearlySame(o Quat, eps float32) bool {
	return Abs(Abs(q.Normalized().Dot(o.Normalized()))-1) <= eps
}
