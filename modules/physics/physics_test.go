package physics_test

import (
	"testing"

	"aego/core/log"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/module"
	"aego/modules/physics"
	"aego/modules/physics/components"
	"aego/modules/physics/systems"
	"aego/modules/physics/world"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

const dt = float32(1.0 / 60)

func allLayers() [32]uint32 {
	var m [32]uint32
	for i := range m {
		m[i] = ^uint32(0)
	}
	return m
}

type fixture struct {
	t   *testing.T
	s   *scene.Scene
	sim *systems.Sim
}

func newTypes(t *testing.T) *scene.Types {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if _, err := module.Load(types, nil, physics.New()); err != nil {
		t.Fatal(err)
	}
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return types
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	s := scene.New(newTypes(t), nil, scene.Options{})
	sim, err := systems.New(s, world.DefaultConfig(), allLayers(), log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sim.Close)
	return &fixture{t: t, s: s, sim: sim}
}

func (f *fixture) node(parent scene.NodeHandle, name string, at mathx.Vec2) scene.NodeHandle {
	if parent.IsZero() {
		parent = f.s.Root()
	}
	h := f.s.Create(parent, name)
	f.s.SetPosition(h, at)
	return h
}

func (f *fixture) body(h scene.NodeHandle, typ components.BodyType) *components.Body2D {
	b := scene.Add[components.Body2D](f.s, h)
	*b = components.DefaultBody2D()
	b.Type = typ
	return b
}

func (f *fixture) box(h scene.NodeHandle, size mathx.Vec2) {
	c := scene.Add[components.BoxCollider2D](f.s, h)
	*c = components.DefaultBox()
	c.Size = size
}

func (f *fixture) ground() scene.NodeHandle {
	g := f.node(scene.NodeHandle{}, "Ground", mathx.Vec2{Y: 5})
	f.box(g, mathx.Vec2{X: 20, Y: 1})
	return g
}

func (f *fixture) crate(at mathx.Vec2) (scene.NodeHandle, *components.Body2D) {
	h := f.node(scene.NodeHandle{}, "Crate", at)
	b := f.body(h, components.Dynamic)
	f.box(h, mathx.Vec2{X: 1, Y: 1})
	return h, b
}

func (f *fixture) step(n int) {
	for i := 0; i < n; i++ {
		f.s.Flush()
		f.sim.Sync()
		f.sim.Step(dt)
		f.sim.Writeback()
	}
}

func (f *fixture) handle(h scene.NodeHandle) world.BodyHandle {
	f.t.Helper()
	bh, ok := f.sim.BodyOf(h)
	if !ok {
		f.t.Fatalf("node %s has no body", f.s.Name(h))
	}
	return bh
}

func near(a, b, eps float32) bool {
	return mathx.Abs(a-b) <= eps
}

func TestBodyRestsOnImplicitStatic(t *testing.T) {
	f := newFixture(t)
	f.ground()
	b, _ := f.crate(mathx.Vec2{})
	f.step(240)
	if p := f.s.Local(b).Position; !near(p.Y, 4, 0.05) || !near(p.X, 0, 0.01) {
		t.Fatalf("crate at %v", p)
	}
	c, _ := scene.Get[components.Body2D](f.s, b)
	if mathx.Abs(c.Velocity.Y) > 0.05 {
		t.Fatalf("velocity %v", c.Velocity)
	}
	if st := f.sim.Stats(); st.Bodies != 2 {
		t.Fatalf("bodies %d", st.Bodies)
	}
	gt, _ := f.sim.World().BodyType(f.handle(f.s.Find(f.s.Root(), "Ground")))
	if gt != world.Static {
		t.Fatalf("ground body is %v", gt)
	}
}

func TestCompoundMassMatchesWorld(t *testing.T) {
	f := newFixture(t)
	p := f.node(scene.NodeHandle{}, "Body", mathx.Vec2{})
	f.body(p, components.Dynamic)
	f.box(f.node(p, "A", mathx.Vec2{X: 1}), mathx.Vec2{X: 1, Y: 1})
	f.box(f.node(p, "B", mathx.Vec2{X: -1}), mathx.Vec2{X: 1, Y: 2})
	c := scene.Add[components.CircleCollider2D](f.s, f.node(p, "C", mathx.Vec2{Y: 1}))
	*c = components.DefaultCircle()
	f.s.Flush()
	f.sim.Sync()
	mass, inertia, center, _ := f.sim.World().BodyMass(f.handle(p))

	ref, _ := world.New(world.DefaultConfig())
	rb, _ := ref.CreateBody(world.DefaultBodyDef())
	add := func(g world.Geometry, at mathx.Vec2) {
		d := world.DefaultShapeDef(g)
		d.Local = world.Transform{Position: at}
		if _, err := ref.AddShape(rb, d); err != nil {
			t.Fatal(err)
		}
	}
	add(world.Box{Half: mathx.Vec2{X: 0.5, Y: 0.5}}, mathx.Vec2{X: 1})
	add(world.Box{Half: mathx.Vec2{X: 0.5, Y: 1}}, mathx.Vec2{X: -1})
	add(world.Circle{R: 0.5}, mathx.Vec2{Y: 1})
	wm, wi, wc, _ := ref.BodyMass(rb)
	if !near(mass, wm, 1e-4) || !near(inertia, wi, 1e-3) || !near(center.X, wc.X, 1e-4) || !near(center.Y, wc.Y, 1e-4) {
		t.Fatalf("compound %v %v %v, world %v %v %v", mass, inertia, center, wm, wi, wc)
	}
}

func TestStaticFollowsNode(t *testing.T) {
	f := newFixture(t)
	g := f.ground()
	f.step(1)
	f.s.SetPosition(g, mathx.Vec2{X: 3, Y: 5})
	f.step(1)
	if p, _, _ := f.sim.World().BodyTransform(f.handle(g)); p != (mathx.Vec2{X: 3, Y: 5}) {
		t.Fatalf("static body at %v", p)
	}
}

func TestWritebackUnderRotatedParent(t *testing.T) {
	f := newFixture(t)
	pivot := f.node(scene.NodeHandle{}, "Pivot", mathx.Vec2{X: 2})
	f.s.SetRotation(pivot, mathx.Pi/2)
	b := f.node(pivot, "Crate", mathx.Vec2{X: 1})
	f.body(b, components.Dynamic)
	f.box(b, mathx.Vec2{X: 1, Y: 1})
	f.step(30)
	p, a, _ := f.sim.World().BodyTransform(f.handle(b))
	if p.Y < 1.5 {
		t.Fatalf("crate did not fall, at %v", p)
	}
	wp := f.s.WorldPosition(b)
	if !near(wp.X, p.X, 1e-3) || !near(wp.Y, p.Y, 1e-3) {
		t.Fatalf("node at %v, body at %v", wp, p)
	}
	m := f.s.World(b)
	d := mathx.Atan2(m[1], m[0]) - a
	for d > mathx.Pi {
		d -= mathx.TwoPi
	}
	for d < -mathx.Pi {
		d += mathx.TwoPi
	}
	if mathx.Abs(d) > 1e-3 {
		t.Fatalf("node angle differs from body angle by %v", d)
	}
}

func TestInterpolationUsesRenderWorld(t *testing.T) {
	f := newFixture(t)
	b, cfg := f.crate(mathx.Vec2{})
	cfg.Interpolate = true
	child := f.node(b, "Sprite", mathx.Vec2{X: 1})
	plain, _ := f.crate(mathx.Vec2{X: 5})
	f.step(10)
	f.sim.Present(0.5)
	w, r := f.s.World(b), f.s.RenderWorld(b)
	if !(r[7] < w[7]) || w[7]-r[7] > 0.1 {
		t.Fatalf("render y %v, world y %v", r[7], w[7])
	}
	if rc := f.s.RenderWorld(child); !near(rc[7], r[7], 1e-5) || !near(rc[6], r[6]+1, 1e-5) {
		t.Fatalf("child render %v %v, parent render %v %v", rc[6], rc[7], r[6], r[7])
	}
	if f.s.World(child)[7] != w[7] {
		t.Fatal("child world moved with the presentation pose")
	}
	if f.s.RenderWorld(plain) != f.s.World(plain) {
		t.Fatal("body without Interpolate has a presentation pose")
	}
}

func TestPresentKeepsOtherWriters(t *testing.T) {
	f := newFixture(t)
	a, cfg := f.crate(mathx.Vec2{})
	cfg.Interpolate = true
	other := f.node(scene.NodeHandle{}, "Other", mathx.Vec2{X: 5})
	pose := scene.Transform2D{Position: mathx.Vec2{X: 9}, Scale: mathx.Vec2{X: 1, Y: 1}}
	f.s.SetPresentationLocal(other, pose)
	f.step(10)
	f.sim.Present(0.5)
	if got, ok := f.s.PresentationLocal(other); !ok || got != pose {
		t.Fatalf("physics dropped another writer's pose: %v %v", got, ok)
	}
	f.s.Destroy(other)
	f.s.Flush()
	if _, ok := f.s.PresentationLocal(other); ok {
		t.Fatal("destroyed node kept its presentation pose")
	}
	f.sim.Present(0.5)
	w, r := f.s.World(a), f.s.RenderWorld(a)
	if !(r[7] < w[7]) || w[7]-r[7] > 0.1 {
		t.Fatalf("render y %v, world y %v", r[7], w[7])
	}
}

func TestDestroyReportsEnd(t *testing.T) {
	f := newFixture(t)
	f.ground()
	b, _ := f.crate(mathx.Vec2{X: 0, Y: 4})
	f.step(30)
	f.s.Destroy(b)
	if f.s.IsAlive(b) {
		t.Fatal("destroyed node is alive")
	}
	f.step(1)
	ends := 0
	for _, e := range f.sim.World().Events().Contacts {
		if e.Kind == world.EndContact {
			ends++
		}
	}
	if ends != 1 || f.sim.Stats().Bodies != 1 {
		t.Fatalf("end events %d, bodies %d", ends, f.sim.Stats().Bodies)
	}
}

func TestDisabledSubtree(t *testing.T) {
	f := newFixture(t)
	p := f.node(scene.NodeHandle{}, "Body", mathx.Vec2{})
	f.body(p, components.Dynamic)
	f.box(f.node(p, "A", mathx.Vec2{X: 1}), mathx.Vec2{X: 1, Y: 1})
	f.box(p, mathx.Vec2{X: 1, Y: 1})
	f.step(1)
	f.s.SetEnabled(p, false)
	f.step(1)
	w := f.sim.World()
	for _, sh := range w.BodyShapes(f.handle(p)) {
		if w.ShapeEnabled(sh) {
			t.Fatal("shape of a disabled node is enabled")
		}
	}
	f.s.SetEnabled(p, true)
	f.step(1)
	for _, sh := range w.BodyShapes(f.handle(p)) {
		if !w.ShapeEnabled(sh) {
			t.Fatal("shape stayed disabled")
		}
	}
}

func hasDiag(f *fixture, code string) bool {
	ds, _ := f.sim.Diagnostics()
	for _, d := range ds {
		if d.Code == code {
			return true
		}
	}
	return false
}

func TestNonUniformCircleExcludesBody(t *testing.T) {
	f := newFixture(t)
	h := f.node(scene.NodeHandle{}, "Ball", mathx.Vec2{})
	f.body(h, components.Dynamic)
	c := scene.Add[components.CircleCollider2D](f.s, h)
	*c = components.DefaultCircle()
	f.s.SetScale(h, mathx.Vec2{X: 2, Y: 1})
	f.step(10)
	if !hasDiag(f, systems.CodeScale) {
		t.Fatal("no diagnostic for a circle under non-uniform scale")
	}
	if p := f.s.Local(h).Position; p.Y != 0 {
		t.Fatalf("excluded body moved to %v", p)
	}
	f.s.SetScale(h, mathx.Vec2{X: 2, Y: 2})
	f.step(10)
	if hasDiag(f, systems.CodeScale) {
		t.Fatal("diagnostic stayed after the fix")
	}
	if f.s.Local(h).Position.Y <= 0 {
		t.Fatal("body did not resume")
	}
}

func TestReparentKeepsPoseAndVelocity(t *testing.T) {
	f := newFixture(t)
	b, _ := f.crate(mathx.Vec2{})
	f.step(20)
	v0, _, _ := f.sim.World().BodyVelocity(f.handle(b))
	p0 := f.s.WorldPosition(b)
	holder := f.node(scene.NodeHandle{}, "Holder", mathx.Vec2{X: 5, Y: -3})
	if err := f.s.Reparent(b, holder, -1); err != nil {
		t.Fatal(err)
	}
	f.s.Flush()
	f.sim.Sync()
	v1, _, _ := f.sim.World().BodyVelocity(f.handle(b))
	p1 := f.s.WorldPosition(b)
	if !near(v0.Y, v1.Y, 1e-4) || !near(p0.X, p1.X, 1e-4) || !near(p0.Y, p1.Y, 1e-4) {
		t.Fatalf("velocity %v → %v, position %v → %v", v0, v1, p0, p1)
	}
	if !hasDiag(f, systems.CodeReparent) {
		t.Fatal("no reparent diagnostic")
	}
}

func TestDistanceJointHoldsLength(t *testing.T) {
	f := newFixture(t)
	anchor := f.node(scene.NodeHandle{}, "Anchor", mathx.Vec2{})
	f.body(anchor, components.Static)
	f.box(anchor, mathx.Vec2{X: 0.2, Y: 0.2})
	bob, _ := f.crate(mathx.Vec2{X: 2})
	j := scene.Add[components.DistanceJoint2D](f.s, bob)
	*j = components.DefaultDistance()
	j.Target.SetNodeRefID(f.s.ID(anchor))
	f.step(120)
	if d := f.s.WorldPosition(bob).Len(); !near(d, 2, 0.05) {
		t.Fatalf("distance %v", d)
	}
	if f.sim.Stats().Joints != 1 {
		t.Fatal("joint is missing")
	}
}

func TestSceneWithoutPhysicsKeepsComponents(t *testing.T) {
	f := newFixture(t)
	f.crate(mathx.Vec2{X: 1})
	f.s.Flush()
	data := sceneio.Save(f.s)
	plain := scene.NewTypes(schema.NewRegistry())
	if err := sceneio.RegisterUnknown(plain); err != nil {
		t.Fatal(err)
	}
	if err := plain.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s, rep, err := sceneio.Load(data, "t", plain, nil, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Unknown) != 2 {
		t.Fatalf("unknown %v", rep.Unknown)
	}
	if got := sceneio.Save(s); string(got) != string(data) {
		t.Fatalf("round trip differs:\n%s\n---\n%s", data, got)
	}
}

func TestAttachOnlyInPlay(t *testing.T) {
	f := newFixture(t)
	m := physics.New()
	w := engine.NewWorld("edit")
	edit := &module.AttachContext{World: w, Scene: f.s, Editor: true}
	if err := m.AttachWorld(edit); err != nil || physics.SimOf(w) != nil {
		t.Fatal("edit world got a physics world")
	}
	m.DetachWorld(edit)
	play := &module.AttachContext{World: w, Scene: f.s, Editor: true, Play: true}
	if err := m.AttachWorld(play); err != nil || physics.SimOf(w) == nil {
		t.Fatalf("play world has no physics: %v", err)
	}
	m.DetachWorld(play)
	if physics.SimOf(w) != nil {
		t.Fatal("detach left the service")
	}
}

func TestStepAllocatesNothing(t *testing.T) {
	f := newFixture(t)
	f.ground()
	for i := 0; i < 20; i++ {
		f.crate(mathx.Vec2{X: float32(i%5) - 2, Y: float32(-i)})
	}
	f.step(120)
	if n := testing.AllocsPerRun(30, func() { f.step(1) }); n != 0 {
		t.Fatalf("%v allocations per step", n)
	}
}
