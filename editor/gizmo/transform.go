package gizmo

import (
	"aego/core/mathx"
)

type Mode uint8

const (
	ModeMove Mode = iota
	ModeRotate
	ModeScale
)

const (
	axisPx   = 90
	rotatePx = 80
	boxPx    = 14
	hitPx    = 9
)

type Transform struct {
	Mode Mode
}

func (g *Transform) axes(rot float32) (mathx.Vec2, mathx.Vec2) {
	if g.Mode == ModeScale {
		c, s := mathx.Cos(rot), mathx.Sin(rot)
		return mathx.Vec2{X: c, Y: s}, mathx.Vec2{X: s, Y: -c}
	}
	return mathx.Vec2{X: 1}, mathx.Vec2{Y: -1}
}

func (g *Transform) Draw(c *DrawCtx) {
	ax, ay := g.axes(c.Rotation)
	p := c.Pivot
	l := c.PxToWorld(axisPx)
	head := c.PxToWorld(16)
	cx, cy := c.Color(PartX, c.Colors.X), c.Color(PartY, c.Colors.Y)
	switch g.Mode {
	case ModeMove:
		c.Thick(p, p.Add(ax.Scale(l-head)), cx, 3)
		c.Triangle(p.Add(ax.Scale(l)), ax, 16, 12, cx)
		c.Thick(p, p.Add(ay.Scale(l-head)), cy, 3)
		c.Triangle(p.Add(ay.Scale(l)), ay, 16, 12, cy)
		o := c.PxToWorld(boxPx * 1.5)
		center := p.Add(ax.Scale(o)).Add(ay.Scale(o))
		cc := c.Color(PartXY, c.Colors.Center)
		fill := cc
		fill.A *= 0.45
		c.FillHandle(center, boxPx, fill)
		c.Handle(center, boxPx, cc)
	case ModeRotate:
		rc := c.Color(PartRotate, c.Colors.Rotate)
		c.Ring(p, rotatePx, 3, rc)
		dir := mathx.Vec2{X: mathx.Cos(c.Rotation), Y: mathx.Sin(c.Rotation)}
		c.Thick(p, p.Add(dir.Scale(c.PxToWorld(rotatePx))), rc, 2)
	case ModeScale:
		cx, cy = c.Color(PartScaleX, c.Colors.X), c.Color(PartScaleY, c.Colors.Y)
		ex, ey := p.Add(ax.Scale(l)), p.Add(ay.Scale(l))
		c.Thick(p, ex, cx, 3)
		c.Thick(p, ey, cy, 3)
		c.FillHandle(ex, boxPx, cx)
		c.FillHandle(ey, boxPx, cy)
		c.FillHandle(p, boxPx, c.Color(PartScaleXY, c.Colors.Center))
		return
	}
	c.FillHandle(p, 5, c.Colors.Center)
}

func inBox(m, center mathx.Vec2, half float32) bool {
	return mathx.Abs(m.X-center.X) <= half && mathx.Abs(m.Y-center.Y) <= half
}

func (g *Transform) HitTest(c *HitCtx) (Hit, bool) {
	sp := c.Space
	ax, ay := g.axes(c.Rotation)
	pp := sp.ToPixels(c.Pivot)
	m := c.MousePx
	tol := sp.Px(hitPx)
	half := sp.Px(boxPx)/2 + sp.Px(2)
	at := func(dir mathx.Vec2, px float32) mathx.Vec2 {
		return sp.ToPixels(c.Pivot.Add(dir.Scale(sp.PxToWorld(px))))
	}
	switch g.Mode {
	case ModeMove:
		o := sp.PxToWorld(boxPx * 1.5)
		if inBox(m, sp.ToPixels(c.Pivot.Add(ax.Scale(o)).Add(ay.Scale(o))), half) {
			return Hit{Part: PartXY}, true
		}
		dx := segDist(m, pp, at(ax, axisPx))
		dy := segDist(m, pp, at(ay, axisPx))
		if dx <= tol && dx <= dy {
			return Hit{Part: PartX, Dist: dx}, true
		}
		if dy <= tol {
			return Hit{Part: PartY, Dist: dy}, true
		}
	case ModeRotate:
		d := mathx.Abs(m.Sub(pp).Len() - sp.Px(rotatePx))
		if d <= tol {
			return Hit{Part: PartRotate, Dist: d}, true
		}
	case ModeScale:
		if inBox(m, pp, half) {
			return Hit{Part: PartScaleXY}, true
		}
		ex, ey := at(ax, axisPx), at(ay, axisPx)
		if inBox(m, ex, half) || segDist(m, pp, ex) <= tol {
			return Hit{Part: PartScaleX}, true
		}
		if inBox(m, ey, half) || segDist(m, pp, ey) <= tol {
			return Hit{Part: PartScaleY}, true
		}
	}
	return Hit{}, false
}

func (g *Transform) Begin(e *EditCtx, h Hit) {
	e.Part = h.Part
	e.AxisX, e.AxisY = g.axes(e.Rotation)
}

func (g *Transform) Update(e *EditCtx) {
	switch e.Part {
	case PartX, PartY, PartXY:
		g.move(e)
	case PartRotate:
		g.rotate(e)
	case PartScaleX, PartScaleY, PartScaleXY:
		g.scale(e)
	}
}

func (g *Transform) End(e *EditCtx) {
	Commit(e)
}

func (g *Transform) move(e *EditCtx) {
	d := e.Mouse.Sub(e.Start)
	switch e.Part {
	case PartX:
		d = e.AxisX.Scale(d.Dot(e.AxisX))
	case PartY:
		d = e.AxisY.Scale(d.Dot(e.AxisY))
	}
	if e.Snap.Enabled {
		d = mathx.Vec2{X: snapTo(d.X, e.Snap.Move), Y: snapTo(d.Y, e.Snap.Move)}
	}
	for i, t := range e.Targets {
		after := t.Start
		after.Position = t.ParentInv.Apply(t.StartWorld.Add(d))
		e.set(i, after)
	}
}

func wrapAngle(a float32) float32 {
	for a > mathx.Pi {
		a -= mathx.TwoPi
	}
	for a < -mathx.Pi {
		a += mathx.TwoPi
	}
	return a
}

func (g *Transform) rotate(e *EditCtx) {
	v0, v1 := e.Start.Sub(e.Pivot), e.Mouse.Sub(e.Pivot)
	if v0.LenSq() == 0 || v1.LenSq() == 0 {
		return
	}
	ang := wrapAngle(mathx.Atan2(v1.Y, v1.X) - mathx.Atan2(v0.Y, v0.X))
	if e.Snap.Enabled && e.Snap.Rotate > 0 {
		ang = snapTo(ang, mathx.Deg2Rad(e.Snap.Rotate))
	}
	for i, t := range e.Targets {
		after := t.Start
		world := e.Pivot.Add(t.StartWorld.Sub(e.Pivot).Rotated(ang))
		after.Position = t.ParentInv.Apply(world)
		after.Rotation = t.Start.Rotation + ang
		e.set(i, after)
	}
}

func (g *Transform) scale(e *EditCtx) {
	v0, v1 := e.Start.Sub(e.Pivot), e.Mouse.Sub(e.Pivot)
	fx, fy := float32(1), float32(1)
	switch e.Part {
	case PartScaleX:
		if d0 := v0.Dot(e.AxisX); mathx.Abs(d0) > 1e-6 {
			fx = v1.Dot(e.AxisX) / d0
		}
	case PartScaleY:
		if d0 := v0.Dot(e.AxisY); mathx.Abs(d0) > 1e-6 {
			fy = v1.Dot(e.AxisY) / d0
		}
	case PartScaleXY:
		if l0 := v0.Len(); l0 > 1e-6 {
			fx = v1.Len() / l0
			fy = fx
		}
	}
	if e.Snap.Enabled && e.Snap.Scale > 0 {
		fx, fy = snapTo(fx, e.Snap.Scale), snapTo(fy, e.Snap.Scale)
	}
	if fx == 0 {
		fx = e.Snap.Scale
	}
	if fy == 0 {
		fy = e.Snap.Scale
	}
	for i, t := range e.Targets {
		after := t.Start
		after.Scale = mathx.Vec2{X: t.Start.Scale.X * fx, Y: t.Start.Scale.Y * fy}
		off := t.StartWorld.Sub(e.Pivot)
		world := e.Pivot.Add(e.AxisX.Scale(off.Dot(e.AxisX) * fx)).Add(e.AxisY.Scale(off.Dot(e.AxisY) * fy))
		after.Position = t.ParentInv.Apply(world)
		e.set(i, after)
	}
}
