//go:build aego_editor

package docsview

import (
	"encoding/json"
	"testing"

	"aego/app/playground"
	"aego/asset"
	"aego/engine/docs"
	"aego/modules/render2d"
	"aego/scene"
)

// This executes the real docs Try session with its existing loader, editor
// mutation and reset. A headless run does not prove GPU or browser rendering.
func TestMesh2DTryLoadsEditsAndResets(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	var entry docs.Entry
	for _, candidate := range p.TryScenes() {
		if candidate.ID == "engine.Mesh2D" {
			entry = candidate
			break
		}
	}
	if entry.Try == "" || entry.TrySelect != "engine.Mesh2D" {
		t.Fatal("the Mesh2D component does not own a selectable Try scene")
	}
	a := newHeadless(t)
	request, err := json.Marshal(map[string]any{"scene": entry.Try, "select": entry.TrySelect, "artifacts": entry.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if reply := a.Call("load", request); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(10); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.loader.Mesh(asset.BuiltinCube); !ok {
		t.Fatal("the built-in mesh dependency is unavailable")
	}
	order := func() int32 {
		t.Helper()
		sc := a.session.Controller().Scene()
		if sc == nil {
			t.Fatal("Try runtime is not running")
		}
		node := sc.Find(sc.Root(), "Mesh 2D")
		mesh, ok := scene.Get[render2d.Mesh2D](sc, node)
		if !ok || mesh.Mesh.ID != asset.BuiltinCube {
			t.Fatal("Try runtime lost the component or its mesh")
		}
		return mesh.Order
	}
	if got := order(); got != 2 {
		t.Fatalf("initial mesh order = %d, want 2", got)
	}
	if err := a.session.Set("engine.Mesh2D", "order", 0); err != nil {
		t.Fatal(err)
	}
	if err := a.Step(2); err != nil {
		t.Fatal(err)
	}
	if got := order(); got != 0 {
		t.Fatalf("edited mesh order = %d, want 0", got)
	}
	if err := a.session.Reset(); err != nil {
		t.Fatal(err)
	}
	if err := a.Step(2); err != nil {
		t.Fatal(err)
	}
	if got := order(); got != 2 {
		t.Fatalf("reset mesh order = %d, want 2", got)
	}
	if errors := errorsOf(a); len(errors) > 0 {
		t.Fatalf("Mesh2D Try faults: %v", errors)
	}
}
