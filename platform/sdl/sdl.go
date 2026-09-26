package sdl

/*
#include <stdlib.h>
#include "aego_sdl.h"
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"

	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/platform"
	"aego/platform/input"
)

func init() {
	runtime.LockOSThread()
}

type Platform struct {
	win   *Window
	cev   C.aego_event
	info  platform.Info
	start uint64
}

func New() (*Platform, error) {
	if !C.aego_init() {
		return nil, errs.New(errs.PlatformInit, "SDL_Init failed", log.F("sdl", lastError()))
	}
	v := C.aego_version()
	p := &Platform{
		info: platform.Info{
			OS:      runtime.GOOS,
			Arch:    runtime.GOARCH,
			Backend: "sdl3",
			Version: fmt.Sprintf("%d.%d.%d",
				int(C.aego_version_major(v)),
				int(C.aego_version_minor(v)),
				int(C.aego_version_micro(v))),
			VideoDriver: C.GoString(C.aego_video_driver()),
		},
		start: uint64(C.aego_ticks_ns()),
	}
	return p, nil
}

func lastError() string {
	return C.GoString(C.aego_error())
}

func boolInt(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

func goStr(p *C.char) string {
	if p == nil {
		return ""
	}
	return C.GoString(p)
}

func (p *Platform) CreateWindow(desc platform.WindowDesc) (platform.Window, error) {
	if p.win != nil {
		return nil, errs.New(errs.PlatformWindow, "only one window is supported")
	}
	if !C.aego_gl_setup(boolInt(desc.Debug), boolInt(desc.SRGB)) {
		return nil, errs.New(errs.PlatformContext, "SDL_GL_SetAttribute failed", log.F("sdl", lastError()))
	}
	title := C.CString(desc.Title)
	defer C.free(unsafe.Pointer(title))
	sw := C.aego_create_window(title, C.int(desc.Width), C.int(desc.Height),
		boolInt(desc.Resizable), boolInt(desc.Fullscreen), boolInt(desc.HighDPI))
	if sw == nil {
		return nil, errs.New(errs.PlatformWindow, "SDL_CreateWindow failed", log.F("sdl", lastError()))
	}
	ctx := C.SDL_GL_CreateContext(sw)
	if ctx == nil {
		err := errs.New(errs.PlatformContext, "SDL_GL_CreateContext failed", log.F("sdl", lastError()))
		C.SDL_DestroyWindow(sw)
		return nil, err
	}
	if !C.SDL_GL_MakeCurrent(sw, ctx) {
		err := errs.New(errs.PlatformContext, "SDL_GL_MakeCurrent failed", log.F("sdl", lastError()))
		C.SDL_GL_DestroyContext(ctx)
		C.SDL_DestroyWindow(sw)
		return nil, err
	}
	w := &Window{p: p, win: sw, ctx: ctx, fullscreen: desc.Fullscreen, cursor: 255}
	w.SetVSync(desc.VSync)
	C.SDL_StopTextInput(sw)
	if !desc.Hidden {
		C.SDL_ShowWindow(sw)
	}
	w.refresh()
	p.win = w
	return w, nil
}

func (p *Platform) PollEvents(fn func(platform.Event)) {
	for C.aego_poll(&p.cev) != 0 {
		if ev, ok := p.translate(); ok {
			fn(ev)
		}
	}
}

func (p *Platform) translate() (platform.Event, bool) {
	c := &p.cev
	var ev platform.Event
	windowEvent := false
	switch c.kind {
	case C.AEGO_EV_QUIT:
		ev.Kind = platform.EvQuit
	case C.AEGO_EV_RESIZED:
		ev.Kind = platform.EvWindowResized
		windowEvent = true
	case C.AEGO_EV_FOCUS_GAINED:
		ev.Kind = platform.EvWindowFocusGained
	case C.AEGO_EV_FOCUS_LOST:
		ev.Kind = platform.EvWindowFocusLost
	case C.AEGO_EV_MINIMIZED:
		ev.Kind = platform.EvWindowMinimized
		windowEvent = true
	case C.AEGO_EV_RESTORED:
		ev.Kind = platform.EvWindowRestored
		windowEvent = true
	case C.AEGO_EV_KEY_DOWN, C.AEGO_EV_KEY_UP:
		ev.Kind = platform.EvKeyUp
		if c.kind == C.AEGO_EV_KEY_DOWN {
			ev.Kind = platform.EvKeyDown
		}
		scan := input.KeyUnknown
		if uint32(c.scancode) < input.ScancodeCount {
			scan = input.Scancode(c.scancode)
		}
		ev.Key = input.KeyEvent{
			Scan:   scan,
			Key:    input.Keycode(c.key),
			Mods:   input.Mods(c.mod),
			Repeat: c.repeat != 0,
		}
	case C.AEGO_EV_TEXT:
		ev.Kind = platform.EvTextInput
		n := int(c.textn)
		for i := 0; i < n; i++ {
			ev.Text[i] = byte(c.text[i])
		}
		ev.TextN = uint8(n)
	case C.AEGO_EV_TEXT_EDITING:
		ev.Kind = platform.EvTextEditing
		ev.Str = goStr(c.data)
		ev.EditStart = int32(c.start)
		ev.EditLen = int32(c.length)
	case C.AEGO_EV_DROP_BEGIN, C.AEGO_EV_DROP_FILE, C.AEGO_EV_DROP_TEXT, C.AEGO_EV_DROP_COMPLETE:
		switch c.kind {
		case C.AEGO_EV_DROP_BEGIN:
			ev.Kind = platform.EvDropBegin
		case C.AEGO_EV_DROP_FILE:
			ev.Kind = platform.EvDropFile
		case C.AEGO_EV_DROP_TEXT:
			ev.Kind = platform.EvDropText
		default:
			ev.Kind = platform.EvDropComplete
		}
		ev.Str = goStr(c.data)
		ev.Mouse.Pos = mathx.Vec2{X: float32(c.x), Y: float32(c.y)}
	case C.AEGO_EV_MOUSE_MOVE:
		ev.Kind = platform.EvMouseMove
		ev.Mouse = input.MouseEvent{
			Pos:   mathx.Vec2{X: float32(c.x), Y: float32(c.y)},
			Delta: mathx.Vec2{X: float32(c.dx), Y: float32(c.dy)},
		}
	case C.AEGO_EV_MOUSE_DOWN, C.AEGO_EV_MOUSE_UP:
		ev.Kind = platform.EvMouseUp
		if c.kind == C.AEGO_EV_MOUSE_DOWN {
			ev.Kind = platform.EvMouseDown
		}
		ev.Mouse = input.MouseEvent{
			Pos:    mathx.Vec2{X: float32(c.x), Y: float32(c.y)},
			Button: input.Button(c.button),
		}
	case C.AEGO_EV_WHEEL:
		ev.Kind = platform.EvMouseWheel
		ev.Mouse = input.MouseEvent{
			Pos:   mathx.Vec2{X: float32(c.x), Y: float32(c.y)},
			Wheel: mathx.Vec2{X: float32(c.wx), Y: float32(c.wy)},
		}
	default:
		return ev, false
	}
	if p.win != nil {
		if windowEvent {
			p.win.refresh()
		}
		ev.Window = p.win.metrics
	}
	return ev, true
}

func (p *Platform) Now() float64 {
	return float64(uint64(C.aego_ticks_ns())-p.start) / 1e9
}

func (p *Platform) Info() platform.Info {
	return p.info
}

func (p *Platform) Shutdown() {
	if p.win != nil {
		p.win.Close()
	}
	C.aego_quit()
}

type Window struct {
	p          *Platform
	win        *C.SDL_Window
	ctx        C.SDL_GLContext
	metrics    platform.WindowMetrics
	vsync      platform.VSyncMode
	fullscreen bool
	cursor     platform.CursorShape
	textInput  bool
}

func (w *Window) refresh() {
	m := C.aego_window_metrics(w.win)
	w.metrics = platform.WindowMetrics{
		LogicalW:     int(m.lw),
		LogicalH:     int(m.lh),
		PixelW:       int(m.pw),
		PixelH:       int(m.ph),
		PixelDensity: float32(m.density),
		DisplayScale: float32(m.scale),
	}
}

func (w *Window) Metrics() platform.WindowMetrics {
	return w.metrics
}

func (w *Window) SetTitle(s string) {
	C.aego_set_title(w.win, (*C.char)(unsafe.Pointer(unsafe.StringData(s))), C.size_t(len(s)))
}

func (w *Window) SetVSync(m platform.VSyncMode) platform.VSyncMode {
	switch m {
	case platform.VSyncAdaptive:
		if C.aego_set_swap_interval(-1) == 0 {
			C.aego_set_swap_interval(1)
		}
	case platform.VSyncOn:
		C.aego_set_swap_interval(1)
	default:
		C.aego_set_swap_interval(0)
	}
	switch v := int(C.aego_get_swap_interval()); {
	case v < 0:
		w.vsync = platform.VSyncAdaptive
	case v > 0:
		w.vsync = platform.VSyncOn
	default:
		w.vsync = platform.VSyncOff
	}
	return w.vsync
}

func (w *Window) SetFullscreen(on bool) {
	if C.SDL_SetWindowFullscreen(w.win, C.bool(on)) {
		w.fullscreen = on
	}
}

func (w *Window) SetTextInput(on bool) {
	if on {
		w.StartTextInput()
	} else {
		w.StopTextInput()
	}
}

func (w *Window) StartTextInput() {
	if w.win == nil || w.textInput {
		return
	}
	C.SDL_StartTextInput(w.win)
	w.textInput = true
}

func (w *Window) StopTextInput() {
	if w.win == nil || !w.textInput {
		return
	}
	C.SDL_StopTextInput(w.win)
	w.textInput = false
}

func (w *Window) SetTextInputArea(r mathx.Rect, cursor int) {
	if w.win == nil {
		return
	}
	C.aego_text_input_area(w.win, C.int(r.X), C.int(r.Y), C.int(r.W), C.int(r.H), C.int(cursor))
}

func (w *Window) ClipboardText() string {
	p := C.SDL_GetClipboardText()
	if p == nil {
		return ""
	}
	s := C.GoString(p)
	C.SDL_free(unsafe.Pointer(p))
	return s
}

func (w *Window) SetClipboardText(s string) {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	C.SDL_SetClipboardText(cs)
}

func (w *Window) SetClipboardData(mime string, data []byte, text string) bool {
	cm := C.CString(mime)
	defer C.free(unsafe.Pointer(cm))
	var dp unsafe.Pointer
	if len(data) > 0 {
		dp = unsafe.Pointer(&data[0])
	}
	var tp *C.char
	if len(text) > 0 {
		tp = (*C.char)(unsafe.Pointer(unsafe.StringData(text)))
	}
	return bool(C.aego_set_clipboard(cm, dp, C.size_t(len(data)), tp, C.size_t(len(text))))
}

func (w *Window) ClipboardData(mime string) ([]byte, bool) {
	cm := C.CString(mime)
	defer C.free(unsafe.Pointer(cm))
	var n C.size_t
	p := C.aego_get_clipboard(cm, &n)
	if p == nil {
		return nil, false
	}
	out := C.GoBytes(p, C.int(n))
	C.SDL_free(p)
	return out, true
}

func (w *Window) SetCursor(shape platform.CursorShape) {
	if shape == w.cursor {
		return
	}
	w.cursor = shape
	C.aego_set_cursor(C.int(shape))
}

func (w *Window) CaptureMouse(on bool) {
	C.SDL_CaptureMouse(C.bool(on))
}

func (w *Window) SwapBuffers() {
	C.SDL_GL_SwapWindow(w.win)
}

func (w *Window) MakeCurrent() error {
	if !C.SDL_GL_MakeCurrent(w.win, w.ctx) {
		return errs.New(errs.PlatformContext, "SDL_GL_MakeCurrent failed", log.F("sdl", lastError()))
	}
	return nil
}

func (w *Window) GLProcAddress(name string) unsafe.Pointer {
	cs := C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	return C.aego_gl_proc(cs)
}

func (w *Window) Close() {
	if w.win == nil {
		return
	}
	if w.ctx != nil {
		C.SDL_GL_DestroyContext(w.ctx)
		w.ctx = nil
	}
	C.SDL_DestroyWindow(w.win)
	w.win = nil
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
