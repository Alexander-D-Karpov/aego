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
