//go:build aego_editor

package docsview

import (
	"encoding/json"
	"math"
	"testing"

	"aego/app/playground"
	"aego/core/mathx"
	"aego/modules/physics"
	"aego/modules/physics/components"
	"aego/scene"
)

func TestRevoluteTryRotatesRealJointAndResets(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("physics.RevoluteJoint2D")
	if !ok || e.Demo == nil {
		t.Fatal("Revolute Joint component contract is absent")
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
	state := func() (*components.RevoluteJoint2D, float32, mathx.Vec2) {
		t.Helper()
		sc := a.session.Controller().Scene()
		arm := sc.Find(sc.Root(), "Arm")
		joint, ok := scene.Get[components.RevoluteJoint2D](sc, arm)
		if !ok {
			t.Fatal("revolute joint is absent")
		}
		sim := physics.SimOf(a.session.Controller().World())
		if sim == nil || sim.Stats().Joints != 1 {
			t.Fatal("the physics world did not create the joint")
		}
		return joint, sc.Local(arm).Rotation, sc.World(arm).Apply(joint.Anchor)
	}
	joint, angle, pivot := state()
	if !joint.EnableMotor || joint.Speed != 0 || math.Abs(float64(angle)) > 0.001 {
		t.Fatalf("zero-speed motor was not stationary: speed=%v angle=%v", joint.Speed, angle)
	}
	if r := a.Call("set", []byte(`{"component":"physics.RevoluteJoint2D","field":"Speed","value":6}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(30); err != nil {
		t.Fatal(err)
	}
	joint, rotated, rotatedPivot := state()
	if joint.Speed != 6 || math.Abs(float64(rotated)) < 0.5 || rotatedPivot.Sub(pivot).Len() > 0.15 {
		t.Fatalf("the real joint motor did not rotate around its pivot: speed=%v angle=%v pivot=%v -> %v", joint.Speed, rotated, pivot, rotatedPivot)
	}
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(10); err != nil {
		t.Fatal(err)
	}
	joint, restored, restoredPivot := state()
	if joint.Speed != 0 || math.Abs(float64(restored)) > 0.001 || restoredPivot.Sub(pivot).Len() > 0.01 {
		t.Fatalf("Reset did not restore the stationary joint: speed=%v angle=%v pivot=%v", joint.Speed, restored, restoredPivot)
	}
	if messages := errorsOf(a); len(messages) != 0 {
		t.Fatalf("Revolute Joint Try runtime errors: %v", messages)
	}
}
