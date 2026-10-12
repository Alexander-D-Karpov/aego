package sceneview

import (
	"aego/editor/document"
	"aego/scene"
)

func ModeFor(vm document.ViewMode, s *scene.Scene) Mode {
	switch vm {
	case document.View2D:
		return Mode2D
	case document.View3D:
		return Mode3D
	}
	if s != nil && s.DefaultSpace() == scene.Space3D {
		return Mode3D
	}
	return Mode2D
}

func (m Mode) ViewMode() document.ViewMode {
	if m == Mode3D {
		return document.View3D
	}
	return document.View2D
}

func (c *Controller3) LoadOrbit(o document.Orbit) {
	if o.Distance <= 0 {
		c.Cam = DefaultCamera3D()
		c.Cam.Ortho = o.Ortho
		return
	}
	c.Cam = Camera3D{Target: o.Target, Distance: o.Distance, Yaw: o.Yaw, Pitch: o.Pitch, FOV: o.FOV, Ortho: o.Ortho}
	if c.Cam.FOV <= 1 {
		c.Cam.FOV = 60
	}
}

func (c *Controller3) Orbit() document.Orbit {
	return document.Orbit{Target: c.Cam.Target, Distance: c.Cam.Distance, Yaw: c.Cam.Yaw, Pitch: c.Cam.Pitch, FOV: c.Cam.FOV, Ortho: c.Cam.Ortho}
}

func (p *Panel) Ortho() bool {
	return p.c3.Cam.Ortho
}

func (p *Panel) ToggleOrtho() {
	p.c3.Cam.Ortho = !p.c3.Cam.Ortho
}

func (p *Panel) Mode() Mode {
	return p.viewMode
}

func (p *Panel) SetMode(m Mode) {
	p.viewMode = m
}
