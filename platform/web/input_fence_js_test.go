//go:build js

package web

import (
	"reflect"
	"syscall/js"
	"testing"

	"aego/core/mathx"
	"aego/platform"
	"aego/platform/input"
)

func fencePlatform() (*Platform, *Window) {
	g := js.Global()
	global := g.Get("Object").New()
	global.Set("devicePixelRatio", 1)
	doc := g.Get("Object").New()
	canvas := g.Get("Object").New()
	doc.Set("activeElement", canvas)
	p := &Platform{global: global, doc: doc, gamepads: js.Undefined(), vpad: js.Undefined()}
	w := &Window{p: p, canvas: canvas, rawDPR: 1}
	p.win = w
	w.FenceTextInputClicks(func(pos mathx.Vec2, _ bool) bool { return pos.X >= 100 })
	return p, w
}

func fenceKeyboardEvent(key, code string, ctrl, alt bool) js.Value {
	makeEvent := js.Global().Get("Function").New("key", "code", "ctrl", "alt", `
		return { key, code, ctrlKey: ctrl, altKey: alt, shiftKey: false, metaKey: false,
			repeat: false, getModifierState() { return false }, preventDefault() {} }`)
	return makeEvent.Invoke(key, code, ctrl, alt)
}

func TestFencedWebKeyAndPollDeliverAfterTextInputStarts(t *testing.T) {
	p, w := fencePlatform()
	p.push(fenceMouse(platform.EvMouseDown, 120, 40, input.MouseLeft))
	p.push(fenceMouse(platform.EvMouseUp, 120, 40, input.MouseLeft))
	e := fenceKeyboardEvent("9", "Digit9", false, false)
	p.key(e, true)
	p.key(e, false)
	enter := fenceKeyboardEvent("Enter", "Enter", false, false)
	p.key(enter, true)
	p.key(enter, false)
	var got []platform.Event
	collect := func(ev platform.Event) { got = append(got, ev) }
	p.PollEvents(collect)
	if !reflect.DeepEqual(kinds(got), []platform.EventKind{platform.EvMouseDown, platform.EvMouseUp}) {
		t.Fatalf("click batch: %+v", got)
	}
	p.PollEvents(collect)
	if len(got) != 2 {
		t.Fatalf("text arrived before the field was ready: %+v", got)
	}
	w.StartTextInput()
	p.PollEvents(collect)
	partial := []platform.EventKind{platform.EvMouseDown, platform.EvMouseUp, platform.EvKeyDown, platform.EvTextInput, platform.EvKeyUp}
	if !reflect.DeepEqual(kinds(got), partial) || string(got[3].TextBytes()) != "9" {
		t.Fatalf("text was not delivered before Enter: %+v", got)
	}
	p.PollEvents(collect)
	want := []platform.EventKind{platform.EvMouseDown, platform.EvMouseUp, platform.EvKeyDown,
		platform.EvTextInput, platform.EvKeyUp, platform.EvKeyDown, platform.EvKeyUp}
	if !reflect.DeepEqual(kinds(got), want) || string(got[3].TextBytes()) != "9" {
		t.Fatalf("wrong replay: %+v", got)
	}
}

func TestFencedWebKeyCapturesOnlyPrintableNonShortcut(t *testing.T) {
	for _, tc := range []struct {
		name, key, code string
		ctrl, alt       bool
		want            string
	}{
		{"unicode", "Ж", "KeyA", false, false, "Ж"},
		{"altgr", "@", "Digit2", true, true, "@"},
		{"shortcut", "c", "KeyC", true, false, ""},
		{"control key", "Enter", "Enter", false, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := fencePlatform()
			p.key(fenceKeyboardEvent(tc.key, tc.code, tc.ctrl, tc.alt), true)
			if len(p.queue) != 1 || string(p.queue[0].TextBytes()) != tc.want || p.queue[0].Kind != platform.EvKeyDown {
				t.Fatalf("early key payload: %+v", p.queue)
			}
		})
	}
}

func TestFencedWebCancelPreservesReleases(t *testing.T) {
	p, w := fencePlatform()
	p.push(fenceMouse(platform.EvMouseDown, 120, 40, input.MouseLeft))
	p.push(fenceMouse(platform.EvMouseUp, 120, 40, input.MouseLeft))
	e := fenceKeyboardEvent("9", "Digit9", false, false)
	p.key(e, true)
	p.key(e, false)
	p.PollEvents(func(platform.Event) {})
	p.push(fenceKey(platform.EvKeyDown, input.KeyA, "a"))
	p.push(fenceKey(platform.EvKeyUp, input.KeyA, ""))
	p.push(platform.Event{Kind: platform.EvWindowResized})
	w.CancelTextInputFence()
	if w.fence.delayed || w.fence.wait {
		t.Fatal("cancel did not clear the fence")
	}
	var got []platform.Event
	p.PollEvents(func(ev platform.Event) { got = append(got, ev) })
	want := []platform.EventKind{platform.EvKeyUp, platform.EvKeyUp, platform.EvWindowResized}
	if !reflect.DeepEqual(kinds(got), want) {
		t.Fatalf("cancel lost releases or delivered stale text: %+v", got)
	}
}

func TestFencedWebCancelBeforeFirstPoll(t *testing.T) {
	p, w := fencePlatform()
	p.push(fenceMouse(platform.EvMouseDown, 120, 40, input.MouseLeft))
	p.push(fenceMouse(platform.EvMouseUp, 120, 40, input.MouseLeft))
	e := fenceKeyboardEvent("9", "Digit9", false, false)
	p.key(e, true)
	p.key(e, false)
	// A synchronous Reset can replace the scene before PollEvents has even
	// recognized the click and activated the fence.
	w.CancelTextInputFence()
	var got []platform.Event
	p.PollEvents(func(ev platform.Event) { got = append(got, ev) })
	want := []platform.EventKind{platform.EvMouseUp, platform.EvKeyUp}
	if !reflect.DeepEqual(kinds(got), want) || w.fence.wait || w.fence.delayed {
		t.Fatalf("queued input from before Reset reached the replacement: %+v", got)
	}
}

func TestFencedWebPointerLockDoesNotDelayGameInput(t *testing.T) {
	p, w := fencePlatform()
	w.locked = true
	p.push(fenceMouse(platform.EvMouseDown, 120, 40, input.MouseLeft))
	p.push(fenceMouse(platform.EvMouseUp, 120, 40, input.MouseLeft))
	e := fenceKeyboardEvent("b", "KeyB", false, false)
	p.key(e, true)
	p.key(e, false)
	var got []platform.Event
	p.PollEvents(func(ev platform.Event) { got = append(got, ev) })
	want := []platform.EventKind{platform.EvMouseDown, platform.EvMouseUp, platform.EvKeyDown, platform.EvKeyUp}
	if !reflect.DeepEqual(kinds(got), want) || w.fence.delayed {
		t.Fatalf("pointer-locked gameplay was delayed: %+v", got)
	}
}
