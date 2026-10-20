package modulecheck

import (
	"slices"
	"testing"

	"aego/app/assembly"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/format/aetext"
	inputmod "aego/modules/input"
	"aego/script/bind"
	"aego/tests/modulecheck/fakephys"
)

func TestFakeProvidesInputActions(t *testing.T) {
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	cat := assembly.Catalog{
		{ID: base.CoreID, New: func(assembly.Options) module.Module { return base.Core{} }},
		{ID: inputmod.ID, New: func(assembly.Options) module.Module { return inputmod.New() }},
		{ID: fakephys.ID, New: func(assembly.Options) module.Module { return &fakephys.Module{} }},
	}
	man, err := cat.Load(types, bind.New(), assembly.Options{})
	if err != nil {
		t.Fatal(err)
	}
	secs, err := man.LoadSections(func(string) *aetext.Node { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := man.Init(&module.InitContext{Sections: secs}); err != nil {
		t.Fatal(err)
	}
	w := engine.NewWorld("fake")
	detach, err := man.Attach(&module.AttachContext{World: w, Play: true})
	if err != nil {
		t.Fatal(err)
	}
	defer detach()
	s := inputmod.Of(w)
	if s == nil || !s.Has(fakephys.ActionKick) || !s.Has("horizontal") {
		t.Fatal("the provided action is not in the play world")
	}
	if !slices.Contains(s.Contexts(), fakephys.ContextFake) {
		t.Fatalf("stack %v", s.Contexts())
	}
}
