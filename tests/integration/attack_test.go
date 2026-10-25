package integration

import (
	"testing"

	"aego/modules/animation"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/scene"
)

// J plays the attack: the Animator enters Hit, the slash arc shows during the swing and is hidden again after it.
func TestAttackShowsTheSlash(t *testing.T) {
	pf := openPlatformer(t)
	pf.step(10)
	sc := pf.app.Scene
	slash := sc.Find(pf.player, "Slash")
	if slash.IsZero() {
		t.Fatal("the Player has no Slash child")
	}
	alpha := func() float32 {
		sr, ok := scene.Get[render2d.SpriteRenderer](sc, slash)
		if !ok {
			t.Fatal("Slash has no SpriteRenderer")
		}
		return sr.Tint.A
	}
	if a := alpha(); a > 0.01 {
		t.Fatalf("the slash shows before the attack: alpha %v", a)
	}
	pf.hold(input.KeyJ, true)
	pf.step(1)
	pf.hold(input.KeyJ, false)
	hit, shown := false, false
	for range 30 {
		pf.step(1)
		a, _ := scene.Get[animation.Animator](sc, pf.player)
		hit = hit || a.State == "Hit"
		shown = shown || alpha() > 0.9
	}
	if !hit || !shown {
		t.Fatalf("attack: Animator in Hit %v, slash shown %v", hit, shown)
	}
	pf.step(10)
	if a := alpha(); a > 0.01 {
		t.Fatalf("the slash stays after the attack: alpha %v", a)
	}
	pf.noFaults(t)
}
