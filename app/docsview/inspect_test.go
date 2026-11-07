//go:build aego_editor

package docsview

import (
	"encoding/json"
	"strings"
	"testing"

	"aego/app/playground"
)

func TestTryInspectionReadsSelectedRunningComponent(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("engine.SpriteRenderer")
	if !ok || e.Try == "" {
		t.Fatal("missing SpriteRenderer Try")
	}
	a := newHeadless(t)
	load, _ := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "artifacts": e.TryArtifacts})
	if r := a.Call("load", load); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(10); err != nil {
		t.Fatal(err)
	}
	req := []byte(`{"component":"engine.SpriteRenderer","fields":["engine.SpriteRenderer.Order"],"dependencies":["ae900000000000000000000000000010"]}`)
	inspect := func(want float64) {
		t.Helper()
		r := a.Call("inspect", req)
		if r.Error != "" || r.Inspection == nil || !r.Inspection.RuntimeExecuting || r.Inspection.Observables["engine.SpriteRenderer.Order"] != int64(want) || len(r.Inspection.Dependencies) != 1 {
			t.Fatalf("inspect: %+v", r)
		}
		wire, err := json.Marshal(r)
		if err != nil || !strings.Contains(string(wire), `"engine.SpriteRenderer.Order":`) {
			t.Fatalf("inspection is not accessible over the real bridge: %s, %v", wire, err)
		}
	}
	inspect(1)
	before := string(a.session.Text())
	inspect(1)
	if string(a.session.Text()) != before || a.session.State().CanUndo {
		t.Fatal("read-only inspection changed the source document")
	}
	if r := a.Call("set", []byte(`{"component":"engine.SpriteRenderer","field":"order","value":-1}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(4); err != nil {
		t.Fatal(err)
	}
	inspect(-1)
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(4); err != nil {
		t.Fatal(err)
	}
	inspect(1)
	for _, invalid := range []string{
		`{"component":"engine.SpriteRenderer","fields":["engine.SpriteRenderer.NoSuchField"]}`,
		`{"component":"engine.SpriteRenderer","dependencies":["ffffffffffffffffffffffffffffffff"]}`,
		`{"component":"engine.SpriteRenderer","dependencies":["ae900000000000000000000000000010","ae900000000000000000000000000010"]}`,
		`{"component":"engine.Mesh2D","fields":["engine.Mesh2D.Order"]}`,
	} {
		if r := a.Call("inspect", []byte(invalid)); r.Error == "" {
			t.Fatalf("accepted invalid inspection %s: %+v", invalid, r)
		}
	}
}
