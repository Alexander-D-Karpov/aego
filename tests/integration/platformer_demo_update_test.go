package integration

import (
	"testing"

	"aego/core/mathx"
	"aego/modules/gameplay"
	"aego/modules/particles"
	"aego/modules/render2d"
	"aego/scene"
)

func TestPlatformerSwordSparksAndFacing(t *testing.T) {
	for _, direction := range []float32{-1, 1} {
		t.Run(map[float32]string{-1: "left", 1: "right"}[direction], func(t *testing.T) {
			pf := openPlatformer(t)
			pf.step(30)
			sc := pf.app.Scene
			slash := sc.Find(pf.player, "Slash")
			sparks := sc.Find(slash, "Sparks")
			if slash.IsZero() || sparks.IsZero() {
				t.Fatal("player is missing its sword or spark emitter")
			}
			fx := particles.Of(pf.app.Engine.MainWorld())
			if fx == nil {
				t.Fatal("particle service is missing")
			}
			in := actionsOf(t, pf)
			if err := in.Inject(0, "Move", direction); err != nil {
				t.Fatal(err)
			}
			pf.step(2)
			if err := in.Inject(0, "Move", 0); err != nil {
				t.Fatal(err)
			}
			if err := in.Inject(0, "Attack", 1); err != nil {
				t.Fatal(err)
			}
			pf.step(1)
			if err := in.Inject(0, "Attack", 0); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 60 && fx.Emitted(sparks) == 0; i++ {
				pf.step(1)
			}
			if got := fx.Emitted(sparks); got != 8 {
				t.Fatalf("one sword attack emitted %d sparks, want 8", got)
			}
			if x := sc.Local(slash).Position.X; mathx.Abs(x-direction*0.7) > 0.001 {
				t.Fatalf("sword x=%v does not match facing %v", x, direction)
			}
			pf.noFaults(t)
		})
	}
}

func TestPlatformerNonlethalStompBounces(t *testing.T) {
	pf := openPlatformer(t)
	pf.step(30)
	sc := pf.app.Scene
	health, ok := scene.Get[gameplay.Health](sc, pf.enemy)
	if !ok {
		t.Fatal("enemy has no health")
	}
	health.Max, health.Current = 3, 3
	e := sc.Local(pf.enemy).Position
	sc.SetPosition(pf.player, mathx.Vec2{X: e.X, Y: e.Y - 2.5})
	for i := 0; i < 120 && health.Current == 3; i++ {
		e = sc.Local(pf.enemy).Position
		p := sc.Local(pf.player).Position
		sc.SetPosition(pf.player, mathx.Vec2{X: e.X, Y: p.Y})
		pf.step(1)
	}
	if health.Current != 2 || health.Dead || !sc.IsAlive(pf.enemy) {
		t.Fatalf("stomp should damage but not kill the enemy: %+v", health)
	}
	// AddVelocity is queued by Hit and consumed on the next fixed update.
	pf.step(1)
	if m := pf.motion(); m.Velocity.Y >= 0 {
		t.Fatalf("nonlethal stomp did not bounce the player upward: %v", m.Velocity)
	}
	if got := pf.field("kills").Int(); got != 0 {
		t.Fatalf("nonlethal stomp counted as %d kills", got)
	}
	pf.noFaults(t)
}

func TestPlatformerFoundryBackdropResolves(t *testing.T) {
	pf := openPlatformer(t)
	pf.step(3)
	sc := pf.app.Scene
	id, ok := pf.app.Pipeline.Lookup("assets/sprites/foundry_backdrop.png")
	if !ok {
		t.Fatal("foundry backdrop is missing from the asset pipeline")
	}
	for _, name := range []string{"FoundrySky1", "FoundrySky2", "FoundrySky3", "FoundrySky4", "FoundrySky5", "FoundrySky6", "FoundrySky7", "FoundrySky8"} {
		node := sc.Find(sc.Root(), name)
		sprite, ok := scene.Get[render2d.SpriteRenderer](sc, node)
		if !ok || sprite.Sprite.ID != id {
			t.Fatalf("%s does not reference the foundry backdrop", name)
		}
	}
	if stats := pf.app.Runner.Stats(); stats.MissingAssets != 0 {
		t.Fatalf("demo has missing assets: %+v", stats)
	}
	pf.noFaults(t)
}
