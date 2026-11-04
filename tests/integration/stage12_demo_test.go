package integration

import (
	"math"
	"testing"

	"aego/modules/render2d"
	"aego/scene"
)

func TestHurtClipFlashesOnlyTheDamagedLitCharacter(t *testing.T) {
	pf := openPlatformer(t)
	s := pf.app.Scene
	flash := func(h scene.NodeHandle) float64 {
		t.Helper()
		v, err := render2d.GetMaterialParameter(s, h, "Flash")
		if err != nil {
			t.Fatal(err)
		}
		return v.F
	}
	pf.step(2)
	player, _ := scene.Get[render2d.SpriteRenderer](s, pf.player)
	enemy, _ := scene.Get[render2d.SpriteRenderer](s, pf.enemy)
	if player.Material.ID != enemy.Material.ID {
		t.Fatal("expected a shared character material")
	}
	original := player.Material.ID
	if flash(pf.player) != 0 || flash(pf.enemy) != 0 {
		t.Fatal("unexpected flash before damage")
	}
	pf.hitPlayer(t)
	lit := false
	for range 36 {
		pf.step(1)
		lit = lit || flash(pf.player) > 0.5
		if flash(pf.enemy) != 0 || player.Material.ID != original {
			t.Fatal("flash changed shared state")
		}
	}
	if !lit || flash(pf.player) != 0 {
		t.Fatal("damage did not trigger and clear the local flash")
	}
	pf.noFaults(t)
}

func TestTopdownTorchFollowsAim(t *testing.T) {
	td := openTopdown(t)
	sc := td.app.Scene
	torch := sc.Find(td.hero, "Torch")
	if _, ok := scene.Get[render2d.Light2D](sc, torch); !ok {
		t.Fatal("missing torch")
	}
	floor, _ := scene.Get[render2d.SpriteRenderer](sc, sc.Find(sc.Root(), "Floor"))
	if floor.Material.ID.IsZero() {
		t.Fatal("floor has no lighting material")
	}
	for _, aim := range []float32{1, -1} {
		if err := td.in.Inject(0, "AimY", aim); err != nil {
			t.Fatal(err)
		}
		td.step()
		if got := sc.Local(torch).Rotation; math.Abs(float64(got)-float64(aim)*math.Pi/2) > 0.01 {
			t.Fatalf("AimY=%v produced angle %v", aim, got)
		}
	}
}
