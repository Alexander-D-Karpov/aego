//go:build aego_editor

package docsview

import (
	"encoding/json"
	"testing"

	"aego/app/playground"
	"aego/modules/savegame"
)

func TestPersistTryActuallySavesLoadsAndResets(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("save.Persist")
	if !ok || e.Demo == nil {
		t.Fatal("Persist contract is absent")
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
	check := func(want float64) {
		t.Helper()
		if err := a.Step(20); err != nil {
			t.Fatal(err)
		}
		sc := a.session.Controller().Scene()
		h := sc.Find(sc.Root(), "Saved target")
		if !sc.IsAlive(h) || float64(sc.WorldPosition(h).X) != want {
			t.Fatalf("loaded target did not have expected position %v", want)
		}
		s := savegame.Of(a.session.Controller().World())
		if s == nil || s.Exists("persist-demo") {
			t.Fatal("save service was absent or the temporary in-memory slot was not removed")
		}
		logs := a.Call("log", nil).Log
		saved, loaded := false, false
		for _, line := range logs {
			if line.Error {
				t.Fatalf("save Try error: %s", line.Text)
			}
			saved = saved || line.Text == "game saved"
			loaded = loaded || line.Text == "game loaded"
		}
		if !saved || !loaded {
			t.Fatalf("real save/load did not complete: %+v", logs)
		}
		r := a.Call("inspect", []byte(`{"component":"save.Persist","fields":["save.Persist.PositionX"]}`))
		if r.Error != "" || r.Inspection == nil || r.Inspection.Observables["save.Persist.PositionX"] != want {
			t.Fatalf("loaded position is not observable: %+v", r)
		}
	}
	check(2)
	if r := a.Call("set", []byte(`{"component":"save.Persist","field":"NoTransform","value":1}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	check(-2)
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	check(2)
}
