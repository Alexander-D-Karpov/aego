package edit

import (
	"strconv"
	"strings"
	"testing"

	"aego/core/mathx"
	"aego/editor/core/play"
	"aego/editor/core/services"
	"aego/editor/core/shape"
	"aego/editor/core/vectors"
	"aego/editor/core/workspace"
	"aego/editor/panels/gameview"
	"aego/gpu"
	"aego/modules/physics"
	"aego/modules/physics/components"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/render/rt"
	"aego/scene"
)

const physicsBoxes = 8

func physicsScene(s *scene.Scene) {
	cam := s.Create(s.Root(), "Camera")
	c := render2d.DefaultCamera2D()
	c.Zoom = 24
	*scene.Add[render2d.Camera2D](s, cam) = c
	s.SetPosition(cam, mathx.Vec2{Y: -1})
	ground := s.Create(s.Root(), "Ground")
	s.SetPosition(ground, mathx.Vec2{Y: 4})
	b := components.DefaultBody2D()
	b.Type = components.Static
	*scene.Add[components.Body2D](s, ground) = b
	e := components.DefaultEdge()
	e.Points = []mathx.Vec2{{X: -10}, {X: 10}}
	*scene.Add[components.EdgeCollider2D](s, ground) = e
	for i := 0; i < physicsBoxes; i++ {
		n := s.Create(s.Root(), "Box"+strconv.Itoa(i))
		s.SetPosition(n, mathx.Vec2{X: float32(i%3) * 0.2, Y: 3 - float32(i)*1.3})
		s.SetRotation(n, float32(i)*0.1)
		*scene.Add[components.Body2D](s, n) = components.DefaultBody2D()
		*scene.Add[components.BoxCollider2D](s, n) = components.DefaultBox()
	}
}

func renderFrame(t *testing.T, s *Session, w, h int, fn func()) {
	t.Helper()
	e := s.Engine
	f := e.GPU().BeginFrame(w, h)
	if err := s.Renderer.BeginFrame(f, w, h, 1); err != nil {
		t.Fatal(err)
	}
	fn()
	s.Renderer.EndFrame()
	e.GPU().EndFrame()
}

func drawPanel(s *Session, pn workspace.Panel) {
	ctx := s.App.Context()
	ui := ctx.UI
	full := mathx.Rect{W: 900, H: 4000}
	ui.Begin(&input.State{}, mathx.Vec2{X: full.W, Y: full.H}, 1)
	ui.Region(full)
	pn.Draw(&services.Frame{UI: ui}, full)
	ui.PopCursor()
	ui.End()
}

func expectSections(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("sections %v, want %v", got, want)
	}
}

func runPhysics(t *testing.T, overlay bool) ([]mathx.Vec2, int) {
	t.Helper()
	p := newProject(t)
	rel := "scenes/physics.aescene"
	writeScene(t, p, rel, physicsScene)
	s, err := Open(p, headless())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ctx := s.App.Context()
	d, err := ctx.Docs.Open(rel)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	ctx.Shapes.AddOverlayIn("test", func(*shape.OverlayCtx) { calls++ }, true)
	ctx.Overlay.Set(gameview.CommandOverlays, overlay)
	ctx.Overlay.Set(vectors.CommandToggle, true)
	pool := rt.New(ctx.Engine.GPU(), gpu.TexSRGBA8, "test.gameview")
	target, err := pool.Resize(nil, 320, 180, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		pool.Release(target)
		pool.Close()
	}()
	pc := s.App.Play()
	if err := pc.Start(d); err != nil {
		t.Fatal(err)
	}
	if physics.SimOf(pc.World()) == nil {
		t.Fatal("physics is not attached to the play world")
	}
	for i := 0; i < 300; i++ {
		pc.Tick(1.0 / 60)
		renderFrame(t, s, 320, 180, func() {
			pc.Render(target)
			if ctx.Overlay.On(gameview.CommandOverlays) {
				gameview.Overlays(s.App.Viewport(), pc)
			}
		})
		if pc.State() == play.Stopped {
			t.Fatalf("play stopped at frame %d", i)
		}
	}
	ps := pc.Scene()
	out := make([]mathx.Vec2, physicsBoxes)
	for i := range out {
		out[i] = ps.Local(ps.Find(ps.Root(), "Box"+strconv.Itoa(i))).Position
	}
	pc.Stop()
	return out, calls
}

func TestGameOverlayKeepsSimulation(t *testing.T) {
	off, offCalls := runPhysics(t, false)
	on, onCalls := runPhysics(t, true)
	if offCalls != 0 {
		t.Fatalf("overlay called %d times while off", offCalls)
	}
	if onCalls == 0 {
		t.Fatal("overlay never ran while on")
	}
	for i := range off {
		if off[i] != on[i] {
			t.Fatalf("box %d: %v without the overlay, %v with it", i, off[i], on[i])
		}
	}
	if y := off[0].Y; y < 3.3 || y > 3.6 {
		t.Fatalf("bottom box at y=%v did not land on the ground", y)
	}
}

func TestProfilerSectionsFollowPlay(t *testing.T) {
	p := newProject(t)
	rel := "scenes/physics.aescene"
	writeScene(t, p, rel, physicsScene)
	writeScene(t, p, "scenes/other.aescene", func(s *scene.Scene) { s.Create(s.Root(), "Empty") })
	s, err := Open(p, headless())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ctx := s.App.Context()
	d, err := ctx.Docs.Open(rel)
	if err != nil {
		t.Fatal(err)
	}
	prof := s.App.Profiler()
	drawPanel(s, prof)
	expectSections(t, prof.Sections(), "Frame", "Memory")
	pc := s.App.Play()
	if err := pc.Start(d); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		s.Step()
	}
	drawPanel(s, prof)
	expectSections(t, prof.Sections(), "Frame", "Memory", "Game", "Scripts", "Physics", "Audio", "Camera", "Gameplay")
	if _, err := ctx.Docs.Open("scenes/other.aescene"); err != nil {
		t.Fatal(err)
	}
	if ctx.Docs.ActiveWorld() != nil || ctx.PlayWorld() == nil {
		t.Fatal("the profiler must not depend on the active tab")
	}
	pc.Stop()
	s.Step()
	drawPanel(s, prof)
	expectSections(t, prof.Sections(), "Frame", "Memory")
}
