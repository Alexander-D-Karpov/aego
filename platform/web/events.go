//go:build js

package web

import (
	"syscall/js"

	"aego/platform"
	"aego/platform/input"
)

func (p *Platform) listen(target js.Value, name string, passive bool, fn func(js.Value)) {
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(args[0])
		} else {
			fn(js.Undefined())
		}
		return nil
	})
	target.Call("addEventListener", name, f, map[string]any{"passive": passive})
	p.listeners = append(p.listeners, listener{target: target, name: name, fn: f})
}

func (p *Platform) push(ev platform.Event) {
	if p.win != nil {
		ev.Window = p.win.metrics
	}
	p.queue = append(p.queue, ev)
}

func prevent(e js.Value) {
	e.Call("preventDefault")
}

func (p *Platform) listenWindow(w *Window) {
	c := w.canvas
	p.listen(c, "keydown", false, func(e js.Value) { p.key(e, true) })
	p.listen(c, "keyup", false, func(e js.Value) { p.key(e, false) })
	p.listen(c, "pointerdown", false, func(e js.Value) { p.button(e, true) })
	p.listen(c, "pointerup", false, func(e js.Value) { p.button(e, false) })
	p.listen(c, "pointercancel", true, p.cancel)
	p.listen(c, "pointermove", true, p.move)
	p.listen(c, "wheel", false, p.wheel)
	p.listen(c, "contextmenu", false, prevent)
	p.listen(c, "focus", true, func(js.Value) { p.push(platform.Event{Kind: platform.EvWindowFocusGained}) })
	// Device invalidation and resource reconstruction can block the browser's
	// event loop. Rebase the first ordinary frame on each edge; simulation may
	// continue to advance normally between the two events.
	for _, event := range []string{"webglcontextlost", "webglcontextrestored"} {
		p.listen(c, event, true, func(js.Value) { p.resetFrameClock = true })
	}
	p.listen(c, "blur", true, func(js.Value) { p.push(platform.Event{Kind: platform.EvWindowFocusLost}) })
	p.listen(p.doc, "paste", true, p.paste)
	p.listen(p.doc, "pointerlockchange", true, p.lockChanged)
	p.listen(p.global, "resize", true, func(js.Value) {
		if p.win != nil {
			p.win.refresh(true)
		}
	})
	if mm := p.global.Get("matchMedia"); mm.Type() == js.TypeFunction {
		w.dark = p.global.Call("matchMedia", "(prefers-color-scheme: dark)")
		p.listen(w.dark, "change", true, func(js.Value) { p.push(platform.Event{Kind: platform.EvSystemTheme}) })
	}
	if ro := p.global.Get("ResizeObserver"); ro.Type() == js.TypeFunction {
		w.resize = js.FuncOf(func(js.Value, []js.Value) any {
			w.refresh(true)
			return nil
		})
		w.observer = ro.New(w.resize)
		w.observer.Call("observe", c)
		w.observing = true
	}
}

func (p *Platform) key(e js.Value, down bool) {
	w := p.win
	if w == nil {
		return
	}
	code := e.Get("code").String()
	key := e.Get("key").String()
	mods := modsOf(
		e.Get("shiftKey").Truthy(), e.Get("ctrlKey").Truthy(), e.Get("altKey").Truthy(), e.Get("metaKey").Truthy(),
		try(e, "getModifierState", "CapsLock").Truthy(), try(e, "getModifierState", "NumLock").Truthy(),
	)
	scan := scancodeOf(code)
	kind := platform.EvKeyUp
	if down {
		kind = platform.EvKeyDown
	}
	p.push(platform.Event{Kind: kind, Key: input.KeyEvent{Scan: scan, Key: keycodeOf(key, scan), Mods: mods, Repeat: down && e.Get("repeat").Truthy()}})
	if down && w.textInput {
		if s, ok := textOf(key, mods); ok {
			ev := platform.Event{Kind: platform.EvTextInput}
			ev.TextN = uint8(copy(ev.Text[:], s))
			p.push(ev)
		}
	}
	if !browserKey(code, mods) {
		e.Call("preventDefault")
	}
	if down {
		w.gesture()
	}
}

func (p *Platform) button(e js.Value, down bool) {
	w := p.win
	if w == nil || !e.Get("isPrimary").Truthy() {
		return
	}
	b, ok := pointerButton(e.Get("button").Int())
	if !ok {
		return
	}
	if !w.locked {
		w.last = w.pos(e)
	}
	kind := platform.EvMouseUp
	if down {
		kind = platform.EvMouseDown
		try(w.canvas, "focus")
		try(w.canvas, "setPointerCapture", e.Get("pointerId"))
		e.Call("preventDefault")
		w.gesture()
	}
	p.push(platform.Event{Kind: kind, Mouse: input.MouseEvent{Pos: w.last, Button: b}})
}

func (p *Platform) cancel(e js.Value) {
	if w := p.win; w != nil && e.Get("isPrimary").Truthy() {
		p.push(platform.Event{Kind: platform.EvMouseUp, Mouse: input.MouseEvent{Pos: w.last, Button: input.MouseLeft}})
	}
}

func (p *Platform) move(e js.Value) {
	w := p.win
	if w == nil || !e.Get("isPrimary").Truthy() {
		return
	}
	if !w.locked {
		w.last = w.pos(e)
	}
	d := input.MouseEvent{Pos: w.last}
	d.Delta.X = float32(e.Get("movementX").Float())
	d.Delta.Y = float32(e.Get("movementY").Float())
	p.push(platform.Event{Kind: platform.EvMouseMove, Mouse: d})
}

func (p *Platform) wheel(e js.Value) {
	w := p.win
	if w == nil {
		return
	}
	e.Call("preventDefault")
	v, precise := wheelOf(e.Get("deltaX").Float(), e.Get("deltaY").Float(), e.Get("deltaMode").Int())
	p.push(platform.Event{Kind: platform.EvMouseWheel, Mouse: input.MouseEvent{Pos: w.pos(e), Wheel: v, Precise: precise}})
}

func (p *Platform) paste(e js.Value) {
	if p.win == nil {
		return
	}
	if d := e.Get("clipboardData"); d.Truthy() {
		p.win.clip = memory{text: d.Call("getData", MimeText).String()}
	}
}

func (p *Platform) lockChanged(js.Value) {
	if w := p.win; w != nil {
		w.locked = p.doc.Get("pointerLockElement").Equal(w.canvas)
	}
}
