package integration

import (
	"testing"

	"aego/contracts/scriptevents"
	"aego/core/log"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/module"
	"aego/modules/physics"
	"aego/modules/physics/components"
	"aego/modules/physics/stdlib"
	"aego/modules/physics/systems"
	"aego/modules/physics/world"
	"aego/scene"
	"aego/schema"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/bind/manifest"
	"aego/script/vm"
)

type physEvent struct {
	node scene.NodeHandle
	id   scriptevents.EventID
	args []vm.Value
}

type fakeSink struct {
	got    []physEvent
	listen map[scriptevents.EventID]bool
}

func (s *fakeSink) Emit(node scene.NodeHandle, id scriptevents.EventID, args []vm.Value) {
	s.got = append(s.got, physEvent{node: node, id: id, args: append([]vm.Value(nil), args...)})
}

func (s *fakeSink) Flush() {}

func (s *fakeSink) ListensOn(scene.NodeHandle, scriptevents.EventID) bool {
	return false
}

type listeningSink struct {
	fakeSink
}

func (s *listeningSink) ListensOn(_ scene.NodeHandle, id scriptevents.EventID) bool {
	return s.listen[id]
}

func physicsScene(t *testing.T) *scene.Scene {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if _, err := module.Load(types, nil, physics.New()); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return scene.New(types, nil, scene.Options{})
}

func physBox(s *scene.Scene, name string, at, size mathx.Vec2, typ components.BodyType, body bool) scene.NodeHandle {
	h := s.Create(s.Root(), name)
	s.SetPosition(h, at)
	if body {
		b := scene.Add[components.Body2D](s, h)
		*b = components.DefaultBody2D()
		b.Type = typ
	}
	c := scene.Add[components.BoxCollider2D](s, h)
	*c = components.DefaultBox()
	c.Size = size
	return h
}

func TestPhysicsCollisionEvents(t *testing.T) {
	s := physicsScene(t)
	ground := physBox(s, "Ground", mathx.Vec2{Y: 5}, mathx.Vec2{X: 20, Y: 1}, components.Static, false)
	crate := physBox(s, "Crate", mathx.Vec2{Y: 3}, mathx.Vec2{X: 1, Y: 1}, components.Dynamic, true)
	var mat [32]uint32
	for i := range mat {
		mat[i] = ^uint32(0)
	}
	sim, err := systems.New(s, world.DefaultConfig(), mat, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer sim.Close()
	lib := stdlib.Install(bind.New())
	if lib == nil {
		t.Fatal("stdlib did not install")
	}
	ids := stdlib.EventIDs{Enter: 0, Stay: 1, Exit: 2, TriggerEnter: 3, TriggerExit: 4}
	em := &stdlib.Emitter{IDs: ids, Lib: lib}
	sink := &listeningSink{fakeSink{listen: map[scriptevents.EventID]bool{}}}
	w := engine.NewWorld("play")
	w.SetService(systems.ServiceID, sim)
	w.SetService(scriptevents.ServiceID, scriptevents.Sink(sink))
	ctx := &engine.Context{World: w}
	tick := func() {
		sink.got = sink.got[:0]
		s.Flush()
		sim.Sync()
		sim.Step(1.0 / 60)
		sim.Writeback()
		em.Run(ctx)
	}
	count := func(id scriptevents.EventID) int {
		n := 0
		for _, e := range sink.got {
			if e.id == id {
				n++
			}
		}
		return n
	}
	entered := false
	for i := 0; i < 120 && !entered; i++ {
		tick()
		if count(ids.Enter) == 0 {
			continue
		}
		entered = true
		for _, e := range sink.got {
			if e.id != ids.Enter {
				continue
			}
			c := lib.Contact(e.args[1:])
			other := bind.NodeOf(e.args[0])
			switch e.node {
			case crate:
				if other != ground || c.Normal.Y >= 0 || c.SelfCollider != crate || c.OtherCollider != ground {
					t.Fatalf("crate contact %+v other %v", c, other)
				}
			case ground:
				if other != crate || c.Normal.Y <= 0 {
					t.Fatalf("ground contact %+v", c)
				}
			default:
				t.Fatalf("event for an unexpected node %v", e.node)
			}
		}
		if count(ids.Enter) != 2 {
			t.Fatalf("enter events %d", count(ids.Enter))
		}
	}
	if !entered {
		t.Fatal("crate never touched the ground")
	}
	tick()
	if count(ids.Stay) != 0 {
		t.Fatal("stay emitted without a listener")
	}
	sink.listen[ids.Stay] = true
	tick()
	if count(ids.Stay) != 2 {
		t.Fatalf("stay events %d", count(ids.Stay))
	}
	s.Destroy(crate)
	tick()
	if count(ids.Exit) != 2 {
		t.Fatalf("exit events %d", count(ids.Exit))
	}
}

func TestPhysicsTriggerEvents(t *testing.T) {
	s := physicsScene(t)
	zone := s.Create(s.Root(), "Zone")
	s.SetPosition(zone, mathx.Vec2{Y: 3})
	z := scene.Add[components.BoxCollider2D](s, zone)
	*z = components.DefaultBox()
	z.IsTrigger, z.Size = true, mathx.Vec2{X: 4, Y: 1}
	ball := physBox(s, "Ball", mathx.Vec2{}, mathx.Vec2{X: 0.5, Y: 0.5}, components.Dynamic, true)
	var mat [32]uint32
	for i := range mat {
		mat[i] = ^uint32(0)
	}
	sim, _ := systems.New(s, world.DefaultConfig(), mat, log.Nop())
	defer sim.Close()
	lib := stdlib.Install(bind.New())
	ids := stdlib.EventIDs{Enter: 0, Stay: 1, Exit: 2, TriggerEnter: 3, TriggerExit: 4}
	em := &stdlib.Emitter{IDs: ids, Lib: lib}
	sink := &fakeSink{}
	w := engine.NewWorld("play")
	w.SetService(systems.ServiceID, sim)
	w.SetService(scriptevents.ServiceID, scriptevents.Sink(sink))
	enter, exit := 0, 0
	for i := 0; i < 180; i++ {
		s.Flush()
		sim.Sync()
		sim.Step(1.0 / 60)
		sim.Writeback()
		sink.got = sink.got[:0]
		em.Run(&engine.Context{World: w})
		for _, e := range sink.got {
			switch e.id {
			case ids.TriggerEnter:
				enter++
				if len(e.args) != 1 {
					t.Fatalf("trigger args %d", len(e.args))
				}
			case ids.TriggerExit:
				exit++
			case ids.Enter:
				t.Fatal("trigger produced a collision event")
			}
		}
	}
	if enter != 2 || exit != 2 {
		t.Fatalf("trigger enter %d exit %d", enter, exit)
	}
	_ = ball
}

func TestPhysicsScriptSurface(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	b := bind.New()
	if _, err := module.Load(types, b, physics.New()); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.FromRegistry(b, types.Schema())
	if err != nil {
		t.Fatal(err)
	}
	for _, fn := range []string{"Raycast", "Linecast", "RaycastAll", "OverlapPoint", "OverlapCircle", "OverlapBox", "Gravity", "SetGravity", "Layer", "LayerMask"} {
		if _, ok := m.FuncDoc(stdlib.ModPhysics, fn); !ok {
			t.Errorf("aego/physics.%s is missing", fn)
		}
	}
	for _, st := range []string{"Contact", "RayHit"} {
		if _, ok := m.StructDoc(stdlib.ModPhysics, st); !ok {
			t.Errorf("struct %s is missing", st)
		}
	}
	poly, ok := m.Component(components.NamePolygon)
	if !ok {
		t.Fatal("polygon collider is not visible to scripts")
	}
	f, ok := poly.Field("Points")
	if !ok || !f.ReadOnly || f.Type.Kind != abi.Slice {
		t.Fatalf("Points field %+v %v", f, ok)
	}
	body, _ := m.Component(components.NameBody)
	if v, ok := body.Field("Velocity"); !ok || !v.ReadOnly {
		t.Fatalf("Velocity field %+v %v", v, ok)
	}
}
