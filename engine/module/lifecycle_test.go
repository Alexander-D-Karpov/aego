package module_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/script/bind"
)

type rec struct {
	id         string
	log        *[]string
	failInit   bool
	failAttach bool
}

func (m *rec) Info() module.Info {
	return module.Info{ID: m.id}
}

func (m *rec) Register(*module.Registry) error {
	return nil
}

func (m *rec) Init(*module.InitContext) error {
	*m.log = append(*m.log, "init "+m.id)
	if m.failInit {
		return errors.New("boom")
	}
	return nil
}

func (m *rec) Shutdown() {
	*m.log = append(*m.log, "shutdown "+m.id)
}

func (m *rec) AttachWorld(ctx *module.AttachContext) error {
	*m.log = append(*m.log, "attach "+m.id)
	if m.failAttach {
		return errors.New("boom")
	}
	ctx.World.SetService(m.id+".state", &struct{ n int }{})
	return nil
}

func (m *rec) DetachWorld(ctx *module.AttachContext) {
	*m.log = append(*m.log, "detach "+m.id)
	ctx.World.RemoveService(m.id + ".state")
}

func manifest(t *testing.T, mods ...module.Module) *module.Manifest {
	t.Helper()
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	m, err := module.Load(types, bind.New(), mods...)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestInitFailureRollsBack(t *testing.T) {
	var log []string
	m := manifest(t, &rec{id: "a", log: &log}, &rec{id: "b", log: &log, failInit: true}, &rec{id: "c", log: &log})
	if err := m.Init(&module.InitContext{}); err == nil {
		t.Fatal("init should fail")
	}
	want := []string{"init a", "init b", "shutdown a"}
	if !slices.Equal(log, want) {
		t.Fatalf("got %v, want %v", log, want)
	}
}

func TestShutdownReversesInit(t *testing.T) {
	var log []string
	m := manifest(t, &rec{id: "a", log: &log}, &rec{id: "b", log: &log}, &rec{id: "c", log: &log})
	if err := m.Init(&module.InitContext{}); err != nil {
		t.Fatal(err)
	}
	log = log[:0]
	m.Shutdown()
	m.Shutdown()
	want := []string{"shutdown c", "shutdown b", "shutdown a"}
	if !slices.Equal(log, want) {
		t.Fatalf("got %v, want %v", log, want)
	}
}

func TestDetachAfterPartialAttach(t *testing.T) {
	var log []string
	m := manifest(t, &rec{id: "a", log: &log}, &rec{id: "b", log: &log, failAttach: true}, &rec{id: "c", log: &log})
	w := engine.NewWorld("play")
	if _, err := m.Attach(&module.AttachContext{World: w, Play: true}); err == nil {
		t.Fatal("attach should fail")
	}
	want := []string{"attach a", "attach b", "detach b", "detach a"}
	if !slices.Equal(log, want) {
		t.Fatalf("got %v, want %v", log, want)
	}
	if _, ok := w.ServiceAny("a.state"); ok {
		t.Fatal("state of a survived the rollback")
	}
}

func TestWorldsKeepTheirOwnState(t *testing.T) {
	var log []string
	m := manifest(t, &rec{id: "a", log: &log})
	edit, play1, play2 := engine.NewWorld("edit"), engine.NewWorld("play1"), engine.NewWorld("play2")
	detEdit, err := m.Attach(&module.AttachContext{World: edit, Editor: true})
	if err != nil {
		t.Fatal(err)
	}
	det1, err := m.Attach(&module.AttachContext{World: play1, Play: true})
	if err != nil {
		t.Fatal(err)
	}
	det2, err := m.Attach(&module.AttachContext{World: play2, Play: true})
	if err != nil {
		t.Fatal(err)
	}
	s1, _ := play1.ServiceAny("a.state")
	s2, _ := play2.ServiceAny("a.state")
	if s1 == s2 {
		t.Fatal("play worlds share module state")
	}
	det1()
	det1()
	if _, ok := play1.ServiceAny("a.state"); ok {
		t.Fatal("detached world keeps its state")
	}
	for _, w := range []*engine.World{edit, play2} {
		if _, ok := w.ServiceAny("a.state"); !ok {
			t.Fatalf("detaching play1 removed the state of %s", w.Name())
		}
	}
	det2()
	detEdit()
}

type tick struct{}

func (tick) Run(*engine.Context) {}

type inner struct{}

func (inner) Info() engine.ModuleInfo {
	return engine.ModuleInfo{ID: "engine.outer"}
}

func (inner) Register(reg *engine.Registry) error {
	return reg.Systems().Register(engine.SystemDesc{ID: "other.tick", Phase: engine.PhaseUpdate, System: tick{}})
}

type outer struct{}

func (outer) Info() module.Info {
	return module.Info{ID: "outer"}
}

func (outer) Register(r *module.Registry) error {
	r.Use(inner{})
	return nil
}

func TestNestedEngineSystemsUseOwnerPrefix(t *testing.T) {
	m := manifest(t, outer{})
	_, err := engine.NewBuilder(engine.Config{Headless: true}).Use(m.Engine()).Build()
	if err == nil || !strings.Contains(err.Error(), "must be named outer.") {
		t.Fatalf("want a prefix error, got %v", err)
	}
}
