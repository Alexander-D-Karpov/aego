package editor

import (
	"math"

	"aego/core/mathx"
	"aego/editor/core/gizmo"
	"aego/modules/particles"
	"aego/scene"
	"aego/schema"
)

type shapeGizmo struct{ td *schema.TypeDescriptor }

const (
	partCenter gizmo.Part = 96 + iota
	partDirection
	partRadius
	partRect
	partEdgeA
	partEdgeB
)

type handle struct {
	part gizmo.Part
	pos  mathx.Vec2
}

func handles(c *particles.ParticleSystem2D) (out [4]handle, n int) {
	dir := mathx.V2(float32(math.Cos(float64(c.Direction))), float32(math.Sin(float64(c.Direction))))
	out[0] = handle{partCenter, c.Offset}
	extent := float32(.8)
	switch c.Shape {
	case particles.Circle:
		extent = max(extent, c.Radius+.4)
		out[2] = handle{partRadius, c.Offset.Add(mathx.V2(c.Radius, 0))}
		n = 3
	case particles.Rect:
		extent = max(extent, c.RectSize.Len()*.5+.3)
		out[2] = handle{partRect, c.Offset.Add(c.RectSize.Scale(.5))}
		n = 3
	case particles.Edge:
		extent = max(extent, c.EdgeA.Len()+.3, c.EdgeB.Len()+.3)
		out[2] = handle{partEdgeA, c.EdgeA.Add(c.Offset)}
		out[3] = handle{partEdgeB, c.EdgeB.Add(c.Offset)}
		n = 4
	default:
		n = 2
	}
	out[1] = handle{partDirection, c.Offset.Add(dir.Scale(extent))}
	return
}
func (g *shapeGizmo) Draw(ctx *gizmo.DrawCtx) {
	h := ctx.Scene.Handle(ctx.Node)
	c, ok := scene.Get[particles.ParticleSystem2D](ctx.Scene, h)
	if !ok {
		return
	}
	m := ctx.Scene.World(h)
	pts, n := handles(c)
	col := ctx.Colors.Field
	ctx.Arrow(m.Apply(pts[0].pos), m.Apply(pts[1].pos), col)
	line := func(a, b mathx.Vec2) { ctx.Line(m.Apply(a.Add(c.Offset)), m.Apply(b.Add(c.Offset)), col) }
	switch c.Shape {
	case particles.Circle:
		last := mathx.V2(c.Radius, 0)
		for i := 1; i <= 40; i++ {
			sn, cs := math.Sincos(2 * math.Pi * float64(i) / 40)
			next := mathx.V2(float32(cs)*c.Radius, float32(sn)*c.Radius)
			line(last, next)
			last = next
		}
	case particles.Rect:
		size := c.RectSize.Scale(.5)
		r := [4]mathx.Vec2{{X: -size.X, Y: -size.Y}, {X: size.X, Y: -size.Y}, size, {X: -size.X, Y: size.Y}}
		for i := range r {
			line(r[i], r[(i+1)%4])
		}
	case particles.Edge:
		line(c.EdgeA, c.EdgeB)
	}
	for _, pt := range pts[:n] {
		ctx.FillHandle(m.Apply(pt.pos), 7, ctx.Color(pt.part, col))
	}
}
func (g *shapeGizmo) HitTest(ctx *gizmo.HitCtx) (gizmo.Hit, bool) {
	h := ctx.Scene.Handle(ctx.Node)
	c, ok := scene.Get[particles.ParticleSystem2D](ctx.Scene, h)
	if !ok {
		return gizmo.Hit{}, false
	}
	m := ctx.Scene.World(h)
	pts, n := handles(c)
	hit := gizmo.Hit{Dist: ctx.Space.Px(9)}
	for _, pt := range pts[:n] {
		if d := ctx.Space.ToPixels(m.Apply(pt.pos)).Sub(ctx.MousePx).Len(); d <= hit.Dist {
			hit = gizmo.Hit{Part: pt.part, Dist: d}
		}
	}
	return hit, hit.Part != gizmo.PartNone
}
func (g *shapeGizmo) Begin(e *gizmo.EditCtx, h gizmo.Hit) { e.Part = h.Part }
func (g *shapeGizmo) End(*gizmo.EditCtx)                  {}
func (g *shapeGizmo) Update(e *gizmo.EditCtx) {
	h := e.Scene.Handle(e.Node)
	c, ok := scene.Get[particles.ParticleSystem2D](e.Scene, h)
	if !ok {
		return
	}
	inv, ok := e.Scene.World(h).Inverse()
	if !ok {
		return
	}
	p := inv.Apply(e.Mouse)
	if e.Snap.Enabled && e.Snap.Move > 0 && e.Part != partDirection {
		q := float64(e.Snap.Move)
		p = mathx.V2(float32(math.Round(float64(p.X)/q)*q), float32(math.Round(float64(p.Y)/q)*q))
	}
	rel := p.Sub(c.Offset)
	var name string
	var value schema.Value
	switch e.Part {
	case partCenter:
		name, value = "offset", schema.Vec2(p)
	case partDirection:
		if rel == (mathx.Vec2{}) {
			return
		}
		angle := mathx.Atan2(rel.Y, rel.X)
		if e.Snap.Enabled && e.Snap.Rotate > 0 {
			angle = float32(math.Round(float64(angle/e.Snap.Rotate))) * e.Snap.Rotate
		}
		name, value = "direction", schema.Float32(angle)
	case partRadius:
		name, value = "radius", schema.Float32(rel.Len())
	case partRect:
		name, value = "rectSize", schema.Vec2(mathx.V2(2*mathx.Abs(rel.X), 2*mathx.Abs(rel.Y)))
	case partEdgeA:
		name, value = "edgeA", schema.Vec2(rel)
	case partEdgeB:
		name, value = "edgeB", schema.Vec2(rel)
	default:
		return
	}
	if f, ok := g.td.Field(name); ok {
		gizmo.SetProperty(e, g.td.ID, f, value)
	}
}
