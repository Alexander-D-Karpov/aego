//go:build aego_editor && aego_gltest && linux && cgo && !aego_nonative

package docsview

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"runtime"
	"testing"

	"aego/app/playground"
	"aego/engine/docs/democheck"
)

// The uniform square remains visually unchanged when flipped. A changed
// Inspector must never count as a changed Game View in pixel evidence.
func TestTryGameViewportExcludesInspectorNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := p.Docs().Get("engine.SpriteRenderer")
	if !ok || entry.Demo == nil {
		t.Fatal("SpriteRenderer demo missing")
	}
	app, err := New(Options{Width: 692, Height: 560})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	app.e.SetLockstep(true)
	if reply := app.Call("control", []byte(`{"enabled":true}`)); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	load, err := json.Marshal(map[string]any{"scene": entry.Try, "select": entry.TrySelect, "artifacts": entry.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if reply := app.Call("load", load); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	instance := &nativeTryInstance{app: app, contract: *entry.Demo, identity: entry.Demo.Identity}
	ctx := context.Background()
	if err := instance.advance(ctx, warmupFrames(instance.contract), nil); err != nil {
		t.Fatal(err)
	}
	capture := func() *image.RGBA {
		t.Helper()
		var img *image.RGBA
		var captureErr error
		app.e.CaptureFrame(func(shot *image.RGBA, err error) { img, captureErr = shot, err })
		if err := app.Step(1); err != nil || captureErr != nil || img == nil {
			t.Fatalf("capture failed: %v, %v", err, captureErr)
		}
		return img
	}
	before, err := instance.Observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fullBefore := capture()
	if err := instance.Action(ctx, democheck.Action{Field: "engine.SpriteRenderer.FlipX", Value: json.RawMessage("1")}); err != nil {
		t.Fatal(err)
	}
	after, err := instance.Observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fullAfter := capture()
	if bytes.Equal(fullBefore.Pix, fullAfter.Pix) {
		t.Fatal("the Inspector checkbox did not visibly change")
	}
	if len(before.Frames) != 1 || len(after.Frames) != 1 || !bytes.Equal(before.Frames[0].Pixels, after.Frames[0].Pixels) {
		t.Fatal("Inspector-only change contaminated the Game View capture")
	}
	if before.Frames[0].Width != 292 {
		t.Fatalf("incorrect Game View width %d", before.Frames[0].Width)
	}
}
