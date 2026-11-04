//go:build aego_editor

package docsview

import (
	"encoding/json"
	"testing"

	"aego/app/playground"
	"aego/core/mathx"
	"aego/modules/render2d"
	"aego/platform"
	"aego/platform/headless"
	"aego/platform/input"
	"aego/scene"
)

func TestCameraOutputTryCyclesSevenPreparedEffects(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	a := newHeadless(t)
	loaded := false
	for _, entry := range p.TryScenes() {
		if entry.ID != render2d.NamePostProcess {
			continue
		}
		if len(entry.TryArtifacts) == 0 || entry.TryScript == "" {
			t.Fatal("incomplete PostFX Try resources")
		}
		code, err := p.TryCode(entry.TryScript)
		if err != nil {
			t.Fatal(err)
		}
		req, err := json.Marshal(map[string]any{"scene": entry.Try, "select": entry.TrySelect, "code": code, "artifacts": entry.TryArtifacts})
		if err != nil {
			t.Fatal(err)
		}
		if r := a.Call("load", req); r.Error != "" {
			t.Fatal(r.Error)
		}
		loaded = true
		break
	}
	if !loaded {
		t.Fatal("PostFX Try not registered")
	}
	if err := a.Step(2); err != nil {
		t.Fatal(err)
	}
	ctrl := a.session.Controller()
	s := ctrl.Scene()
	stack, ok := scene.Get[render2d.PostProcess](s, s.Find(s.Root(), "Effects Camera"))
	if !ok || len(stack.Effects) != 7 {
		t.Fatal("authored stack missing")
	}
	if _, ok := a.loader.Material(stack.Effects[6].Material.ID); !ok {
		t.Fatal("custom material not in bundle")
	}
	if _, ok := a.loader.Texture(stack.Effects[1].LUT.ID); !ok {
		t.Fatal("LUT not in bundle")
	}
	key := input.KeyEvent{Scan: input.KeySpace}
	for step := range 9 {
		mode := step % 8
		for i, e := range stack.Effects {
			if e.Enabled != (i == mode) {
				t.Fatalf("step %d effect %s enabled=%v", step, e.TypeID, e.Enabled)
			}
		}
		if step == 8 {
			break
		}
		ctrl.SetCapture(true, mathx.Vec2{}, mathx.Vec2{X: 960, Y: 540})
		a.e.Platform().(*headless.Platform).Push(platform.Event{Kind: platform.EvKeyDown, Key: key})
		if err := a.Step(1); err != nil {
			t.Fatal(err)
		}
		a.e.Platform().(*headless.Platform).Push(platform.Event{Kind: platform.EvKeyUp, Key: key})
		if err := a.Step(1); err != nil {
			t.Fatal(err)
		}
	}
	if errors := errorsOf(a); len(errors) > 0 {
		t.Fatal(errors)
	}
}
