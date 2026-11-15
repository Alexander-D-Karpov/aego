//go:build aego_editor && aego_gltest && linux && cgo && !aego_nonative

package docsview

import (
	"context"
	"math"
	"runtime"
	"testing"

	"aego/app/playground"
	"aego/modules/camera"
)

// In addition to the rendered-frame contract, check the actual camera service
// so a moving decoration cannot stand in for a changing camera pose.
func TestCameraShakeComponentDemoDrivesRealCamera(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := p.Docs().Get("camera.CameraShake2D")
	if !ok || entry.Demo == nil || len(entry.Demo.Actions) != 1 {
		t.Fatal("camera shake contract is absent")
	}
	ctx := context.Background()
	opened, err := (&nativeTryRunner{p: p}).Open(ctx, *entry.Demo, "native")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := opened.Close(); err != nil {
			t.Error(err)
		}
	}()
	instance := opened.(*nativeTryInstance)
	service := func() *camera.Service {
		t.Helper()
		s := camera.Of(instance.app.session.Controller().World())
		if s == nil || s.Trauma("") < 0.99 {
			t.Fatal("the real camera service did not receive the hit")
		}
		return s
	}
	checkStill := func() {
		t.Helper()
		s := service()
		if diff := math.Abs(float64(s.Render("").Rotation - s.Logical("").Rotation)); diff > 1e-6 {
			t.Fatalf("zero Max Angle unexpectedly rotated the view: %g", diff)
		}
	}
	checkStill()
	if err := instance.Action(ctx, entry.Demo.Actions[0]); err != nil {
		t.Fatal(err)
	}
	// The contract captures immediately after this warmup. A zero crossing of
	// the real shake noise must not make the changed view look stationary.
	current := service()
	if delta := math.Abs(float64(current.Render("").Rotation - current.Logical("").Rotation)); delta < 0.05 {
		t.Fatalf("camera barely rotated at the contract capture: %g radians", delta)
	}
	world := instance.app.session.Controller().World()
	startTick := world.Time.Tick()
	var peak float64
	for range 60 {
		// Presentation frames do not advance a controlled Try world. Advance
		// each simulation frame explicitly before sampling its camera pose.
		if err := instance.advance(ctx, 1, nil); err != nil {
			t.Fatal(err)
		}
		s := service()
		peak = max(peak, math.Abs(float64(s.Render("").Rotation-s.Logical("").Rotation)))
	}
	if world.Time.Tick() <= startTick {
		t.Fatal("camera samples did not advance the Try simulation")
	}
	if peak < 0.01 {
		t.Fatalf("changing Max Angle did not move the real camera pose: peak %g", peak)
	}
	if err := instance.Reset(ctx, entry.Demo.Reset); err != nil {
		t.Fatal(err)
	}
	checkStill()
	if messages := errorsOf(instance.app); len(messages) != 0 {
		t.Fatalf("Try runtime errors: %v", messages)
	}
}
