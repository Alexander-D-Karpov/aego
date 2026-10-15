package sceneview

import (
	"testing"

	"aego/asset"
	"aego/core/mathx"
	"aego/editor/core/shape"
	"aego/engine/module"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
	"aego/schema"
)

type Tile struct {
	Size  mathx.Vec2 `aego:"size"`
	Layer int32      `aego:"layer"`
}

type Block struct {
	Half float32 `aego:"half"`
}

type shapesModule struct{}

func (shapesModule) Info() module.Info {
	return module.Info{ID: "test"}
}

func (shapesModule) Register(r *module.Registry) error {
	if err := module.Component[Tile](r, schema.Name("test.Tile"), schema.Space(schema.Space2D)).Err(); err != nil {
		return err
	}
	return module.Component[Block](r, schema.Name("test.Block"), schema.Space(schema.Space3D)).Err()
}

func tileShape(s *scene.Scene, _ asset.Resolver, h scene.NodeHandle) (shape.Shape, bool) {
	q, ok := scene.Get[Tile](s, h)
	if !ok {
		return shape.Shape{}, false
	}
	r := mathx.Rect{X: -q.Size.X / 2, Y: -q.Size.Y / 2, W: q.Size.X, H: q.Size.Y}
	return shape.Shape{Kind: shape.Rect, Local: r, Order: int64(q.Layer)}, true
}

func blockShape(s *scene.Scene, _ asset.Resolver, h scene.NodeHandle) (shape.Shape, bool) {
	b, ok := scene.Get[Block](s, h)
	if !ok {
		return shape.Shape{}, false
	}
	e := mathx.Vec3{X: b.Half, Y: b.Half, Z: b.Half}
	return shape.Shape{Kind: shape.Box, Box: mathx.AABB3{Min: e.Neg(), Max: e}}, true
}

func testTypes(t *testing.T) *scene.Types {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := frame.RegisterTypes(types); err != nil {
		t.Fatal(err)
	}
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Load(types, nil, shapesModule{}); err != nil {
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
	if err := reg.Register("test", scene.Store[Tile](s).Type(), tileShape); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register("test", scene.Store[Block](s).Type(), blockShape); err != nil {
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
	*scene.Add[Tile](s, h) = Tile{Size: size, Layer: layer}
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
