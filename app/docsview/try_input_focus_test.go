//go:build aego_editor

package docsview

import (
	"encoding/json"
	"testing"

	"aego/app/playground"
	"aego/core/mathx"
	"aego/modules/animation"
	"aego/platform"
	"aego/platform/headless"
	"aego/platform/input"
	"aego/scene"
	"aego/ui/core"
)

func newAnimationGuide(t *testing.T) (*App, *animation.AnimationPlayer) {
	t.Helper()
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := p.Docs().Get("animation/player")
	if !ok {
		t.Fatal("Animation Player guide is missing")
	}
	code, err := p.TryCode(entry.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	a := newHeadless(t)
	req, err := json.Marshal(map[string]any{"scene": entry.Try, "select": entry.TrySelect, "code": code, "prefabs": entry.TryPrefabs, "artifacts": entry.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if reply := a.Call("load", req); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(30); err != nil {
		t.Fatal(err)
	}
	sc := a.session.Controller().Scene()
	player, ok := scene.Get[animation.AnimationPlayer](sc, sc.Find(sc.Root(), "Runner"))
	if !ok || player.Speed != 1 {
		t.Fatalf("runner animation not ready: %+v", player)
	}
	return a, player
}

func pushGuideClickAndTap(a *App, pos mathx.Vec2, button input.Button) {
	backend := a.e.Platform().(*headless.Platform)
	mouse := input.MouseEvent{Pos: pos, Button: button}
	backend.Push(platform.Event{Kind: platform.EvMouseDown, Mouse: mouse})
	backend.Push(platform.Event{Kind: platform.EvMouseUp, Mouse: mouse})
	backend.Push(platform.Event{Kind: platform.EvKeyDown, Key: input.KeyEvent{Scan: input.KeyB}})
	backend.Push(platform.Event{Kind: platform.EvKeyUp, Key: input.KeyEvent{Scan: input.KeyB}})
}

// A click and a short keyboard tap can arrive in the same browser frame. The
// guide promises B reverses the runner immediately after the view is focused.
func TestGuideFocusAndTapInSameFrame(t *testing.T) {
	for _, preCaptured := range []bool{true, false} {
		name := "click-and-tap"
		if preCaptured {
			name = "already-captured"
		}
		t.Run(name, func(t *testing.T) {
			a, player := newAnimationGuide(t)
			if preCaptured {
				a.session.Controller().SetCapture(true, mathx.Vec2{}, mathx.Vec2{X: 500, Y: 440})
				backend := a.e.Platform().(*headless.Platform)
				backend.Push(platform.Event{Kind: platform.EvKeyDown, Key: input.KeyEvent{Scan: input.KeyB}})
				backend.Push(platform.Event{Kind: platform.EvKeyUp, Key: input.KeyEvent{Scan: input.KeyB}})
			} else {
				if a.session.Controller().Captured() {
					t.Fatal("game view unexpectedly captured before click")
				}
				pushGuideClickAndTap(a, mathx.Vec2{X: 100, Y: 100}, input.MouseLeft)
			}
			if err := a.Step(2); err != nil {
				t.Fatal(err)
			}
			if !a.session.Controller().Captured() {
				t.Fatal("the game view did not capture the click")
			}
			if player.Speed != -1 {
				t.Fatalf("B tap was lost after focusing the running guide: speed=%v", player.Speed)
			}
			if messages := errorsOf(a); len(messages) != 0 {
				t.Fatalf("guide runtime errors: %v", messages)
			}
		})
	}
}

func TestGuideFocusDoesNotStealUIInput(t *testing.T) {
	tests := []struct {
		name        string
		position    mathx.Vec2
		button      input.Button
		setup       func(*App)
		captured    bool
		pendingText bool
	}{
		{name: "inspector", position: mathx.Vec2{X: 800, Y: 100}, button: input.MouseLeft},
		{name: "toolbar", position: mathx.Vec2{X: 800, Y: 10}, button: input.MouseLeft},
		{name: "right-click", position: mathx.Vec2{X: 100, Y: 100}, button: input.MouseRight},
		{name: "modal", position: mathx.Vec2{X: 100, Y: 100}, button: input.MouseLeft, setup: func(a *App) { a.ui.OpenModal(core.ID(42)) }},
		{name: "mouse-capture", position: mathx.Vec2{X: 100, Y: 100}, button: input.MouseLeft, setup: func(a *App) { a.ui.CaptureMouse(core.ID(43)) }},
		{name: "stale-viewport", position: mathx.Vec2{X: 100, Y: 100}, button: input.MouseLeft, captured: true, setup: func(a *App) { a.viewport.CanvasWidth++ }},
		{name: "focused-edit", position: mathx.Vec2{X: 100, Y: 100}, button: input.MouseLeft, captured: true, pendingText: true, setup: func(a *App) {
			a.ui.SetFocus(core.ID(44))
			a.ui.EditBuffer(core.ID(44), "unsaved draft")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, player := newAnimationGuide(t)
			if tc.setup != nil {
				tc.setup(a)
			}
			pushGuideClickAndTap(a, tc.position, tc.button)
			if err := a.Step(2); err != nil {
				t.Fatal(err)
			}
			if got := a.session.Controller().Captured(); got != tc.captured {
				t.Fatalf("game capture=%v, want %v", got, tc.captured)
			}
			if player.Speed != 1 {
				t.Fatalf("UI or unverified click incorrectly routed B into the game: speed=%v", player.Speed)
			}
			if tc.pendingText {
				if text, ok := a.ui.LostText(core.ID(44)); !ok || text != "unsaved draft" {
					t.Fatalf("click lost pending text: %q, %v", text, ok)
				}
			}
		})
	}
}
