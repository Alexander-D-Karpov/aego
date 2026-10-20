package headless

import (
	"runtime"
	"strconv"

	"aego/core/clock"
	"aego/core/errs"
	"aego/core/mathx"
	"aego/platform"
)

type Options struct {
	FrameTime float64
	Displays  []platform.Display
}

type Platform struct {
	opts     Options
	clock    *clock.Clock
	sim      float64
	win      *Window
	queue    []platform.Event
	events   []platform.Event
	clipText string
	clipData map[string][]byte
}

func New(opts Options) *Platform {
	return &Platform{
		opts:     opts,
		clock:    clock.New(),
		queue:    make([]platform.Event, 0, 16),
		events:   make([]platform.Event, 0, 16),
		clipData: map[string][]byte{},
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
	textInput  bool
	textArea   mathx.Rect
	cursor     platform.CursorShape
	captured   bool
	mode       platform.WindowMode
	display    int
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

func (w *Window) StartTextInput() {
	w.textInput = true
}

func (w *Window) StopTextInput() {
	w.textInput = false
}

func (w *Window) TextInputActive() bool {
	return w.textInput
}

func (w *Window) SetTextInputArea(r mathx.Rect, cursor int) {
	w.textArea = r
}

func (w *Window) ClipboardText() string {
	if w.p == nil {
		return ""
	}
	return w.p.clipText
}

func (w *Window) SetClipboardText(s string) {
	if w.p == nil {
		return
	}
	w.p.clipText = s
	clear(w.p.clipData)
}

func (w *Window) SetClipboardData(mime string, data []byte, text string) bool {
	if w.p == nil {
		return false
	}
	clear(w.p.clipData)
	w.p.clipData[mime] = append([]byte(nil), data...)
	w.p.clipText = text
	return true
}

func (w *Window) ClipboardData(mime string) ([]byte, bool) {
	if w.p == nil {
		return nil, false
	}
	d, ok := w.p.clipData[mime]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), d...), true
}

func (w *Window) SetCursor(s platform.CursorShape) {
	w.cursor = s
}

func (w *Window) Cursor() platform.CursorShape {
	return w.cursor
}

func (w *Window) CaptureMouse(on bool) {
	w.captured = on
}

func (w *Window) SwapBuffers() {}

func (w *Window) Close() {
	if w.p != nil && w.p.win == w {
		w.p.win = nil
	}
}

var (
	_ platform.TextInputer   = (*Window)(nil)
	_ platform.Clipboard     = (*Window)(nil)
	_ platform.Cursorer      = (*Window)(nil)
	_ platform.MouseCapturer = (*Window)(nil)
)

func defaultDisplays() []platform.Display {
	return []platform.Display{{
		ID: 1, Name: "Headless", Current: platform.DisplayMode{W: 1920, H: 1080, Hz: 60},
		Modes: []platform.DisplayMode{{W: 1920, H: 1080, Hz: 60}, {W: 1280, H: 720, Hz: 60}},
	}}
}

func (w *Window) Displays() []platform.Display {
	if w.p != nil && len(w.p.opts.Displays) > 0 {
		return w.p.opts.Displays
	}
	return defaultDisplays()
}

func (w *Window) WindowDisplay() int {
	return w.display
}

func (w *Window) WindowMode() platform.WindowMode {
	return w.mode
}

func (w *Window) SetWindowMode(m platform.WindowMode, display int, size platform.DisplayMode) error {
	ds := w.Displays()
	if display < 0 || display >= len(ds) {
		return errs.New(errs.PlatformWindow, "there is no display "+strconv.Itoa(display))
	}
	d := &ds[display]
	switch m {
	case platform.Fullscreen:
		if size.W <= 0 {
			size = d.Current
		}
		if !d.Has(size) {
			return errs.New(errs.PlatformWindow, "display "+d.Name+" has no mode "+strconv.Itoa(size.W)+"x"+strconv.Itoa(size.H))
		}
	case platform.Borderless:
		size = d.Current
	}
	w.mode, w.display, w.fullscreen = m, display, m != platform.Windowed
	if size.W > 0 && size.H > 0 && w.p != nil {
		mt := w.metrics
		mt.LogicalW, mt.LogicalH, mt.PixelW, mt.PixelH = size.W, size.H, size.W, size.H
		w.p.Push(platform.Event{Kind: platform.EvWindowResized, Window: mt})
	}
	return nil
}

var _ platform.Displayer = (*Window)(nil)
