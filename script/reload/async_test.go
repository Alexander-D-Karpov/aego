package reload_test

import (
	"path/filepath"
	"strings"
	"testing"

	"aego/engine"
	"aego/scene"
	"aego/scene/scriptcomp"
	"aego/script/bind/manifest"
	"aego/script/reload"
	"aego/script/runtime"
	"aego/script/workspace"
)

const hubV1 = `package demo

script Hub {
	signal A()
	signal B(n int)

	var hits int
	var task Coroutine

	func Ready() {
		B.Connect(OnB)
		task = start Loop()
	}

	func Update(dt float) {
		B.Emit(1)
	}

	func OnB(n int) {
		hits += n
	}

	func Loop() coroutine {
		for {
			yield
		}
	}
}
`

const hubV2 = `package demo

script Hub {
	signal B(n int)
	signal A()

	var hits int
	var task Coroutine

	func Ready() {
		B.Connect(OnB)
		task = start Loop()
	}

	func Update(dt float) {
		B.Emit(1)
	}

	func OnB(n int) {
		hits += n * 10
	}

	func Loop() coroutine {
		for {
			yield
		}
	}

	func Reloaded() {
		task = start Loop()
	}
}
`

const otherSrc = `package other

script Other {
	var ticks int

	func Ready() {
		start Loop()
	}

	func Loop() coroutine {
		for {
			ticks++
			yield
		}
	}
}
`

func TestReloadCancelsAndRebinds(t *testing.T) {
	root := t.TempDir()
	hubFile := filepath.Join(root, "scripts", "demo", "demo.aes")
	write(t, hubFile, hubV1)
	write(t, filepath.Join(root, "scripts", "other", "other.aes"), otherSrc)
	types, reg, err := prepare()
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.FromRegistry(reg, types.Schema())
	if err != nil {
		t.Fatal(err)
	}
	tab, err := m.Table()
	if err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.Open(workspace.Config{Root: root, Module: "game", Bindings: tab})
	if err != nil {
		t.Fatal(err)
	}
	p1 := build(t, ws)
	rt := runtime.New(runtime.Options{Bindings: reg, Schema: types.Schema()})
	if err := rt.Link(p1.Ordered()); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	add := func(name, typ string) scene.NodeHandle {
		h := s.Create(s.Root(), name)
		sc := scene.Add[scriptcomp.ScriptComponent](s, h)
		sc.Attachments = append(sc.Attachments, scriptcomp.Attachment{TypeName: typ, Enabled: true})
		return h
	}
	hub := add("Hub", "game.demo.Hub")
	other := add("Other", "game.other.Other")
	s.Flush()
	if err := rt.Instantiate(s); err != nil {
		t.Fatal(err)
	}
	ctx := &engine.Context{Dt: 1.0 / 60}
	frame := func() {
		ctx.Frame++
		rt.Update(ctx)
		rt.Coroutines(ctx)
	}
	for i := 0; i < 3; i++ {
		frame()
	}
	hid, _ := rt.InstanceOf(hub, "game.demo.Hub")
	oid, _ := rt.InstanceOf(other, "game.other.Other")
	if got := field(t, rt, hid, "hits").Int(); got != 3 {
		t.Fatalf("hits before reload %d", got)
	}
	ticks := field(t, rt, oid, "ticks").Int()

	write(t, hubFile, hubV2)
	ws.Changed([]string{hubFile})
	p2 := build(t, ws)
	plan, err := reload.Build(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := reload.Apply(rt, types.Schema(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Canceled != 1 || rep.Disconnected != 0 {
		t.Fatalf("report %s", rep)
	}
	if !strings.Contains(rep.String(), "1 coroutines canceled, 0 handlers disconnected") {
		t.Fatalf("console line %q", rep.String())
	}
	if live := rt.CoStats().Live; live != 2 {
		t.Fatalf("live coroutines after reload %d", live)
	}
	frame()
	if got := field(t, rt, hid, "hits").Int(); got != 13 {
		t.Fatalf("rebound handler did not run the new code: hits %d", got)
	}
	if got := field(t, rt, oid, "ticks").Int(); got <= ticks {
		t.Fatal("coroutine of an unchanged script stopped")
	}
}
