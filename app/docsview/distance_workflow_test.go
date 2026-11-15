//go:build aego_editor

package docsview

import (
	"encoding/json"
	"math"
	"testing"

	"aego/app/playground"
	"aego/modules/physics"
	"aego/modules/physics/components"
	"aego/scene"
)

func TestDistanceTryChangesRealJointAndResets(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("physics.DistanceJoint2D")
	if !ok || e.Demo == nil {
		t.Fatal("Distance Joint component contract is absent")
	}
	a := newHeadless(t)
	req, err := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "prefabs": e.TryPrefabs, "artifacts": e.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if r := a.Call("load", req); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(10); err != nil {
		t.Fatal(err)
	}
	state := func() (*components.DistanceJoint2D, float32) {
		t.Helper()
		sc := a.session.Controller().Scene()
		bob := sc.Find(sc.Root(), "Bob")
		anchor := sc.Find(sc.Root(), "Anchor")
		joint, ok := scene.Get[components.DistanceJoint2D](sc, bob)
		if !ok || !sc.IsAlive(anchor) {
			t.Fatal("distance joint or anchor is absent")
		}
		sim := physics.SimOf(a.session.Controller().World())
		if sim == nil || sim.Stats().Joints != 1 {
			t.Fatal("the physics world did not create the distance joint")
		}
		return joint, sc.WorldPosition(bob).Sub(sc.WorldPosition(anchor)).Len()
	}
	joint, distance := state()
	if joint.Length != 4 || math.Abs(float64(distance-4)) > 0.05 {
		t.Fatalf("initial joint length=%v distance=%v", joint.Length, distance)
	}
	if r := a.Call("set", []byte(`{"component":"physics.DistanceJoint2D","field":"Length","value":2}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(30); err != nil {
		t.Fatal(err)
	}
	joint, distance = state()
	if joint.Length != 2 || math.Abs(float64(distance-2)) > 0.1 {
		t.Fatalf("the real joint did not shorten: length=%v distance=%v", joint.Length, distance)
	}
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(10); err != nil {
		t.Fatal(err)
	}
	joint, distance = state()
	if joint.Length != 4 || math.Abs(float64(distance-4)) > 0.05 {
		t.Fatalf("Reset did not restore the joint: length=%v distance=%v", joint.Length, distance)
	}
	if messages := errorsOf(a); len(messages) != 0 {
		t.Fatalf("Distance Joint Try runtime errors: %v", messages)
	}
}
