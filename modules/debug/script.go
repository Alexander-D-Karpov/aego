package debug

import (
	"aego/core/mathx"
	"aego/script/bind"
)

func scriptService(c *bind.Ctx) *Service {
	if c.World == nil {
		return nil
	}
	return Of(c.World)
}

func installScript(m *bind.Module) {
	draw := func(c *bind.Ctx, col mathx.Color, fn func(*Service)) {
		if s := scriptService(c); s != nil && s.cfg.Enabled {
			s.scriptStyle(c.Self, c.Node, col)
			fn(s)
		}
	}
	bind.Proc3(m, "Line", func(c *bind.Ctx, a, b mathx.Vec2, col mathx.Color) {
		draw(c, col, func(s *Service) { s.Line(a, b, &s.tmp) })
	}, bind.Doc("Line draws a line in world units. It lasts one frame unless Pen sets a lifetime."),
		bind.Params("a", "b", "color"), bind.Example("debug.Line(node.WorldPosition(), Target.WorldPosition(), Color{1, 0, 0, 1})"))
	bind.Proc3(m, "Arrow", func(c *bind.Ctx, from, to mathx.Vec2, col mathx.Color) {
		draw(c, col, func(s *Service) { s.Arrow(from, to, &s.tmp) })
	}, bind.Doc("Arrow draws an arrow from one point to another."),
		bind.Params("from", "to", "color"), bind.Example("debug.Arrow(p, p + body.Velocity * 0.2, Color{0, 1, 0, 1})"))
	bind.Proc3(m, "Ray", func(c *bind.Ctx, origin, dir mathx.Vec2, col mathx.Color) {
		draw(c, col, func(s *Service) { s.Ray(origin, dir, &s.tmp) })
	}, bind.Doc("Ray draws an arrow from origin along dir; the length of dir is the length of the arrow."),
		bind.Params("origin", "dir", "color"), bind.Example("debug.Ray(node.WorldPosition(), Vec2{0, 2}, Color{1, 1, 0, 1})"))
	bind.Proc3(m, "Circle", func(c *bind.Ctx, center mathx.Vec2, r float32, col mathx.Color) {
		draw(c, col, func(s *Service) { s.Circle(center, r, &s.tmp) })
	}, bind.Doc("Circle draws a circle outline with radius r in world units."),
		bind.Params("center", "radius", "color"), bind.Example("debug.Circle(node.WorldPosition(), SightRange, Color{0, 0.6, 1, 1})"))
	bind.Proc2(m, "Rect", func(c *bind.Ctx, r mathx.Rect, col mathx.Color) {
		draw(c, col, func(s *Service) { s.Rect(r, &s.tmp) })
	}, bind.Doc("Rect draws a rectangle outline."),
		bind.Params("rect", "color"), bind.Example("debug.Rect(Rect{0, 0, 4, 2}, Color{1, 1, 1, 1})"))
	bind.Proc3(m, "Cross", func(c *bind.Ctx, p mathx.Vec2, size float32, col mathx.Color) {
		draw(c, col, func(s *Service) { s.Cross(p, size, &s.tmp) })
	}, bind.Doc("Cross draws a cross of the given size in world units."),
		bind.Params("p", "size", "color"), bind.Example("debug.Cross(goal, 0.5, Color{1, 0, 1, 1})"))
	bind.Proc2(m, "Point", func(c *bind.Ctx, p mathx.Vec2, col mathx.Color) {
		draw(c, col, func(s *Service) { s.Point(p, &s.tmp) })
	}, bind.Doc("Point marks a point with a small cross of constant screen size."),
		bind.Params("p", "color"), bind.Example("debug.Point(hit.Point, Color{1, 0, 0, 1})"))
	bind.Proc3(m, "Text", func(c *bind.Ctx, p mathx.Vec2, t string, col mathx.Color) {
		draw(c, col, func(s *Service) { s.Text(p, t, &s.tmp) })
	}, bind.Doc("Text draws a label at a world position; it keeps its screen size at any zoom."),
		bind.Params("p", "text", "color"), bind.Example("debug.Text(node.WorldPosition(), string(state), Color{1, 1, 1, 1})"))
	bind.Proc2(m, "Pen", func(c *bind.Ctx, ch string, ttl float32) {
		if s := scriptService(c); s != nil {
			s.SetPen(c.Self, ch, ttl, false)
		}
	}, bind.Doc("Pen sets the channel and the lifetime of the next primitives this script draws: ttl 0 lasts one frame, a positive ttl lasts that many game seconds, a negative ttl lasts until Clear."),
		bind.Params("channel", "ttl"), bind.Example("debug.Pen(\"ai.path\", 2)"))
	bind.Proc2(m, "PenUnscaled", func(c *bind.Ctx, ch string, ttl float32) {
		if s := scriptService(c); s != nil {
			s.SetPen(c.Self, ch, ttl, true)
		}
	}, bind.Doc("PenUnscaled is Pen with a lifetime in real seconds; such primitives fade out while the game is paused."),
		bind.Params("channel", "ttl"), bind.Example("debug.PenUnscaled(\"ui\", 1)"))
	bind.Proc1(m, "Clear", func(c *bind.Ctx, prefix string) {
		if s := scriptService(c); s != nil {
			s.ClearChannel(prefix)
		}
	}, bind.Doc("Clear removes every primitive whose channel starts with prefix."),
		bind.Params("channel"), bind.Example("debug.Clear(\"ai\")"))
}
