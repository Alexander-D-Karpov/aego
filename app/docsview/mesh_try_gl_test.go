//go:build aego_editor && aego_gltest && linux && cgo && !aego_nonative

package docsview

import (
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aego/app/playground"
	"aego/engine/docs"
)

// These run the actual docs renderer in SDL/OpenGL; Set and Reset use the
// existing Try session. They do not claim a physical GPU or OS pointer input.
func TestMesh2DTryNativeVisibleOrderAndReset(t *testing.T) {
	tryVisibleOrderAndReset(t, "engine.Mesh2D", 0, "AEGO_MESH_TRY_SHOTS")
}

func TestSpriteRendererTryNativeVisibleOrderAndReset(t *testing.T) {
	tryVisibleOrderAndReset(t, "engine.SpriteRenderer", -1, "AEGO_SPRITE_TRY_SHOTS")
}

func tryVisibleOrderAndReset(t *testing.T, component string, order float64, shotEnv string) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	var entry docs.Entry
	for _, candidate := range p.TryScenes() {
		if candidate.ID == component {
			entry = candidate
			break
		}
	}
	if entry.Try == "" {
		t.Fatalf("%s Try is missing", component)
	}
	a, err := New(Options{Width: 960, Height: 440})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	request, err := json.Marshal(map[string]any{"scene": entry.Try, "select": entry.TrySelect, "artifacts": entry.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if reply := a.Call("load", request); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(30); err != nil {
		t.Fatal(err)
	}
	capture := func(name string) *image.RGBA {
		t.Helper()
		var shot *image.RGBA
		var shotErr error
		a.e.CaptureFrame(func(img *image.RGBA, err error) { shot, shotErr = img, err })
		if err := a.Step(1); err != nil {
			t.Fatal(err)
		}
		if shotErr != nil || shot == nil {
			t.Fatalf("capture %s: %v", name, shotErr)
		}
		if dir := os.Getenv(shotEnv); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(filepath.Join(dir, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			encodeErr := png.Encode(f, shot)
			closeErr := f.Close()
			if encodeErr != nil || closeErr != nil {
				t.Fatalf("capture write: %v %v", encodeErr, closeErr)
			}
		}
		return shot
	}
	before := capture("before")
	if err := a.session.Set(component, "order", order); err != nil {
		t.Fatal(err)
	}
	if err := a.Step(4); err != nil {
		t.Fatal(err)
	}
	after := capture("edited")
	if err := a.session.Reset(); err != nil {
		t.Fatal(err)
	}
	if err := a.Step(4); err != nil {
		t.Fatal(err)
	}
	reset := capture("reset")
	if before.Bounds() != after.Bounds() || before.Bounds() != reset.Bounds() {
		t.Fatal("Try window changed size")
	}
	// The left half is Game View; exclude Inspector labels and the toolbar.
	area := before.Bounds().Intersect(image.Rect(0, 30, before.Bounds().Dx()/2, before.Bounds().Dy()))
	different := func(x, y *image.RGBA) int {
		count := 0
		for py := area.Min.Y; py < area.Max.Y; py++ {
			for px := area.Min.X; px < area.Max.X; px++ {
				a, b := x.RGBAAt(px, py), y.RGBAAt(px, py)
				if a != b {
					count++
				}
			}
		}
		return count
	}
	changed, residual := different(before, after), different(before, reset)
	t.Logf("native Game View changed pixels=%d, reset difference=%d", changed, residual)
	if changed < 100 || residual != 0 {
		t.Fatalf("%s order/reset did not visibly work: changed=%d residual=%d", component, changed, residual)
	}
}
