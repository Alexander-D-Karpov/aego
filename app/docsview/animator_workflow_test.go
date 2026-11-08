//go:build aego_editor

package docsview

import (
	"encoding/json"
	"strings"
	"testing"

	"aego/app/playground"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/modules/animation"
	"aego/modules/render2d"
	"aego/platform"
	"aego/platform/headless"
	"aego/platform/input"
	"aego/scene"
)

func TestAnimatorTryAdvancesActualRunStateAndResets(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("animation.Animator")
	if !ok || e.Demo == nil {
		t.Fatal("Animator component contract is absent")
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
	if reply := a.Call("load", req); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(30); err != nil {
		t.Fatal(err)
	}
	state := func() (*animation.Animator, *render2d.SpriteRenderer) {
		t.Helper()
		sc := a.session.Controller().Scene()
		h := sc.Find(sc.Root(), "Runner")
		animator, ok := scene.Get[animation.Animator](sc, h)
		if !ok {
			t.Fatal("runner lost its Animator")
		}
		sprite, ok := scene.Get[render2d.SpriteRenderer](sc, h)
		if !ok {
			t.Fatal("runner lost its Sprite Renderer")
		}
		return animator, sprite
	}
	animator, sprite := state()
	if animator.Speed != 0 || animator.State != "Run" || animator.Time != 0 {
		t.Fatalf("controller did not hold the Run state at zero speed: %+v", animator)
	}
	beforeTint := sprite.Tint
	if reply := a.Call("set", []byte(`{"component":"animation.Animator","field":"Speed","value":8}`)); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	seen := map[ids.AssetID]bool{}
	var maxTime float32
	for range 60 {
		if err := a.Step(1); err != nil {
			t.Fatal(err)
		}
		animator, sprite = state()
		if animator.State == "Run" {
			maxTime = max(maxTime, animator.Time)
			seen[sprite.Sprite.ID] = true
		}
	}
	if animator.Speed != 8 || maxTime <= 0.05 || len(seen) < 2 || sprite.Tint == beforeTint {
		t.Fatalf("the real Run clip did not advance: time=%v frames=%v tint=%v", maxTime, seen, sprite.Tint)
	}
	if reply := a.Call("reset", nil); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(30); err != nil {
		t.Fatal(err)
	}
	animator, sprite = state()
	if animator.Speed != 0 || animator.Time != 0 || sprite.Tint != beforeTint {
		t.Fatalf("Reset did not restore the held state: animator=%+v tint=%v", animator, sprite.Tint)
	}
	if messages := errorsOf(a); len(messages) > 0 {
		t.Fatalf("Animator Try runtime errors: %v", messages)
	}
}

func TestAnimatorGuideKeepsKeyboardExample(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("animation/animator")
	if !ok || !strings.Contains(e.TryScript, "input.KeyD") || !strings.Contains(e.TryScript, `SetTrigger("Attack")`) {
		t.Fatal("the animation guide lost its original keyboard example")
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
	if reply := a.Call("load", req); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(10); err != nil {
		t.Fatal(err)
	}
	a.session.Controller().SetCapture(true, mathx.Vec2{}, mathx.Vec2{X: 960, Y: 540})
	a.e.Platform().(*headless.Platform).Push(platform.Event{Kind: platform.EvKeyDown, Key: input.KeyEvent{Scan: input.KeyD}})
	if err := a.Step(30); err != nil {
		t.Fatal(err)
	}
	sc := a.session.Controller().Scene()
	animator, ok := scene.Get[animation.Animator](sc, sc.Find(sc.Root(), "Runner"))
	if !ok || animator.State != "Run" {
		t.Fatalf("the guide's D key did not select the Run state: %+v", animator)
	}
	if messages := errorsOf(a); len(messages) > 0 {
		t.Fatalf("Animator guide runtime errors: %v", messages)
	}
}
