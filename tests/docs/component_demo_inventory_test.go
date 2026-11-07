//go:build !aego_runtime

package docs_test

import (
	"context"
	"strings"
	"testing"
)

// This is an inventory and fail-closed test, not a substitute for running the
// component demos. The complete acceptance gate still needs a real Try runner.
func TestComponentDemoInventoryRequiresRuntimeProbe(t *testing.T) {
	r, bm := build(t)
	entries := r.All()
	want := make(map[string]bool)
	withTry, withContract := 0, 0
	for _, e := range entries {
		if e.Component == nil {
			continue
		}
		want[e.ID] = true
		if e.Try != "" {
			withTry++
		} else {
			t.Logf("component without a Try scene: %s", e.ID)
		}
		if e.Demo != nil {
			withContract++
		}
	}
	if len(want) == 0 {
		t.Fatal("the actual docs registry has no public component entries")
	}
	for _, c := range bm.Components {
		if !want[c.Name] {
			t.Errorf("manifest component %s missing from the docs component inventory", c.Name)
		}
	}
	seen := make(map[string]bool)
	for _, c := range r.ComponentDemos() {
		if !want[c.ID] || seen[c.ID] {
			t.Errorf("unexpected or duplicate component %s", c.ID)
		}
		if c.ID == "engine.Mesh2D" {
			t.Logf("Mesh2D Try identity: %s", c.Identity)
		}
		seen[c.ID] = true
	}
	if len(seen) != len(want) {
		t.Fatalf("component inventory contains %d of %d actual entries", len(seen), len(want))
	}
	report := r.CheckComponentDemos(context.Background(), nil)
	if report.Passed() || report.VerifiedComponents != 0 || report.PublicComponents != len(want) {
		t.Fatalf("a missing runtime probe must never count as acceptance: %+v", report)
	}
	for _, result := range report.Results {
		if len(result.Errors) == 0 {
			t.Errorf("%s was reported without a missing probe or contract error", result.ComponentID)
		}
		if (result.ComponentID == "engine.Camera2D" || result.ComponentID == "engine.Camera3D" || result.ComponentID == "engine.LightOccluder2D" || result.ComponentID == "engine.Mesh2D" || result.ComponentID == "engine.SpriteRenderer") && (len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "runtime probe is absent")) {
			t.Errorf("%s contract is invalid or stale: %v", result.ComponentID, result.Errors)
		}
	}
	t.Logf("registry entries=%d public components=%d existing Try scenes=%d demo contracts=%d verified without runner=%d",
		len(entries), len(want), withTry, withContract, report.VerifiedComponents)
}
