package headless

import (
	"runtime"

	"aego/core/clock"
	"aego/core/errs"
	"aego/platform"
)

type Options struct {
	FrameTime float64
}

type Platform struct {
	opts   Options
	clock  *clock.Clock
	sim    float64
	win    *Window
	queue  []platform.Event
	events []platform.Event
}

func New(opts Options) *Platform {
	return &Platform{
		opts:   opts,
		clock:  clock.New(),
		queue:  make([]platform.Event, 0, 16),
		events: make([]platform.Event, 0, 16),
	}
}

func (p *Platform) CreateWindow(desc platform.WindowDesc) (platform.Window, error) {
	if p.win != nil {
		return nil, errs.New(errs.PlatformWindow, "headless platform supports a single window")
	}
	w, h := desc.Width, desc.Height
	if w <= 0 {
		w = 1280
	}
	if h <= 0 {
		h = 720
	}
	p.win = &Window{
		p:     p,
		vsync: desc.VSync,
		metrics: platform.WindowMetrics{
			LogicalW: w, LogicalH: h,
			PixelW: w, PixelH: h,
			PixelDensity: 1, DisplayScale: 1,
		},
	}
	return p.win, nil
}

func (p *Platform) Push(ev platform.Event) {
	if ev.Window == (platform.WindowMetrics{}) && p.win != nil {
		ev.Window = p.win.metrics
	}
	if ev.Kind == platform.EvWindowResized && p.win != nil {
		p.win.metrics = ev.Window
	}
	p.queue = append(p.queue, ev)
}

func (p *Platform) PollEvents(fn func(platform.Event)) {
	if p.opts.FrameTime > 0 {
		p.sim += p.opts.FrameTime
	}
	if len(p.queue) == 0 {
		return
	}
	p.events, p.queue = p.queue, p.events[:0]
	for _, ev := range p.events {
		fn(ev)
	}
	p.events = p.events[:0]
}

func (p *Platform) Now() float64 {
	if p.opts.FrameTime > 0 {
		return p.sim
	}
	return p.clock.Now()
}

func (p *Platform) Info() platform.Info {
	return platform.Info{
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		Backend:     "headless",
		Version:     "-",
		VideoDriver: "none",
	}
}

func (p *Platform) Shutdown() {
	p.win = nil
}

type Window struct {
	p          *Platform
	metrics    platform.WindowMetrics
	vsync      platform.VSyncMode
	fullscreen bool
}

func (w *Window) Metrics() platform.WindowMetrics {
	return w.metrics
}

func (w *Window) SetTitle(string) {}

func (w *Window) SetVSync(m platform.VSyncMode) platform.VSyncMode {
	w.vsync = m
	return m
}

func (w *Window) SetFullscreen(on bool) {
	w.fullscreen = on
}

func (w *Window) Fullscreen() bool {
	return w.fullscreen
}

func (w *Window) SwapBuffers() {}

func (w *Window) Close() {
	if w.p != nil && w.p.win == w {
		w.p.win = nil
	}
}
