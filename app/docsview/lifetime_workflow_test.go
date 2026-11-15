//go:build aego_editor

package docsview

import (
	"encoding/json"
	"strings"
	"testing"

	"aego/app/playground"
	"aego/modules/gameplay"
	"aego/scene"
)

func TestLifetimeComponentTryActuallyExpiresAndResets(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("gameplay.Lifetime")
	if !ok || e.Demo == nil {
		t.Fatal("Lifetime component contract is absent")
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
	if reply := a.Call("load", req); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(4); err != nil {
		t.Fatal(err)
	}
	if reply := a.Call("set", []byte(`{"component":"gameplay.Lifetime","field":"Seconds","value":8}`)); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(4); err != nil {
		t.Fatal(err)
	}
	sc := a.session.Controller().Scene()
	h := sc.Find(sc.Root(), "Timer")
	life, ok := scene.Get[gameplay.Lifetime](sc, h)
	if !ok || life.Seconds != 8 || life.Remaining <= 0 || life.Remaining >= 8 {
		t.Fatalf("actual Lifetime did not start the shortened countdown: %+v", life)
	}
	if err := a.Step(600); err != nil {
		t.Fatal(err)
	}
	if sc.IsAlive(h) {
		t.Fatal("the Lifetime component did not destroy its node after eight game seconds")
	}
	if reply := a.Call("reset", nil); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(4); err != nil {
		t.Fatal(err)
	}
	sc = a.session.Controller().Scene()
	h = sc.Find(sc.Root(), "Timer")
	life, ok = scene.Get[gameplay.Lifetime](sc, h)
	if !ok || life.Seconds != 60 || life.Remaining <= 30 {
		t.Fatalf("Reset did not recreate the original running Lifetime: %+v", life)
	}
	if messages := errorsOf(a); len(messages) > 0 {
		t.Fatalf("Lifetime Try runtime errors: %v", messages)
	}
}

func TestLifetimeGuideKeepsPoolExample(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("gameplay/lifetime")
	if !ok || !strings.Contains(e.Try, `script "game.demo.Spawner"`) || !strings.Contains(e.TryScript, "scene.Spawn") || len(e.TryPrefabs) != 1 {
		t.Fatalf("the guide lost its spawning scene, script or prefab: %+v", e)
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
	if reply := a.Call("load", req); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(200); err != nil {
		t.Fatal(err)
	}
	sc := a.session.Controller().Scene()
	count := 0
	scene.Query1[gameplay.Lifetime](sc).Each(func(_ scene.NodeHandle, _ *gameplay.Lifetime) { count++ })
	if count < 2 {
		t.Fatalf("the guide's prefab spawner has only %d running Lifetime components", count)
	}
	if messages := errorsOf(a); len(messages) > 0 {
		t.Fatalf("guide runtime errors: %v", messages)
	}
}
