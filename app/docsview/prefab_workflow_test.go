//go:build aego_editor

package docsview

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"aego/app/playground"
	"aego/core/ids"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/scene/scriptcomp"
)

func TestPrefabTryUsesRealIndependentInstancesWithHistoryAndReset(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get(sceneio.PrefabName)
	if !ok || e.Demo == nil {
		t.Fatal("PrefabInstance contract is absent")
	}
	code, err := p.TryCode(e.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	const prefabID = "d5600000000000000000000000000f02"
	id, err := ids.ParseAssetID(prefabID)
	if err != nil {
		t.Fatal(err)
	}
	load := func(app *App, text string) {
		t.Helper()
		req, err := json.Marshal(map[string]any{"scene": text, "select": e.TrySelect, "code": code, "prefabs": e.TryPrefabs, "artifacts": e.TryArtifacts})
		if err != nil {
			t.Fatal(err)
		}
		if r := app.Call("load", req); r.Error != "" {
			t.Fatal(r.Error)
		}
	}
	check := func(app *App, enabled bool, x float32) {
		t.Helper()
		if err := app.Step(10); err != nil {
			t.Fatal(err)
		}
		sc := app.session.Controller().Scene()
		selected, reference := sc.Find(sc.Root(), "Selected instance"), sc.Find(sc.Root(), "Reference instance")
		var rootKeys [2]ids.NodeID
		for i, h := range []scene.NodeHandle{selected, reference} {
			if h.IsZero() || !sc.IsAlive(h) {
				t.Fatal("prefab copy is absent")
			}
			pi, ok := sceneio.InstanceOf(sc, h)
			if !ok || !pi.Owner || pi.Missing || pi.Prefab != id {
				t.Fatalf("copy is not linked to the resolved prefab: %+v", pi)
			}
			key, ok := pi.Keys[sc.ID(h)]
			if !ok {
				t.Fatal("copy has no prefab root key")
			}
			rootKeys[i] = key.ID()
			scripts, ok := scriptcomp.ScriptsOf(sc, h)
			if !ok || len(scripts.Attachments) != 1 || scripts.Attachments[0].Enabled != (i == 0 && enabled) {
				t.Fatal("script attachment leaked between instances")
			}
		}
		if rootKeys[0] != rootKeys[1] || sc.ID(selected) == sc.ID(reference) {
			t.Fatal("copies must have distinct scene IDs and the same prefab root key")
		}
		if sc.WorldPosition(selected).X != x || sc.WorldPosition(reference).X != 2 {
			t.Fatalf("instance positions changed unexpectedly: %v / %v", sc.WorldPosition(selected), sc.WorldPosition(reference))
		}
		req := []byte(`{"component":"engine.PrefabInstance","fields":["engine.PrefabInstance.PositionX","engine.PrefabInstance.ReferencePositionX","engine.PrefabInstance.LinkedPrefab","engine.PrefabInstance.ReferencePrefab"],"dependencies":["d5600000000000000000000000000f02"]}`)
		r := app.Call("inspect", req)
		if r.Error != "" || r.Inspection == nil || !r.Inspection.RuntimeExecuting || len(r.Inspection.Dependencies) != 1 || r.Inspection.Observables["engine.PrefabInstance.PositionX"] != float64(x) || r.Inspection.Observables["engine.PrefabInstance.ReferencePositionX"] != float64(2) || r.Inspection.Observables["engine.PrefabInstance.LinkedPrefab"] != prefabID || r.Inspection.Observables["engine.PrefabInstance.ReferencePrefab"] != prefabID {
			t.Fatalf("actual prefab inspection: %+v", r)
		}
		if messages := errorsOf(app); len(messages) != 0 {
			t.Fatalf("Prefab Try runtime errors: %v", messages)
		}
	}
	a := newHeadless(t)
	load(a, e.Try)
	check(a, false, -2)
	if r := a.Call("scriptEnabled", []byte(`{"script":"game.demo.MovePrefab","enabled":true}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	check(a, true, -1)
	// Serialize and reload the edited document independently, so a transient
	// mutation or a flattened node cannot pass as a saved instance override.
	edited := string(a.session.Text())
	if !strings.Contains(edited, "#enabled") || !strings.Contains(edited, prefabID) {
		t.Fatalf("edited source does not contain the prefab/script override: %s", edited)
	}
	b := newHeadless(t)
	load(b, edited)
	check(b, true, -1)
	if r := a.Call("undo", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	check(a, false, -2)
	if r := a.Call("redo", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	check(a, true, -1)
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	check(a, false, -2)
}

func TestPrefabGuideOriginalStillResolvesAndAnimates(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("prefabs/example")
	if !ok || e.Try == "" || e.TryScript == "" {
		t.Fatal("original prefab guide is unavailable")
	}
	const lampID = "d5600000000000000000000000000f01"
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(e.TryPrefabs[lampID]))); got != "8f7f5b5f59540e6c079d7f032abb2ac16d564733872578ff5da221953e85ec08" {
		t.Fatalf("original lamp prefab changed: %s", got)
	}
	code, err := p.TryCode(e.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	a := newHeadless(t)
	req, err := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "code": code, "prefabs": e.TryPrefabs})
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
	for _, name := range []string{"Red", "Lamp", "Big", "Blue"} {
		h := sc.Find(sc.Root(), name)
		pi, ok := sceneio.InstanceOf(sc, h)
		if !ok || pi.Missing || pi.Prefab.String() != lampID {
			t.Fatalf("original instance %s did not resolve", name)
		}
	}
	head := sc.Find(sc.Root(), "Red/Head")
	if head.IsZero() || !sc.IsAlive(head) || sc.Local(head).Rotation == 0 {
		t.Fatal("original lamp script did not rotate its head")
	}
	if messages := errorsOf(a); len(messages) != 0 {
		t.Fatalf("original prefab guide runtime errors: %v", messages)
	}
}

func TestPrefabInspectionRejectsMissingInstance(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get(sceneio.PrefabName)
	if !ok {
		t.Fatal("PrefabInstance Try is unavailable")
	}
	code, err := p.TryCode(e.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	a := newHeadless(t)
	// The scene loader deliberately keeps a placeholder if the prefab is
	// missing; inspection must not report that placeholder as a resolved copy.
	req, err := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "code": code})
	if err != nil {
		t.Fatal(err)
	}
	if r := a.Call("load", req); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(4); err != nil {
		t.Fatal(err)
	}
	if r := a.Call("inspect", []byte(`{"component":"engine.PrefabInstance","fields":["engine.PrefabInstance.LinkedPrefab"]}`)); r.Error == "" {
		t.Fatal("a missing prefab placeholder was accepted as a linked instance")
	}
}
