package gameplay_test

import (
	"strings"
	"testing"

	"aego/core/mathx"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/modules/gameplay"
	"aego/modules/physics/components"
	"aego/scene"
)

func (r *rig) node(p mathx.Vec2) scene.NodeHandle {
	h := r.s.Create(r.s.Root(), "n")
	r.s.SetPosition(h, p)
	return h
}

func (r *rig) target(p mathx.Vec2, team, max int32) scene.NodeHandle {
	h := r.node(p)
	hp := gameplay.DefaultHealth()
	hp.Max, hp.Team, hp.InvulnTime = max, team, 0
	*scene.Add[gameplay.Health](r.s, h) = hp
	*scene.Add[gameplay.Hurtbox2D](r.s, h) = gameplay.DefaultHurtbox()
	return h
}

func (r *rig) hitbox(p mathx.Vec2, pol gameplay.Policy, team, dmg int32) scene.NodeHandle {
	h := r.node(p)
	hb := gameplay.DefaultHitbox()
	hb.Policy, hb.Period, hb.Team, hb.Damage = pol, 1.5*dt, team, dmg
	*scene.Add[gameplay.Hitbox2D](r.s, h) = hb
	return h
}

func (r *rig) health(h scene.NodeHandle) *gameplay.Health {
	c, ok := scene.Get[gameplay.Health](r.s, h)
	if !ok {
		r.t.Fatal("no Health")
	}
	return c
}

func TestHitPolicies(t *testing.T) {
	for _, tc := range []struct {
		pol  gameplay.Policy
		want int32
	}{{gameplay.OnEnter, 1}, {gameplay.OncePerActivation, 1}, {gameplay.Periodic, 3}} {
		r := newRig(t)
		tg := r.target(mathx.Vec2{}, 0, 10)
		hb := r.hitbox(mathx.Vec2{X: 0.3}, tc.pol, 1, 1)
		r.ticks(5)
		if got := 10 - r.health(tg).Current; got != tc.want {
			t.Fatalf("policy %d: %d hits in 5 overlapping steps, want %d", tc.pol, got, tc.want)
		}
		if tc.pol == gameplay.OncePerActivation {
			c, _ := scene.Get[gameplay.Hitbox2D](r.s, hb)
			c.Activate()
			r.tick()
			if got := 10 - r.health(tg).Current; got != 2 {
				t.Fatalf("Activate must allow a second hit, got %d", got)
			}
		}
	}
}

func TestOnEnterHitsAgainAfterLeaving(t *testing.T) {
	r := newRig(t)
	tg := r.target(mathx.Vec2{}, 0, 10)
	r.hitbox(mathx.Vec2{}, gameplay.OnEnter, 1, 1)
	r.ticks(2)
	r.s.SetPosition(tg, mathx.Vec2{X: 5})
	r.ticks(2)
	r.s.SetPosition(tg, mathx.Vec2{})
	r.ticks(2)
	if got := 10 - r.health(tg).Current; got != 2 {
		t.Fatalf("%d hits, want 2", got)
	}
}

func TestTeams(t *testing.T) {
	r := newRig(t)
	tg := r.target(mathx.Vec2{}, 0, 10)
	hb := r.hitbox(mathx.Vec2{}, gameplay.OnEnter, 0, 1)
	r.ticks(2)
	if r.health(tg).Current != 10 {
		t.Fatal("same team hit without friendly fire")
	}
	c, _ := scene.Get[gameplay.Hitbox2D](r.s, hb)
	c.FriendlyFire = true
	c.Activate()
	r.s.SetPosition(tg, mathx.Vec2{X: 5})
	r.tick()
	r.s.SetPosition(tg, mathx.Vec2{})
	r.tick()
	if r.health(tg).Current != 9 {
		t.Fatal("friendly fire did not hit")
	}
}

func TestSimultaneousHitsDieOnce(t *testing.T) {
	r := newRig(t)
	tg := r.target(mathx.Vec2{}, 0, 4)
	r.health(tg).InvulnTime = 1
	r.hitbox(mathx.Vec2{}, gameplay.OncePerActivation, 1, 2)
	r.hitbox(mathx.Vec2{X: 0.2}, gameplay.OncePerActivation, 1, 2)
	r.ticks(3)
	if hp := r.health(tg); !hp.Dead || hp.Current != 0 {
		t.Fatalf("dead=%v current=%d", hp.Dead, hp.Current)
	}
	if st := r.svc.Stats(); st.Deaths != 1 || st.Hits != 2 {
		t.Fatalf("stats %+v, want two hits in one step and one death", st)
	}
}

func TestInvulnerability(t *testing.T) {
	r := newRig(t)
	tg := r.target(mathx.Vec2{}, 0, 10)
	r.health(tg).InvulnTime = 0.1
	r.hitbox(mathx.Vec2{}, gameplay.Periodic, 1, 1)
	r.ticks(5)
	if got := 10 - r.health(tg).Current; got != 1 {
		t.Fatalf("%d hits within 0.1 s of invulnerability", got)
	}
	r.ticks(10)
	if got := 10 - r.health(tg).Current; got < 2 || got > 3 {
		t.Fatalf("%d hits after invulnerability ended", got)
	}
}

func TestDamageRejectsZeroAndNegative(t *testing.T) {
	r := newRig(t)
	tg := r.target(mathx.Vec2{}, 0, 5)
	hp := r.health(tg)
	if hp.Damage(0, scene.NodeHandle{}) || hp.Damage(-2, scene.NodeHandle{}) || hp.Current != 5 {
		t.Fatal("Damage must ignore amounts below 1")
	}
}

func TestKnockbackMovesMotion(t *testing.T) {
	r := floorRig(t)
	h := r.hero(mathx.Vec2{Y: 4}, gameplay.Platformer)
	*scene.Add[gameplay.Health](r.s, h) = gameplay.DefaultHealth()
	*scene.Add[gameplay.Hurtbox2D](r.s, h) = gameplay.DefaultHurtbox()
	r.ticks(30)
	x0 := r.pos(h).X
	hb := r.hitbox(mathx.Vec2{X: x0 - 0.3, Y: 4}, gameplay.OncePerActivation, 1, 1)
	c, _ := scene.Get[gameplay.Hitbox2D](r.s, hb)
	c.Knockback = mathx.Vec2{X: 8}
	r.ticks(10)
	if r.pos(h).X-x0 < 0.2 {
		t.Fatalf("knockback moved the character %v", r.pos(h).X-x0)
	}
}

func TestKillZoneRespawns(t *testing.T) {
	r := floorRig(t)
	h := r.hero(mathx.Vec2{Y: 4}, gameplay.Platformer)
	hp := gameplay.DefaultHealth()
	hp.InvulnTime = 0
	*scene.Add[gameplay.Health](r.s, h) = hp
	rs := gameplay.DefaultRespawn()
	rs.Delay = 0.1
	*scene.Add[gameplay.Respawn](r.s, h) = rs
	kz := r.node(mathx.Vec2{X: 4, Y: 4})
	z := gameplay.DefaultKillZone()
	z.Area = mathx.Rect{X: -0.5, Y: -2, W: 1, H: 4}
	*scene.Add[gameplay.KillZone2D](r.s, kz) = z
	r.ticks(30)
	start := r.pos(h)
	r.motion(h).Move(1)
	for range 120 {
		r.tick()
		if r.health(h).Dead {
			break
		}
	}
	if !r.health(h).Dead {
		t.Fatal("the kill zone did not kill")
	}
	r.motion(h).Move(0)
	r.ticks(20)
	if d := r.pos(h).Sub(start).Len(); d > 0.1 || r.health(h).Current != r.health(h).Max {
		t.Fatalf("respawned %v from the start, hp %d", d, r.health(h).Current)
	}
}

func TestCheckpointPriority(t *testing.T) {
	r := newRig(t)
	who := r.node(mathx.Vec2{X: -10})
	*scene.Add[gameplay.Respawn](r.s, who) = gameplay.DefaultRespawn()
	cp := func(x float32, prio int32) {
		c := gameplay.DefaultCheckpoint()
		c.Priority = prio
		*scene.Add[gameplay.Checkpoint2D](r.s, r.node(mathx.Vec2{X: x})) = c
	}
	cp(0, 1)
	cp(10, 0)
	cp(20, 5)
	r.tick()
	for _, x := range []float32{0, 10} {
		r.s.SetPosition(who, mathx.Vec2{X: x})
		r.tick()
	}
	rc, _ := scene.Get[gameplay.Respawn](r.s, who)
	rc.Trigger()
	r.s.SetPosition(who, mathx.Vec2{X: -10})
	r.ticks(40)
	if x := r.pos(who).X; x != 0 {
		t.Fatalf("respawned at x=%v, the higher priority checkpoint is at 0", x)
	}
	r.s.SetPosition(who, mathx.Vec2{X: 20})
	r.tick()
	rc, _ = scene.Get[gameplay.Respawn](r.s, who)
	rc.Trigger()
	r.s.SetPosition(who, mathx.Vec2{X: -10})
	r.ticks(40)
	if x := r.pos(who).X; x != 20 {
		t.Fatalf("respawned at x=%v, want 20", x)
	}
}

func TestPickupOnce(t *testing.T) {
	r := floorRig(t)
	r.hero(mathx.Vec2{Y: 4}, gameplay.Platformer)
	p := r.node(mathx.Vec2{Y: 4})
	*scene.Add[gameplay.Pickup2D](r.s, p) = gameplay.DefaultPickup()
	r.ticks(3)
	if r.s.IsAlive(p) || r.svc.Stats().Pickups != 1 {
		t.Fatalf("pickup alive=%v collected=%d", r.s.IsAlive(p), r.svc.Stats().Pickups)
	}
}

func TestLifetime(t *testing.T) {
	r := newRig(t)
	h := r.node(mathx.Vec2{})
	l := gameplay.DefaultLifetime()
	l.Seconds = 0.1
	*scene.Add[gameplay.Lifetime](r.s, h) = l
	r.s.Flush()
	r.s.UpdateTransforms()
	r.svc.Frame(0.06, 0.06)
	if !r.s.IsAlive(h) {
		t.Fatal("removed too early")
	}
	r.svc.Frame(0.06, 0.06)
	if r.s.IsAlive(h) {
		t.Fatal("still alive after its lifetime")
	}
}

func TestNotKinematicBlocksPlay(t *testing.T) {
	r := floorRig(t)
	h := r.hero(mathx.Vec2{Y: 4}, gameplay.Platformer)
	r.s.Flush()
	codes := func() []string {
		var out []string
		for _, d := range module.ValidateScene(r.s, nil) {
			if d.Code == gameplay.CodeNotKinematic && d.BlocksPlay {
				out = append(out, d.Code)
			}
		}
		return out
	}
	if c := codes(); len(c) != 0 {
		t.Fatalf("kinematic body reported %v", c)
	}
	b, _ := scene.Get[components.Body2D](r.s, h)
	b.Type = components.Dynamic
	if c := codes(); len(c) != 1 {
		t.Fatalf("dynamic body under Motion 2D reported %v, want GMP001", c)
	}
	td, _ := r.s.Registry().ByName(gameplay.NameMotion)
	if !module.RunOnAdd(module.AddContext{Scene: r.s, Node: h}, td) || b.Type != components.Kinematic {
		t.Fatalf("adding Motion 2D left the body %v", b.Type)
	}
}

func TestTemplatesBuild(t *testing.T) {
	r := newRig(t)
	n := 0
	for _, td := range r.man.Templates {
		if !strings.HasPrefix(td.ID, gameplay.ID+".") {
			continue
		}
		setup, ok := td.Setup.(func(*scene.Scene, scene.NodeHandle))
		if !ok {
			t.Fatalf("%s has no setup", td.ID)
		}
		setup(r.s, r.s.Create(r.s.Root(), td.Name))
		n++
	}
	r.s.Flush()
	if n != 7 {
		t.Fatalf("%d gameplay templates, want 7", n)
	}
	for _, d := range module.ValidateScene(r.s, nil) {
		if d.Severity == problems.Error {
			t.Errorf("template node: %s %s", d.Code, d.Message)
		}
	}
	r.ticks(30)
}

func TestCombatZeroAllocs(t *testing.T) {
	r := floorRig(t)
	for i := range 20 {
		x := float32(i)*3 - 30
		tg := r.target(mathx.Vec2{X: x, Y: 4}, 0, 1000)
		r.health(tg).InvulnTime = 0
		r.hitbox(mathx.Vec2{X: x + 0.2, Y: 4}, gameplay.Periodic, 1, 1)
		p := r.node(mathx.Vec2{X: x, Y: -20})
		*scene.Add[gameplay.Pickup2D](r.s, p) = gameplay.DefaultPickup()
	}
	kz := r.node(mathx.Vec2{Y: 30})
	*scene.Add[gameplay.KillZone2D](r.s, kz) = gameplay.DefaultKillZone()
	r.ticks(30)
	if a := testing.AllocsPerRun(50, r.tick); a != 0 {
		t.Fatalf("%v allocations per tick", a)
	}
}
