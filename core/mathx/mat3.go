package mathx

type Mat3 [9]float32

func Mat3Identity() Mat3 {
	return Mat3{1, 0, 0, 0, 1, 0, 0, 0, 1}
}

func Mat3TRS(pos Vec2, rad float32, scale Vec2) Mat3 {
	c, s := Cos(rad), Sin(rad)
	return Mat3{
		c * scale.X, s * scale.X, 0,
		-s * scale.Y, c * scale.Y, 0,
		pos.X, pos.Y, 1,
	}
}

func (m Mat3) Mul(o Mat3) Mat3 {
	var r Mat3
	for c := 0; c < 3; c++ {
		for row := 0; row < 3; row++ {
			var sum float32
			for k := 0; k < 3; k++ {
				sum += m[k*3+row] * o[c*3+k]
			}
			r[c*3+row] = sum
		}
	}
	return r
}

func (m Mat3) Apply(p Vec2) Vec2 {
	return Vec2{
		X: m[0]*p.X + m[3]*p.Y + m[6],
		Y: m[1]*p.X + m[4]*p.Y + m[7],
	}
}

func (m Mat3) ApplyVector(v Vec2) Vec2 {
	return Vec2{
		X: m[0]*v.X + m[3]*v.Y,
		Y: m[1]*v.X + m[4]*v.Y,
	}
}

func (m Mat3) Inverse() (Mat3, bool) {
	a := m
	det := a[0]*(a[4]*a[8]-a[5]*a[7]) -
		a[1]*(a[3]*a[8]-a[5]*a[6]) +
		a[2]*(a[3]*a[7]-a[4]*a[6])
	if Abs(det) < 1e-20 {
		return Mat3Identity(), false
	}
	inv := 1 / det
	return Mat3{
		(a[4]*a[8] - a[5]*a[7]) * inv,
		(a[2]*a[7] - a[1]*a[8]) * inv,
		(a[1]*a[5] - a[2]*a[4]) * inv,
		(a[5]*a[6] - a[3]*a[8]) * inv,
		(a[0]*a[8] - a[2]*a[6]) * inv,
		(a[2]*a[3] - a[0]*a[5]) * inv,
		(a[3]*a[7] - a[4]*a[6]) * inv,
		(a[1]*a[6] - a[0]*a[7]) * inv,
		(a[0]*a[4] - a[1]*a[3]) * inv,
	}, true
}
