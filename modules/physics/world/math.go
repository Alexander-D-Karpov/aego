package world

import (
	"math"

	"aego/core/mathx"
)

type (
	Vec2 = mathx.Vec2
	AABB = mathx.AABB2
)

const fltEpsilon float32 = 1.1920929e-7

type Rot struct {
	C, S float32
}

func makeRot(a float32) Rot {
	return Rot{C: mathx.Cos(a), S: mathx.Sin(a)}
}

func (q Rot) apply(v Vec2) Vec2 {
	return Vec2{X: q.C*v.X - q.S*v.Y, Y: q.S*v.X + q.C*v.Y}
}

func (q Rot) invApply(v Vec2) Vec2 {
	return Vec2{X: q.C*v.X + q.S*v.Y, Y: -q.S*v.X + q.C*v.Y}
}

func (q Rot) invMul(o Rot) Rot {
	return Rot{C: q.C*o.C + q.S*o.S, S: q.C*o.S - q.S*o.C}
}

type xform struct {
	p Vec2
	q Rot
}

func identityXf() xform {
	return xform{q: Rot{C: 1}}
}

func (x xform) apply(v Vec2) Vec2 {
	return x.q.apply(v).Add(x.p)
}

func (x xform) invApply(v Vec2) Vec2 {
	return x.q.invApply(v.Sub(x.p))
}

func invMulXf(a, b xform) xform {
	return xform{p: a.q.invApply(b.p.Sub(a.p)), q: a.q.invMul(b.q)}
}

func crossSV(s float32, v Vec2) Vec2 {
	return Vec2{X: -s * v.Y, Y: s * v.X}
}

func crossVS(v Vec2, s float32) Vec2 {
	return Vec2{X: s * v.Y, Y: -s * v.X}
}

func rightPerp(v Vec2) Vec2 {
	return Vec2{X: v.Y, Y: -v.X}
}

func normalizeLen(v Vec2) (Vec2, float32) {
	l := v.Len()
	if l < fltEpsilon {
		return Vec2{}, 0
	}
	return v.Scale(1 / l), l
}

func min2(a, b Vec2) Vec2 {
	return Vec2{X: min(a.X, b.X), Y: min(a.Y, b.Y)}
}

func max2(a, b Vec2) Vec2 {
	return Vec2{X: max(a.X, b.X), Y: max(a.Y, b.Y)}
}

func abs2(v Vec2) Vec2 {
	return Vec2{X: mathx.Abs(v.X), Y: mathx.Abs(v.Y)}
}

func clampf(v, lo, hi float32) float32 {
	return mathx.Clamp(v, lo, hi)
}

func finite(f float32) bool {
	g := float64(f)
	return !math.IsNaN(g) && !math.IsInf(g, 0)
}

func finite2(v Vec2) bool {
	return finite(v.X) && finite(v.Y)
}

func boxOverlap(a, b AABB) bool {
	return a.Min.X <= b.Max.X && b.Min.X <= a.Max.X && a.Min.Y <= b.Max.Y && b.Min.Y <= a.Max.Y
}

func boxContains(outer, inner AABB) bool {
	return outer.Min.X <= inner.Min.X && outer.Min.Y <= inner.Min.Y && inner.Max.X <= outer.Max.X && inner.Max.Y <= outer.Max.Y
}

func boxPerimeter(a AABB) float32 {
	return 2 * ((a.Max.X - a.Min.X) + (a.Max.Y - a.Min.Y))
}

func boxUnion(a, b AABB) AABB {
	return AABB{Min: min2(a.Min, b.Min), Max: max2(a.Max, b.Max)}
}

func boxExpand(a AABB, m float32) AABB {
	return AABB{Min: Vec2{X: a.Min.X - m, Y: a.Min.Y - m}, Max: Vec2{X: a.Max.X + m, Y: a.Max.Y + m}}
}

func boxSweep(a AABB, d Vec2) AABB {
	b := a
	if d.X < 0 {
		b.Min.X += d.X
	} else {
		b.Max.X += d.X
	}
	if d.Y < 0 {
		b.Min.Y += d.Y
	} else {
		b.Max.Y += d.Y
	}
	return b
}
