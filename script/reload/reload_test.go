package reload_test

import (
	"os"
	"path/filepath"
	"testing"

	"aego/engine"
	scriptstdlib "aego/modules/scripting/stdlib"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/scriptcomp"
	"aego/schema"
	"aego/script/bind"
	"aego/script/bind/manifest"
	"aego/script/diag"
	"aego/script/reload"
	"aego/script/runtime"
	"aego/script/stdlib"
	"aego/script/vm"
	"aego/script/workspace"
)

const v1 = `package demo

enum State {
	A
	B
}

script Counter {
	var Count int
	var Speed float = 2
	var Mode State
	var Tags []string

	func Ready() {
		Mode = State.B
		Tags = append(Tags, "x")
	}

	func Update(dt float) {
		Count++
	}
}
`

const v2 = `package demo

enum State {
	B
	A
}

script Counter {
	var Extra int = 7
	var Mode State
	var Speed string
	var Count int
	var Tags []string

	func Update(dt float) {
		Count += 2
	}

	func Reloaded() {
		Count += 100
	}
}
`

const counter = "game.demo.Counter"

func write(t *testing.T, path, src string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func build(t *testing.T, ws *workspace.Workspace) reload.Program {
	t.Helper()
	g, ds := ws.Build()
	if diag.HasErrors(ds) {
		t.Fatalf("build failed: %v", ds)
	}
	return reload.NewProgram(g.ID, g.Modules, g.Hashes)
}

func field(t *testing.T, rt *runtime.Runtime, id vm.InstanceID, name string) vm.Value {
	t.Helper()
	v, ok := rt.Field(id, name)
	if !ok {
		t.Fatalf("field %s is missing", name)
	}
	return v
}

func TestReloadMigratesInstance(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "scripts", "demo", "demo.aes")
	write(t, file, v1)
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
	h := s.Create(s.Root(), "Node")
	sc := scene.Add[scriptcomp.ScriptComponent](s, h)
	sc.Attachments = append(sc.Attachments, scriptcomp.Attachment{TypeName: counter, Enabled: true})
	s.Flush()
	if err := rt.Instantiate(s); err != nil {
		t.Fatal(err)
	}
	ctx := &engine.Context{Dt: 1.0 / 60}
	for i := 1; i <= 3; i++ {
		ctx.Frame = uint64(i)
		rt.Update(ctx)
	}
	id, ok := rt.InstanceOf(h, counter)
	if !ok {
		t.Fatal("instance is missing")
	}
	if got := field(t, rt, id, "Count").Int(); got != 3 {
		t.Fatalf("count before reload = %d", got)
	}

	write(t, file, v2)
	ws.Changed([]string{file})
	p2 := build(t, ws)
	plan, err := reload.Build(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := reload.Apply(rt, types.Schema(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Migrated != 1 || rep.Reset != 1 || rep.Generation != p2.ID {
		t.Fatalf("report %s", rep)
	}
	id2, ok := rt.InstanceOf(h, counter)
	if !ok || id2 != id {
		t.Fatal("instance id changed across reload")
	}
	if got := field(t, rt, id, "Count").Int(); got != 103 {
		t.Fatalf("count after Reloaded = %d", got)
	}
	if got := field(t, rt, id, "Extra").Int(); got != 7 {
		t.Fatalf("new field default = %d", got)
	}
	if got := field(t, rt, id, "Mode").Int(); got != 0 {
		t.Fatalf("enum not migrated by name: %d", got)
	}
	if got := field(t, rt, id, "Speed"); !got.IsZero() {
		t.Fatal("incompatible field was not reset")
	}
	elems, _, ok := rt.Machine().SliceData(field(t, rt, id, "Tags"))
	if !ok || len(elems) != 1 {
		t.Fatal("slice did not survive reload")
	}
	if s, ok := rt.Machine().Str(elems[0]); !ok || s != "x" {
		t.Fatalf("slice element = %q", s)
	}
	ctx.Frame = 4
	rt.Update(ctx)
	if got := field(t, rt, id, "Count").Int(); got != 105 {
		t.Fatalf("new Update did not run: %d", got)
	}
}

func TestBrokenPlanKeepsGeneration(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "scripts", "demo", "demo.aes")
	write(t, file, v1)
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
	write(t, file, "package demo\n\nscript Counter {\n\tfunc Update(dt float) {\n\t\tx\n\t}\n}\n")
	ws.Changed([]string{file})
	g, ds := ws.Build()
	if !diag.HasErrors(ds) || g.ID != p1.ID {
		t.Fatal("a broken save produced a new generation")
	}
}

func prepare() (*scene.Types, *bind.Registry, error) {
	types := scene.NewTypes(schema.NewRegistry())
	if err := frame.RegisterTypes(types); err != nil {
		return nil, nil, err
	}
	if err := scriptcomp.Register(types); err != nil {
		return nil, nil, err
	}
	reg := bind.NewManifest()
	scriptstdlib.Install(reg, stdlib.Options{Seed: 1, Testing: true})
	return types, reg, nil
}
