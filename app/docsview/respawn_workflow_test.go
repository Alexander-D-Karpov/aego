//go:build aego_editor

package docsview

import (
	"encoding/json"
	"math"
	"testing"

	"aego/app/playground"
	"aego/modules/gameplay"
)

func TestRespawnAndCheckpointTriesUseRealRespawn(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, field string
		value     float64
		before    uint64
	}{
		{"gameplay.Checkpoint2D", "Priority", 10, 1},
		{"gameplay.Respawn", "Delay", 0, 0},
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
			if r := a.Call("load", req); r.Error != "" {
				t.Fatal(r.Error)
			}
			if err := a.Step(12); err != nil {
				t.Fatal(err)
			}
			state := func() (float32, uint64) {
				t.Helper()
				sc := a.session.Controller().Scene()
				target := sc.Find(sc.Root(), "Target")
				if !sc.IsAlive(target) {
					t.Fatal("respawn target is absent")
				}
				service := gameplay.Of(a.session.Controller().World())
				if service == nil {
					t.Fatal("gameplay service is absent")
				}
				return sc.WorldPosition(target).X, service.Stats().Respawns
			}
			x, n := state()
			if math.Abs(float64(x+2)) > 0.05 || n != tc.before {
				t.Fatalf("initial target position=%v respawns=%d, want -2/%d", x, n, tc.before)
			}
			set, err := json.Marshal(map[string]any{"component": tc.id, "field": tc.field, "value": tc.value})
			if err != nil {
				t.Fatal(err)
			}
			if r := a.Call("set", set); r.Error != "" {
				t.Fatal(r.Error)
			}
			if err := a.Step(12); err != nil {
				t.Fatal(err)
			}
			x, n = state()
			if math.Abs(float64(x-2)) > 0.05 || n != 1 {
				t.Fatalf("actual respawn did not reach the selected point: x=%v count=%d", x, n)
			}
			if r := a.Call("reset", nil); r.Error != "" {
				t.Fatal(r.Error)
			}
			if err := a.Step(12); err != nil {
				t.Fatal(err)
			}
			x, n = state()
			if math.Abs(float64(x+2)) > 0.05 || n != tc.before {
				t.Fatalf("Reset did not restore position and respawn state: x=%v count=%d", x, n)
			}
			if messages := errorsOf(a); len(messages) != 0 {
				t.Fatalf("respawn Try runtime errors: %v", messages)
			}
		})
	}
}
