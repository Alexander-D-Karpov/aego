package web

import (
	"fmt"
	"reflect"
	"testing"

	"aego/core/mathx"
	"aego/platform"
	"aego/platform/input"
)

// The UI sees per-frame input snapshots, so event iteration order alone does
// not prove that editing operations are applied in their original order.
func replayFenceInputFrames(t *testing.T, events []platform.Event) []string {
	t.Helper()
	f := inspectorFence()
	f.delayed = true
	f.start = mathx.Vec2{X: 120, Y: 40}
	var state input.State
	var observed []string
	for frame := 0; len(events) != 0 && frame < 32; frame++ {
		state.BeginFrame()
		n := f.dispatch(events, true, true, func(ev platform.Event) {
			switch ev.Kind {
			case platform.EvKeyDown:
				state.ApplyKeyDown(ev.Key.Scan, ev.Key.Key, ev.Key.Mods, ev.Key.Repeat)
			case platform.EvKeyUp:
				state.ApplyKeyUp(ev.Key.Scan, ev.Key.Key, ev.Key.Mods)
			case platform.EvTextInput:
				state.ApplyText(ev.TextBytes())
			}
		})
		if n <= 0 || n > len(events) {
			t.Fatalf("frame %d did not make progress: %d", frame, n)
		}
		events = events[n:]
		var actions []string
		if len(state.Text()) > 0 {
			actions = append(actions, "text:"+string(state.Text()))
		}
		for _, key := range []input.Scancode{input.KeyBackspace, input.KeyLeft, input.KeyEnter, input.KeyA} {
			if state.KeyRepeated(key) {
				actions = append(actions, fmt.Sprintf("key:%s:ctrl=%t", key, state.Mods().Has(input.ModCtrl)))
			}
		}
		if len(actions) > 1 {
			t.Fatalf("distinct editing actions collapsed in one UI snapshot: %v", actions)
		}
		observed = append(observed, actions...)
	}
	if len(events) != 0 {
		t.Fatalf("replay did not finish: %d remaining", len(events))
	}
	return observed
}

func orderedFenceKeyPair(key input.Scancode, text string, mods input.Mods) []platform.Event {
	down := fenceKey(platform.EvKeyDown, key, text)
	up := fenceKey(platform.EvKeyUp, key, "")
	down.Key.Mods, up.Key.Mods = mods, mods
	return []platform.Event{down, up}
}

func TestInputFenceOrderedEditingSnapshots(t *testing.T) {
	var events []platform.Event
	for _, x := range []struct {
		key  input.Scancode
		text string
	}{
		{input.Key9, "9"}, {input.KeyBackspace, ""}, {input.Key2, "2"}, {input.KeyEnter, ""},
	} {
		events = append(events, orderedFenceKeyPair(x.key, x.text, 0)...)
	}
	got := replayFenceInputFrames(t, events)
	want := []string{"text:9", "key:Backspace:ctrl=false", "text:2", "key:Enter:ctrl=false"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("edit actions out of order: got %v, want %v", got, want)
	}
}

func TestInputFenceOrderedModifierSnapshot(t *testing.T) {
	down := fenceKey(platform.EvKeyDown, input.KeyLCtrl, "")
	down.Key.Mods = input.ModCtrl
	up := fenceKey(platform.EvKeyUp, input.KeyLCtrl, "")
	var events []platform.Event
	events = append(events, down)
	events = append(events, orderedFenceKeyPair(input.KeyA, "", input.ModCtrl)...)
	events = append(events, up)
	events = append(events, orderedFenceKeyPair(input.Key2, "2", 0)...)
	got := replayFenceInputFrames(t, events)
	want := []string{"key:A:ctrl=true", "text:2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("modifier and text out of order: got %v, want %v", got, want)
	}
}

func TestInputFenceOrderedRepeatedActions(t *testing.T) {
	events := append(orderedFenceKeyPair(input.KeyBackspace, "", 0), orderedFenceKeyPair(input.KeyBackspace, "", 0)...)
	got := replayFenceInputFrames(t, events)
	want := []string{"key:Backspace:ctrl=false", "key:Backspace:ctrl=false"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("repeated keys collapsed: got %v, want %v", got, want)
	}
}
