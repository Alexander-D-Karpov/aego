//go:build aego_editor

package docsview

import (
	"encoding/json"
	"strings"
	"testing"

	"aego/app/playground"
	"aego/asset"
	"aego/modules/animation"
	"aego/modules/render2d"
	"aego/scene"
)

func TestAnimationPlayerTryPlaysActualFramesAndResets(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("animation.AnimationPlayer")
	if !ok || e.Demo == nil {
		t.Fatal("Animation Player component contract is absent")
	}
	a := newHeadless(t)
	req, err := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "prefabs": e.TryPrefabs, "artifacts": e.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if reply := a.Call("load", req); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(30); err != nil {
		t.Fatal(err)
	}
	state := func() (*animation.AnimationPlayer, *render2d.SpriteRenderer) {
		t.Helper()
		sc := a.session.Controller().Scene()
		h := sc.Find(sc.Root(), "Runner")
		player, ok := scene.Get[animation.AnimationPlayer](sc, h)
		if !ok {
			t.Fatal("runner lost its Animation Player")
		}
		sprite, ok := scene.Get[render2d.SpriteRenderer](sc, h)
		if !ok {
			t.Fatal("runner lost its Sprite Renderer")
		}
		return player, sprite
	}
	player, sprite := state()
	if player.Speed != 0 || player.Time != 0 || sprite.Sprite.ID != asset.BuiltinRunner(0) {
		t.Fatalf("zero-speed animation did not hold the first frame: player=%+v sprite=%v", player, sprite.Sprite.ID)
	}
	if reply := a.Call("set", []byte(`{"component":"animation.AnimationPlayer","field":"Speed","value":8}`)); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(15); err != nil {
		t.Fatal(err)
	}
	player, sprite = state()
	if player.Speed != 8 || player.Playing || sprite.Sprite.ID != asset.BuiltinRunner(7) {
		t.Fatalf("the actual clip did not play once to its last frame: player=%+v sprite=%v", player, sprite.Sprite.ID)
	}
	if reply := a.Call("reset", nil); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(4); err != nil {
		t.Fatal(err)
	}
	player, sprite = state()
	if player.Speed != 0 || player.Time != 0 || sprite.Sprite.ID != asset.BuiltinRunner(0) {
		t.Fatalf("Reset did not restore the first frame: player=%+v sprite=%v", player, sprite.Sprite.ID)
	}
	if messages := errorsOf(a); len(messages) > 0 {
		t.Fatalf("Animation Player Try runtime errors: %v", messages)
	}
}

func TestAnimationPlayerGuideKeepsOriginalExample(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("animation/player")
	if !ok || !strings.Contains(e.Try, `name="Pendulum"`) || !strings.Contains(e.TryScript, "OnAnimEvent") || !strings.Contains(e.TryScript, "input.KeyR") {
		t.Fatal("the animation guide lost its original runner and pendulum example")
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
	if err := a.Step(120); err != nil {
		t.Fatal(err)
	}
	sc := a.session.Controller().Scene()
	for _, name := range []string{"Runner", "Pendulum"} {
		player, ok := scene.Get[animation.AnimationPlayer](sc, sc.Find(sc.Root(), name))
		if !ok || !player.Playing {
			t.Fatalf("the guide's %s animation is not playing", name)
		}
	}
	if messages := errorsOf(a); len(messages) > 0 {
		t.Fatalf("animation guide runtime errors: %v", messages)
	}
}
