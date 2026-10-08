package sceneview

import (
	"testing"

	"aego/core/mathx"
	"aego/scene"
	"aego/scene/components"
	"aego/scene/sceneio"
	"aego/schema"
)

func newScene(t *testing.T) *scene.Scene {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := components.Register(types); err != nil {
		t.Fatal(err)
	}
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return scene.New(types, nil, scene.Options{})
}

func sprite(s *scene.Scene, name string, pos mathx.Vec2, rot float32, size mathx.Vec2, layer int32) scene.NodeHandle {
	h := s.Create(s.Root(), name)
	s.SetLocal(h, scene.Transform2D{Position: pos, Rotation: rot, Scale: mathx.Vec2{X: 1, Y: 1}})
	sr := scene.Add[components.SpriteRenderer](s, h)
	*sr = components.DefaultSpriteRenderer()
	sr.Size = size
	sr.Layer = layer
	return h
}

func TestPickRotatedSprite(t *testing.T) {
	s := newScene(t)
	h := sprite(s, "Bar", mathx.Vec2{}, mathx.Pi/2, mathx.Vec2{X: 4, Y: 1}, 0)
	s.Flush()
	if got, ok := Pick(s, nil, mathx.Vec2{Y: 1.5}); !ok || got != h {
		t.Fatal("point inside the rotated bar missed")
	}
	if _, ok := Pick(s, nil, mathx.Vec2{X: 1.5}); ok {
		t.Fatal("point outside the rotated bar hit")
	}
}

func TestPickTopmostWins(t *testing.T) {
	s := newScene(t)
	sprite(s, "Back", mathx.Vec2{}, 0, mathx.Vec2{X: 2, Y: 2}, 0)
	front := sprite(s, "Front", mathx.Vec2{}, 0, mathx.Vec2{X: 2, Y: 2}, 3)
	sprite(s, "Later", mathx.Vec2{}, 0, mathx.Vec2{X: 2, Y: 2}, 0)
	s.Flush()
	if got, _ := Pick(s, nil, mathx.Vec2{}); got != front {
		t.Fatalf("picked %s", s.Name(got))
	}
}

func TestBoxSelect(t *testing.T) {
	s := newScene(t)
	a := sprite(s, "A", mathx.Vec2{X: 0}, 0, mathx.Vec2{X: 1, Y: 1}, 0)
	sprite(s, "B", mathx.Vec2{X: 10}, 0, mathx.Vec2{X: 1, Y: 1}, 0)
	empty := s.Create(s.Root(), "Empty")
	s.SetPosition(empty, mathx.Vec2{X: 1, Y: 1})
	s.Flush()
	got := BoxSelect(s, nil, mathx.AABB2{Min: mathx.Vec2{X: -1, Y: -1}, Max: mathx.Vec2{X: 2, Y: 2}})
	if len(got) != 2 || got[0] != s.ID(a) || got[1] != s.ID(empty) {
		t.Fatalf("box select = %v", got)
	}
}
