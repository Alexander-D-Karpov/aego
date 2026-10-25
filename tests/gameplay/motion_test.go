package gameplay_test

import (
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/core/log"
	"aego/core/mathx"
	"aego/core/rewind"
	"aego/engine/module"
	"aego/modules/gameplay"
	"aego/modules/physics/components"
	"aego/modules/physics/systems"
	"aego/modules/physics/world"
	"aego/scene"
	"aego/script/bind"
)

const dt = float32(1.0 / 60)

type rig struct {
	t   testing.TB
	man *module.Manifest
	s   *scene.Scene
	sim *systems.Sim
	svc *gameplay.Service
}

func newRig(t testing.TB) *rig {
	t.Helper()
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	man, err := builtin.Default().Load(types, bind.New(), assembly.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	var m [32]uint32
	for i := range m {
		m[i] = ^uint32(0)
	}
	sim, err := systems.New(s, world.DefaultConfig(), m, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sim.Close)
	return &rig{t: t, man: man, s: s, sim: sim, svc: gameplay.NewService(s, nil, sim)}
}

func (r *rig) solid(p, size mathx.Vec2, rot float32) scene.NodeHandle {
	h := r.s.Create(r.s.Root(), "solid")
	l := r.s.Local(h)
	l.Position, l.Rotation = p, rot
	r.s.SetLocal(h, l)
	b := components.DefaultBody2D()
	b.Type = components.Static
	*scene.Add[components.Body2D](r.s, h) = b
	c := components.DefaultBox()
	c.Size = size
	*scene.Add[components.BoxCollider2D](r.s, h) = c
	return h
}

func (r *rig) hero(p mathx.Vec2, mode gameplay.Mode) scene.NodeHandle {
	h := r.s.Create(r.s.Root(), "hero")
	r.s.SetPosition(h, p)
	b := components.DefaultBody2D()
	b.Type, b.FixedRotation = components.Kinematic, true
	*scene.Add[components.Body2D](r.s, h) = b
	c := components.DefaultBox()
	c.Size = mathx.Vec2{X: 0.5, Y: 1}
	*scene.Add[components.BoxCollider2D](r.s, h) = c
	m := gameplay.DefaultMotion()
	m.Mode = mode
	*scene.Add[gameplay.Motion2D](r.s, h) = m
	return h
}

func (r *rig) motion(h scene.NodeHandle) *gameplay.Motion2D {
	m, ok := scene.Get[gameplay.Motion2D](r.s, h)
	if !ok {
		r.t.Fatal("no Motion2D")
	}
	return m
}

func (r *rig) pos(h scene.NodeHandle) mathx.Vec2 {
	return r.s.WorldPosition(h)
}

func (r *rig) tick() {
	r.s.Flush()
	r.s.UpdateTransforms()
	r.svc.Fixed(dt)
	r.sim.Sync()
	r.sim.Step(dt)
	r.sim.Writeback()
	r.s.UpdateTransforms()
	r.svc.Post()
}

func (r *rig) ticks(n int) {
	for range n {
		r.tick()
	}
}

func floorRig(t testing.TB) *rig {
	r := newRig(t)
	r.solid(mathx.Vec2{Y: 5}, mathx.Vec2{X: 60, Y: 1}, 0)
	return r
}

func TestFallsAndLands(t *testing.T) {
	r := floorRig(t)
	h := r.hero(mathx.Vec2{Y: 2}, gameplay.Platformer)
	r.ticks(120)
	if m := r.motion(h); !m.Grounded {
		t.Fatal("not grounded after falling")
	}
	if y := r.pos(h).Y; y < 3.9 || y > 4.0 {
		t.Fatalf("rests at y=%v, want just above 4", y)
	}
}

func TestStopsAtWall(t *testing.T) {
	r := floorRig(t)
	r.solid(mathx.Vec2{X: 3, Y: 3}, mathx.Vec2{X: 1, Y: 4}, 0)
	h := r.hero(mathx.Vec2{Y: 4}, gameplay.Platformer)
	r.ticks(30)
	r.motion(h).Move(1)
	r.ticks(120)
	x := r.pos(h).X
	if x > 2.25 || x < 2.1 {
		t.Fatalf("x=%v, the wall face is at 2.5 minus half width 0.25", x)
	}
	if m := r.motion(h); !m.OnWall || !m.Grounded {
		t.Fatalf("onWall=%v grounded=%v", m.OnWall, m.Grounded)
	}
}

func TestJumpHeight(t *testing.T) {
	r := floorRig(t)
	h := r.hero(mathx.Vec2{Y: 3.5}, gameplay.Platformer)
	r.ticks(60)
	y0 := r.pos(h).Y
	r.motion(h).Jump()
	top := y0
	for range 120 {
		r.tick()
		top = min(top, r.pos(h).Y)
	}
	want := gameplay.DefaultMotion().JumpHeight
	if rise := y0 - top; rise < want*0.95 || rise > want*1.05 {
		t.Fatalf("jump rose %v, want %v", rise, want)
	}
	if !r.motion(h).Grounded {
		t.Fatal("not grounded after the jump")
	}
}

func TestSlopeHolds(t *testing.T) {
	r := newRig(t)
	r.solid(mathx.Vec2{Y: 5}, mathx.Vec2{X: 30, Y: 1}, -mathx.Deg2Rad(30))
	h := r.hero(mathx.Vec2{Y: 2}, gameplay.Platformer)
	r.ticks(120)
	if !r.motion(h).Grounded {
		t.Fatal("not grounded on a 30 degree slope")
	}
	p0 := r.pos(h)
	r.ticks(120)
	if d := r.pos(h).Sub(p0).Len(); d > 0.01 {
		t.Fatalf("slid %v on a 30 degree slope", d)
	}
}

func TestSteepSlopeIsWall(t *testing.T) {
	r := floorRig(t)
	r.solid(mathx.Vec2{X: 4, Y: 3}, mathx.Vec2{X: 1, Y: 6}, -mathx.Deg2Rad(30))
	h := r.hero(mathx.Vec2{Y: 4}, gameplay.Platformer)
	r.ticks(30)
	r.motion(h).Move(1)
	r.ticks(120)
	if y := r.pos(h).Y; y < 3.8 {
		t.Fatalf("climbed a 60 degree slope to y=%v", y)
	}
}

func TestStepUp(t *testing.T) {
	r := floorRig(t)
	r.solid(mathx.Vec2{X: 3, Y: 4.4}, mathx.Vec2{X: 2, Y: 0.2}, 0)
	h := r.hero(mathx.Vec2{Y: 4}, gameplay.Platformer)
	r.motion(h).StepHeight = 0.3
	r.ticks(30)
	r.motion(h).Move(1)
	r.ticks(90)
	if x := r.pos(h).X; x < 4 {
		t.Fatalf("stuck at x=%v below a 0.2 step", x)
	}

	r2 := floorRig(t)
	r2.solid(mathx.Vec2{X: 3, Y: 4.2}, mathx.Vec2{X: 2, Y: 0.6}, 0)
	h2 := r2.hero(mathx.Vec2{Y: 4}, gameplay.Platformer)
	r2.motion(h2).StepHeight = 0.3
	r2.ticks(30)
	r2.motion(h2).Move(1)
	r2.ticks(90)
	if x := r2.pos(h2).X; x > 2 {
		t.Fatalf("climbed a 0.6 block to x=%v", x)
	}
}

func TestCoyoteJump(t *testing.T) {
	r := newRig(t)
	r.solid(mathx.Vec2{Y: 5}, mathx.Vec2{X: 4, Y: 1}, 0)
	h := r.hero(mathx.Vec2{Y: 4}, gameplay.Platformer)
	r.ticks(30)
	m := r.motion(h)
	m.Move(1)
	for range 120 {
		r.tick()
		if !r.motion(h).Grounded {
			break
		}
	}
	r.tick()
	r.motion(h).Jump()
	y := r.pos(h).Y
	r.ticks(10)
	if r.pos(h).Y >= y {
		t.Fatal("a jump right after the ledge did not lift the character")
	}
}

func TestTopDownSlidesAlongWall(t *testing.T) {
	r := newRig(t)
	r.solid(mathx.Vec2{X: 3}, mathx.Vec2{X: 1, Y: 40}, 0)
	h := r.hero(mathx.Vec2{}, gameplay.TopDown)
	r.motion(h).MoveDir(mathx.Vec2{X: 1, Y: 1})
	r.ticks(120)
	p := r.pos(h)
	if p.X > 2.25 || p.Y < 3 {
		t.Fatalf("top-down at %v, want stopped at the wall and moving along it", p)
	}
}

func TestZeroAllocs(t *testing.T) {
	r := floorRig(t)
	var hs []scene.NodeHandle
	for i := range 100 {
		hs = append(hs, r.hero(mathx.Vec2{X: float32(i%25)*2 - 25, Y: 3 + float32(i/25)*-2}, gameplay.Platformer))
	}
	r.ticks(60)
	for i, h := range hs {
		r.motion(h).Move(float32(i%3 - 1))
	}
	r.ticks(10)
	if a := testing.AllocsPerRun(50, r.tick); a != 0 {
		t.Fatalf("%v allocations per tick", a)
	}
}

func BenchmarkMotion100(b *testing.B) {
	r := floorRig(b)
	for i := range 100 {
		h := r.hero(mathx.Vec2{X: float32(i%25)*2 - 25, Y: 3 + float32(i/25)*-2}, gameplay.Platformer)
		r.motion(h).Move(float32(i%3 - 1))
	}
	r.ticks(60)
	b.ReportAllocs()
	for b.Loop() {
		r.tick()
	}
}

func TestHashSeesControllerState(t *testing.T) {
	r := floorRig(t)
	h := r.hero(mathx.Vec2{Y: 3.5}, gameplay.Platformer)
	r.ticks(30)
	sum := func() [32]byte {
		hs := rewind.NewHasher()
		r.svc.Hash(hs)
		return hs.Sum()
	}
	a := sum()
	r.motion(h).Jump()
	if sum() == a {
		t.Fatal("a pending jump does not change the hash")
	}
	b := sum()
	r.motion(h).AddDisplacement(mathx.Vec2{X: 0.5})
	if sum() == b {
		t.Fatal("a pending displacement does not change the hash")
	}
	if n := testing.AllocsPerRun(20, func() { r.svc.Capture(nil) }); n != 0 {
		t.Fatalf("%v allocations per capture", n)
	}
}

func TestDisplacementMovesThroughCollisions(t *testing.T) {
	r := floorRig(t)
	r.solid(mathx.Vec2{X: 3, Y: 3}, mathx.Vec2{X: 1, Y: 4}, 0)
	h := r.hero(mathx.Vec2{Y: 4}, gameplay.Platformer)
	r.ticks(30)
	x0 := r.pos(h).X
	for range 20 {
		if !r.svc.AddDisplacement(h, mathx.Vec2{X: 0.25}) {
			t.Fatal("the hero has Motion2D")
		}
		r.tick()
	}
	x := r.pos(h).X
	if x <= x0+1 || x < 2.1 || x > 2.25 {
		t.Fatalf("x=%v after pushing into the wall at 2.5 minus half width", x)
	}
	m := r.motion(h)
	if v := m.Velocity.X; v > 1e-3 || v < -1e-3 {
		t.Fatalf("displacement changed the velocity: %v", v)
	}
	if !m.Grounded {
		t.Fatal("the hero must stay on the floor")
	}
	r.ticks(10)
	if d := r.pos(h).X - x; d > 1e-3 || d < -1e-3 {
		t.Fatalf("the hero kept moving after the last displacement: %v", d)
	}
}

func TestDisplacementNeedsMotion(t *testing.T) {
	r := newRig(t)
	plain := r.s.Create(r.s.Root(), "plain")
	r.s.Flush()
	if r.svc.AddDisplacement(plain, mathx.Vec2{X: 1}) {
		t.Fatal("a node without Motion2D has no receiver")
	}
}

func TestDisabledDropsDisplacement(t *testing.T) {
	r := floorRig(t)
	h := r.hero(mathx.Vec2{Y: 4}, gameplay.Platformer)
	r.ticks(30)
	x0 := r.pos(h).X
	r.s.SetEnabled(h, false)
	r.svc.AddDisplacement(h, mathx.Vec2{X: 2})
	r.ticks(2)
	r.s.SetEnabled(h, true)
	r.ticks(2)
	if d := r.pos(h).X - x0; d > 1e-3 || d < -1e-3 {
		t.Fatalf("a displacement given while disabled moved the hero by %v", d)
	}
}

func TestHashSeesDisabledControllers(t *testing.T) {
	r := floorRig(t)
	h := r.hero(mathx.Vec2{Y: 3.5}, gameplay.Platformer)
	r.ticks(30)
	r.s.SetEnabled(h, false)
	r.tick()
	sum := func() [32]byte {
		hs := rewind.NewHasher()
		r.svc.Hash(hs)
		return hs.Sum()
	}
	a := sum()
	r.motion(h).Jump()
	if sum() == a {
		t.Fatal("a pending jump of a disabled controller does not change the hash")
	}
}

func TestWalksWhenCloserThanTheMargin(t *testing.T) {
	r := floorRig(t)
	h := r.hero(mathx.Vec2{Y: 3.999}, gameplay.Platformer)
	r.ticks(2)
	r.s.SetPosition(h, mathx.Vec2{Y: 3.999})
	r.motion(h).Teleport(mathx.Vec2{Y: 3.999})
	r.ticks(2)
	x := r.pos(h).X
	r.motion(h).Move(1)
	r.ticks(30)
	if d := r.pos(h).X - x; d < 1 {
		t.Fatalf("moved %v in half a second resting %v above the floor", d, 4.5-0.5-r.pos(h).Y)
	}
}

func TestJumpsAlongATouchedWall(t *testing.T) {
	r := floorRig(t)
	r.solid(mathx.Vec2{X: 1, Y: 2}, mathx.Vec2{X: 1, Y: 6}, 0)
	h := r.hero(mathx.Vec2{X: 0.25 - 0.001, Y: 4}, gameplay.Platformer)
	r.ticks(10)
	r.motion(h).Move(1)
	r.ticks(10)
	y := r.pos(h).Y
	t.Logf("before jump %v grounded=%v wall=%v", r.pos(h), r.motion(h).Grounded, r.motion(h).OnWall)
	r.motion(h).Jump()
	r.ticks(10)
	if d := y - r.pos(h).Y; d < 0.5 {
		t.Fatalf("rose %v pressed against a wall", d)
	}
}

func TestRespawnResetVelocity(t *testing.T) {
	for _, reset := range []bool{true, false} {
		r := floorRig(t)
		h := r.hero(mathx.Vec2{Y: 4}, gameplay.Platformer)
		rs := gameplay.DefaultRespawn()
		rs.Delay, rs.ResetVelocity = 0, reset
		*scene.Add[gameplay.Respawn](r.s, h) = rs
		r.ticks(20)
		m := r.motion(h)
		m.AddVelocity(mathx.Vec2{X: 30})
		r.tick()
		c, _ := scene.Get[gameplay.Respawn](r.s, h)
		c.Trigger()
		r.tick()
		r.tick()
		if v := r.motion(h).Velocity.X; reset && v > 1 || !reset && v < 1 {
			t.Fatalf("resetVelocity=%v: velocity %v after respawn", reset, v)
		}
	}
}

func TestRespawnWaitsForDelay(t *testing.T) {
	r := floorRig(t)
	h := r.hero(mathx.Vec2{Y: 4}, gameplay.Platformer)
	rs := gameplay.DefaultRespawn()
	rs.Delay = 0.5
	*scene.Add[gameplay.Respawn](r.s, h) = rs
	r.ticks(20)
	start := r.pos(h)
	c, _ := scene.Get[gameplay.Respawn](r.s, h)
	c.Trigger()
	r.s.SetPosition(h, mathx.Vec2{X: 5, Y: 4})
	r.motion(h).Teleport(mathx.Vec2{X: 5, Y: 4})
	r.ticks(int(0.4 / dt))
	if r.pos(h).X < 4 {
		t.Fatal("respawned before Delay")
	}
	r.ticks(int(0.2/dt) + 1)
	if d := r.pos(h).Sub(start).Len(); d > 0.1 {
		t.Fatalf("%v from the start after Delay", d)
	}
}
