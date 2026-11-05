//go:build aego_gltest

package edit

import (
	"bytes"
	"runtime"
	"testing"

	"aego/asset"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/workspace"
	"aego/modules/render2d"
	render2deditor "aego/modules/render2d/editor"
	"aego/scene"
	"aego/scene/frame"
)

// The toolbar must leave the debug image, not just clear its checked state.
func TestSceneViewLightingToggleClearsDebugPixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := newProject(t)
	const path = "scenes/debug-toggle.aescene"
	writeScene(t, p, path, func(s *scene.Scene) {
		h := s.Create(s.Root(), "Red")
		sr := scene.Add[render2d.SpriteRenderer](s, h)
		*sr = render2d.DefaultSpriteRenderer()
		sr.Sprite = asset.NewRef[asset.Texture](asset.BuiltinWhite)
		sr.Material = asset.NewRef[asset.Material](asset.BuiltinMaterialLit)
		sr.Tint = mathx.Color{R: 1, A: 1}
		sr.Size = mathx.V2(4, 4)
	})
	s, err := Open(p, Config{Width: 800, Height: 600, Hidden: true, NoWatch: true, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ed := s.App.Context()
	doc, err := ed.Docs.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	doc.View.Camera.Position = mathx.Vec2{}
	s.App.SceneView().SetZoom(40)
	ed.Settings.Grid = false
	for range 3 {
		s.Step()
	}
	panel := s.App.SceneView()
	raw, ok := panel.LightingWorld().ServiceAny(frame.ServiceID)
	if !ok {
		t.Fatal("missing Scene View runner")
	}
	runner := raw.(*frame.Runner)
	read := func() []byte {
		t.Helper()
		for range 2 {
			s.Step()
		}
		target, w, h := runner.Target()
		pixels := make([]byte, w*h*4)
		if target.IsZero() || ed.Renderer.Device().ReadPixels(target, 0, 0, w, h, pixels) != nil {
			t.Fatal("cannot read the Scene View")
		}
		return pixels
	}
	press := func(id string) {
		t.Helper()
		if err := ed.Commands.Run(id); err != nil {
			t.Fatal(err)
		}
	}
	press(render2deditor.CmdLighting)
	unlit := read()
	for _, debug := range []string{render2deditor.CmdLightingNormals, render2deditor.CmdLightingShadows} {
		press(debug)
		if bytes.Equal(unlit, read()) {
			t.Fatalf("%s did not change the rendered image", debug)
		}
		press(render2deditor.CmdLighting)
		if off := read(); !bytes.Equal(unlit, off) {
			t.Fatalf("turning off lighting left %s in the rendered image", debug)
		}
	}
}

func TestLightingAuthoringWorkflowNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	lightingAuthoringWorkflow(t, Config{Hidden: true, NoWatch: true, Logger: log.Nop()})
	checkSceneViewLitAdditiveBackdrop(t)
}

// A Lit receiver opts Scene View into a linear intermediate; the additive sprite needs the real opaque editor grid.
// Read the panel attachment so chrome or overlays cannot produce a false pass.
func checkSceneViewLitAdditiveBackdrop(t *testing.T) {
	t.Helper()
	p := newProject(t)
	const path = "scenes/lit-additive-grid.aescene"
	writeScene(t, p, path, func(s *scene.Scene) {
		cam := scene.Add[render2d.Camera2D](s, s.Create(s.Root(), "Camera"))
		*cam = render2d.DefaultCamera2D()
		cam.Zoom, cam.ClearColor = 40, mathx.Color{A: 1}
		for i, name := range []string{"Lit red", "Additive green"} {
			h := s.Create(s.Root(), name)
			sr := scene.Add[render2d.SpriteRenderer](s, h)
			*sr = render2d.DefaultSpriteRenderer()
			sr.Sprite = asset.NewRef[asset.Texture](asset.BuiltinWhite)
			sr.Material = asset.NewRef[asset.Material](asset.BuiltinMaterialLit)
			sr.Tint = mathx.Color{R: 1, A: 1}
			pose := s.Local(h)
			pose.Position.X = -2
			if i == 1 {
				sr.Material = asset.NewRef[asset.Material](asset.BuiltinMaterialAdditive)
				sr.Tint = mathx.Color{G: 1, A: 1}
				pose.Position.X = 2
			}
			sr.Size = mathx.Vec2{X: 2, Y: 2}
			s.SetLocal(h, pose)
		}
	})
	s, err := Open(p, Config{Width: 1103, Height: 777, Hidden: true, NoWatch: true, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ed := s.App.Context()
	doc, err := ed.Docs.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	panel := s.App.SceneView()
	doc.View.Camera.Position = mathx.Vec2{}
	panel.SetZoom(40)
	ed.Settings.Grid = true
	s.Step()
	s.Step()
	if panel.LightingWorld() == nil {
		t.Fatal("no presentation world for Scene View")
	}
	raw, ok := panel.LightingWorld().ServiceAny(frame.ServiceID)
	if !ok {
		t.Fatal("fixed presentation runner unavailable")
	}
	runner := raw.(*frame.Runner)
	if len(runner.Views()) != 1 {
		t.Fatalf("fixed views: %d", len(runner.Views()))
	}
	target, w, h := runner.Target()
	vp := runner.Views()[0].Viewport
	if target.IsZero() || vp.W <= 0 || vp.H <= 0 || (int(vp.W) == w && int(vp.H) == h) {
		t.Fatalf("fixture requires a padded pooled attachment: active %v, bucket %dx%d", vp, w, h)
	}
	read := func() []byte {
		t.Helper()
		if err := ed.Renderer.LightingError(); err != nil {
			t.Fatalf("lost Scene View world on resolve: %v", err)
		}
		if ed.Renderer.Stats().ResolvePasses == 0 {
			t.Fatal("fixture did not exercise the lighting display resolve")
		}
		pixels := make([]byte, w*h*4)
		if err := ed.Renderer.Device().ReadPixels(target, 0, 0, w, h, pixels); err != nil {
			t.Fatal(err)
		}
		red, green := 0, 0
		for k := 0; k < len(pixels); k += 4 {
			r, g, b, a := pixels[k], pixels[k+1], pixels[k+2], pixels[k+3]
			if a != 255 {
				t.Fatalf("backdrop left coverage %d at (%d,%d), bucket %dx%d", a, (k/4)%w, (k/4)/w, w, h)
			}
			if r > 120 && g < 100 && b < 100 {
				red++
			}
			if g > 120 && r < 100 && b < 100 {
				green++
			}
		}
		if red < 600 || green < 600 {
			t.Fatalf("world missing from Scene View: Lit red %d px, Additive green %d px", red, green)
		}
		return pixels
	}
	on := read()
	ed.Settings.Grid = false
	s.Step()
	off := read()
	changed, overRed := 0, 0
	for k := 0; k < len(on); k += 4 {
		if on[k] != off[k] || on[k+1] != off[k+1] || on[k+2] != off[k+2] {
			changed++
			if off[k] > 120 && off[k+1] < 40 && off[k+2] < 40 {
				overRed++
			}
		}
	}
	if changed < 100 || overRed > 0 {
		t.Fatalf("grid must be visible behind the world: changed %d pixels, over opaque red %d", changed, overRed)
	}
	// The same legacy Unlit/Additive world must still reach the display
	// directly: no lighting resolve, no tone map applied to solid primaries.
	sc := doc.Scene()
	sr, ok := scene.Get[render2d.SpriteRenderer](sc, sc.Find(sc.Root(), "Lit red"))
	if !ok {
		t.Fatal("missing fixture receiver")
	}
	sr.Material = asset.Ref[asset.Material]{}
	s.Step()
	stats := ed.Renderer.Stats()
	if stats.ResolvePasses != 0 || stats.ShadowDraws != 0 || stats.CookieCopies != 0 {
		t.Fatalf("Unlit scene acquired lighting passes: %+v", stats)
	}
	legacy := make([]byte, w*h*4)
	if err := ed.Renderer.Device().ReadPixels(target, 0, 0, w, h, legacy); err != nil {
		t.Fatal(err)
	}
	red := 0
	for k := 0; k < len(legacy); k += 4 {
		if legacy[k] >= 254 && legacy[k+1] < 2 && legacy[k+2] < 2 && legacy[k+3] == 255 {
			red++
		}
	}
	if red < 600 {
		t.Fatalf("Unlit primary was not preserved on display: %d pixels", red)
	}

	// Game View uses the scene camera, not the editor backdrop; its full opaque clear owns the dedicated pool's padding.
	// Transparent exports receive no implicit backdrop.
	sr.Material = asset.NewRef[asset.Material](asset.BuiltinMaterialLit)
	if err := s.App.Play().Start(doc); err != nil {
		t.Fatal(err)
	}
	s.App.Workspace().Show(workspace.Game)
	s.Step()
	s.Step()
	owner, ok := s.App.Play().World().ServiceAny(frame.ServiceID)
	if !ok {
		t.Fatal("Game View has no presentation runner")
	}
	game := owner.(*frame.Runner)
	target, w, h = game.Target()
	if len(game.Views()) != 1 || target.IsZero() {
		t.Fatal("Game View has no camera or target")
	}
	gv := game.Views()[0]
	if game.Fixed() || (int(gv.Viewport.W) == w && int(gv.Viewport.H) == h) {
		t.Fatalf("Game View fixture requires scene camera and bucket padding: %v %dx%d", gv.Viewport, w, h)
	}
	_ = read()

}
