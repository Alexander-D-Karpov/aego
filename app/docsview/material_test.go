//go:build aego_editor

package docsview

import (
	"encoding/json"
	"math"
	"testing"

	playmods "aego/app/assembly/playground"
	"aego/app/playground"
	"aego/engine/docs"
	"aego/modules/render2d"
)

func TestMaterialTryCompilesItsQualifiedScriptAndAnimatesFlash(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	entries := p.TryScenes()
	for _, id := range []string{"render2d/materials"} {
		t.Run(id, func(t *testing.T) {
			var entry docs.Entry
			for _, e := range entries {
				if e.ID == id {
					entry = e
				}
			}
			if entry.Try == "" || entry.TryScript == "" {
				t.Fatal("material try scene or adjacent script missing")
			}
			code, err := p.TryCode(entry.TryScript)
			if err != nil {
				t.Fatal(err)
			}
			mods, err := playmods.DecodeTry(code)
			if err != nil {
				t.Fatal(err)
			}
			compiled := false
			for _, mod := range mods {
				for _, script := range mod.Scripts {
					compiled = compiled || script.FullName == "game.demo.MaterialPulse"
				}
			}
			if !compiled {
				t.Fatal("TryCode did not prepare the qualified material script")
			}
			a := newHeadless(t)
			request, err := json.Marshal(map[string]any{"scene": entry.Try, "select": entry.TrySelect, "code": code, "prefabs": entry.TryPrefabs})
			if err != nil {
				t.Fatal(err)
			}
			if reply := a.Call("load", request); reply.Error != "" {
				t.Fatal(reply.Error)
			}
			if err := a.Step(2); err != nil {
				t.Fatal(err)
			}
			sc := a.session.Controller().Scene()
			if sc == nil {
				t.Fatal("try did not start a play scene")
			}
			node := sc.Find(sc.Root(), "Flash instance")
			if node.IsZero() {
				t.Fatal("animated material node missing")
			}
			read := func() float64 {
				t.Helper()
				value, err := render2d.GetMaterialParameter(sc, node, "Flash")
				if err != nil {
					t.Fatal(err)
				}
				return value.F
			}
			first := read()
			low, high := first, first
			for range 12 {
				if err := a.Step(15); err != nil {
					t.Fatal(err)
				}
				value := read()
				low, high = math.Min(low, value), math.Max(high, value)
			}
			if low < 0 || high > 1 || high-low < .8 {
				t.Fatalf("Try material parameter did not animate through its range: first=%g range=[%g,%g]", first, low, high)
			}
			if errors := errorsOf(a); len(errors) != 0 {
				t.Fatalf("try emitted script/runtime errors: %v", errors)
			}
		})
	}
}
