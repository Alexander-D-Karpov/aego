//go:build aego_editor

package docsview

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"aego/app/playground"
	"aego/modules/render2d"
	"aego/scene"
)

func TestTagsTryEditsRealTagIndexWithHistoryAndReset(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("engine.Tags")
	if !ok || e.Demo == nil {
		t.Fatal("Tags contract is absent")
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
	check := func(tags []string, hasEnemy bool) {
		t.Helper()
		if err := a.Step(10); err != nil {
			t.Fatal(err)
		}
		sc := a.session.Controller().Scene()
		h := sc.Find(sc.Root(), "Target")
		if !sc.IsAlive(h) || sc.HasTag(h, "Enemy") != hasEnemy {
			t.Fatal("actual tag index has the wrong result")
		}
		buf := make([]scene.NodeHandle, 4)
		count := sc.WithTag("Enemy", buf)
		if (count == 1 && buf[0] == h) != hasEnemy || (!hasEnemy && count != 0) {
			t.Fatalf("WithTag returned unexpected nodes: %v/%d", buf, count)
		}
		component, ok := scene.Get[scene.Tags](sc, h)
		if !ok || !slices.Equal(component.Tags, tags) {
			t.Fatalf("runtime tag list: %+v", component)
		}
		r := a.Call("inspect", []byte(`{"component":"engine.Tags","fields":["engine.Tags.Tags","engine.Tags.HasEnemy"]}`))
		if r.Error != "" || r.Inspection == nil || !reflect.DeepEqual(r.Inspection.Observables["engine.Tags.Tags"], tags) || r.Inspection.Observables["engine.Tags.HasEnemy"] != hasEnemy {
			t.Fatalf("runtime tag inspection: %+v", r)
		}
	}
	check([]string{"Enemy.Flying"}, true)
	sc := a.session.Controller().Scene()
	marker, _ := scene.Get[render2d.SpriteRenderer](sc, sc.Find(sc.Root(), "Target"))
	before := marker.Tint
	beforeText, beforeState := string(a.session.Text()), a.session.State()
	tooMany := "[" + strings.Repeat(`"Ally",`, 64) + `"Ally"]`
	tooManyBytes := "[" + strings.Repeat(fmt.Sprintf(`%q,`, strings.Repeat("x", 4096)), 4) + `"x"]`
	for _, value := range []string{`null`, `true`, `123`, `"Ally"`, `[123]`, `["Enemy",null]`, `[{"tag":"Enemy"}]`, `["not a tag"]`, fmt.Sprintf(`[%q]`, strings.Repeat("x", 4097)), tooMany, tooManyBytes} {
		if r := a.Call("set", []byte(fmt.Sprintf(`{"component":"engine.Tags","field":"Tags","value":%s}`, value))); r.Error == "" {
			t.Fatalf("invalid string list was accepted: %.100s", value)
		}
	}
	if r := a.Call("set", []byte(`{"component":"engine.Camera2D","field":"Zoom","value":["Ally"]}`)); r.Error == "" {
		t.Fatal("a string list was accepted for a numeric field")
	}
	if state := a.session.State(); state.CanUndo || state.Restarts != beforeState.Restarts || string(a.session.Text()) != beforeText {
		t.Fatal("invalid lists changed the scene, history, or runtime")
	}
	check([]string{"Enemy.Flying"}, true)
	if r := a.Call("set", []byte(`{"component":"engine.Tags","field":"Tags","value":["Ally"]}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	check([]string{"Ally"}, false)
	sc = a.session.Controller().Scene()
	marker, _ = scene.Get[render2d.SpriteRenderer](sc, sc.Find(sc.Root(), "Target"))
	if marker.Tint == before {
		t.Fatal("rendered marker did not follow the real tag query")
	}
	if r := a.Call("undo", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	check([]string{"Enemy.Flying"}, true)
	if r := a.Call("redo", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	check([]string{"Ally"}, false)
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	check([]string{"Enemy.Flying"}, true)
	if r := a.Call("set", []byte(`{"component":"engine.Tags","field":"Tags","value":[]}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	check([]string{}, false)
	if r := a.Call("undo", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	check([]string{"Enemy.Flying"}, true)
	if messages := errorsOf(a); len(messages) != 0 {
		t.Fatalf("Tags Try runtime errors: %v", messages)
	}
}

// The older multi-node spotlight remains an executable guide, not just an
// archived copy of its source bytes.
func TestTagsGuideOriginalRunsRealQueries(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("tags/example")
	if !ok || e.Try == "" || e.TryScript == "" {
		t.Fatal("the original Tags guide is not available")
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
	if err := a.Step(15); err != nil {
		t.Fatal(err)
	}
	sc := a.session.Controller().Scene()
	bat := sc.Find(sc.Root(), "Bat")
	if !sc.IsAlive(bat) || !sc.HasTag(bat, "Enemy") || sc.Local(bat).Scale.X <= 1 {
		t.Fatal("the original spotlight did not query and animate the Enemy-tagged bat")
	}
	if messages := errorsOf(a); len(messages) != 0 {
		t.Fatalf("original Tags guide runtime errors: %v", messages)
	}
}
