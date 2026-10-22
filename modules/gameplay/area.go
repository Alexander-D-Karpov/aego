package gameplay

import "aego/core/mathx"

type area struct {
	c      mathx.Vec2
	ax, ay mathx.Vec2
	r      float32
	circle bool
}

func areaOf(m mathx.Mat3, r mathx.Rect, shape Shape) area {
	c := m.Apply(mathx.Vec2{X: r.X + r.W/2, Y: r.Y + r.H/2})
	ax := mathx.Vec2{X: m[0], Y: m[1]}.Scale(r.W / 2)
	ay := mathx.Vec2{X: m[3], Y: m[4]}.Scale(r.H / 2)
	if shape == ShapeCircle {
		return area{c: c, r: min(ax.Len(), ay.Len()), circle: true}
	}
	return area{c: c, ax: ax, ay: ay}
}

func (a *area) project(n mathx.Vec2) float32 {
	if a.circle {
		return a.r
	}
	return mathx.Abs(a.ax.Dot(n)) + mathx.Abs(a.ay.Dot(n))
}

func (a *area) closest(p mathx.Vec2) mathx.Vec2 {
	d := p.Sub(a.c)
	out := a.c
	for _, ax := range [2]mathx.Vec2{a.ax, a.ay} {
		if l2 := ax.LenSq(); l2 > 0 {
			out = out.Add(ax.Scale(mathx.Clamp(d.Dot(ax)/l2, -1, 1)))
		}
	}
	return out
}

func (a *area) contains(p mathx.Vec2) bool {
	if a.circle {
		return p.Sub(a.c).LenSq() <= a.r*a.r
	}
	d := p.Sub(a.c)
	for _, ax := range [2]mathx.Vec2{a.ax, a.ay} {
		if l2 := ax.LenSq(); l2 == 0 || mathx.Abs(d.Dot(ax)) > l2 {
			return false
		}
	}
	return true
}

func overlap(a, b *area) bool {
	switch {
	case a.circle && b.circle:
		r := a.r + b.r
		return a.c.Sub(b.c).LenSq() <= r*r
	case a.circle:
		return b.closest(a.c).Sub(a.c).LenSq() <= a.r*a.r
	case b.circle:
		return a.closest(b.c).Sub(b.c).LenSq() <= b.r*b.r
	}
	d := b.c.Sub(a.c)
	for _, ax := range [4]mathx.Vec2{a.ax, a.ay, b.ax, b.ay} {
		n, l := normalize(ax)
		if l == 0 {
			continue
		}
		if mathx.Abs(d.Dot(n)) > a.project(n)+b.project(n) {
			return false
		}
	}
	return true
}
