package fieldgizmo

import (
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/gizmo"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/scene"
	"aego/scene/scriptcomp"
	"aego/schema"
)

const partBase = 32

type field struct {
	t  *schema.TypeDescriptor
	fd *schema.FieldDescriptor
}

type geom struct {
	base mathx.Vec2
	tip  mathx.Vec2
	r    float32
}

func gizmoFields(td *schema.TypeDescriptor) bool {
	for i := range td.Fields {
		if td.Fields[i].Hints.Gizmo != "" {
			return true
		}
	}
	return false
}

func collect(s *scene.Scene, h scene.NodeHandle, t schema.TypeID, script bool, out []field) []field {
	reg := s.Registry()
	add := func(td *schema.TypeDescriptor) {
		for i := range td.Fields {
			if fd := &td.Fields[i]; fd.Hints.Gizmo != "" {
				out = append(out, field{t: td, fd: fd})
			}
		}
	}
	if !script {
		if td, ok := reg.ByID(t); ok {
			add(td)
		}
		return out
	}
	sc, ok := scriptcomp.ScriptsOf(s, h)
	if !ok {
		return out
	}
	for i := range sc.Attachments {
		if td, ok := reg.ByName(sc.Attachments[i].TypeName); ok && td.Kind == schema.TypeScript {
			add(td)
		}
	}
	return out
}

func shapeOf(s *scene.Scene, h scene.NodeHandle, f field, scale float32) (geom, bool) {
	v, err := s.GetProperty(h, f.t.ID, f.fd.ID)
	if err != nil {
		return geom{}, false
	}
	m := s.World(h)
	base := mathx.Vec2{X: m[6], Y: m[7]}
	local := f.fd.Hints.GizmoSpace != schema.GizmoWorld
	switch f.fd.Hints.Gizmo {
	case schema.GizmoVector:
		d := v.V
		if local {
			d = m.Apply(d).Sub(base)
		}
		return geom{base: base, tip: base.Add(d.Scale(scale))}, true
	case schema.GizmoPoint:
		p := v.V
		if local {
			p = m.Apply(p)
		}
		return geom{base: base, tip: p}, true
	case schema.GizmoRadius:
		r := float32(v.AsFloat())
		if local {
			r *= mathx.Sqrt(m[0]*m[0] + m[1]*m[1])
		}
		return geom{base: base, tip: base.Add(mathx.Vec2{X: r}), r: r}, true
	}
	return geom{}, false
}

type Gizmo struct {
	ctx    *context.Editor
	t      schema.TypeID
	script bool
	list   []field
	active field
}

func Register(ctx *context.Editor, reg *gizmo.Registry) error {
	for _, td := range ctx.Types.Schema().Components() {
		if td.ID == scriptcomp.ScriptTypeID || !gizmoFields(td) {
			continue
		}
		if err := reg.Register(td.ID, &Gizmo{ctx: ctx, t: td.ID}); err != nil {
			return err
		}
	}
	if err := reg.Register(scriptcomp.ScriptTypeID, &Gizmo{ctx: ctx, script: true}); err != nil {
		return err
	}
	l := &layer{ctx: ctx}
	ctx.Vectors = append(ctx.Vectors, context.VectorLayer{ID: "fields", Title: i18n.T("editor.fieldgizmo.fields", "Fields"), Draw: l.draw})
	return nil
}

func (g *Gizmo) scale() float32 {
	if g.ctx == nil {
		return context.DefaultVectorScale
	}
	return g.ctx.Overlay.VectorScale()
}

func (g *Gizmo) mods() input.Mods {
	if g.ctx == nil || g.ctx.UI == nil {
		return 0
	}
	return g.ctx.UI.Mods()
}

func (g *Gizmo) Draw(c *gizmo.DrawCtx) {
	s := c.Scene
	h := s.Handle(c.Node)
	if h.IsZero() || !s.IsAlive(h) {
		return
	}
	g.list = collect(s, h, g.t, g.script, g.list[:0])
	sc := g.scale()
	for i, f := range g.list {
		gm, ok := shapeOf(s, h, f, sc)
		if !ok {
			continue
		}
		col := c.Color(gizmo.Part(partBase+i), c.Colors.Field)
		switch f.fd.Hints.Gizmo {
		case schema.GizmoVector:
			c.Line(gm.base, gm.tip, col)
			if d := gm.tip.Sub(gm.base); d.LenSq() > 0 {
				c.Triangle(gm.tip, d, 12, 9, col)
			}
		case schema.GizmoPoint:
			dim := col
			dim.A *= 0.4
			c.Line(gm.base, gm.tip, dim)
		case schema.GizmoRadius:
			c.Debug.Circle(gm.base, gm.r, 48, col)
		}
		c.FillHandle(gm.tip, 8, col)
	}
}

func (g *Gizmo) HitTest(c *gizmo.HitCtx) (gizmo.Hit, bool) {
	s := c.Scene
	h := s.Handle(c.Node)
	if h.IsZero() || !s.IsAlive(h) {
		return gizmo.Hit{}, false
	}
	g.list = collect(s, h, g.t, g.script, g.list[:0])
	sc := g.scale()
	best, bestD := -1, c.Space.Px(9)
	for i, f := range g.list {
		gm, ok := shapeOf(s, h, f, sc)
		if !ok {
			continue
		}
		if d := c.Space.ToPixels(gm.tip).Sub(c.MousePx).Len(); d <= bestD {
			best, bestD = i, d
		}
	}
	if best < 0 {
		return gizmo.Hit{}, false
	}
	return gizmo.Hit{Part: gizmo.Part(partBase + best), Dist: bestD}, true
}

func (g *Gizmo) Begin(e *gizmo.EditCtx, hit gizmo.Hit) {
	e.Part = hit.Part
	s := e.Scene
	g.list = collect(s, s.Handle(e.Node), g.t, g.script, g.list[:0])
	g.active = field{}
	if i := int(hit.Part) - partBase; i >= 0 && i < len(g.list) {
		g.active = g.list[i]
	}
}

func snapAngle(d mathx.Vec2, step float32) mathx.Vec2 {
	l := d.Len()
	if l == 0 {
		return d
	}
	a := mathx.Atan2(d.Y, d.X)
	a = float32(int(a/step+mathx.Sign(a)*0.5)) * step
	return mathx.Vec2{X: mathx.Cos(a) * l, Y: mathx.Sin(a) * l}
}

func snapGrid(v mathx.Vec2, step float32) mathx.Vec2 {
	if step <= 0 {
		return v
	}
	r := func(f float32) float32 { return float32(int(f/step+mathx.Sign(f)*0.5)) * step }
	return mathx.Vec2{X: r(v.X), Y: r(v.Y)}
}

func (g *Gizmo) grid() float32 {
	if g.ctx == nil {
		return 0
	}
	return g.ctx.Settings.SnapMove
}

func (g *Gizmo) Update(e *gizmo.EditCtx) {
	f := g.active
	if f.fd == nil {
		return
	}
	s := e.Scene
	h := s.Handle(e.Node)
	if h.IsZero() {
		return
	}
	m := s.World(h)
	inv, ok := m.Inverse()
	if !ok {
		return
	}
	base := mathx.Vec2{X: m[6], Y: m[7]}
	local := f.fd.Hints.GizmoSpace != schema.GizmoWorld
	mods := g.mods()
	ctrl := mods.Primary()
	var val schema.Value
	switch f.fd.Hints.Gizmo {
	case schema.GizmoVector:
		d := e.Mouse.Sub(base)
		if mods.Has(input.ModShift) {
			d = snapAngle(d, mathx.Deg2Rad(15))
		}
		w := d.Scale(1 / g.scale())
		if local {
			w = inv.Apply(base.Add(w)).Sub(inv.Apply(base))
		}
		if ctrl {
			w = snapGrid(w, g.grid())
		}
		val = schema.Vec2(w)
	case schema.GizmoPoint:
		p := e.Mouse
		if ctrl {
			p = snapGrid(p, g.grid())
		}
		if local {
			p = inv.Apply(p)
		}
		val = schema.Vec2(p)
	case schema.GizmoRadius:
		r := e.Mouse.Sub(base).Len()
		if local {
			if sx := mathx.Sqrt(m[0]*m[0] + m[1]*m[1]); sx > 0 {
				r /= sx
			}
		}
		if ctrl {
			r = snapGrid(mathx.Vec2{X: r}, g.grid()).X
		}
		if f.fd.Kind == schema.KindFloat64 {
			val = schema.Float64(float64(r))
		} else {
			val = schema.Float32(r)
		}
	default:
		return
	}
	gizmo.SetProperty(e, f.t.ID, f.fd, val)
}

func (g *Gizmo) End(*gizmo.EditCtx) {}
