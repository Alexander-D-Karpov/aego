package integration

import (
	"testing"

	"aego/core/mathx"
	"aego/modules/animation"
	"aego/modules/gameplay"
	"aego/platform/input"
	"aego/scene"
)

const (
	platformTop  = 1.05
	platformLeft = 3.0
	feetBelow    = 0.5
)

func besideLedge(t *testing.T, below float32, climb bool) *platformer {
	t.Helper()
	pf := openPlatformer(t)
	pf.step(5)
	if !climb {
		pf.motion().LedgeClimb = 0
	}
	pf.app.Scene.SetPosition(pf.player, mathx.Vec2{X: platformLeft - 0.32, Y: platformTop - feetBelow + below})
	pf.hold(input.KeyRight, true)
	return pf
}

// Moving into the floating platform from beside it climbs on when its top is within Ledge Climb, and the Animator shows Climb.
func TestLedgeClimbOntoPlatform(t *testing.T) {
	for _, below := range []float32{0.02, 0.1, 0.2, 0.3, 0.45} {
		pf := besideLedge(t, below, true)
		climbed := false
		for range 40 {
			pf.step(1)
			if pf.motion().Climbing {
				a, _ := scene.Get[animation.Animator](pf.app.Scene, pf.player)
				climbed = climbed || a.State == "Climb"
			}
		}
		if y := pf.y(); !pf.motion().Grounded || mathx.Abs(y-(platformTop-feetBelow)) > 0.05 {
			t.Fatalf("%.2f below the top: ended at y %.3f grounded %v, want on the platform", below, y, pf.motion().Grounded)
		}
		if !climbed {
			t.Fatalf("%.2f below the top: the Animator never showed Climb", below)
		}
	}
	pf := besideLedge(t, 0.55, true)
	pf.step(60)
	if pf.y() < 2 {
		t.Fatal("a ledge higher than Ledge Climb must not be climbed")
	}
}

// Without ledge climbing the character never hangs on the platform corner: it falls past it.
func TestNoSnagOnPlatformCorner(t *testing.T) {
	for _, below := range []float32{0.02, 0.1, 0.2, 0.3} {
		pf := besideLedge(t, below, false)
		pf.step(60)
		if y := pf.y(); y < 2 {
			t.Fatalf("%.2f below the top: still at y %.3f after a second, hanging on the corner", below, y)
		}
	}
}

func startClimb(t *testing.T) *platformer {
	t.Helper()
	pf := besideLedge(t, 0.3, true)
	for range 10 {
		pf.step(1)
		if pf.motion().Climbing {
			return pf
		}
	}
	t.Fatal("the climb did not start")
	return nil
}

// Knockback, Stop and Dash end a climb at once; root motion is ignored while climbing.
func TestClimbInterruptions(t *testing.T) {
	for name, act := range map[string]func(m *gameplay.Motion2D){
		"knockback": func(m *gameplay.Motion2D) { m.AddVelocity(mathx.Vec2{X: -6, Y: -4}) },
		"stop":      func(m *gameplay.Motion2D) { m.Stop() },
		"dash":      func(m *gameplay.Motion2D) { m.Dash(mathx.Vec2{X: -8}, 0.2) },
	} {
		pf := startClimb(t)
		act(pf.motion())
		pf.step(1)
		if pf.motion().Climbing {
			t.Fatalf("%s did not end the climb", name)
		}
	}
	a, b := startClimb(t), startClimb(t)
	svc := gameplay.Of(b.app.Engine.MainWorld())
	for range 6 {
		svc.AddDisplacement(b.player, mathx.Vec2{X: -0.2})
		a.step(1)
		b.step(1)
	}
	if pa, pb := a.app.Scene.Local(a.player).Position, b.app.Scene.Local(b.player).Position; pa != pb {
		t.Fatalf("root motion moved a climbing character: %v vs %v", pb, pa)
	}
}

// Rewinding into the middle of a climb and replaying gives the same state hash.
func TestClimbRewind(t *testing.T) {
	s, pc := rewindSession(t)
	pc.SetKeyEvery(1)
	movePlayer(t, pc, mathx.Vec2{X: platformLeft - 0.32, Y: platformTop - feetBelow + 0.3})
	e := s.Host.Engine
	mid, hash := 0, [32]byte{}
	for i := 1; i <= 40; i++ {
		e.Input().BeginFrame()
		keyHold(e, input.KeyD, true)
		pc.Tick(1.0 / 60)
		if m, _ := scene.Get[gameplay.Motion2D](pc.Scene(), pc.Scene().Find(pc.Scene().Root(), "Player")); mid == 0 && m.Climbing && m.Velocity == (mathx.Vec2{}) {
			mid = i + 3
		}
		if i == mid && mid > 0 {
			hash = pausedHash(pc)
		}
	}
	if mid == 0 || hash == ([32]byte{}) {
		t.Fatal("the player never climbed")
	}
	if err := pc.Seek(mid); err != nil {
		t.Fatal(err)
	}
	if m, _ := scene.Get[gameplay.Motion2D](pc.Scene(), pc.Scene().Find(pc.Scene().Root(), "Player")); !m.Climbing {
		t.Fatal("rewinding into the climb lost it")
	}
	if pc.StateHash() != hash {
		t.Fatal("the state hash differs after rewinding into a climb")
	}
}
