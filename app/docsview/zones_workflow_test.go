//go:build aego_editor

package docsview

import (
	"encoding/json"
	"testing"

	"aego/app/playground"
	"aego/modules/gameplay"
	"aego/modules/render2d"
	"aego/scene"
)

func TestTaggedZoneTriesTriggerRealGameplayAndReset(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"gameplay.KillZone2D", "gameplay.Pickup2D"} {
		t.Run(id, func(t *testing.T) {
			e, ok := p.Docs().Get(id)
			if !ok || e.Demo == nil {
				t.Fatal("zone contract is absent")
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
			if err := a.Step(12); err != nil {
				t.Fatal(err)
			}
			state := func() (gameplay.Stats, *gameplay.Health, *render2d.SpriteRenderer) {
				t.Helper()
				world := a.session.Controller().World()
				service := gameplay.Of(world)
				if service == nil {
					t.Fatal("gameplay service is absent")
				}
				sc := a.session.Controller().Scene()
				target := sc.Find(sc.Root(), "Target")
				hp, _ := scene.Get[gameplay.Health](sc, target)
				marker := sc.Find(sc.Root(), "Indicator")
				sprite, ok := scene.Get[render2d.SpriteRenderer](sc, marker)
				if !ok {
					t.Fatal("zone indicator is absent")
				}
				return service.Stats(), hp, sprite
			}
			stats, hp, marker := state()
			original := marker.Tint
			if stats.Deaths != 0 || stats.Pickups != 0 || (hp != nil && hp.Dead) {
				t.Fatalf("mismatched tag triggered a zone: stats=%+v health=%+v", stats, hp)
			}
			set, err := json.Marshal(map[string]any{"component": id, "field": "Tag", "value": "Player"})
			if err != nil {
				t.Fatal(err)
			}
			if reply := a.Call("set", set); reply.Error != "" {
				t.Fatal(reply.Error)
			}
			if err := a.Step(12); err != nil {
				t.Fatal(err)
			}
			stats, hp, marker = state()
			if marker.Tint == original {
				t.Fatal("actual zone event did not change the indicator")
			}
			if id == "gameplay.KillZone2D" && (hp == nil || !hp.Dead || hp.Current != 0 || stats.Deaths != 1) {
				t.Fatalf("kill zone did not kill the target exactly once: stats=%+v health=%+v", stats, hp)
			}
			if id == "gameplay.Pickup2D" && stats.Pickups != 1 {
				t.Fatalf("pickup did not collect the target exactly once: %+v", stats)
			}
			if reply := a.Call("reset", nil); reply.Error != "" {
				t.Fatal(reply.Error)
			}
			if err := a.Step(12); err != nil {
				t.Fatal(err)
			}
			stats, hp, marker = state()
			if stats.Deaths != 0 || stats.Pickups != 0 || (hp != nil && hp.Dead) || marker.Tint != original {
				t.Fatalf("reset did not restore the untouched target: stats=%+v health=%+v color=%v", stats, hp, marker.Tint)
			}
			if messages := errorsOf(a); len(messages) != 0 {
				t.Fatalf("zone Try runtime errors: %v", messages)
			}
		})
	}
}
