package systems

import (
	"errors"
	"fmt"

	"aego/core/mathx"
	"aego/modules/physics/components"
	"aego/modules/physics/world"
	"aego/scene"
)

type colliderSig struct {
	c   components.Collider
	pts []mathx.Vec2
}

func (g *colliderSig) equal(c *components.Collider) bool {
	a := &g.c
	if a.Kind != c.Kind || a.Common != c.Common || a.Size != c.Size || a.Radius != c.Radius || a.Length != c.Length ||
		a.Loop != c.Loop || a.OneSided != c.OneSided || len(g.pts) != len(c.Points) {
		return false
	}
	for i := range g.pts {
		if g.pts[i] != c.Points[i] {
			return false
		}
	}
	return true
}

func (g *colliderSig) set(c *components.Collider) {
	g.c = *c
	g.c.Points = nil
	g.pts = append(g.pts[:0], c.Points...)
}

func scaleErr(sx, sy float32) error {
	return fmt.Errorf("non-uniform scale %.3g × %.3g, circles and capsules need a uniform scale", sx, sy)
}

func ShapeDef(c *components.Collider, rel mathx.Mat3, node scene.NodeHandle, pts []mathx.Vec2) (world.ShapeDef, []mathx.Vec2, string, error) {
	sx := mathx.Sqrt(rel[0]*rel[0] + rel[1]*rel[1])
	sy := mathx.Sqrt(rel[3]*rel[3] + rel[4]*rel[4])
	uniform := mathx.Abs(sx-sy) <= 1e-3*max(sx, sy)
	local := world.Transform{Position: rel.Apply(c.Common.Offset), Angle: mathx.Atan2(rel[1], rel[0])}
	var g world.Geometry
	switch c.Kind {
	case components.KindBox:
		g = world.Box{Half: mathx.Vec2{X: c.Size.X * 0.5 * sx, Y: c.Size.Y * 0.5 * sy}}
	case components.KindCircle:
		if !uniform {
			return world.ShapeDef{}, pts, CodeScale, scaleErr(sx, sy)
		}
		g = world.Circle{R: c.Radius * sx}
	case components.KindCapsule:
		if !uniform {
			return world.ShapeDef{}, pts, CodeScale, scaleErr(sx, sy)
		}
		g = world.Capsule{HalfLen: (c.Length*0.5 - c.Radius) * sx, R: c.Radius * sx}
	case components.KindPolygon:
		if len(c.Points) > components.MaxPolygonPoints {
			return world.ShapeDef{}, pts, CodeGeometry, fmt.Errorf("polygon has %d points, the limit is %d", len(c.Points), components.MaxPolygonPoints)
		}
		var p world.Polygon
		for i, pt := range c.Points {
			p.Pts[i] = rel.Apply(pt.Add(c.Common.Offset))
		}
		p.N = len(c.Points)
		g, local = p, world.Transform{}
	case components.KindEdge:
		pts = pts[:0]
		for _, pt := range c.Points {
			pts = append(pts, rel.Apply(pt.Add(c.Common.Offset)))
		}
		g, local = world.Chain{Pts: pts, Loop: c.Loop}, world.Transform{}
	default:
		return world.ShapeDef{}, pts, CodeGeometry, errors.New("unknown collider kind")
	}
	return world.ShapeDef{
		Geometry:    g,
		Local:       local,
		IsTrigger:   c.Common.IsTrigger,
		Friction:    c.Common.Friction,
		Restitution: c.Common.Restitution,
		Density:     c.Common.Density,
		OneSided:    c.Kind == components.KindEdge && c.OneSided,
		UserData:    Pack(node),
	}, pts, CodeGeometry, nil
}

func (m *Sim) dropShape(sh *shapeRec) {
	if !sh.h.IsZero() && m.w.Alive(sh.h) {
		m.w.RemoveShape(sh.h)
	}
	sh.h, sh.built, sh.enabled = world.ShapeHandle{}, false, false
}

func (m *Sim) buildShape(sh *shapeRec) {
	b := m.byBody[sh.owner]
	if b == nil || b.h.IsZero() {
		sh.h, sh.built, sh.enabled = world.ShapeHandle{}, false, false
		return
	}
	s := m.s
	c := &m.col
	if !components.Read(s, sh.key.node, sh.key.kind, c) {
		m.dirty = true
		return
	}
	rel := relative(s.World(b.node), s.World(sh.key.node))
	if sh.built && sh.body == b.h && sh.sig.equal(c) && near(rel, sh.rel) {
		if sh.bad {
			b.excluded = true
		}
		return
	}
	prev := sh.body
	sh.sig.set(c)
	sh.rel, sh.built, sh.body = rel, true, b.h
	key := shapeKeyName(sh.key.kind)
	def, pts, code, err := ShapeDef(c, rel, sh.key.node, m.pts)
	m.pts = pts
	if err == nil {
		if prev == b.h && !sh.h.IsZero() && m.w.Alive(sh.h) {
			err = m.w.SetShape(sh.h, def)
		} else {
			var h world.ShapeHandle
			h, err = m.w.AddShape(b.h, def)
			sh.h, sh.enabled = h, err == nil
		}
	}
	if err != nil {
		if !sh.h.IsZero() && m.w.Alive(sh.h) {
			m.w.RemoveShape(sh.h)
		}
		sh.h, sh.bad, sh.enabled = world.ShapeHandle{}, true, false
		b.excluded = true
		m.report(sh.key.node, key, code, components.Names[sh.key.kind]+": "+err.Error(), true)
		return
	}
	sh.bad = false
	m.resolve(sh.key.node, key)
}

func (m *Sim) enableShape(sh *shapeRec) {
	if sh.h.IsZero() {
		return
	}
	b := m.byBody[sh.owner]
	on := m.s.EffectiveEnabled(sh.key.node) && (b == nil || !b.excluded)
	if on != sh.enabled && m.w.SetShapeEnabled(sh.h, on) == nil {
		sh.enabled = on
	}
}
