package mathx

type Mat4 [16]float32

func Mat4Identity() Mat4 {
	return Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
}

func Mat4Ortho(left, right, bottom, top, near, far float32) Mat4 {
	var m Mat4
	m[0] = 2 / (right - left)
	m[5] = 2 / (top - bottom)
	m[10] = -2 / (far - near)
	m[12] = -(right + left) / (right - left)
	m[13] = -(top + bottom) / (top - bottom)
	m[14] = -(far + near) / (far - near)
	m[15] = 1
	return m
}

func Mat4FromMat3(m Mat3) Mat4 {
	return Mat4{
		m[0], m[1], 0, 0,
		m[3], m[4], 0, 0,
		0, 0, 1, 0,
		m[6], m[7], 0, 1,
	}
}

func (m Mat4) Mul(o Mat4) Mat4 {
	var r Mat4
	for c := 0; c < 4; c++ {
		for row := 0; row < 4; row++ {
			var sum float32
			for k := 0; k < 4; k++ {
				sum += m[k*4+row] * o[c*4+k]
			}
			r[c*4+row] = sum
		}
	}
	return r
}

func (m Mat4) Apply(p Vec2) Vec2 {
	x := m[0]*p.X + m[4]*p.Y + m[12]
	y := m[1]*p.X + m[5]*p.Y + m[13]
	w := m[3]*p.X + m[7]*p.Y + m[15]
	if w != 0 {
		return Vec2{X: x / w, Y: y / w}
	}
	return Vec2{X: x, Y: y}
}

func Mat4Translate(p Vec3) Mat4 {
	m := Mat4Identity()
	m[12], m[13], m[14] = p.X, p.Y, p.Z
	return m
}

func Mat4Scale(s Vec3) Mat4 {
	return Mat4{s.X, 0, 0, 0, 0, s.Y, 0, 0, 0, 0, s.Z, 0, 0, 0, 0, 1}
}

func Mat4TRS(pos Vec3, rot Quat, scale Vec3) Mat4 {
	x, y, z := rot.Normalized().axes()
	x, y, z = x.Scale(scale.X), y.Scale(scale.Y), z.Scale(scale.Z)
	return Mat4{
		x.X, x.Y, x.Z, 0,
		y.X, y.Y, y.Z, 0,
		z.X, z.Y, z.Z, 0,
		pos.X, pos.Y, pos.Z, 1,
	}
}

func Mat4Perspective(fovY, aspect, near, far float32) Mat4 {
	if aspect == 0 {
		aspect = 1
	}
	if near == far || fovY <= 0 || fovY >= Pi {
		return Mat4Identity()
	}
	f := 1 / Tan(fovY/2)
	var m Mat4
	m[0] = f / aspect
	m[5] = f
	m[10] = (far + near) / (near - far)
	m[11] = -1
	m[14] = 2 * far * near / (near - far)
	return m
}

func Mat4LookAt(eye, target, up Vec3) Mat4 {
	f := target.Sub(eye).Normalized()
	s := f.Cross(up)
	if s.Len() < 1e-6 {
		s = f.Cross(Vec3{X: 1})
		if s.Len() < 1e-6 {
			s = f.Cross(Vec3{Z: 1})
		}
	}
	s = s.Normalized()
	u := s.Cross(f)
	return Mat4{
		s.X, u.X, -f.X, 0,
		s.Y, u.Y, -f.Y, 0,
		s.Z, u.Z, -f.Z, 0,
		-s.Dot(eye), -u.Dot(eye), f.Dot(eye), 1,
	}
}

func Mat4Lift(m Mat3) Mat4 {
	return Mat4{
		m[0], -m[1], 0, 0,
		-m[3], m[4], 0, 0,
		0, 0, 1, 0,
		m[6], -m[7], 0, 1,
	}
}

func (m Mat4) Col(i int) Vec3 {
	return Vec3{X: m[i*4], Y: m[i*4+1], Z: m[i*4+2]}
}

func (m Mat4) Translation() Vec3 {
	return Vec3{X: m[12], Y: m[13], Z: m[14]}
}

func (m Mat4) MulVec4(v Vec4) Vec4 {
	return Vec4{
		X: m[0]*v.X + m[4]*v.Y + m[8]*v.Z + m[12]*v.W,
		Y: m[1]*v.X + m[5]*v.Y + m[9]*v.Z + m[13]*v.W,
		Z: m[2]*v.X + m[6]*v.Y + m[10]*v.Z + m[14]*v.W,
		W: m[3]*v.X + m[7]*v.Y + m[11]*v.Z + m[15]*v.W,
	}
}

func (m Mat4) TransformPoint(p Vec3) Vec3 {
	v := m.MulVec4(Vec4{X: p.X, Y: p.Y, Z: p.Z, W: 1})
	if v.W != 0 {
		return Vec3{X: v.X / v.W, Y: v.Y / v.W, Z: v.Z / v.W}
	}
	return v.XYZ()
}

func (m Mat4) TransformDir(d Vec3) Vec3 {
	return Vec3{
		X: m[0]*d.X + m[4]*d.Y + m[8]*d.Z,
		Y: m[1]*d.X + m[5]*d.Y + m[9]*d.Z,
		Z: m[2]*d.X + m[6]*d.Y + m[10]*d.Z,
	}
}

func (m Mat4) Transpose() Mat4 {
	var r Mat4
	for c := 0; c < 4; c++ {
		for row := 0; row < 4; row++ {
			r[row*4+c] = m[c*4+row]
		}
	}
	return r
}

func (m Mat4) Inverse() (Mat4, bool) {
	a00, a01, a02, a03 := m[0], m[1], m[2], m[3]
	a10, a11, a12, a13 := m[4], m[5], m[6], m[7]
	a20, a21, a22, a23 := m[8], m[9], m[10], m[11]
	a30, a31, a32, a33 := m[12], m[13], m[14], m[15]

	b00 := a00*a11 - a01*a10
	b01 := a00*a12 - a02*a10
	b02 := a00*a13 - a03*a10
	b03 := a01*a12 - a02*a11
	b04 := a01*a13 - a03*a11
	b05 := a02*a13 - a03*a12
	b06 := a20*a31 - a21*a30
	b07 := a20*a32 - a22*a30
	b08 := a20*a33 - a23*a30
	b09 := a21*a32 - a22*a31
	b10 := a21*a33 - a23*a31
	b11 := a22*a33 - a23*a32

	det := b00*b11 - b01*b10 + b02*b09 + b03*b08 - b04*b07 + b05*b06
	if Abs(det) < 1e-20 {
		return Mat4Identity(), false
	}
	d := 1 / det

	return Mat4{
		(a11*b11 - a12*b10 + a13*b09) * d,
		(a02*b10 - a01*b11 - a03*b09) * d,
		(a31*b05 - a32*b04 + a33*b03) * d,
		(a22*b04 - a21*b05 - a23*b03) * d,
		(a12*b08 - a10*b11 - a13*b07) * d,
		(a00*b11 - a02*b08 + a03*b07) * d,
		(a32*b02 - a30*b05 - a33*b01) * d,
		(a20*b05 - a22*b02 + a23*b01) * d,
		(a10*b10 - a11*b08 + a13*b06) * d,
		(a01*b08 - a00*b10 - a03*b06) * d,
		(a30*b04 - a31*b02 + a33*b00) * d,
		(a21*b02 - a20*b04 - a23*b00) * d,
		(a11*b07 - a10*b09 - a12*b06) * d,
		(a00*b09 - a01*b07 + a02*b06) * d,
		(a31*b01 - a30*b03 - a32*b00) * d,
		(a20*b03 - a21*b01 + a22*b00) * d,
	}, true
}

func (m Mat4) InverseAffine() (Mat4, bool) {
	x, y, z := m.Col(0), m.Col(1), m.Col(2)
	det := x.Cross(y).Dot(z)
	if Abs(det) < 1e-12 {
		return Mat4Identity(), false
	}
	d := 1 / det
	rx := y.Cross(z).Scale(d)
	ry := z.Cross(x).Scale(d)
	rz := x.Cross(y).Scale(d)
	t := m.Translation()
	return Mat4{
		rx.X, ry.X, rz.X, 0,
		rx.Y, ry.Y, rz.Y, 0,
		rx.Z, ry.Z, rz.Z, 0,
		-rx.Dot(t), -ry.Dot(t), -rz.Dot(t), 1,
	}, true
}

func (m Mat4) Decompose() (Vec3, Quat, Vec3, bool) {
	x, y, z := m.Col(0), m.Col(1), m.Col(2)
	s := Vec3{X: x.Len(), Y: y.Len(), Z: z.Len()}
	if s.X < 1e-8 || s.Y < 1e-8 || s.Z < 1e-8 {
		return m.Translation(), QuatIdentity(), s, false
	}
	if x.Cross(y).Dot(z) < 0 {
		s.X = -s.X
	}
	q := quatFromAxes(x.Scale(1/s.X), y.Scale(1/s.Y), z.Scale(1/s.Z))
	return m.Translation(), q, s, true
}

func (m Mat4) NearlyEqual(o Mat4, eps float32) bool {
	for i := range m {
		if !NearlyEqual(m[i], o[i], eps) {
			return false
		}
	}
	return true
}
