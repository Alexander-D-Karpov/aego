//go:build aego_editor

package docsview

import (
	"encoding/json"
	"strings"
	"testing"

	"aego/app/playground"
	"aego/asset"
)

func TestTryAudioDependencyRequiresPlayableArtifact(t *testing.T) {
	raw, ok := asset.BuiltinAudio(asset.BuiltinHum)
	if !ok || !validTryAudio(raw) {
		t.Fatal("the real builtin Hum was not resolved and decoded")
	}
	broken := asset.EncodeAudio(&asset.AudioArtifact{Rate: 32000, Channels: 1, Frames: 100, Data: []byte("not audio")})
	for _, data := range [][]byte{nil, raw[:20], broken} {
		if validTryAudio(data) {
			t.Fatal("invalid or truncated audio dependency was accepted")
		}
	}
}

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
	// Animation dependencies are resolved through the real asset loader too;
	// merely finding a builtin ID must not stand in for loading its resource.
	animDeps := []byte(`{"component":"engine.SpriteRenderer","dependencies":["ae900000000000000000000000000b10","ae900000000000000000000000000b20"]}`)
	if r := a.Call("inspect", animDeps); r.Error != "" || r.Inspection == nil || len(r.Inspection.Dependencies) != 2 {
		t.Fatalf("cannot resolve the actual animation and controller: %+v", r)
	}
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

func TestTryPrefabDependencyMustParseAndExpand(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := p.Docs().Get("engine.SpriteRenderer")
	if !ok {
		t.Fatal("SpriteRenderer Try is unavailable")
	}
	prefab, ok := p.Docs().Get("engine.PrefabInstance")
	if !ok {
		t.Fatal("PrefabInstance Try is unavailable")
	}
	code, err := p.TryCode(prefab.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	const id = "d5600000000000000000000000000f02"
	const nested = "d5600000000000000000000000000f03"
	for _, tc := range []struct {
		name    string
		sources map[string]string
		valid   bool
	}{
		{"actual prefab", map[string]string{id: prefab.TryPrefabs[id]}, true},
		{"valid nested prefab", map[string]string{id: `prefab version=1 { node "d5600000000000000000000000000311" prefab=asset("` + nested + `") {} }`, nested: prefab.TryPrefabs[id]}, true},
		{"truncated source", map[string]string{id: "prefab version=1 {"}, false},
		{"missing nested root", map[string]string{id: `prefab version=1 { node "d5600000000000000000000000000311" prefab=asset("d5600000000000000000000000000fff") {} }`}, false},
		{"missing nested child", map[string]string{id: `prefab version=1 { node "d5600000000000000000000000000311" name="root" { node "d5600000000000000000000000000312" prefab=asset("d5600000000000000000000000000fff") {} } }`}, false},
		{"prefab cycle", map[string]string{id: `prefab version=1 { node "d5600000000000000000000000000311" prefab=asset("` + id + `") {} }`}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newHeadless(t)
			req, err := json.Marshal(map[string]any{"scene": entry.Try, "select": entry.TrySelect, "code": code, "prefabs": tc.sources, "artifacts": entry.TryArtifacts})
			if err != nil {
				t.Fatal(err)
			}
			if r := a.Call("load", req); r.Error != "" {
				t.Fatal(r.Error)
			}
			if err := a.Step(4); err != nil {
				t.Fatal(err)
			}
			r := a.Call("inspect", []byte(`{"component":"engine.SpriteRenderer","dependencies":["d5600000000000000000000000000f02"]}`))
			if tc.valid && (r.Error != "" || r.Inspection == nil || len(r.Inspection.Dependencies) != 1) {
				t.Fatalf("valid prefab was not resolved: %+v", r)
			}
			if !tc.valid && r.Error == "" {
				t.Fatal("an invalid prefab was accepted as a resolved dependency")
			}
		})
	}
}
