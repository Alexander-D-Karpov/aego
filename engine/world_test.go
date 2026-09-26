package engine

import (
	"testing"

	"aego/core/log"
	"aego/platform/headless"
)

type worldRecorder struct {
	tag string
	out *[]string
}

func (r worldRecorder) Run(ctx *Context) {
	name := "nil"
	if ctx.World != nil {
		name = ctx.World.Name()
	}
	*r.out = append(*r.out, r.tag+"@"+name)
}

func TestScopesAndWorlds(t *testing.T) {
	var got []string
	b := NewBuilder(Config{
		Headless: true, HeadlessFrameTime: 1.0 / 60,
		ResRoot: t.TempDir(), UserRoot: t.TempDir(), AppID: "aego-test",
	}).WithLogger(log.Nop()).WithPlatform(headless.New(headless.Options{FrameTime: 1.0 / 60}))
	sys := b.Registry().Systems()
	if err := sys.Register(SystemDesc{ID: "play", Phase: PhaseUpdate, System: worldRecorder{"play", &got}}); err != nil {
		t.Fatal(err)
	}
	if err := sys.Register(SystemDesc{ID: "edit", Phase: PhaseUpdate, Scope: ScopeEditor, System: worldRecorder{"edit", &got}}); err != nil {
		t.Fatal(err)
	}
	e, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	e.Step(&testApp{})
	if len(got) != 1 || got[0] != "play@main" {
		t.Fatalf("main loop ran %v", got)
	}
	got = got[:0]
	w := NewWorld("edit")
	e.RunPhase(w, PhaseUpdate, ScopeEditor, 0, 0)
	if len(got) != 1 || got[0] != "edit@edit" {
		t.Fatalf("editor scope ran %v", got)
	}
	got = got[:0]
	e.RunPhase(w, PhaseUpdate, ScopeWorld, 0, 0)
	if len(got) != 2 {
		t.Fatalf("world scope ran %v", got)
	}
}

func TestServicesArePerWorld(t *testing.T) {
	w1, w2 := NewWorld("a"), NewWorld("b")
	w1.SetService("x", 1)
	if _, ok := ServiceOf[int](w2, "x"); ok {
		t.Fatal("services leaked between worlds")
	}
	if v, ok := ServiceOf[int](w1, "x"); !ok || v != 1 {
		t.Fatal("service lost")
	}
	w1.RemoveService("x")
	if _, ok := ServiceOf[int](w1, "x"); ok {
		t.Fatal("remove failed")
	}
}
