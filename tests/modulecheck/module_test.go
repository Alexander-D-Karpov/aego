package modulecheck

import (
	"slices"
	"testing"

	"aego/app/assembly"
	"aego/contracts/scriptevents"
	"aego/engine/base"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/format/aetext"
	"aego/scene"
	"aego/script/bind"
	"aego/tests/modulecheck/fakephys"
)

func catalog(m *fakephys.Module) assembly.Catalog {
	return assembly.Catalog{
		{ID: base.CoreID, New: func(assembly.Options) module.Module { return base.Core{} }},
		{ID: fakephys.ID, New: func(assembly.Options) module.Module { return m }},
	}
}

func build(t *testing.T, m *fakephys.Module) (*scene.Types, *module.Manifest) {
	t.Helper()
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	man, err := catalog(m).Load(types, bind.New(), assembly.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return types, man
}

func block(t *testing.T, src string) func(string) *aetext.Node {
	t.Helper()
	n, err := aetext.Parse([]byte(src), "test")
	if err != nil {
		t.Fatal(err)
	}
	return func(head string) *aetext.Node {
		if head == n.Head {
			return n
		}
		return nil
	}
}

func TestSectionReachesInit(t *testing.T) {
	m := &fakephys.Module{}
	_, man := build(t, m)
	secs, err := man.LoadSections(block(t, "fakephys { gain=2 }"))
	if err != nil {
		t.Fatal(err)
	}
	if err := man.Init(&module.InitContext{Sections: secs}); err != nil {
		t.Fatal(err)
	}
	if m.Gain != 2 {
		t.Fatalf("section did not reach Init, gain %v", m.Gain)
	}
	if got := secs.Names(fakephys.ID, fakephys.Lists); len(got) != 2 {
		t.Fatalf("names hint: %v", got)
	}
	if !slices.ContainsFunc(man.Events, func(e scriptevents.Event) bool { return e.Name == fakephys.EventTick }) {
		t.Fatal("script event is not in the manifest")
	}
}

func TestBadSectionFailsBuild(t *testing.T) {
	_, man := build(t, &fakephys.Module{})
	if _, err := man.LoadSections(block(t, "fakephys { bad=true }")); err == nil {
		t.Fatal("bad=true must fail the project load")
	}
}

func TestValidateBlocksPlay(t *testing.T) {
	types, _ := build(t, &fakephys.Module{})
	s := scene.New(types, nil, scene.Options{})
	h := s.Create(s.Root(), "n")
	*scene.Add[fakephys.Body](s, h) = fakephys.Body{}
	s.Flush()
	svc := problems.New()
	svc.Add(problems.Source{
		Revision: func() uint64 { return s.Revision() },
		Append:   func(dst []problems.Diagnostic) []problems.Diagnostic { return module.ValidateScene(s, dst) },
	})
	first, n := svc.Blocking()
	if n != 1 || first.Owner != fakephys.ID || first.Subject.Field != "fakephys.Body" {
		t.Fatalf("want one blocking fakephys problem, got %d %+v", n, first)
	}
}
