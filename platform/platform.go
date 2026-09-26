package platform

import "aego/core/errs"

type VSyncMode uint8

const (
	VSyncOff VSyncMode = iota
	VSyncOn
	VSyncAdaptive
)

func (m VSyncMode) String() string {
	switch m {
	case VSyncOff:
		return "off"
	case VSyncOn:
		return "on"
	case VSyncAdaptive:
		return "adaptive"
	}
	return "unknown"
}

func ParseVSync(s string) (VSyncMode, error) {
	switch s {
	case "off", "0", "false":
		return VSyncOff, nil
	case "on", "1", "true":
		return VSyncOn, nil
	case "adaptive", "-1":
		return VSyncAdaptive, nil
	}
	return VSyncOff, errs.New(errs.PlatformConfig, "unknown vsync mode: "+s)
}

type WindowDesc struct {
	Title      string
	Width      int
	Height     int
	Resizable  bool
	Fullscreen bool
	HighDPI    bool
	SRGB       bool
	Debug      bool
	// Hidden creates the window without mapping it. The GL context is still
	// current, so benchmarks render at full rate with no compositor in the way.
	Hidden bool
	VSync  VSyncMode
}

type WindowMetrics struct {
	LogicalW, LogicalH int
	PixelW, PixelH     int
	PixelDensity       float32
	DisplayScale       float32
}

func (m WindowMetrics) Aspect() float32 {
	if m.PixelH == 0 {
		return 1
	}
	return float32(m.PixelW) / float32(m.PixelH)
}

func (m WindowMetrics) Density() float32 {
	if m.LogicalW > 0 && m.PixelW > 0 {
		return float32(m.PixelW) / float32(m.LogicalW)
	}
	if m.PixelDensity > 0 {
		return m.PixelDensity
	}
	return 1
}

func (m WindowMetrics) UIScale() float32 {
	if m.DisplayScale > 0 {
		return m.DisplayScale
	}
	return 1
}

type Info struct {
	OS, Arch, Backend, Version, VideoDriver string
}

type Platform interface {
	CreateWindow(WindowDesc) (Window, error)
	PollEvents(fn func(Event))
	Now() float64
	Info() Info
	Shutdown()
}

type Window interface {
	Metrics() WindowMetrics
	SetTitle(string)
	SetVSync(VSyncMode) VSyncMode
	SetFullscreen(bool)
	SwapBuffers()
	Close()
}
