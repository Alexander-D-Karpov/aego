package sceneview

import (
	"testing"

	"aego/asset"
	"aego/core/mathx"
	"aego/editor/shape"
	"aego/engine/module"
	"aego/render2d"
	render2deditor "aego/render2d/editor"
	"aego/render3d"
	render3deditor "aego/render3d/editor"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
	"aego/schema"
)

func testTypes(t *testing.T) *scene.Types {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := frame.RegisterTypes(types); err != nil {
		t.Fatal(err)
	}
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Load(types, nil, render2d.New(), render3d.New()); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return types
}

func testShapes(t *testing.T, s *scene.Scene, res asset.Resolver) shape.Set {
	t.Helper()
	reg := shape.NewRegistry()
	if err := reg.Register("render2d", scene.Store[render2d.SpriteRenderer](s).Type(), render2deditor.SpriteShape); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register("render3d", scene.Store[render3d.MeshRenderer](s).Type(), render3deditor.MeshShape); err != nil {
		t.Fatal(err)
	}
	return shape.Set{Reg: reg, Res: res}
}

func newScene(t *testing.T) *scene.Scene {
	t.Helper()
	return scene.New(testTypes(t), nil, scene.Options{})
}

func sprite(s *scene.Scene, name string, pos mathx.Vec2, rot float32, size mathx.Vec2, layer int32) scene.NodeHandle {
	h := s.Create(s.Root(), name)
	s.SetLocal(h, scene.Transform2D{Position: pos, Rotation: rot, Scale: mathx.Vec2{X: 1, Y: 1}})
	sr := scene.Add[render2d.SpriteRenderer](s, h)
	*sr = render2d.DefaultSpriteRenderer()
	sr.Size = size
	sr.Layer = layer
	return h
}

func TestPickRotatedSprite(t *testing.T) {
	s := newScene(t)
	h := sprite(s, "Bar", mathx.Vec2{}, mathx.Pi/2, mathx.Vec2{X: 4, Y: 1}, 0)
	s.Flush()
	if got, ok := Pick(s, testShapes(t, s, nil), mathx.Vec2{Y: 1.5}); !ok || got != h {
		t.Fatal("point inside the rotated bar missed")
	}
	if _, ok := Pick(s, testShapes(t, s, nil), mathx.Vec2{X: 1.5}); ok {
		t.Fatal("point outside the rotated bar hit")
	}
}

func TestPickTopmostWins(t *testing.T) {
	s := newScene(t)
	sprite(s, "Back", mathx.Vec2{}, 0, mathx.Vec2{X: 2, Y: 2}, 0)
	front := sprite(s, "Front", mathx.Vec2{}, 0, mathx.Vec2{X: 2, Y: 2}, 3)
	sprite(s, "Later", mathx.Vec2{}, 0, mathx.Vec2{X: 2, Y: 2}, 0)
	s.Flush()
	if got, _ := Pick(s, testShapes(t, s, nil), mathx.Vec2{}); got != front {
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
	got := BoxSelect(s, testShapes(t, s, nil), mathx.AABB2{Min: mathx.Vec2{X: -1, Y: -1}, Max: mathx.Vec2{X: 2, Y: 2}})
	if len(got) != 2 || got[0] != s.ID(a) || got[1] != s.ID(empty) {
		t.Fatalf("box select = %v", got)
	}
}
