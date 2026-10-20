package editor

import (
	"aego/core/mathx"
	"aego/editor/core/shape"
	"aego/modules/camera"
	"aego/render/debug"
	"aego/scene"
)

func drawZones(d *debug.Draw, s *scene.Scene, col mathx.Color) {
	scene.Query1[camera.CameraZone2D](s).Each(func(h scene.NodeHandle, z *camera.CameraZone2D) {
		if !s.EffectiveEnabled(h) {
			return
		}
		c := camera.ZoneCorners(s.World(h), z.Area)
		for i := range c {
			d.Line(c[i], c[(i+1)%4], col)
		}
	})
}

func (p *plugin) frames() bool {
	return p.ed != nil && p.ed.Overlay.On(CmdFrames)
}

func (p *plugin) sceneOverlay(c *shape.OverlayCtx) {
	if c.Mode3D || c.Game || c.Debug == nil || c.Scene == nil || !p.frames() {
		return
	}
	drawZones(c.Debug, c.Scene, colDim)
	scene.Query1[camera.VirtualCamera2D](c.Scene).Each(func(h scene.NodeHandle, vc *camera.VirtualCamera2D) {
		if vc.UseBounds {
			c.Debug.Rect(vc.Bounds, colDim)
		}
	})
}

func (p *plugin) gameOverlay(c *shape.OverlayCtx) {
	if c.Mode3D || c.Debug == nil || c.Scene == nil || !p.frames() {
		return
	}
	s := camera.Of(c.World)
	if s == nil {
		return
	}
	s.Outputs(func(o camera.OutputInfo) {
		if !o.HasView || o.Active.IsZero() {
			return
		}
		size := camera.ViewSize(o.View, o.Logical.Zoom)
		if f, ok := scene.Get[camera.CameraFollow2D](c.Scene, o.Active); ok {
			c.Debug.Rect(camera.DeadzoneRect(o.Logical.Position, size, f.Deadzone), colDeadzone)
			if th, ok := f.Target.Resolve(c.Scene); ok && c.Scene.IsAlive(th) {
				c.Debug.Cross(c.Scene.WorldPosition(th), c.Px*10, colDeadzone)
			}
		}
		if vc, ok := scene.Get[camera.VirtualCamera2D](c.Scene, o.Active); ok && vc.UseBounds {
			c.Debug.Rect(vc.Bounds, colBounds)
		}
	})
	drawZones(c.Debug, c.Scene, colZone)
}
