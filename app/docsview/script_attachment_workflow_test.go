//go:build aego_editor

package docsview

import (
	"encoding/json"
	"testing"

	"aego/app/playground"
	"aego/scene/scriptcomp"
)

func TestScriptTryTogglesActualAttachmentWithHistoryAndReset(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("engine.Script")
	if !ok || e.Demo == nil {
		t.Fatal("Script contract is absent")
	}
	code, err := p.TryCode(e.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	a := newHeadless(t)
	req, err := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "code": code, "prefabs": e.TryPrefabs, "artifacts": e.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if r := a.Call("load", req); r.Error != "" {
		t.Fatal(r.Error)
	}
	check := func(enabled bool, x float64) {
		t.Helper()
		if err := a.Step(10); err != nil {
			t.Fatal(err)
		}
		sc := a.session.Controller().Scene()
		h := sc.Find(sc.Root(), "Script target")
		scripts, ok := scriptcomp.ScriptsOf(sc, h)
		if !ok || len(scripts.Attachments) != 1 || scripts.Attachments[0].TypeName != "game.demo.MoveOnReady" || scripts.Attachments[0].Enabled != enabled || float64(sc.WorldPosition(h).X) != x {
			t.Fatalf("real script attachment/position does not match enabled=%v x=%v", enabled, x)
		}
		r := a.Call("inspect", []byte(`{"component":"engine.Script","fields":["engine.Script.AttachmentEnabled","engine.Script.PositionX"]}`))
		if r.Error != "" || r.Inspection == nil || r.Inspection.Observables["engine.Script.AttachmentEnabled"] != enabled || r.Inspection.Observables["engine.Script.PositionX"] != x {
			t.Fatalf("real script inspection: %+v", r)
		}
	}
	check(false, -2)
	for _, payload := range []string{
		`{}`, `{"script":"game.demo.MoveOnReady"}`,
		`{"script":"game.demo.MoveOnReady","enabled":null}`,
		`{"script":"game.demo.MoveOnReady","enabled":1}`,
		`{"script":"game.demo.Missing","enabled":true}`,
	} {
		if r := a.Call("scriptEnabled", []byte(payload)); r.Error == "" {
			t.Fatalf("accepted invalid script attachment action %s", payload)
		}
	}
	if a.session.State().CanUndo {
		t.Fatal("invalid attachment actions changed history")
	}
	if r := a.Call("scriptEnabled", []byte(`{"script":"game.demo.MoveOnReady","enabled":true}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	check(true, 2)
	if r := a.Call("undo", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	check(false, -2)
	if r := a.Call("redo", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	check(true, 2)
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	check(false, -2)
	if messages := errorsOf(a); len(messages) != 0 {
		t.Fatalf("Script Try runtime errors: %v", messages)
	}
}

func TestScriptGuideOriginalStillAnimates(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("language/script-example")
	if !ok || e.Try == "" || e.TryScript == "" {
		t.Fatal("original Script guide is unavailable")
	}
	code, err := p.TryCode(e.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	a := newHeadless(t)
	req, err := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "code": code, "prefabs": e.TryPrefabs, "artifacts": e.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if r := a.Call("load", req); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(20); err != nil {
		t.Fatal(err)
	}
	sc := a.session.Controller().Scene()
	h := sc.Find(sc.Root(), "Spinner")
	if !sc.IsAlive(h) || sc.Local(h).Rotation == 0 {
		t.Fatal("the original Spinner script did not animate")
	}
	if messages := errorsOf(a); len(messages) != 0 {
		t.Fatalf("original Script guide runtime errors: %v", messages)
	}
}
