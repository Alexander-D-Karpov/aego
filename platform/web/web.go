//go:build js

package web

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"syscall/js"

	"aego/core/errs"
	"aego/core/mathx"
	"aego/platform"
	"aego/platform/input"
)

const (
	MimeText    = "text/plain"
	CanvasAttr  = "data-aego"
	contextName = "webgl2"
)

type Options struct {
	Canvas   js.Value
	Selector string
}

type listener struct {
	target js.Value
	name   string
	fn     js.Func
}

type Platform struct {
	opt       Options
	global    js.Value
	doc       js.Value
	nav       js.Value
	raf       js.Value
	now       js.Value
	gamepads  js.Value
	info      platform.Info
	win       *Window
	queue     []platform.Event
	spare     []platform.Event
	listeners []listener
	noop      js.Func
	start     float64
	hiddenAt  float64
	hiddenFor float64
	hidden    bool
	padLive   int
	pads      [input.MaxPads]padSlot
	pad       padFrame
}

func try(v js.Value, name string, args ...any) js.Value {
	if !v.Truthy() || v.Get(name).Type() != js.TypeFunction {
		return js.Undefined()
	}
	return v.Call(name, args...)
}

func New(opt Options) (*Platform, error) {
	g := js.Global()
	doc := g.Get("document")
	if !doc.Truthy() {
		return nil, errs.New(errs.PlatformInit, "the web platform needs a browser document")
	}
	perf := g.Get("performance")
	nav := g.Get("navigator")
	p := &Platform{
		opt:    opt,
		global: g,
		doc:    doc,
		nav:    nav,
		raf:    g.Get("requestAnimationFrame").Call("bind", g),
		now:    perf.Get("now").Call("bind", perf),
		queue:  make([]platform.Event, 0, 64),
		spare:  make([]platform.Event, 0, 64),
	}
	if f := nav.Get("getGamepads"); f.Type() == js.TypeFunction {
		p.gamepads = f.Call("bind", nav)
	}
	p.info = platform.Info{OS: "js", Arch: "wasm", Backend: "web", Version: nav.Get("userAgent").String(), VideoDriver: contextName}
	p.noop = js.FuncOf(func(js.Value, []js.Value) any { return nil })
	p.start = p.ms()
	p.hidden = doc.Get("hidden").Truthy()
	p.hiddenAt = p.start
	p.listen(doc, "visibilitychange", true, p.visibility)
	p.listen(g, "gamepadconnected", true, func(js.Value) { p.padLive++ })
	p.listen(g, "gamepaddisconnected", true, func(js.Value) { p.padLive = max(0, p.padLive-1) })
	p.listen(g, "blur", true, func(js.Value) { p.push(platform.Event{Kind: platform.EvWindowFocusLost}) })
	return p, nil
}

func (p *Platform) ms() float64 {
	return p.now.Invoke().Float()
}

func (p *Platform) Now() float64 {
	t := p.ms()
	if p.hidden {
		t = p.hiddenAt
	}
	return (t - p.start - p.hiddenFor) / 1000
}

func (p *Platform) visibility(js.Value) {
	hidden := p.doc.Get("hidden").Truthy()
	if hidden == p.hidden {
		return
	}
	t := p.ms()
	if hidden {
		p.hiddenAt = t
		p.push(platform.Event{Kind: platform.EvWindowFocusLost})
	} else {
		p.hiddenFor += t - p.hiddenAt
	}
	p.hidden = hidden
}

func (p *Platform) quiet(v js.Value) {
	if v.Truthy() && v.Get("catch").Type() == js.TypeFunction {
		v.Call("catch", p.noop)
	}
}

func (p *Platform) writeText(s string) {
	p.quiet(try(p.nav.Get("clipboard"), "writeText", s))
}

func (p *Platform) canvas() js.Value {
	c := p.opt.Canvas
	if !c.Truthy() && p.opt.Selector != "" {
		c = p.doc.Call("querySelector", p.opt.Selector)
	}
	if !c.Truthy() {
		c = p.doc.Call("querySelector", "canvas["+CanvasAttr+"]")
	}
	if !c.Truthy() {
		c = p.doc.Call("createElement", "canvas")
		c.Call("setAttribute", CanvasAttr, "")
		p.doc.Get("body").Call("appendChild", c)
	}
	return c
}

func (p *Platform) CreateWindow(desc platform.WindowDesc) (platform.Window, error) {
	if p.win != nil {
		return nil, errs.New(errs.PlatformWindow, "only one window is supported")
	}
	c := p.canvas()
	ctx := c.Call("getContext", contextName, map[string]any{
		"alpha": false, "antialias": false, "depth": false, "stencil": false,
		"premultipliedAlpha": false, "preserveDrawingBuffer": false, "powerPreference": "high-performance",
	})
	if !ctx.Truthy() {
		return nil, errs.New(errs.PlatformContext, "this browser has no WebGL2")
	}
	w := &Window{p: p, canvas: c, style: c.Get("style"), ctx: ctx, cursor: 255}
	c.Set("tabIndex", 0)
	w.style.Set("touchAction", "none")
	w.style.Set("outline", "none")
	if r := c.Call("getBoundingClientRect"); r.Get("width").Float() == 0 || r.Get("height").Float() == 0 {
		w.style.Set("width", strconv.Itoa(max(desc.Width, 1))+"px")
		w.style.Set("height", strconv.Itoa(max(desc.Height, 1))+"px")
	}
	p.win = w
	w.refresh(false)
	p.listenWindow(w)
	w.wantFull = desc.Fullscreen
	return w, nil
}

func (p *Platform) PollEvents(fn func(platform.Event)) {
	if w := p.win; w != nil && p.global.Get("devicePixelRatio").Float() != w.dpr {
		w.refresh(true)
	}
	p.pollPads()
	if len(p.queue) == 0 {
		return
	}
	p.spare, p.queue = p.queue, p.spare[:0]
	for i := range p.spare {
		fn(p.spare[i])
	}
	p.spare = p.spare[:0]
}

func (p *Platform) Drive(step func() bool) {
	done := make(chan struct{})
	var frame js.Func
	frame = js.FuncOf(func(js.Value, []js.Value) any {
		if !step() {
			frame.Release()
			close(done)
			return nil
		}
		p.raf.Invoke(frame)
		return nil
	})
	p.raf.Invoke(frame)
	<-done
}

func (p *Platform) Info() platform.Info {
	return p.info
}

func (p *Platform) Shutdown() {
	if p.win != nil {
		p.win.Close()
	}
	for _, l := range p.listeners {
		l.target.Call("removeEventListener", l.name, l.fn)
		l.fn.Release()
	}
	p.listeners = nil
}

type memory struct {
	mime string
	data []byte
	text string
}

type Window struct {
	p         *Platform
	canvas    js.Value
	style     js.Value
	ctx       js.Value
	observer  js.Value
	resize    js.Func
	observing bool
	metrics   platform.WindowMetrics
	dpr       float64
	last      mathx.Vec2
	textInput bool
	cursor    platform.CursorShape
	wantLock  bool
	locked    bool
	wantFull  bool
	clip      memory
	closed    bool
}

func (w *Window) WebGL2() js.Value {
	return w.ctx
}

func (w *Window) Canvas() js.Value {
	return w.canvas
}

func (w *Window) refresh(push bool) {
	r := w.canvas.Call("getBoundingClientRect")
	cw, ch := r.Get("width").Float(), r.Get("height").Float()
	dpr := w.p.global.Get("devicePixelRatio").Float()
	if dpr <= 0 {
		dpr = 1
	}
	w.dpr = dpr
	m := platform.WindowMetrics{
		LogicalW:     int(math.Round(cw)),
		LogicalH:     int(math.Round(ch)),
		PixelW:       max(1, int(math.Round(cw*dpr))),
		PixelH:       max(1, int(math.Round(ch*dpr))),
		PixelDensity: float32(dpr),
		DisplayScale: float32(dpr),
	}
	if m == w.metrics {
		return
	}
	if w.canvas.Get("width").Int() != m.PixelW {
		w.canvas.Set("width", m.PixelW)
	}
	if w.canvas.Get("height").Int() != m.PixelH {
		w.canvas.Set("height", m.PixelH)
	}
	w.metrics = m
	if push {
		w.p.push(platform.Event{Kind: platform.EvWindowResized})
	}
}

func (w *Window) pos(e js.Value) mathx.Vec2 {
	return mathx.Vec2{X: float32(e.Get("offsetX").Float()), Y: float32(e.Get("offsetY").Float())}
}

func (w *Window) gesture() {
	if w.wantLock && !w.locked {
		w.p.quiet(try(w.canvas, "requestPointerLock"))
	}
	if w.wantFull {
		w.wantFull = false
		if !w.p.doc.Get("fullscreenElement").Truthy() {
			w.p.quiet(try(w.canvas, "requestFullscreen"))
		}
	}
}

func (w *Window) Metrics() platform.WindowMetrics {
	return w.metrics
}

func (w *Window) SetTitle(s string) {
	w.p.doc.Set("title", s)
}

func (w *Window) SetVSync(platform.VSyncMode) platform.VSyncMode {
	return platform.VSyncOn
}

func (w *Window) SetFullscreen(on bool) {
	w.wantFull = on
	if !on && w.p.doc.Get("fullscreenElement").Truthy() {
		w.p.quiet(try(w.p.doc, "exitFullscreen"))
	}
}

func (w *Window) SetPointerLock(on bool) {
	w.wantLock = on
	if !on && w.locked {
		try(w.p.doc, "exitPointerLock")
	}
}

func (w *Window) PointerLocked() bool {
	return w.locked
}

func (w *Window) SwapBuffers() {}

func (w *Window) StartTextInput() {
	w.textInput = true
}

func (w *Window) StopTextInput() {
	w.textInput = false
}

func (w *Window) SetTextInputArea(mathx.Rect, int) {}

func textMime(m string) bool {
	return m == MimeText || strings.HasPrefix(m, MimeText+";")
}

func (w *Window) ClipboardText() string {
	return w.clip.text
}

func (w *Window) SetClipboardText(s string) {
	w.clip = memory{text: s}
	w.p.writeText(s)
}

func (w *Window) SetClipboardData(mime string, data []byte, text string) bool {
	w.clip = memory{mime: mime, data: bytes.Clone(data), text: text}
	w.p.writeText(text)
	return true
}

func (w *Window) ClipboardData(mime string) ([]byte, bool) {
	switch {
	case mime != "" && mime == w.clip.mime:
		return bytes.Clone(w.clip.data), true
	case textMime(mime) && w.clip.text != "":
		return []byte(w.clip.text), true
	}
	return nil, false
}

func (w *Window) SetCursor(s platform.CursorShape) {
	if s == w.cursor {
		return
	}
	w.cursor = s
	w.style.Set("cursor", cursorName(s))
}

func (w *Window) CaptureMouse(bool) {}

func (w *Window) Close() {
	if w.closed {
		return
	}
	w.closed = true
	if w.observing {
		w.observer.Call("disconnect")
		w.resize.Release()
		w.observing = false
	}
	if w.p.win == w {
		w.p.win = nil
	}
}

var (
	_ platform.Driver        = (*Platform)(nil)
	_ platform.TextInputer   = (*Window)(nil)
	_ platform.Clipboard     = (*Window)(nil)
	_ platform.Cursorer      = (*Window)(nil)
	_ platform.MouseCapturer = (*Window)(nil)
)
