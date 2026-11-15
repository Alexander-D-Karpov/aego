//go:build aego_gltest

package editor

import (
	"bytes"
	"runtime"
	"testing"

	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/engine"
	"aego/gpu"
	"aego/render"
	"aego/ui/core"
)

func TestImagePreviewLitPixelsAndLifetime(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	e, err := engine.NewBuilder(engine.Config{Title: "image-preview-test", Width: 64, Height: 64, Hidden: true, ResRoot: t.TempDir(), UserRoot: t.TempDir(), AppID: "image-preview-test"}).WithLogger(log.Nop()).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.Caps().Renderer == "null" {
		t.Fatal("real GL image preview test requested, but no graphics context is available")
	}
	r, err := render.New(e.GPU(), e.Caps(), e.VFS(), log.Nop(), render.Options{MaxSprites: 8, MaxMeshes: 4, MaxSprites3D: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ed := &context.Editor{Renderer: r, UI: core.New(nil)}
	var p imagePreview
	p.init()
	p.controls.Ambient = 0
	p.controls.Intensity = 1
	p.controls.Height = 1
	candidate := normalPreviewPixels(255, 128, 128)
	surface := imageSurface{Albedo: r.White(), Size: mathx.Vec2{X: 2, Y: 2}}
	capture := func() []byte {
		t.Helper()
		ed.UI.Begin(nil, mathx.Vec2{X: 64, Y: 64}, 1)
		if err := r.BeginFrame(e.GPU().BeginFrame(64, 64), 64, 64, 1); err != nil {
			t.Fatal(err)
		}
		p.draw(ed, mathx.Rect{W: 64, H: 64}, surface, candidate, [4]int{0, 0, 2, 2})
		ed.RunRenderHooks()
		r.EndFrame()
		e.GPU().EndFrame()
		ed.UI.End()
		if p.err != "" {
			t.Fatal(p.err)
		}
		out := make([]byte, 64*64*4)
		if err := e.GPU().ReadPixels(p.target.Texture, 0, 0, 64, 64, out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	center := func(data []byte) byte { return data[(32*64+32)*4] }
	p.controls.Position = mathx.Vec2{X: 2}
	right := capture()
	p.controls.Position = mathx.Vec2{X: -2}
	left := capture()
	if center(right) < 100 || center(left) > 20 {
		t.Fatalf("actual Lit preview does not respond to the normal/light: right=%d left=%d", center(right), center(left))
	}
	p.controls.Normal = false
	flat := capture()
	if center(flat) <= center(left)+25 {
		t.Fatal("Normal toggle did not restore the flat-normal surface")
	}
	p.controls.Normal = true
	p.controls.FlipX = true
	flipped := capture()
	if center(flipped) < 100 {
		t.Fatalf("Flip X failed to transform the tangent normal: %d", center(flipped))
	}
	p.controls.Lit = false
	unlit := capture()
	if center(unlit) < 250 || bytes.Equal(unlit, flipped) {
		t.Fatal("Unlit preview is not the real lighting-disabled albedo path")
	}
	resources := e.GPU().(gpu.ResourceReporter)
	if resources.ResourceStats().Textures == 0 {
		t.Fatal("preview did not allocate a real GPU target")
	}
	// Exercise the private HDR/Bloom route before measuring the close baseline.
	p.controls.Lit, p.controls.Bloom = true, true
	capture()
	if r.Stats().PostFXPasses < 4 {
		t.Fatal("preview Bloom lifetime test did not execute the real PostFX stack")
	}
	p.close(ed)
	// Warm shared renderer resources once; repeated isolated sessions must not
	// retain old targets, normal candidates or output-scoped lighting textures.
	baseline := resources.ResourceStats().Textures
	for i := range 30 {
		p.controls.Lit = true
		p.controls.Bloom = i%2 == 0
		capture()
		p.close(ed)
		if got := resources.ResourceStats().Textures; got != baseline {
			t.Fatalf("image tab close leaked textures: %d != %d", got, baseline)
		}
	}
	exerciseArtThumbnailResources(t, e.GPU(), r)
	t.Logf("GL image preview: %s; normal/flat/FlipX/Unlit/Bloom and 30 close cycles passed", e.Caps().Renderer)
}
