package editor

import (
	"math"
	"slices"

	"aego/contracts/shapes2d"
	"aego/core/mathx"
	"aego/editor/core/gizmo"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/render/lighting"
	"aego/scene"
	"aego/schema"
)

const (
	partLightRange gizmo.Part = 40 + iota
	partLightDirection
	partLightInner
	partLightOuter
	partOccluderVertex
	partOccluderInsert
)

func finite(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }
func lightingSnap(v, step float32) float32 {
	if step <= 0 {
		return v
	}
	return float32(math.Round(float64(v/step))) * step
}

type lightHandle struct {
	part gizmo.Part
	at   mathx.Vec2
}
type lightGizmo struct {
	td     *schema.TypeDescriptor
	active gizmo.Part
}

func lightHandles(l *render2d.Light2D, at, dir mathx.Vec2, px float32) (out [4]lightHandle, n int) {
	if l.Type == lighting.Directional {
		out[0] = lightHandle{partLightDirection, at.Add(dir.Scale(max(px*48, .5)))}
		return out, 1
	}
	if l.Range <= 0 || !finite(l.Range) {
		return out, 0
	}
	out[n] = lightHandle{partLightRange, at.Add(dir.Scale(l.Range))}
	n++
	if l.Type == lighting.Spot || l.Type == lighting.Sprite {
		out[n] = lightHandle{partLightDirection, at.Add(dir.Scale(l.Range * .55))}
		n++
	}
	if l.Type == lighting.Spot {
		out[n] = lightHandle{partLightInner, at.Add(dir.Rotated(-l.InnerAngle / 2).Scale(l.Range * .8))}
		n++
		out[n] = lightHandle{partLightOuter, at.Add(dir.Rotated(l.OuterAngle / 2).Scale(l.Range * .8))}
		n++
	}
	return
}
func (g *lightGizmo) Draw(c *gizmo.DrawCtx) {
	h, l, ok := selectedLight(c.Scene, c.Node)
	if !ok {
		return
	}
	at, dir := lightPose(c.Scene, h)
	col := c.Colors.Field
	if l.Type != lighting.Directional && l.Range > 0 && finite(l.Range) {
		c.Debug.Circle(at, l.Range, 64, col)
	}
	if l.Type == lighting.Spot {
		for _, angle := range []float32{l.InnerAngle, l.OuterAngle} {
			if !finite(angle) {
				continue
			}
			angle *= .5
			last := at.Add(dir.Rotated(-angle).Scale(l.Range))
			c.Line(at, last, col)
			for i := 1; i <= 32; i++ {
				pt := at.Add(dir.Rotated(-angle + 2*angle*float32(i)/32).Scale(l.Range))
				c.Line(last, pt, col)
				last = pt
			}
			c.Line(at, last, col)
		}
	}
	handles, n := lightHandles(l, at, dir, c.PxToWorld(1))
	for _, handle := range handles[:n] {
		c.FillHandle(handle.at, 8, c.Color(handle.part, col))
	}
	if l.Type == lighting.Directional {
		c.Arrow(at, handles[0].at, col)
	}
}
func (g *lightGizmo) HitTest(c *gizmo.HitCtx) (gizmo.Hit, bool) {
	h, l, ok := selectedLight(c.Scene, c.Node)
	if !ok {
		return gizmo.Hit{}, false
	}
	at, dir := lightPose(c.Scene, h)
	handles, n := lightHandles(l, at, dir, c.Space.PxToWorld(1))
	best := gizmo.Hit{Dist: c.Space.Px(10)}
	found := false
	for _, handle := range handles[:n] {
		if dist := c.Space.ToPixels(handle.at).Sub(c.MousePx).Len(); dist <= best.Dist {
			best = gizmo.Hit{Part: handle.part, Dist: dist}
			found = true
		}
	}
	return best, found
}
func (g *lightGizmo) Begin(e *gizmo.EditCtx, h gizmo.Hit) { g.active = h.Part; e.Part = h.Part }
func (g *lightGizmo) Update(e *gizmo.EditCtx) {
	h, l, ok := selectedLight(e.Scene, e.Node)
	if !ok {
		return
	}
	at, dir := lightPose(e.Scene, h)
	delta := e.Mouse.Sub(at)
	distance := delta.Len()
	if !finite(distance) {
		return
	}
	var name string
	var value float32
	switch g.active {
	case partLightRange:
		name, value = "range", distance
		if e.Snap.Enabled {
			value = lightingSnap(value, e.Snap.Move)
		}
		value = max(.001, value)
	case partLightInner, partLightOuter:
		if l.Type != lighting.Spot || distance < 1e-6 {
			return
		}
		value = 2 * float32(math.Abs(math.Atan2(float64(dir.X*delta.Y-dir.Y*delta.X), float64(dir.Dot(delta)))))
		if e.Snap.Enabled {
			value = lightingSnap(value, e.Snap.Rotate*mathx.Pi/180)
		}
		if g.active == partLightInner {
			name = "innerAngle"
			value = mathx.Clamp(value, 0, l.OuterAngle)
		} else {
			name = "outerAngle"
			value = mathx.Clamp(value, l.InnerAngle, mathx.Pi)
		}
	case partLightDirection:
		if distance < 1e-6 {
			return
		}
		parentInv, valid := e.Scene.World(e.Scene.Parent(h)).Inverse()
		if !valid {
			return
		}
		localDelta := parentInv.Apply(e.Mouse).Sub(parentInv.Apply(at))
		angle := mathx.Atan2(localDelta.Y, localDelta.X)
		tr := e.Scene.Local(h)
		if tr.Scale.X < 0 {
			angle -= mathx.Pi
		}
		if e.Snap.Enabled {
			angle = lightingSnap(angle, e.Snap.Rotate*mathx.Pi/180)
		}
		tr.Rotation = angle
		e.Scene.SetLocal(h, tr)
		e.Scene.UpdateTransforms()
		return
	default:
		return
	}
	if field, ok := g.td.Field(name); ok {
		gizmo.SetProperty(e, g.td.ID, field, schema.Float32(value))
	}
}
func (g *lightGizmo) End(*gizmo.EditCtx) { g.active = gizmo.PartNone }

type occluderGizmo struct {
	td                      *schema.TypeDescriptor
	p                       *lightingEditor
	hoverIndex, activeIndex int
	hoverInsert, done       bool
	points, work            []mathx.Vec2
}

func (g *occluderGizmo) component(s *scene.Scene, idNode *gizmo.HitCtx) (scene.NodeHandle, *render2d.LightOccluder2D, bool) {
	h := s.Handle(idNode.Node)
	o, ok := scene.Get[render2d.LightOccluder2D](s, h)
	return h, o, ok && o.Source == render2d.OccluderPolygon
}
func (g *occluderGizmo) Draw(c *gizmo.DrawCtx) {
	h := c.Scene.Handle(c.Node)
	o, ok := scene.Get[render2d.LightOccluder2D](c.Scene, h)
	if !ok || o.Source != render2d.OccluderPolygon {
		return
	}
	m := c.Scene.World(h)
	col := c.Colors.Field
	if shapes2d.ValidateContour(o.Polygon, o.Closed) != nil {
		col = c.Colors.X
	}
	n := len(o.Polygon)
	edges := n - 1
	if o.Closed {
		edges = n
	}
	if edges < 0 {
		return
	}
	for i := 0; i < edges; i++ {
		a, b := m.Apply(o.Polygon[i]), m.Apply(o.Polygon[(i+1)%n])
		c.Line(a, b, col)
		if n < shapes2d.MaxContourPoints {
			hc := col
			if c.Active == gizmo.PartNone && c.Hot == partOccluderInsert && g.hoverInsert && g.hoverIndex == i {
				hc = c.Colors.Hot
			}
			c.Handle(a.Lerp(b, .5), 6, hc)
		}
	}
	for i, v := range o.Polygon {
		hc := col
		if c.Active == partOccluderVertex && g.activeIndex == i || c.Active == gizmo.PartNone && c.Hot == partOccluderVertex && !g.hoverInsert && g.hoverIndex == i {
			hc = c.Colors.Hot
		}
		c.FillHandle(m.Apply(v), 8, hc)
	}
}
func (g *occluderGizmo) HitTest(c *gizmo.HitCtx) (gizmo.Hit, bool) {
	g.hoverIndex = -1
	g.hoverInsert = false
	h, o, ok := g.component(c.Scene, c)
	if !ok {
		return gizmo.Hit{}, false
	}
	m := c.Scene.World(h)
	best := c.Space.Px(9)
	for i, v := range o.Polygon {
		if dist := c.Space.ToPixels(m.Apply(v)).Sub(c.MousePx).Len(); dist <= best {
			best = dist
			g.hoverIndex = i
		}
	}
	if g.hoverIndex >= 0 {
		return gizmo.Hit{Part: partOccluderVertex, Dist: best}, true
	}
	n := len(o.Polygon)
	if n < 2 || n >= shapes2d.MaxContourPoints {
		return gizmo.Hit{}, false
	}
	edges := n - 1
	if o.Closed {
		edges = n
	}
	for i := 0; i < edges; i++ {
		mid := o.Polygon[i].Lerp(o.Polygon[(i+1)%n], .5)
		if dist := c.Space.ToPixels(m.Apply(mid)).Sub(c.MousePx).Len(); dist <= best {
			best = dist
			g.hoverIndex = i
			g.hoverInsert = true
		}
	}
	return gizmo.Hit{Part: partOccluderInsert, Dist: best}, g.hoverIndex >= 0
}
func pointsProperty(points []mathx.Vec2) schema.Value {
	values := make([]schema.Value, len(points))
	for i, v := range points {
		values[i] = schema.Vec2(v)
	}
	return schema.Value{Kind: schema.KindList, List: values}
}
func (g *occluderGizmo) set(e *gizmo.EditCtx, pts []mathx.Vec2) {
	if f, ok := g.td.Field("polygon"); ok {
		gizmo.SetProperty(e, g.td.ID, f, pointsProperty(pts))
	}
}
func (g *occluderGizmo) Begin(e *gizmo.EditCtx, hit gizmo.Hit) {
	e.Part = partOccluderVertex
	g.done = true
	g.activeIndex = -1
	h := e.Scene.Handle(e.Node)
	o, ok := scene.Get[render2d.LightOccluder2D](e.Scene, h)
	if !ok || o.Source != render2d.OccluderPolygon {
		return
	}
	g.points = append(g.points[:0], o.Polygon...)
	i := g.hoverIndex
	if i < 0 || i >= len(g.points) {
		return
	}
	g.activeIndex = i
	alt := g.p != nil && g.p.ed != nil && g.p.ed.UI != nil && g.p.ed.UI.Mods().Has(input.ModAlt)
	if hit.Part == partOccluderVertex && alt {
		minimum := 2
		if o.Closed {
			minimum = 3
		}
		if len(g.points) > minimum {
			g.points = slices.Delete(g.points, i, i+1)
			g.set(e, g.points)
		}
		return
	}
	if hit.Part == partOccluderInsert {
		if len(g.points) >= shapes2d.MaxContourPoints {
			return
		}
		mid := g.points[i].Lerp(g.points[(i+1)%len(g.points)], .5)
		g.points = slices.Insert(g.points, i+1, mid)
		g.activeIndex = i + 1
		g.set(e, g.points)
	}
	g.done = false
}
func (g *occluderGizmo) Update(e *gizmo.EditCtx) {
	if g.done || g.activeIndex < 0 || g.activeIndex >= len(g.points) {
		return
	}
	h := e.Scene.Handle(e.Node)
	if h.IsZero() {
		return
	}
	inv, ok := e.Scene.World(h).Inverse()
	if !ok {
		return
	}
	pt := inv.Apply(e.Mouse)
	if !finite(pt.X) || !finite(pt.Y) {
		return
	}
	if e.Snap.Enabled {
		pt.X = lightingSnap(pt.X, e.Snap.Move)
		pt.Y = lightingSnap(pt.Y, e.Snap.Move)
	}
	g.work = append(g.work[:0], g.points...)
	g.work[g.activeIndex] = pt
	g.set(e, g.work)
}
func (g *occluderGizmo) End(*gizmo.EditCtx) { g.done = true; g.activeIndex = -1 }
