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
	if w != 0 && w != 1 {
		return Vec2{X: x / w, Y: y / w}
	}
	return Vec2{X: x, Y: y}
}
