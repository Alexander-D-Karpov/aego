package web

import (
	"aego/core/mathx"
	"aego/platform"
	"aego/platform/input"
)

// inputFence delays Inspector keys one UI frame for focus, preserving order without delaying game-view input.
type inputFence struct {
	hit     func(mathx.Vec2, bool) bool
	start   mathx.Vec2
	armed   bool
	wait    bool
	delayed bool
}

func (f *inputFence) reset() {
	f.armed, f.wait, f.delayed = false, false, false
}

func pointerEvent(kind platform.EventKind) bool {
	switch kind {
	case platform.EvMouseDown, platform.EvMouseUp, platform.EvMouseMove, platform.EvMouseWheel:
		return true
	}
	return false
}

func modifierKey(scan input.Scancode) bool {
	return scan >= input.KeyLCtrl && scan <= input.KeyRGUI
}

func interruptsFence(kind platform.EventKind) bool {
	switch kind {
	case platform.EvQuit, platform.EvWindowFocusLost, platform.EvWindowResized, platform.EvWindowMinimized,
		platform.EvDropBegin, platform.EvDropFile, platform.EvDropText, platform.EvDropComplete:
		return true
	}
	return false
}

// dispatch consumes a prefix and emits browser text captured before input was enabled only if delayed input still belongs to the canvas and text field.
func (f *inputFence) dispatch(events []platform.Event, focused, textInput bool, emit func(platform.Event)) int {
	if f.hit == nil {
		for _, ev := range events {
			emit(ev)
		}
		return len(events)
	}
	if f.delayed {
		// Interrupt stale Inspector input when focus, scene, viewport or modal state changes.
		interrupt := !focused || !f.hit(f.start, true)
		for _, ev := range events {
			interrupt = interrupt || interruptsFence(ev.Kind)
		}
		if interrupt {
			for _, ev := range events {
				if !isHeldInput(ev.Kind) {
					emit(ev)
				}
			}
			f.reset()
			return len(events)
		}
	}
	if f.wait {
		f.wait = false
		return 0
	}
	replay, keyboard, action := f.delayed, false, false
	for i, ev := range events {
		// Deliver earlier delayed keys before a later click can change their target.
		if replay && keyboard && pointerEvent(ev.Kind) {
			return i
		}
		// Keep each non-modifier key and its modifiers in a separate frame to preserve editing order.
		if replay && action && (ev.Kind == platform.EvKeyDown || ev.Kind == platform.EvKeyUp) &&
			(modifierKey(ev.Key.Scan) || ev.Kind == platform.EvKeyDown) {
			return i
		}
		if interruptsFence(ev.Kind) {
			f.armed = false
		}
		if replay && ev.Kind == platform.EvTextInput && !textInput {
			continue
		}
		switch ev.Kind {
		case platform.EvMouseDown:
			f.armed = ev.Mouse.Button == input.MouseLeft && f.hit(ev.Mouse.Pos, false)
			f.start = ev.Mouse.Pos
		case platform.EvMouseMove:
			if ev.Mouse.Pos.Sub(f.start).Len() >= 3 {
				f.armed = false
			}
		case platform.EvMouseUp:
			clicked := f.armed && ev.Mouse.Button == input.MouseLeft && ev.Mouse.Pos.Sub(f.start).Len() < 3 && f.hit(ev.Mouse.Pos, true)
			f.armed = false
			emit(ev)
			if clicked {
				f.wait, f.delayed = true, true
				return i + 1
			}
			continue
		case platform.EvKeyDown:
			keyboard = true
			if !modifierKey(ev.Key.Scan) {
				action = true
			}
		case platform.EvKeyUp:
			keyboard = true
		case platform.EvTextInput:
			keyboard = true
			action = true
		}
		emit(ev)
		if replay && textInput && ev.Kind == platform.EvKeyDown && ev.TextN > 0 {
			emit(platform.Event{Kind: platform.EvTextInput, Text: ev.Text, TextN: ev.TextN, Window: ev.Window})
		}
	}
	f.delayed = false
	return len(events)
}

// discardHeldInput prevents stale input reaching a new Inspector while preserving releases and system events.
func discardHeldInput(events []platform.Event) []platform.Event {
	out := events[:0]
	for _, ev := range events {
		if isHeldInput(ev.Kind) {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func isHeldInput(kind platform.EventKind) bool {
	switch kind {
	case platform.EvKeyDown, platform.EvTextInput, platform.EvMouseDown, platform.EvMouseMove, platform.EvMouseWheel:
		return true
	}
	return false
}
