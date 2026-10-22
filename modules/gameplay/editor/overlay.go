package editor

import (
	"aego/core/mathx"
	"aego/editor/core/shape"
	"aego/modules/gameplay"
	"aego/render/debug"
	"aego/scene"
)

func outline(d *debug.Draw, m mathx.Mat3, r mathx.Rect, col mathx.Color) {
	p := [4]mathx.Vec2{{X: r.X, Y: r.Y}, {X: r.X + r.W, Y: r.Y}, {X: r.X + r.W, Y: r.Y + r.H}, {X: r.X, Y: r.Y + r.H}}
	for i := range p {
		d.Line(m.Apply(p[i]), m.Apply(p[(i+1)%4]), col)
	}
}

func eachArea[T any](d *debug.Draw, s *scene.Scene, col mathx.Color, get func(*T) (mathx.Rect, bool)) {
	scene.Query1[T](s).Each(func(h scene.NodeHandle, c *T) {
		if r, ok := get(c); ok && s.EffectiveEnabled(h) {
			outline(d, s.World(h), r, col)
		}
	})
}

func zones(d *debug.Draw, s *scene.Scene, dim bool) {
	pick := func(c mathx.Color) mathx.Color {
		if dim {
			return colDim
		}
		return c
	}
	eachArea(d, s, pick(colKill), func(c *gameplay.KillZone2D) (mathx.Rect, bool) { return c.Area, true })
	eachArea(d, s, pick(colCheck), func(c *gameplay.Checkpoint2D) (mathx.Rect, bool) { return c.Area, true })
	eachArea(d, s, pick(colPick), func(c *gameplay.Pickup2D) (mathx.Rect, bool) { return c.Area, true })
}

func (p *plugin) sceneOverlay(c *shape.OverlayCtx) {
	if c.Mode3D || c.Game || c.Debug == nil || c.Scene == nil || !p.areas() {
		return
	}
	zones(c.Debug, c.Scene, true)
	eachArea(c.Debug, c.Scene, colDim, func(h *gameplay.Hitbox2D) (mathx.Rect, bool) { return h.Area, true })
	eachArea(c.Debug, c.Scene, colDim, func(h *gameplay.Hurtbox2D) (mathx.Rect, bool) { return h.Area, true })
}

func (p *plugin) gameOverlay(c *shape.OverlayCtx) {
	if c.Mode3D || c.Debug == nil || c.Scene == nil || !p.areas() {
		return
	}
	s, d := c.Scene, c.Debug
	zones(d, s, false)
	eachArea(d, s, colHurt, func(h *gameplay.Hurtbox2D) (mathx.Rect, bool) { return h.Area, h.Active })
	eachArea(d, s, colHit, func(h *gameplay.Hitbox2D) (mathx.Rect, bool) { return h.Area, h.Active })
	scene.Query1[gameplay.Health](s).Each(func(h scene.NodeHandle, hp *gameplay.Health) {
		if !s.EffectiveEnabled(h) || hp.Max <= 0 {
			return
		}
		at := s.WorldPosition(h).Add(mathx.Vec2{Y: -0.9})
		w := float32(0.8)
		k := mathx.Clamp(float32(hp.Current)/float32(hp.Max), 0, 1)
		d.Line(at.Sub(mathx.Vec2{X: w / 2}), at.Add(mathx.Vec2{X: w / 2}), colHurtBar)
		d.Line(at.Sub(mathx.Vec2{X: w / 2}), at.Add(mathx.Vec2{X: w*k - w/2}), colHealth)
	})
}
