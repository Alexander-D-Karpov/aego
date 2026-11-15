//go:build aego_editor && aego_gltest && linux && cgo && !aego_nonative

package docsview

import (
	"encoding/json"
	"image"
	"runtime"
	"testing"

	"aego/app/playground"
)

// Synthetic bridge input drives the real SDL/OpenGL Try runtime. Opening the
// guide uses Go APIs; this is not physical OS input or hardware-GPU coverage.
func TestControlledGuideRealNativeViewportAndReset(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("gameplay/motion")
	if !ok {
		t.Fatal("Motion guide missing")
	}
	code, err := p.TryCode(e.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{Width: 692, Height: 560})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.e.SetLockstep(true)
	if r := a.Call("control", []byte(`{"enabled":true}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	req, err := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "code": code, "prefabs": e.TryPrefabs, "artifacts": e.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if r := a.Call("load", req); r.Error != "" {
		t.Fatal(r.Error)
	}
	advance := func(request string, frames int) {
		t.Helper()
		if r := a.Call("advance", []byte(request)); r.Error != "" {
			t.Fatal(r.Error)
		}
		if err := a.Step(frames); err != nil || a.control.Remaining != 0 {
			t.Fatalf("native controlled input did not finish: %v", err)
		}
	}
	capture := func() ([]byte, image.Rectangle) {
		t.Helper()
		var img *image.RGBA
		var captureErr error
		a.e.CaptureFrame(func(value *image.RGBA, err error) { img, captureErr = value, err })
		if err := a.Step(1); err != nil || captureErr != nil || img == nil {
			t.Fatalf("native framebuffer capture: %v, %v", err, captureErr)
		}
		viewport := a.Call("viewport", nil).Viewport
		if viewport == nil {
			t.Fatal("native viewport was not reported")
		}
		area, err := viewport.pixelRect(img.Bounds().Dx(), img.Bounds().Dy())
		if err != nil {
			t.Fatal(err)
		}
		// The Inspector starts to the right of the real Game View. A half-canvas
		// crop would include it at this 692px layout.
		if area.Min.Y <= 0 || area.Max.X >= img.Bounds().Dx()/2 {
			t.Fatalf("native Game View crop includes toolbar or Inspector: %v", area)
		}
		pixels := make([]byte, 0, area.Dx()*area.Dy()*4)
		for y := area.Min.Y; y < area.Max.Y; y++ {
			start := img.PixOffset(area.Min.X, y)
			pixels = append(pixels, img.Pix[start:start+area.Dx()*4]...)
		}
		return pixels, area
	}
	advance(`{"frames":30}`, 30)
	sc := a.session.Controller().Scene()
	player := sc.Find(sc.Root(), "Player")
	if player.IsZero() || !sc.IsAlive(player) {
		t.Fatal("native guide Player missing")
	}
	origin := sc.WorldPosition(player)
	before, area := capture()
	advance(`{"frames":30,"keys":["D"]}`, 30)
	if moved := sc.WorldPosition(player); moved.X <= origin.X+0.1 {
		t.Fatalf("native controlled input did not move the Player: %v -> %v", origin, moved)
	}
	after, areaAfter := capture()
	if areaAfter != area || len(before) != len(after) {
		t.Fatal("native Game View viewport changed during controlled input")
	}
	changed := 0
	for i := 0; i < len(before); i += 4 {
		if before[i] != after[i] || before[i+1] != after[i+1] || before[i+2] != after[i+2] || before[i+3] != after[i+3] {
			changed++
		}
	}
	if changed < 100 {
		t.Fatalf("native Game View did not visibly change: %d pixels", changed)
	}
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	advance(`{"frames":30}`, 30)
	sc = a.session.Controller().Scene()
	player = sc.Find(sc.Root(), "Player")
	if got := sc.WorldPosition(player); got != origin {
		t.Fatalf("native Reset did not restore the Player: %v != %v", got, origin)
	}
	if errors := errorsOf(a); len(errors) != 0 {
		t.Fatalf("native guide runtime errors: %v", errors)
	}
	t.Logf("actual Game View changed %d pixels; Inspector excluded", changed)
}
