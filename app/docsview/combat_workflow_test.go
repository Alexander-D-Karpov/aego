//go:build aego_editor

package docsview

import (
	"encoding/json"
	"fmt"
	"testing"

	"aego/app/playground"
	"aego/modules/gameplay"
	"aego/modules/render2d"
	"aego/scene"
)

// The indicator follows the target's actual Health after the real gameplay
// system processes the overlap; changing the edited field alone is not enough.
func TestCombatComponentTriesDealDamageAndReset(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, field string
		value     int
		wantHP    int32
	}{
		{"gameplay.Hitbox2D", "Damage", 3, 7},
		{"gameplay.Hurtbox2D", "Multiplier", 2, 6},
	} {
		t.Run(tc.id, func(t *testing.T) {
			e, ok := p.Docs().Get(tc.id)
			if !ok || e.Demo == nil {
				t.Fatal("component contract is absent")
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
			state := func() (*gameplay.Health, *render2d.SpriteRenderer) {
				t.Helper()
				sc := a.session.Controller().Scene()
				target := sc.Find(sc.Root(), "Target")
				hp, ok := scene.Get[gameplay.Health](sc, target)
				if !ok {
					t.Fatal("target Health is absent")
				}
				marker, ok := scene.Get[render2d.SpriteRenderer](sc, target)
				if !ok {
					t.Fatal("target indicator is absent")
				}
				return hp, marker
			}
			hp, marker := state()
			initialColor := marker.Tint
			if hp.Max != 10 || hp.Current != 10 || hp.Dead {
				t.Fatalf("initial target health: %+v", hp)
			}
			payload := fmt.Sprintf(`{"component":%q,"field":%q,"value":%d}`, tc.id, tc.field, tc.value)
			if reply := a.Call("set", []byte(payload)); reply.Error != "" {
				t.Fatal(reply.Error)
			}
			if err := a.Step(12); err != nil {
				t.Fatal(err)
			}
			hp, marker = state()
			if hp.Current != tc.wantHP || hp.Dead || marker.Tint == initialColor {
				t.Fatalf("the actual hit and indicator disagree: health=%d want=%d tint=%v", hp.Current, tc.wantHP, marker.Tint)
			}
			if reply := a.Call("reset", nil); reply.Error != "" {
				t.Fatal(reply.Error)
			}
			if err := a.Step(12); err != nil {
				t.Fatal(err)
			}
			hp, marker = state()
			if hp.Current != 10 || hp.Dead || marker.Tint != initialColor {
				t.Fatalf("Reset did not restore target health and indicator: health=%d tint=%v", hp.Current, marker.Tint)
			}
			if messages := errorsOf(a); len(messages) != 0 {
				t.Fatalf("combat Try runtime errors: %v", messages)
			}
		})
	}
}
