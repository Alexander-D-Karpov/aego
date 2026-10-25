package sdl

/*
#include <stdlib.h>
#include "aego_sdl.h"
*/
import "C"

import (
	"fmt"
	"image"
	"image/draw"
	"runtime"
	"slices"
	"strconv"
	"unsafe"

	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/platform"
	"aego/platform/input"
)

func init() {
	runtime.LockOSThread()
	platform.SetAlert(alert)
}

type Platform struct {
	win       *Window
	audio     *Audio
	cev       C.aego_event
	info      platform.Info
	start     uint64
	dialogs   map[int]func(string, bool)
	dialogSeq int
}

func fractional(v float32) bool {
	return v != float32(int32(v))
}

func cstr(s string) *C.char {
	if s == "" {
		return nil
	}
	return C.CString(s)
}

func setMetadata(m platform.AppMetadata) {
	if m.Name == "" {
		return
	}
	name, ver, id := cstr(m.Name), cstr(m.Version), cstr(m.Identifier)
	defer C.free(unsafe.Pointer(name))
	if ver != nil {
		defer C.free(unsafe.Pointer(ver))
	}
	if id != nil {
		defer C.free(unsafe.Pointer(id))
	}
	C.SDL_SetAppMetadata(name, ver, id)
}

func glMessage(found string) string {
	if found == "" {
		found = "no OpenGL at all"
	}
	return "OpenGL 3.3 is required, the graphics driver provides " + found +
		". Install the driver from the GPU vendor; in a virtual machine or over Remote Desktop turn on 3D acceleration or run Aego on the host"
}

func New() (*Platform, error) {
	setMetadata(platform.AppMeta())
	platform.Boot("sdl", "init")
	if !C.aego_init() {
		msg := lastError()
		platform.Boot("sdl", "init failed: "+msg)
		return nil, errs.New(errs.PlatformInit, "SDL_Init failed", log.F("sdl", msg))
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
	platform.Boot("sdl", "SDL "+p.info.Version+", video "+p.info.VideoDriver)
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
	platform.Boot("window", fmt.Sprintf("%dx%d hidden=%v", desc.Width, desc.Height, desc.Hidden))
	sw := C.aego_create_window(title, C.int(desc.Width), C.int(desc.Height),
		boolInt(desc.Resizable), boolInt(desc.Fullscreen), boolInt(desc.HighDPI))
	if sw == nil {
		msg := lastError()
		platform.Boot("window", "failed: "+msg)
		return nil, errs.New(errs.PlatformWindow, "cannot open a window: "+msg, log.F("sdl", msg))
	}
	ctx := C.SDL_GL_CreateContext(sw)
	if ctx == nil {
		msg := lastError()
		found := C.GoString(C.aego_gl_probe(sw))
		platform.Boot("gl", "3.3 core context failed: "+msg+"; driver has "+found)
		C.SDL_DestroyWindow(sw)
		return nil, errs.New(errs.PlatformContext, glMessage(found), log.F("sdl", msg))
	}
	if !C.SDL_GL_MakeCurrent(sw, ctx) {
		msg := lastError()
		platform.Boot("gl", "make current failed: "+msg)
		C.SDL_GL_DestroyContext(ctx)
		C.SDL_DestroyWindow(sw)
		return nil, errs.New(errs.PlatformContext, "SDL_GL_MakeCurrent failed", log.F("sdl", msg))
	}
	platform.Boot("gl", C.GoString(C.aego_gl_string(0x1F02))+" ("+C.GoString(C.aego_gl_string(0x1F01))+")")
	w := &Window{p: p, win: sw, ctx: ctx, fullscreen: desc.Fullscreen, cursor: 255}
	w.SetVSync(desc.VSync)
	C.SDL_StopTextInput(sw)
	w.SetIcon(platform.AppIcon())
	if !desc.Hidden {
		C.SDL_ShowWindow(sw)
	}
	w.refresh()
	p.win = w
	platform.Boot("window", "shown")
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
		w := mathx.Vec2{X: float32(c.wx), Y: float32(c.wy)}
		ev.Mouse = input.MouseEvent{
			Pos:     mathx.Vec2{X: float32(c.x), Y: float32(c.y)},
			Wheel:   w,
			Precise: fractional(w.X) || fractional(w.Y),
			Flipped: c.flipped != 0,
		}
	case C.AEGO_EV_PAD_ADDED:
		ev.Kind = platform.EvPadAdded
		ev.Pad = input.PadEvent{ID: uint32(c.pad), Name: C.GoStringN(&c.text[0], c.textn)}
	case C.AEGO_EV_PAD_REMOVED:
		ev.Kind = platform.EvPadRemoved
		ev.Pad = input.PadEvent{ID: uint32(c.pad)}
	case C.AEGO_EV_PAD_DOWN, C.AEGO_EV_PAD_UP:
		ev.Kind = platform.EvPadButtonDown
		if c.kind == C.AEGO_EV_PAD_UP {
			ev.Kind = platform.EvPadButtonUp
		}
		ev.Pad = input.PadEvent{ID: uint32(c.pad), Button: input.PadButton(c.button)}
	case C.AEGO_EV_PAD_AXIS:
		ev.Kind = platform.EvPadAxis
		ev.Pad = input.PadEvent{ID: uint32(c.pad), Axis: input.PadAxis(c.button), Value: float32(c.wx)}
	case C.AEGO_EV_AUDIO_ADDED, C.AEGO_EV_AUDIO_REMOVED, C.AEGO_EV_AUDIO_FORMAT:
		switch c.kind {
		case C.AEGO_EV_AUDIO_ADDED:
			ev.Kind = platform.EvAudioDeviceAdded
		case C.AEGO_EV_AUDIO_REMOVED:
			ev.Kind = platform.EvAudioDeviceRemoved
		default:
			ev.Kind = platform.EvAudioDeviceFormatChanged
		}
		id := uint32(c.device)
		ev.Audio = platform.AudioDevice{ID: id, Default: p.audio != nil && p.audio.dev == id}
	case C.AEGO_EV_SYSTEM_THEME:
		ev.Kind = platform.EvSystemTheme
	case C.AEGO_EV_DIALOG:
		p.finishDialog(c)
		return ev, false
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
	if p.audio != nil {
		p.audio.Close()
	}
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
	mode       platform.WindowMode
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

func (w *Window) SystemTheme() platform.SystemTheme {
	switch C.SDL_GetSystemTheme() {
	case C.SDL_SYSTEM_THEME_LIGHT:
		return platform.ThemeLight
	case C.SDL_SYSTEM_THEME_DARK:
		return platform.ThemeDark
	}
	return platform.ThemeUnknown
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
	_ platform.Displayer     = (*Window)(nil)
	_ platform.FileDialogs   = (*Window)(nil)
	_ platform.Iconer        = (*Window)(nil)
)

func nrgba(src image.Image) *image.NRGBA {
	b := src.Bounds()
	if n, ok := src.(*image.NRGBA); ok && b.Min == (image.Point{}) && n.Stride == 4*b.Dx() {
		return n
	}
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

func surface(src image.Image) *C.SDL_Surface {
	n := nrgba(src)
	b := n.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return nil
	}
	return C.aego_icon_surface(C.int(b.Dx()), C.int(b.Dy()), unsafe.Pointer(&n.Pix[0]))
}

func (w *Window) SetIcon(imgs []image.Image) {
	if w.win == nil || len(imgs) == 0 {
		return
	}
	list := slices.Clone(imgs)
	slices.SortFunc(list, func(a, b image.Image) int { return a.Bounds().Dx() - b.Bounds().Dx() })
	at := 0
	for at < len(list)-1 && list[at].Bounds().Dx() < 32 {
		at++
	}
	base := surface(list[at])
	if base == nil {
		return
	}
	for _, im := range list[at+1:] {
		if s := surface(im); s != nil {
			C.aego_icon_alternate(base, s)
		}
	}
	C.aego_set_icon(w.win, base)
}

func toMode(m C.aego_mode) platform.DisplayMode {
	return platform.DisplayMode{W: int(m.w), H: int(m.h), Hz: float32(m.hz)}
}

func (w *Window) displayIDs() []C.SDL_DisplayID {
	var ids [16]C.SDL_DisplayID
	n := int(C.aego_displays(&ids[0], 16))
	return append([]C.SDL_DisplayID(nil), ids[:n]...)
}

func (w *Window) Displays() []platform.Display {
	ids := w.displayIDs()
	out := make([]platform.Display, 0, len(ids))
	var modes [64]C.aego_mode
	for _, id := range ids {
		d := platform.Display{ID: uint32(id), Name: C.GoString(C.SDL_GetDisplayName(id))}
		var cur C.aego_mode
		if C.aego_display_current(id, &cur) {
			d.Current = toMode(cur)
		}
		n := int(C.aego_display_modes(id, &modes[0], 64))
		for i := 0; i < n; i++ {
			d.Modes = append(d.Modes, toMode(modes[i]))
		}
		out = append(out, d)
	}
	return out
}

func (w *Window) WindowDisplay() int {
	id := C.SDL_GetDisplayForWindow(w.win)
	for i, x := range w.displayIDs() {
		if x == id {
			return i
		}
	}
	return 0
}

func (w *Window) WindowMode() platform.WindowMode {
	return w.mode
}

func (w *Window) SetWindowMode(m platform.WindowMode, display int, size platform.DisplayMode) error {
	ids := w.displayIDs()
	if display < 0 || display >= len(ids) {
		return errs.New(errs.PlatformWindow, "there is no display "+strconv.Itoa(display))
	}
	if m == platform.Fullscreen && size.W <= 0 {
		var cur C.aego_mode
		if C.aego_display_current(ids[display], &cur) {
			size = toMode(cur)
		}
	}
	if !C.aego_set_window_mode(w.win, C.int(m), ids[display], C.int(size.W), C.int(size.H), C.float(size.Hz)) {
		return errs.New(errs.PlatformWindow, "cannot change the window mode", log.F("sdl", lastError()))
	}
	w.mode = m
	w.fullscreen = m != platform.Windowed
	w.refresh()
	return nil
}

func alert(title, msg string) bool {
	t, m := C.CString(title), C.CString(msg)
	defer C.free(unsafe.Pointer(t))
	defer C.free(unsafe.Pointer(m))
	return bool(C.aego_alert(t, m))
}

func freeAll(list ...*C.char) {
	for _, s := range list {
		if s != nil {
			C.free(unsafe.Pointer(s))
		}
	}
}

func (w *Window) ShowFileDialog(d platform.FileDialog, done func(string, bool)) error {
	p := w.p
	if p == nil || w.win == nil {
		return errs.New(errs.PlatformWindow, "the window is closed")
	}
	p.dialogSeq++
	id := p.dialogSeq
	if p.dialogs == nil {
		p.dialogs = map[int]func(string, bool){}
	}
	p.dialogs[id] = done
	start, name, pat := cstr(d.Start), cstr(d.Filter), cstr(d.Pattern)
	defer freeAll(start, name, pat)
	if !C.aego_dialog(C.int(id), C.int(d.Kind), w.win, start, name, pat) {
		delete(p.dialogs, id)
		return errs.New(errs.PlatformWindow, "file dialogs are not available", log.F("sdl", lastError()))
	}
	return nil
}

func (p *Platform) finishDialog(c *C.aego_event) {
	id := int(c.start)
	path := goStr(c.data)
	if c.data != nil {
		C.SDL_free(unsafe.Pointer(c.data))
	}
	fn := p.dialogs[id]
	delete(p.dialogs, id)
	if fn != nil {
		fn(path, c.length > 0)
	}
}
