//go:build aego_editor

package docsview

import (
	"encoding/json"
	"strings"
	"testing"

	"aego/app/playground"
	"aego/core/mathx"
	"aego/modules/gameplay"
	"aego/modules/render2d"
	"aego/platform"
	"aego/platform/headless"
	"aego/platform/input"
	"aego/scene"
)

func TestMotionTryMovesRealBodyAndResets(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("gameplay.Motion2D")
	if !ok || e.Demo == nil {
		t.Fatal("Motion component contract is absent")
	}
	code, err := p.TryCode(e.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	a := newHeadless(t)
	req, err := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "code": code, "prefabs": e.TryPrefabs, "artifacts": e.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if r := a.Call("load", req); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(10); err != nil {
		t.Fatal(err)
	}
	state := func() (*gameplay.Motion2D, *render2d.SpriteRenderer, mathx.Vec2) {
		t.Helper()
		sc := a.session.Controller().Scene()
		h := sc.Find(sc.Root(), "Motion marker")
		motion, ok := scene.Get[gameplay.Motion2D](sc, h)
		if !ok {
			t.Fatal("Motion 2D is absent")
		}
		sprite, ok := scene.Get[render2d.SpriteRenderer](sc, h)
		if !ok {
			t.Fatal("motion marker sprite is absent")
		}
		return motion, sprite, sc.WorldPosition(h)
	}
	motion, sprite, origin := state()
	idleColor := sprite.Tint
	if motion.MaxSpeed != 0 || motion.Velocity.X != 0 {
		t.Fatalf("zero speed did not stop the actual body: %+v", motion)
	}
	if r := a.Call("set", []byte(`{"component":"gameplay.Motion2D","field":"MaxSpeed","value":6}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(30); err != nil {
		t.Fatal(err)
	}
	motion, sprite, moved := state()
	if motion.MaxSpeed != 6 || motion.Velocity.X < 5 || moved.X <= origin.X+0.5 || sprite.Tint == idleColor {
		t.Fatalf("Motion did not accelerate and move the real body: velocity=%v origin=%v moved=%v tint=%v", motion.Velocity, origin, moved, sprite.Tint)
	}
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(10); err != nil {
		t.Fatal(err)
	}
	motion, sprite, reset := state()
	if motion.MaxSpeed != 0 || motion.Velocity.X != 0 || reset != origin || sprite.Tint != idleColor {
		t.Fatalf("Reset did not restore the stationary body: velocity=%v position=%v tint=%v", motion.Velocity, reset, sprite.Tint)
	}
	if messages := errorsOf(a); len(messages) != 0 {
		t.Fatalf("Motion Try runtime errors: %v", messages)
	}
}

func TestMotionGuidesKeepOriginalKeyboardScene(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"gameplay", "gameplay/motion"} {
		e, ok := p.Docs().Get(id)
		if !ok || !strings.Contains(e.Try, `name="Slope"`) || !strings.Contains(e.TryScript, "input.KeyD") {
			t.Fatalf("%s lost its original lift, slope or keyboard example", id)
		}
		code, err := p.TryCode(e.TryScript)
		if err != nil {
			t.Fatal(err)
		}
		a := newHeadless(t)
		req, err := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "code": code, "prefabs": e.TryPrefabs, "artifacts": e.TryArtifacts})
		if err != nil {
			t.Fatal(err)
		}
		if r := a.Call("load", req); r.Error != "" {
			t.Fatal(r.Error)
		}
		if err := a.Step(30); err != nil {
			t.Fatal(err)
		}
		sc := a.session.Controller().Scene()
		player := sc.Find(sc.Root(), "Player")
		origin := sc.WorldPosition(player)
		a.session.Controller().SetCapture(true, mathx.Vec2{}, mathx.Vec2{X: 960, Y: 540})
		a.e.Platform().(*headless.Platform).Push(platform.Event{Kind: platform.EvKeyDown, Key: input.KeyEvent{Scan: input.KeyD}})
		if err := a.Step(30); err != nil {
			t.Fatal(err)
		}
		if moved := sc.WorldPosition(player); moved.X <= origin.X+0.1 {
			t.Fatalf("%s D-key input did not move the guide player: %v -> %v", id, origin, moved)
		}
		if messages := errorsOf(a); len(messages) != 0 {
			t.Fatalf("%s guide runtime errors: %v", id, messages)
		}
	}
}
