//go:build aego_editor

package docsview

import (
	"encoding/json"
	"testing"

	"aego/app/playground"
	"aego/modules/render2d"
	"aego/scene"
)

func TestMaterialOverrideTryChangesOnlyOneRealInstanceAndResets(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("engine.MaterialOverride")
	if !ok || e.Demo == nil {
		t.Fatal("MaterialOverride contract is absent")
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
	check := func(want float64, explicit bool) {
		t.Helper()
		if err := a.Step(12); err != nil {
			t.Fatal(err)
		}
		sc := a.session.Controller().Scene()
		target, reference := sc.Find(sc.Root(), "Flash instance"), sc.Find(sc.Root(), "Reference instance")
		m1, err := render2d.NodeMaterial(sc, target)
		if err != nil {
			t.Fatal(err)
		}
		m2, err := render2d.NodeMaterial(sc, reference)
		if err != nil || m1 != m2 {
			t.Fatalf("both instances must use the same resolved material: %v", err)
		}
		value, err := render2d.GetMaterialParameter(sc, target, "Flash")
		if err != nil || value.F != want {
			t.Fatalf("actual target Flash=%v, want %v: %v", value.F, want, err)
		}
		b, err := render2d.GetMaterialParameter(sc, reference, "Flash")
		if err != nil || b.F != 0 {
			t.Fatalf("shared material/reference changed: %v, %v", b.F, err)
		}
		o, ok := scene.Get[render2d.MaterialOverride](sc, target)
		if !ok || o.ExplicitFor(m1, "Flash") != explicit {
			t.Fatalf("actual per-node override explicit=%v", explicit)
		}
		lanes, err := o.Lanes(m1)
		param, found := m1.Shader().Schema.Param("Flash")
		if err != nil || !found || param.Lane < 0 || param.Comp < 0 || param.Lane*4+param.Comp >= len(lanes) || lanes[param.Lane*4+param.Comp] != float32(want) {
			t.Fatalf("shader lanes do not contain the instance value: %v", err)
		}
		r := a.Call("inspect", []byte(`{"component":"engine.MaterialOverride","fields":["engine.MaterialOverride.FlashValue","engine.MaterialOverride.FlashExplicit","engine.MaterialOverride.ReferenceFlash"]}`))
		if r.Error != "" || r.Inspection == nil || r.Inspection.Observables["engine.MaterialOverride.FlashValue"] != want || r.Inspection.Observables["engine.MaterialOverride.FlashExplicit"] != explicit || r.Inspection.Observables["engine.MaterialOverride.ReferenceFlash"] != float64(0) {
			t.Fatalf("actual material inspection: %+v", r)
		}
	}
	check(0, false)
	if r := a.Call("scriptEnabled", []byte(`{"script":"game.demo.SetFlash","enabled":true}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	check(1, true)
	if r := a.Call("undo", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	check(0, false)
	if r := a.Call("redo", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	check(1, true)
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	check(0, false)
	if messages := errorsOf(a); len(messages) != 0 {
		t.Fatalf("MaterialOverride Try runtime errors: %v", messages)
	}
}
