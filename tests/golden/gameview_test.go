//go:build aego_golden

package golden

import (
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/app/edit"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/panels/gameview"
	"aego/gpu"
	"aego/modules/physics/components"
	physicseditor "aego/modules/physics/editor"
	"aego/modules/render2d"
	"aego/project"
	"aego/render/rt"
	"aego/scene"
	"aego/scene/sceneio"
)

func overlayScene(s *scene.Scene) {
	cam := s.Create(s.Root(), "Camera")
	c := render2d.DefaultCamera2D()
	c.Zoom = 24
	*scene.Add[render2d.Camera2D](s, cam) = c
	s.SetPosition(cam, mathx.Vec2{Y: 1})
	ground := s.Create(s.Root(), "Ground")
	s.SetPosition(ground, mathx.Vec2{Y: 4})
	b := components.DefaultBody2D()
	b.Type = components.Static
	*scene.Add[components.Body2D](s, ground) = b
	e := components.DefaultEdge()
	e.Points = []mathx.Vec2{{X: -5}, {X: 5}}
	*scene.Add[components.EdgeCollider2D](s, ground) = e
	for i := 0; i < 4; i++ {
		n := s.Create(s.Root(), "Box"+strconv.Itoa(i))
		s.SetPosition(n, mathx.Vec2{X: float32(i)*1.2 - 2, Y: 3.5})
		*scene.Add[components.Body2D](s, n) = components.DefaultBody2D()
		*scene.Add[components.BoxCollider2D](s, n) = components.DefaultBox()
		sr := scene.Add[render2d.SpriteRenderer](s, n)
		*sr = render2d.DefaultSpriteRenderer()
		sr.Size = mathx.Vec2{X: 1, Y: 1}
	}
	ball := s.Create(s.Root(), "Ball")
	s.SetPosition(ball, mathx.Vec2{X: -1.4, Y: 1})
	*scene.Add[components.Body2D](s, ball) = components.DefaultBody2D()
	*scene.Add[components.CircleCollider2D](s, ball) = components.DefaultCircle()
}

func TestGameViewPhysicsOverlay(t *testing.T) {
	runtime.LockOSThread()
	p, err := project.Create(t.TempDir(), "OverlayGolden", false)
	if err != nil {
		t.Fatal(err)
	}
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, nil, assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	src := scene.New(types, nil, scene.Options{})
	overlayScene(src)
	src.Flush()
	rel := "scenes/overlay.aescene"
	if err := os.WriteFile(filepath.Join(p.Root, filepath.FromSlash(rel)), sceneio.Save(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Hidden: true, NoWatch: true, Width: size, Height: size, Logger: log.Nop()})
	if err != nil {
		t.Skipf("no GL context available: %v", err)
	}
	defer s.Close()
	e := s.Engine
	if name := e.Caps().Renderer; !softwareRenderer(name) && os.Getenv("AEGO_GOLDEN_ALLOW_HARDWARE") == "" {
		t.Fatalf("goldens require a software rasterizer, got %q", name)
	}
	s.Step()
	ctx := s.App.Context()
	d, err := ctx.Docs.Open(rel)
	if err != nil {
		t.Fatal(err)
	}
	pc := s.App.Play()
	if err := pc.Start(d); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 90; i++ {
		pc.Tick(1.0 / 60)
	}
	ctx.Overlay.Set(physicseditor.FlagContacts, true)
	pool := rt.New(e.GPU(), gpu.TexSRGBA8, "golden.gameview")
	tgt, err := pool.Resize(nil, size, size, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		pool.Release(tgt)
		pool.Close()
	}()
	frame := e.GPU().BeginFrame(size, size)
	if err := s.Renderer.BeginFrame(frame, size, size, 1); err != nil {
		t.Fatal(err)
	}
	pc.Render(tgt)
	gameview.Overlays(s.App.Viewport(), pc)
	s.Renderer.EndFrame()
	e.GPU().EndFrame()
	buf := make([]byte, size*size*4)
	err = e.GPU().ReadPixels(tgt.Texture, 0, 0, size, size, buf)
	img := &image.NRGBA{Pix: buf, Stride: size * 4, Rect: image.Rect(0, 0, size, size)}
	checkGolden(t, "gameview_physics_overlay", img, err)
}
