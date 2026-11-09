//go:build aego_editor && aego_gltest && linux && cgo && !aego_nonative

package docsview

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"runtime"
	"slices"
	"testing"

	"aego/app/playground"
	"aego/engine/docs/democheck"
)

// Check the real AnimationPlayer after the contract's full warmup, not merely
// the runner's short bootstrap. This is synthetic SDL input and may use Mesa.
func TestComponentContractObservationWarmupNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := p.Docs().Get("animation.AnimationPlayer")
	if !ok || entry.Demo == nil || len(entry.Demo.Actions) != 1 || entry.Demo.Actions[0].Field != "animation.AnimationPlayer.Speed" {
		t.Fatal("the eight-frame AnimationPlayer speed contract is missing")
	}
	contract := *entry.Demo
	// The currently shipped example deliberately starts the clip at speed 0.
	// Exercise a 12-frame warmup using its real scene rather than relying on a
	// different checkpoint's autoplay scene or changing the published contract.
	contract.WarmupFrames = 12
	if !slices.ContainsFunc(contract.Actions[0].Expected, func(expectation democheck.Expectation) bool {
		return expectation.Observable == "animation.AnimationPlayer.Time"
	}) {
		t.Fatal("the published AnimationPlayer contract does not inspect its runtime time")
	}
	ctx := context.Background()
	instance, err := (&nativeTryRunner{p: p}).Open(ctx, contract, "native")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := instance.Close(); err != nil {
			t.Error(err)
		}
	}()
	app := instance.(*nativeTryInstance).app
	if got := app.session.State().Frames; got != 4 {
		t.Fatalf("bootstrap frames = %d, want 4", got)
	}
	assertFrames := func(phase string) {
		t.Helper()
		if got, want := app.session.State().Frames, uint64(warmupFrames(contract)); got != want {
			t.Fatalf("%s observed %d frames, want %d", phase, got, want)
		}
	}
	if err := instance.Reset(ctx, contract.Reset); err != nil {
		t.Fatal(err)
	}
	before, err := instance.Observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertFrames("before")
	var animationTime float64
	if err := json.Unmarshal(before.Observables["animation.AnimationPlayer.Time"], &animationTime); err != nil {
		t.Fatal(err)
	}
	if math.Abs(animationTime) > 1e-6 {
		t.Fatalf("zero-speed clip advanced before the action: %f", animationTime)
	}
	if err := instance.Action(ctx, contract.Actions[0]); err != nil {
		t.Fatal(err)
	}
	after, err := instance.Observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertFrames("action")
	if err := json.Unmarshal(after.Observables["animation.AnimationPlayer.Time"], &animationTime); err != nil {
		t.Fatal(err)
	}
	// At speed 8 the real 0.8-second clip reaches its held final frame after
	// the full warmup; a four-frame bootstrap alone reaches only about 0.53 s.
	if math.Abs(animationTime-0.8) > 1e-5 {
		t.Fatalf("warmed clip time = %f, want the 0.8-second final frame", animationTime)
	}
	if err := instance.Reset(ctx, contract.Reset); err != nil {
		t.Fatal(err)
	}
	reset, err := instance.Observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertFrames("reset")
	if err := json.Unmarshal(reset.Observables["animation.AnimationPlayer.Time"], &animationTime); err != nil || math.Abs(animationTime) > 1e-6 {
		t.Fatalf("reset did not restore the held clip time: %f, %v", animationTime, err)
	}
	if len(before.Frames) != 1 || len(after.Frames) != 1 || len(reset.Frames) != 1 || bytes.Equal(before.Frames[0].Pixels, after.Frames[0].Pixels) || !bytes.Equal(before.Frames[0].Pixels, reset.Frames[0].Pixels) {
		t.Fatal("actual warmed animation frames did not change and reset")
	}
}
