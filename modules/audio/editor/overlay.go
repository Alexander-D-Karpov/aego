package editor

import (
	"aego/core/mathx"
	"aego/editor/core/shape"
	"aego/modules/audio"
	"aego/render/debug"
	"aego/scene"
)

func drawZones(d *debug.Draw, s *scene.Scene, col mathx.Color) {
	scene.Query1[audio.AudioZone2D](s).Each(func(h scene.NodeHandle, z *audio.AudioZone2D) {
		if !s.EffectiveEnabled(h) {
			return
		}
		m := s.World(h)
		a := z.Area
		pts := [4]mathx.Vec2{{X: a.X, Y: a.Y}, {X: a.X + a.W, Y: a.Y}, {X: a.X + a.W, Y: a.Y + a.H}, {X: a.X, Y: a.Y + a.H}}
		for i := range pts {
			d.Line(m.Apply(pts[i]), m.Apply(pts[(i+1)%4]), col)
		}
	})
}

func (p *plugin) sceneOverlay(c *shape.OverlayCtx) {
	if c.Mode3D || c.Game || c.Debug == nil || c.Scene == nil || !p.ranges() {
		return
	}
	s := c.Scene
	drawZones(c.Debug, s, colDim)
	scene.Query1[audio.AudioSource2D](s).Each(func(h scene.NodeHandle, src *audio.AudioSource2D) {
		if src.Spatial && s.EffectiveEnabled(h) {
			c.Debug.Circle(s.WorldPosition(h), src.MaxDistance, 48, colDim)
		}
	})
}

func (p *plugin) gameOverlay(c *shape.OverlayCtx) {
	if c.Mode3D || c.Debug == nil || !p.ranges() {
		return
	}
	svc := audio.Of(c.World)
	if svc == nil {
		return
	}
	ear := svc.Listener()
	c.Debug.Cross(ear, c.Px*12, colEar)
	svc.EachVoice(func(v audio.VoiceInfo) {
		if !v.Spatial {
			return
		}
		col := colVoice
		col.A = 0.25 + 0.75*min(v.Gain, 1)
		c.Debug.Circle(v.Position, c.Px*6, 16, col)
		c.Debug.Line(v.Position, ear, col)
	})
	if c.Scene != nil {
		drawZones(c.Debug, c.Scene, colZone)
	}
}
