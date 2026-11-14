//go:build aego_editor

package docsview

import (
	"encoding/json"
	"testing"

	"aego/app/playground"
	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
)

func TestInspectorInputFenceUsesOnlyPresentedInspector(t *testing.T) {
	tests := []struct {
		name  string
		pos   mathx.Vec2
		setup func(*App)
		want  bool
	}{
		{name: "inspector", pos: mathx.Vec2{X: 800, Y: 100}, want: true},
		{name: "already editing", pos: mathx.Vec2{X: 800, Y: 100}, want: true, setup: func(a *App) {
			a.ui.SetFocus(core.ID(44))
			a.ui.EditBuffer(core.ID(44), "unsaved draft")
		}},
		{name: "game", pos: mathx.Vec2{X: 100, Y: 100}},
		{name: "toolbar", pos: mathx.Vec2{X: 800, Y: 10}},
		{name: "outside", pos: mathx.Vec2{X: 1000, Y: 100}},
		{name: "modal", pos: mathx.Vec2{X: 800, Y: 100}, setup: func(a *App) { a.ui.OpenModal(core.ID(42)) }},
		{name: "popup", pos: mathx.Vec2{X: 800, Y: 100}, setup: func(a *App) { a.ui.OpenPopup(core.ID(43), mathx.Vec2{}) }},
		{name: "mouse capture", pos: mathx.Vec2{X: 800, Y: 100}, setup: func(a *App) { a.ui.CaptureMouse(core.ID(45)) }},
		{name: "stale size", pos: mathx.Vec2{X: 800, Y: 100}, setup: func(a *App) { a.viewport.CanvasWidth++ }},
		{name: "stale layout", pos: mathx.Vec2{X: 800, Y: 100}, setup: func(a *App) { a.viewport.Width++ }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := newAnimationGuide(t)
			if tc.setup != nil {
				tc.setup(a)
			}
			if got := a.fenceInspectorClick(tc.pos, false); got != tc.want {
				t.Fatalf("fence=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestInspectorInputFenceReleaseRechecksLayoutDuringCapture(t *testing.T) {
	a, _ := newAnimationGuide(t)
	pos := mathx.Vec2{X: 800, Y: 100}
	if !a.fenceInspectorClick(pos, false) {
		t.Fatal("could not start the Inspector click")
	}
	a.ui.CaptureMouse(core.ID(45))
	if !a.fenceInspectorClick(pos, true) || a.fenceInspectorClick(pos, false) {
		t.Fatal("the matching release must allow its own mouse capture, but a new press must not")
	}
	a.viewport.CanvasWidth++
	if a.fenceInspectorClick(pos, true) {
		t.Fatal("a release on a stale layout must not keep the fence")
	}
}

func TestInspectorInputFenceStackedMobileLayout(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := p.Docs().Get("animation/player")
	if !ok {
		t.Fatal("animation guide is missing")
	}
	code, err := p.TryCode(entry.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{Headless: true, FrameTime: 1.0 / 60, Width: 390, Height: 560})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	req, err := json.Marshal(map[string]any{"scene": entry.Try, "select": entry.TrySelect, "code": code, "prefabs": entry.TryPrefabs, "artifacts": entry.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if r := a.Call("load", req); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(3); err != nil {
		t.Fatal(err)
	}
	v := a.viewport
	game := mathx.Vec2{X: v.X + 10, Y: v.Y + 10}
	inspector := mathx.Vec2{X: 10, Y: v.Y + v.Height + 10}
	if a.fenceInspectorClick(game, false) || !a.fenceInspectorClick(inspector, false) {
		t.Fatalf("incorrect stacked hit test: viewport=%+v game=%+v inspector=%+v", v, game, inspector)
	}
}

// A direct game click still routes a same-frame keyboard edge without going
// through the Inspector's one-frame browser input fence.
func TestInspectorInputFenceDoesNotCaptureGame(t *testing.T) {
	a, player := newAnimationGuide(t)
	pos := mathx.Vec2{X: 100, Y: 100}
	if a.fenceInspectorClick(pos, false) {
		t.Fatal("game view was classified as an Inspector click")
	}
	pushGuideClickAndTap(a, pos, input.MouseLeft)
	if err := a.Step(2); err != nil {
		t.Fatal(err)
	}
	if player.Speed != -1 {
		t.Fatalf("game lost the first B tap: speed=%v", player.Speed)
	}
}

func TestInspectorInputFenceRequiresLoadedScene(t *testing.T) {
	a := newHeadless(t)
	if err := a.Step(1); err != nil {
		t.Fatal(err)
	}
	if a.fenceInspectorClick(mathx.Vec2{X: 800, Y: 100}, false) {
		t.Fatal("unloaded Inspector accepted a click")
	}
}
