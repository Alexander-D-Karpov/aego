//go:build aego_editor

package docsview

import (
	"encoding/json"
	"strings"
	"testing"

	"aego/app/playground"
	"aego/modules/gameplay"
	"aego/modules/render2d"
	"aego/scene"
)

func TestStateMachineTryRunsHooksTransitionAndReset(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("gameplay.StateMachine")
	if !ok || e.Demo == nil {
		t.Fatal("State Machine contract is absent")
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
	state := func() (*gameplay.StateMachine, *render2d.SpriteRenderer) {
		t.Helper()
		sc := a.session.Controller().Scene()
		h := sc.Find(sc.Root(), "State indicator")
		machine, ok := scene.Get[gameplay.StateMachine](sc, h)
		if !ok {
			t.Fatal("state machine is absent")
		}
		sprite, ok := scene.Get[render2d.SpriteRenderer](sc, h)
		if !ok {
			t.Fatal("state indicator sprite is absent")
		}
		return machine, sprite
	}
	machine, sprite := state()
	if machine.Initial != "Idle" || machine.Current != "Idle" {
		t.Fatalf("machine did not start in Idle: %+v", machine)
	}
	idleColor := sprite.Tint
	if r := a.Call("set", []byte(`{"component":"gameplay.StateMachine","field":"Initial","value":"Run"}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(10); err != nil {
		t.Fatal(err)
	}
	machine, sprite = state()
	if machine.Initial != "Run" || machine.Current != "Run" || sprite.Tint == idleColor {
		t.Fatalf("Run state and its Enter hook did not execute: machine=%+v tint=%+v", machine, sprite.Tint)
	}
	if !machine.Go("Idle") {
		t.Fatal("the running machine rejected its Idle transition")
	}
	if err := a.Step(10); err != nil {
		t.Fatal(err)
	}
	machine, sprite = state()
	if machine.Current != "Idle" || machine.Previous != "Run" || sprite.Tint != idleColor {
		t.Fatalf("the queued transition did not execute the Idle hook: machine=%+v tint=%+v", machine, sprite.Tint)
	}
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(10); err != nil {
		t.Fatal(err)
	}
	machine, sprite = state()
	if machine.Initial != "Idle" || machine.Current != "Idle" || sprite.Tint != idleColor {
		t.Fatalf("Reset did not restore Idle: machine=%+v tint=%+v", machine, sprite.Tint)
	}
	if messages := errorsOf(a); len(messages) > 0 {
		t.Fatalf("State Machine Try runtime errors: %v", messages)
	}
}

func TestStateMachineGuideKeepsOriginalMotionExample(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("gameplay/statemachine")
	if !ok || !strings.Contains(e.Try, "gameplay.Motion2D") || !strings.Contains(e.TryScript, "func UpdateAir") {
		t.Fatal("the guide lost its original walking and jumping example")
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
	if err := a.Step(150); err != nil {
		t.Fatal(err)
	}
	sc := a.session.Controller().Scene()
	machine, ok := scene.Get[gameplay.StateMachine](sc, sc.Find(sc.Root(), "Player"))
	if !ok || machine.Current == "" {
		t.Fatalf("the guide state machine did not start: %+v", machine)
	}
	if messages := errorsOf(a); len(messages) > 0 {
		t.Fatalf("state-machine guide runtime errors: %v", messages)
	}
}
