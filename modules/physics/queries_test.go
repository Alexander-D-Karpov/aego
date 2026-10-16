package physics_test

import (
	"testing"

	"aego/contracts/physics2d"
	"aego/core/mathx"
	"aego/scene"
)

func TestQueriesMapToNodes(t *testing.T) {
	f := newFixture(t)
	g := f.ground()
	b, _ := f.crate(mathx.Vec2{})
	f.step(1)
	q := physics2d.Queries(f.sim)
	all := physics2d.QueryFilter{LayerMask: ^uint32(0)}
	hit, ok := q.Raycast(mathx.Vec2{Y: -5}, mathx.Vec2{Y: 1}, 20, all)
	if !ok || hit.Node != b || hit.ColliderNode != b || !near(hit.Distance, 4.5, 0.05) {
		t.Fatalf("hit %+v %v", hit, ok)
	}
	skip := all
	skip.ExcludeNode = b
	if hit, ok = q.Raycast(mathx.Vec2{Y: -5}, mathx.Vec2{Y: 1}, 20, skip); !ok || hit.Node != g {
		t.Fatalf("excluded hit %+v %v", hit, ok)
	}
	out := make([]scene.NodeHandle, 4)
	if n := q.OverlapPoint(mathx.Vec2{Y: 5}, all, out); n != 1 || out[0] != g {
		t.Fatalf("overlap %d %v", n, out[:n])
	}
	if n := q.OverlapCircle(mathx.Vec2{}, 20, all, out); n != 2 {
		t.Fatalf("circle overlap %d", n)
	}
	if h, ok := q.BodyAt(mathx.Vec2{Y: 0.1}, all); !ok || h != b {
		t.Fatalf("body at %v %v", h, ok)
	}
}

func TestTeleportMovesBodyAndNode(t *testing.T) {
	f := newFixture(t)
	b, _ := f.crate(mathx.Vec2{})
	f.step(10)
	if err := f.sim.Teleport(b, mathx.Vec2{X: 7, Y: -2}, 0.5); err != nil {
		t.Fatal(err)
	}
	p, a, _ := f.sim.World().BodyTransform(f.handle(b))
	if p != (mathx.Vec2{X: 7, Y: -2}) || a != 0.5 {
		t.Fatalf("body %v %v", p, a)
	}
	wp := f.s.WorldPosition(b)
	if !near(wp.X, 7, 1e-5) || !near(wp.Y, -2, 1e-5) {
		t.Fatalf("node %v", wp)
	}
	f.step(1)
	if f.s.WorldPosition(b).Y <= -2 {
		t.Fatal("body did not keep falling after the teleport")
	}
}
