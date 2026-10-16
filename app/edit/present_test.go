package edit

import (
	"testing"

	"aego/engine"
	"aego/modules/physics/components"
	"aego/modules/render2d"
	"aego/scene"
)

func TestPlayPresentsWithoutGameView(t *testing.T) {
	p := newProject(t)
	rel := "scenes/fall.aescene"
	writeScene(t, p, rel, func(s *scene.Scene) {
		cam := s.Create(s.Root(), "Camera")
		*scene.Add[render2d.Camera2D](s, cam) = render2d.DefaultCamera2D()
		n := s.Create(s.Root(), "Crate")
		b := scene.Add[components.Body2D](s, n)
		*b = components.DefaultBody2D()
		b.Interpolate = true
		*scene.Add[components.BoxCollider2D](s, n) = components.DefaultBox()
	})
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
	pc := s.App.Play()
	if err := pc.Start(d); err != nil {
		t.Fatal(err)
	}
	ps := pc.Scene()
	h := ps.Find(ps.Root(), "Crate")
	for i := 0; i < 5; i++ {
		pc.Tick(1.0 / 60)
	}
	ctx.Engine.RunPhase(pc.World(), engine.PhaseRenderPrepare, engine.ScopePlay, 0, 0)
	start := ps.RenderWorld(h)[7]
	for i := 0; i < 30; i++ {
		pc.Tick(1.0 / 60)
	}
	got, body := ps.RenderWorld(h)[7], ps.World(h)[7]
	if got == start {
		t.Fatal("presentation froze while the Game View was not rendering")
	}
	if diff := got - body; diff > 0.2 || diff < -0.2 {
		t.Fatalf("presented y %v is far from the body y %v", got, body)
	}
	pc.Stop()
}
