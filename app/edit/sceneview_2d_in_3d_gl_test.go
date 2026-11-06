//go:build aego_gltest

package edit

import (
	"runtime"
	"testing"

	"aego/asset"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/modules/render2d"
	"aego/scene"
	"aego/scene/frame"
)

func TestSceneView3DShows2DSpritePixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := newProject(t)
	const path = "scenes/2d-in-3d.aescene"
	writeScene(t, p, path, func(s *scene.Scene) {
		h := s.Create(s.Root(), "Red sprite")
		sr := scene.Add[render2d.SpriteRenderer](s, h)
		*sr = render2d.DefaultSpriteRenderer()
		sr.Sprite = asset.NewRef[asset.Texture](asset.BuiltinWhite)
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
	doc.View.Mode = document.View3D
	doc.View.Orbit = document.Orbit{Distance: 10, FOV: 60}
	ed.Settings.Grid = false
	for range 3 {
		s.Step()
	}
	raw, ok := s.App.SceneView().LightingWorld().ServiceAny(frame.ServiceID)
	if !ok {
		t.Fatal("missing Scene View runner")
	}
	runner := raw.(*frame.Runner)
	readRed := func() int {
		t.Helper()
		for range 2 {
			s.Step()
		}
		target, w, h := runner.Target()
		pixels := make([]byte, w*h*4)
		if target.IsZero() {
			t.Fatal("no Scene View target")
		}
		if err := ed.Renderer.Device().ReadPixels(target, 0, 0, w, h, pixels); err != nil {
			t.Fatal(err)
		}
		count := 0
		for i := 0; i+3 < len(pixels); i += 4 {
			if pixels[i] > 160 && pixels[i+1] < 80 && pixels[i+2] < 80 {
				count++
			}
		}
		return count
	}
	visible := readRed()
	if visible < 100 {
		t.Fatalf("2D sprite produced only %d red pixels in 3D Scene View", visible)
	}
	sc := doc.Scene()
	h := sc.Find(sc.Root(), "Red sprite")
	sc.SetEnabled(h, false)
	if hidden := readRed(); hidden >= visible/2 {
		t.Fatalf("disabling the sprite left %d of %d red pixels", hidden, visible)
	}
}
