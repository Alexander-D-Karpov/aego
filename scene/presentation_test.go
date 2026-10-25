package scene_test

import (
	"testing"

	"aego/core/mathx"
	"aego/scene"
	"aego/schema"
)

func TestPresentationMovesRenderWorldOnly(t *testing.T) {
	s := scene.New(scene.NewTypes(schema.NewRegistry()), nil, scene.Options{})
	body := s.Create(s.Root(), "Body")
	child := s.Create(body, "Sprite")
	s.SetPosition(child, mathx.Vec2{X: 1})
	s.Flush()
	world := s.World(child)
	one := mathx.Vec2{X: 1, Y: 1}
	for _, x := range []float32{5, 7} {
		s.SetPresentationLocal(body, scene.Transform2D{Position: mathx.Vec2{X: x}, Scale: one})
		if got := s.RenderWorld(child)[6]; got != x+1 {
			t.Fatalf("render x = %v, want %v", got, x+1)
		}
		if s.World(child) != world {
			t.Fatal("presentation changed the simulation transform")
		}
	}
	s.ClearPresentation()
	if s.RenderWorld(child) != world {
		t.Fatal("clearing the presentation must fall back to the world transform")
	}
}

func TestDropPresentationLocalKeepsOthers(t *testing.T) {
	s := scene.New(scene.NewTypes(schema.NewRegistry()), nil, scene.Options{})
	one := mathx.Vec2{X: 1, Y: 1}
	var ns [3]scene.NodeHandle
	for i := range ns {
		ns[i] = s.Create(s.Root(), "N")
		s.SetPresentationLocal(ns[i], scene.Transform2D{Position: mathx.Vec2{X: float32(i + 1)}, Scale: one})
	}
	s.Flush()
	s.DropPresentationLocal(ns[0])
	if _, ok := s.PresentationLocal(ns[0]); ok {
		t.Fatal("dropped pose is still there")
	}
	for i := 1; i < 3; i++ {
		if p, ok := s.PresentationLocal(ns[i]); !ok || p.Position.X != float32(i+1) {
			t.Fatalf("node %d pose %v %v", i, p, ok)
		}
	}
	s.SetPresentationLocal(ns[2], scene.Transform2D{Position: mathx.Vec2{X: 7}, Scale: one})
	if p, _ := s.PresentationLocal(ns[1]); p.Position.X != 2 {
		t.Fatalf("moved entry has a stale index: %v", p)
	}
	s.Destroy(ns[2])
	s.Flush()
	if p, ok := s.PresentationLocal(ns[1]); !ok || p.Position.X != 2 || s.RenderWorld(ns[1])[6] != 2 {
		t.Fatalf("destroy dropped the wrong pose: %v %v", p, ok)
	}
}
