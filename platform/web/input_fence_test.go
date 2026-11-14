package web

import (
	"reflect"
	"testing"

	"aego/core/mathx"
	"aego/platform"
	"aego/platform/input"
)

func fenceMouse(kind platform.EventKind, x, y float32, button input.Button) platform.Event {
	return platform.Event{Kind: kind, Mouse: input.MouseEvent{Pos: mathx.Vec2{X: x, Y: y}, Button: button}}
}

func fenceKey(kind platform.EventKind, key input.Scancode, text string) platform.Event {
	ev := platform.Event{Kind: kind, Key: input.KeyEvent{Scan: key}}
	ev.TextN = uint8(copy(ev.Text[:], text))
	return ev
}

func inspectorFence() inputFence {
	return inputFence{hit: func(pos mathx.Vec2, _ bool) bool { return pos.X >= 100 }}
}

func dispatchFence(f *inputFence, events []platform.Event, focused, text bool) (int, []platform.Event) {
	var got []platform.Event
	n := f.dispatch(events, focused, text, func(ev platform.Event) { got = append(got, ev) })
	return n, got
}

func TestInputFenceReplaysQuickTextAfterInspectorClick(t *testing.T) {
	f := inspectorFence()
	events := []platform.Event{
		fenceMouse(platform.EvMouseDown, 120, 40, input.MouseLeft),
		fenceMouse(platform.EvMouseUp, 120, 40, input.MouseLeft),
		fenceKey(platform.EvKeyDown, input.Key9, "9"), fenceKey(platform.EvKeyUp, input.Key9, ""),
		fenceKey(platform.EvKeyDown, input.KeyEnter, ""), fenceKey(platform.EvKeyUp, input.KeyEnter, ""),
	}
	n, first := dispatchFence(&f, events, true, false)
	if n != 2 || !reflect.DeepEqual(kinds(first), []platform.EventKind{platform.EvMouseDown, platform.EvMouseUp}) {
		t.Fatalf("click batch: consumed %d, got %v", n, kinds(first))
	}
	events = events[n:]
	if n, got := dispatchFence(&f, events, true, false); n != 0 || len(got) != 0 {
		t.Fatalf("the numeric editor needs a separate frame: %d, %v", n, kinds(got))
	}
	n, got := dispatchFence(&f, events, true, true)
	want := []platform.EventKind{platform.EvKeyDown, platform.EvTextInput, platform.EvKeyUp}
	if n != 2 || !reflect.DeepEqual(kinds(got), want) || string(got[1].TextBytes()) != "9" {
		t.Fatalf("replayed batch: consumed %d, got %+v", n, got)
	}
	events = events[n:]
	n, got = dispatchFence(&f, events, true, true)
	if n != len(events) || !reflect.DeepEqual(kinds(got), []platform.EventKind{platform.EvKeyDown, platform.EvKeyUp}) {
		t.Fatalf("Enter was not delivered after the text frame: consumed %d, got %+v", n, got)
	}
	if f.wait || f.delayed {
		t.Fatal("fence remained active after the batch")
	}
}

func TestInputFenceDoesNotDelayGameOrDrag(t *testing.T) {
	for _, tc := range []struct {
		name   string
		events []platform.Event
	}{
		{"game", []platform.Event{fenceMouse(platform.EvMouseDown, 10, 40, input.MouseLeft), fenceMouse(platform.EvMouseUp, 10, 40, input.MouseLeft)}},
		{"right button", []platform.Event{fenceMouse(platform.EvMouseDown, 120, 40, input.MouseRight), fenceMouse(platform.EvMouseUp, 120, 40, input.MouseRight)}},
		{"drag", []platform.Event{fenceMouse(platform.EvMouseDown, 120, 40, input.MouseLeft), fenceMouse(platform.EvMouseMove, 125, 40, 0), fenceMouse(platform.EvMouseUp, 125, 40, input.MouseLeft)}},
		{"release elsewhere", []platform.Event{fenceMouse(platform.EvMouseDown, 100, 40, input.MouseLeft), fenceMouse(platform.EvMouseUp, 99, 40, input.MouseLeft)}},
		{"resize during click", []platform.Event{fenceMouse(platform.EvMouseDown, 120, 40, input.MouseLeft), {Kind: platform.EvWindowResized}, fenceMouse(platform.EvMouseUp, 120, 40, input.MouseLeft)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := inspectorFence()
			events := append(tc.events, fenceKey(platform.EvKeyDown, input.KeyB, "b"), fenceKey(platform.EvKeyUp, input.KeyB, ""))
			n, got := dispatchFence(&f, events, true, false)
			if n != len(events) || !reflect.DeepEqual(got, events) || f.wait || f.delayed {
				t.Fatalf("ordinary input changed: consumed %d, got %+v", n, got)
			}
		})
	}
}

func TestInputFenceRetainsClickAcrossFrames(t *testing.T) {
	hits := 0
	f := inputFence{hit: func(_ mathx.Vec2, release bool) bool { hits++; return release == (hits == 2) }}
	down := []platform.Event{fenceMouse(platform.EvMouseDown, 120, 40, input.MouseLeft)}
	if n, got := dispatchFence(&f, down, true, false); n != 1 || len(got) != 1 || !f.armed {
		t.Fatalf("press was not tracked: %d, %+v", n, got)
	}
	// After this frame the field captures the mouse. The release-specific
	// predicate may permit that capture while still rechecking the geometry.
	upAndKey := []platform.Event{fenceMouse(platform.EvMouseUp, 120, 40, input.MouseLeft), fenceKey(platform.EvKeyDown, input.Key9, "9")}
	if n, got := dispatchFence(&f, upAndKey, true, false); n != 1 || len(got) != 1 || !f.delayed || hits != 2 {
		t.Fatalf("release did not fence the key: %d, %+v, hit calls %d", n, got, hits)
	}
}

func TestInputFenceInterruptsWaitingOnResize(t *testing.T) {
	f := inspectorFence()
	f.wait, f.delayed = true, true
	f.start = mathx.Vec2{X: 120, Y: 40}
	events := []platform.Event{fenceKey(platform.EvKeyDown, input.Key9, "9"), fenceKey(platform.EvKeyUp, input.Key9, ""),
		{Kind: platform.EvWindowResized}}
	n, got := dispatchFence(&f, events, true, false)
	if n != len(events) || !reflect.DeepEqual(kinds(got), []platform.EventKind{platform.EvKeyUp, platform.EvWindowResized}) || f.wait || f.delayed {
		t.Fatalf("resize must cancel stale input immediately: %d, %+v", n, got)
	}
}

func TestInputFenceGatesSpeculativeTextAndPreservesExistingText(t *testing.T) {
	for _, tc := range []struct {
		name     string
		ready    bool
		input    []platform.Event
		want     []platform.EventKind
		wantText string
	}{
		{"unfocused field", false, []platform.Event{fenceKey(platform.EvKeyDown, input.Key9, "9")}, []platform.EventKind{platform.EvKeyDown}, ""},
		{"unicode", true, []platform.Event{fenceKey(platform.EvKeyDown, input.KeyA, "Ж")}, []platform.EventKind{platform.EvKeyDown, platform.EvTextInput}, "Ж"},
		{"existing text", true, []platform.Event{fenceKey(platform.EvKeyDown, input.Key9, ""), {Kind: platform.EvTextInput, Text: [32]byte{'9'}, TextN: 1}}, []platform.EventKind{platform.EvKeyDown, platform.EvTextInput}, "9"},
		{"old text focus gone", false, []platform.Event{fenceKey(platform.EvKeyDown, input.Key9, ""), {Kind: platform.EvTextInput, Text: [32]byte{'9'}, TextN: 1}}, []platform.EventKind{platform.EvKeyDown}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := inspectorFence()
			f.delayed = true
			f.start = mathx.Vec2{X: 120, Y: 40}
			n, got := dispatchFence(&f, tc.input, true, tc.ready)
			if n != len(tc.input) || !reflect.DeepEqual(kinds(got), tc.want) {
				t.Fatalf("consumed %d, got %+v, want %v", n, got, tc.want)
			}
			if tc.wantText != "" && string(got[len(got)-1].TextBytes()) != tc.wantText {
				t.Fatalf("wrong text: %+v", got)
			}
		})
	}
}

func TestInputFenceProcessesKeysBeforeLaterClick(t *testing.T) {
	f := inspectorFence()
	f.delayed = true
	f.start = mathx.Vec2{X: 120, Y: 40}
	events := []platform.Event{
		fenceKey(platform.EvKeyDown, input.Key9, "9"), fenceKey(platform.EvKeyUp, input.Key9, ""),
		fenceMouse(platform.EvMouseDown, 130, 40, input.MouseLeft), fenceMouse(platform.EvMouseUp, 130, 40, input.MouseLeft),
		fenceKey(platform.EvKeyDown, input.KeyA, "a"),
	}
	n, got := dispatchFence(&f, events, true, true)
	if n != 2 || !reflect.DeepEqual(kinds(got), []platform.EventKind{platform.EvKeyDown, platform.EvTextInput, platform.EvKeyUp}) {
		t.Fatalf("later click joined earlier text: consumed %d, got %+v", n, got)
	}
	events = events[n:]
	n, got = dispatchFence(&f, events, true, true)
	if n != 2 || !reflect.DeepEqual(kinds(got), []platform.EventKind{platform.EvMouseDown, platform.EvMouseUp}) {
		t.Fatalf("second click: consumed %d, got %+v", n, got)
	}
	events = events[n:]
	if n, got = dispatchFence(&f, events, true, false); n != 0 || len(got) != 0 {
		t.Fatal("second editor did not get its own frame")
	}
	n, got = dispatchFence(&f, events, true, true)
	if n != 1 || len(got) != 2 || string(got[1].TextBytes()) != "a" {
		t.Fatalf("second field input: consumed %d, got %+v", n, got)
	}
}

func TestInputFenceDropsStaleKeysButPreservesReleasesAndSystemEvents(t *testing.T) {
	events := []platform.Event{
		fenceKey(platform.EvKeyDown, input.Key9, "9"), {Kind: platform.EvTextInput, Text: [32]byte{'9'}, TextN: 1},
		fenceKey(platform.EvKeyUp, input.Key9, ""), fenceMouse(platform.EvMouseDown, 120, 40, input.MouseLeft),
		fenceMouse(platform.EvMouseUp, 120, 40, input.MouseLeft), {Kind: platform.EvWindowFocusLost}, {Kind: platform.EvWindowResized},
	}
	f := inspectorFence()
	f.delayed = true
	f.start = mathx.Vec2{X: 120, Y: 40}
	n, got := dispatchFence(&f, events, false, true)
	want := []platform.EventKind{platform.EvKeyUp, platform.EvMouseUp, platform.EvWindowFocusLost, platform.EvWindowResized}
	if n != len(events) || !reflect.DeepEqual(kinds(got), want) {
		t.Fatalf("focus loss: consumed %d, got %v", n, kinds(got))
	}
	filtered := discardHeldInput(append([]platform.Event(nil), events...))
	if !reflect.DeepEqual(kinds(filtered), want) {
		t.Fatalf("scene reset discarded releases/system events: %v", kinds(filtered))
	}
}

func TestInputFenceCancelsWhenTargetChanges(t *testing.T) {
	valid := true
	f := inputFence{hit: func(_ mathx.Vec2, _ bool) bool { return valid }}
	events := []platform.Event{fenceMouse(platform.EvMouseDown, 120, 40, input.MouseLeft),
		fenceMouse(platform.EvMouseUp, 120, 40, input.MouseLeft), fenceKey(platform.EvKeyDown, input.Key9, "9"),
		fenceKey(platform.EvKeyUp, input.Key9, "")}
	n, _ := dispatchFence(&f, events, true, false)
	valid = false // A modal or a different presented layout appeared.
	remaining := events[n:]
	n, got := dispatchFence(&f, remaining, true, true)
	if n != len(remaining) || !reflect.DeepEqual(kinds(got), []platform.EventKind{platform.EvKeyUp}) || f.delayed || f.wait {
		t.Fatalf("stale input reached a replacement target: %d %+v", n, got)
	}
}

func TestInputFenceIsOptIn(t *testing.T) {
	var f inputFence
	events := []platform.Event{fenceMouse(platform.EvMouseDown, 120, 40, input.MouseLeft),
		fenceMouse(platform.EvMouseUp, 120, 40, input.MouseLeft), fenceKey(platform.EvKeyDown, input.Key9, "9")}
	n, got := dispatchFence(&f, events, true, true)
	if n != len(events) || !reflect.DeepEqual(got, events) {
		t.Fatalf("disabled fence changed input: consumed %d, got %+v", n, got)
	}
}
