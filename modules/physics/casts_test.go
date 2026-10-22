package physics_test

import (
	"testing"

	"aego/contracts/physics2d"
	"aego/core/mathx"
	"aego/modules/physics/components"
	"aego/scene"
)

type castScene struct {
	*fixture
	ground, groundCol scene.NodeHandle
	mover, foot       scene.NodeHandle
	q                 physics2d.Queries
	all               physics2d.QueryFilter
}

// newCastScene: a static ground whose collider sits on a child node and a kinematic mover with a body box and a foot box.
func newCastScene(t *testing.T) *castScene {
	f := newFixture(t)
	c := &castScene{fixture: f, all: physics2d.QueryFilter{LayerMask: ^uint32(0)}}
	c.ground = f.node(scene.NodeHandle{}, "Ground", mathx.Vec2{Y: 5})
	f.body(c.ground, components.Static)
	c.groundCol = f.node(c.ground, "GroundShape", mathx.Vec2{})
	f.box(c.groundCol, mathx.Vec2{X: 20, Y: 1})
	c.mover = f.node(scene.NodeHandle{}, "Mover", mathx.Vec2{})
	f.body(c.mover, components.Kinematic)
	f.box(c.mover, mathx.Vec2{X: 1, Y: 1})
	c.foot = f.node(c.mover, "Foot", mathx.Vec2{Y: 0.5})
	f.box(c.foot, mathx.Vec2{X: 0.5, Y: 0.5})
	f.step(1)
	c.q = f.sim
	return c
}

func TestCastBodyMapsToNodes(t *testing.T) {
	c := newCastScene(t)
	h, ok := c.q.CastBody(c.mover, mathx.Vec2{}, mathx.Vec2{Y: 10}, c.all)
	if !ok || h.Initial || h.Node != c.ground || h.ColliderNode != c.groundCol {
		t.Fatalf("hit %+v ok=%v", h, ok)
	}
	if !near(h.Fraction, 0.375, 0.005) || !near(h.Normal.Y, -1, 1e-3) || !near(h.Point.Y, 4.5, 0.01) {
		t.Fatalf("hit geometry %+v", h)
	}
	if h, ok := c.q.CastBody(c.ground, mathx.Vec2{}, mathx.Vec2{Y: -10}, c.all); !ok || h.Node != c.mover || h.ColliderNode != c.foot {
		t.Fatalf("ground cast up %+v ok=%v", h, ok)
	}
	if _, ok := c.q.CastBody(c.groundCol, mathx.Vec2{}, mathx.Vec2{Y: 1}, c.all); ok {
		t.Fatal("a collider node is not a body")
	}
}

func TestPenetrationByNode(t *testing.T) {
	c := newCastScene(t)
	if err := c.q.Teleport(c.mover, mathx.Vec2{Y: 3.9}, 0); err != nil {
		t.Fatal(err)
	}
	out := make([]physics2d.CastHit, 4)
	n := c.q.Penetration(c.mover, c.all, out)
	if n != 1 || out[0].Node != c.ground || out[0].ColliderNode != c.groundCol {
		t.Fatalf("penetration %d %+v", n, out[:n])
	}
	if !near(out[0].Depth, 0.15, 1e-3) || !near(out[0].Normal.Y, -1, 1e-3) || !out[0].Initial {
		t.Fatalf("depth and normal %+v", out[0])
	}
}

func TestMoveKinematicReachesTargetAndPushes(t *testing.T) {
	c := newCastScene(t)
	crate, _ := c.crate(mathx.Vec2{X: 2.2})
	c.step(1)
	target := mathx.Vec2{X: 1}
	if err := c.q.MoveKinematic(c.mover, target, 0, dt); err != nil {
		t.Fatal(err)
	}
	c.step(1)
	p, a, ok := c.q.BodyPose(c.mover)
	if !ok || !near(p.X, 1, 1e-4) || !near(p.Y, 0, 1e-4) || a != 0 {
		t.Fatalf("pose %v %v %v", p, a, ok)
	}
	if v, _, _ := c.q.BodyVelocity(c.mover); !near(v.X, 60, 1e-2) {
		t.Fatalf("mover velocity %v", v)
	}
	for range 5 {
		p, _, _ := c.q.BodyPose(c.mover)
		if err := c.q.MoveKinematic(c.mover, p.Add(mathx.Vec2{X: 0.1}), 0, dt); err != nil {
			t.Fatal(err)
		}
		c.step(1)
	}
	if v, _, _ := c.q.BodyVelocity(crate); v.X <= 0 {
		t.Fatalf("crate was not pushed: %v", v)
	}
	if err := c.q.MoveKinematic(c.groundCol, target, 0, dt); err == nil {
		t.Fatal("MoveKinematic on a collider node must fail")
	}
	if err := c.q.ApplyImpulse(crate, mathx.Vec2{X: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestExcludeBodyMatchesExcludeNode(t *testing.T) {
	c := newCastScene(t)
	from := mathx.Vec2{Y: -3}
	if h, ok := c.q.CastCircle(from, 0.25, mathx.Vec2{Y: 10}, c.all); !ok || h.Node != c.mover {
		t.Fatalf("unfiltered %+v", h)
	}
	for _, f := range []physics2d.QueryFilter{
		{LayerMask: ^uint32(0), ExcludeBody: c.mover},
		{LayerMask: ^uint32(0), ExcludeNode: c.mover},
		{LayerMask: ^uint32(0), ExcludeNode: c.foot},
	} {
		if h, ok := c.q.CastCircle(from, 0.25, mathx.Vec2{Y: 10}, f); !ok || h.Node != c.ground {
			t.Fatalf("filter %+v: %+v", f, h)
		}
	}
}
