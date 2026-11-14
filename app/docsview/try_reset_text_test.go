//go:build aego_editor

package docsview

import (
	"encoding/json"
	"testing"

	"aego/app/playground"
	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/platform"
	"aego/platform/headless"
	"aego/platform/input"
	"aego/ui/core"
)

func TestTryResetDiscardsPendingInspectorText(t *testing.T) {
	for _, loseFocus := range []bool{false, true} {
		name := "active edit"
		if loseFocus {
			name = "queued focus loss"
		}
		t.Run(name, func(t *testing.T) {
			a, _ := newAnimationGuide(t)
			id := core.ID(44)
			a.ui.SetFocus(id)
			a.ui.EditBuffer(id, "draft")
			if loseFocus {
				a.ui.SetFocus(core.ID(45))
			}
			if reply := a.Call("reset", nil); reply.Error != "" {
				t.Fatal(reply.Error)
			}
			if a.ui.Focus() != 0 || a.ui.EditingID() != 0 {
				t.Fatal("Reset retained the old Inspector edit")
			}
			if err := a.Step(1); err != nil {
				t.Fatal(err)
			}
			if text, ok := a.ui.LostText(id); ok {
				t.Fatalf("old Inspector edit survived Reset: %q", text)
			}
		})
	}
}

func TestTryLoadDiscardsPendingInspectorTextOnlyOnSuccess(t *testing.T) {
	a, _ := newAnimationGuide(t)
	id := core.ID(44)
	a.ui.SetFocus(id)
	a.ui.EditBuffer(id, "draft")
	if reply := a.Call("load", []byte(`{"scene":"not a scene"}`)); reply.Error == "" {
		t.Fatal("invalid scene unexpectedly loaded")
	}
	if a.ui.Focus() != id || a.ui.EditingID() != id || a.ui.EditBuffer(id, "").String() != "draft" {
		t.Fatal("failed Load discarded the pending edit")
	}
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
	req, err := json.Marshal(map[string]any{"scene": entry.Try, "select": entry.TrySelect, "code": code,
		"prefabs": entry.TryPrefabs, "artifacts": entry.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if reply := a.Call("load", req); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if a.ui.Focus() != 0 || a.ui.EditingID() != 0 {
		t.Fatal("successful Load retained the previous document's edit")
	}
}

func TestTryFailedResetPreservesPendingText(t *testing.T) {
	a := newHeadless(t)
	id := core.ID(44)
	a.ui.SetFocus(id)
	a.ui.EditBuffer(id, "draft")
	if reply := a.Call("reset", nil); reply.Error == "" {
		t.Fatal("Reset succeeded without a loaded scene")
	}
	if a.ui.Focus() != id || a.ui.EditingID() != id || a.ui.EditBuffer(id, "").String() != "draft" {
		t.Fatal("failed Reset discarded the pending edit")
	}
}

func TestTryToolbarResetDiscardsPendingInspectorText(t *testing.T) {
	a, _ := newAnimationGuide(t)
	if reply := a.Call("set", []byte(`{"component":"animation.AnimationPlayer","field":"speed","value":2}`)); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if !a.session.State().CanUndo {
		t.Fatal("test edit did not create history")
	}
	id := core.ID(44)
	a.ui.SetFocus(id)
	a.ui.EditBuffer(id, "draft")
	// Compute the same current-language button geometry as Toolbar.TextButton.
	pause := i18n.Tr("editor.docsview.pause", "Pause")
	reset := i18n.Tr("editor.docsview.reset", "Reset")
	x := float32(6) + a.ui.TextSize(pause).X + 14 + 2 + (a.ui.TextSize(reset).X+14)/2
	pos := mathx.Vec2{X: x, Y: a.ui.Theme.BarHeight / 2}
	mouse := input.MouseEvent{Pos: pos, Button: input.MouseLeft}
	backend := a.e.Platform().(*headless.Platform)
	backend.Push(platform.Event{Kind: platform.EvMouseDown, Mouse: mouse})
	if err := a.Step(1); err != nil {
		t.Fatal(err)
	}
	backend.Push(platform.Event{Kind: platform.EvMouseUp, Mouse: mouse})
	if err := a.Step(1); err != nil {
		t.Fatal(err)
	}
	if a.session.State().CanUndo {
		t.Fatal("toolbar click did not reset the scene")
	}
	if text, ok := a.ui.LostText(id); ok {
		t.Fatalf("toolbar Reset retained stale focus-loss text: %q", text)
	}
}
