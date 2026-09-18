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
	w := &Window{p: p, win: sw, ctx: ctx, fullscreen: desc.Fullscreen}
	w.SetVSync(desc.VSync)
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
		C.SDL_StartTextInput(w.win)
	} else {
		C.SDL_StopTextInput(w.win)
	}
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
