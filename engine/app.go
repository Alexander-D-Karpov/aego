package engine

import "aego/platform"

const (
	SystemAppFixed  = "app.fixed"
	SystemAppUpdate = "app.update"
	SystemAppRender = "app.render"
	ZoneFrameWait   = "app.frame_wait"
	ZoneIdleWait    = "app.idle_wait"
)

type App interface {
	Init(*Engine) error
	Event(platform.Event)
	FixedUpdate(dt float64)
	Update(dt float64)
	Render(alpha float64)
	Shutdown()
}

// FramePacer is an optional app capability invoked once after the render phase.
// Deliberate waits belong here, outside the render system's trace and render time.
type FramePacer interface {
	PaceFrame()
}

type appSystemKind uint8

const (
	appFixed appSystemKind = iota
	appUpdate
	appRender
)

type appSystem struct {
	e    *Engine
	kind appSystemKind
}

func (s *appSystem) Run(ctx *Context) {
	if s.e.app == nil {
		return
	}
	switch s.kind {
	case appFixed:
		s.e.app.FixedUpdate(ctx.Dt)
	case appUpdate:
		s.e.app.Update(ctx.Dt)
	case appRender:
		s.e.app.Render(ctx.Alpha)
	}
}
