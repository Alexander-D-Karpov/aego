package schema

import "testing"

type hpA struct{ A int32 }
type hpB struct{ B int32 }
type hpC struct{ C int32 }

func TestComponentNamedPrefersProject(t *testing.T) {
	r := NewRegistry()
	engine, err := Register[hpA](r, Name("gameplay.Health"))
	if err != nil {
		t.Fatal(err)
	}
	engine.Builtin = true
	if td, ok := r.ComponentNamed("Health"); !ok || td != engine {
		t.Fatalf("engine only: %v %v", td, ok)
	}
	project, err := Register[hpB](r, Name("game.Health"))
	if err != nil {
		t.Fatal(err)
	}
	if td, ok := r.ComponentNamed("Health"); !ok || td != project {
		t.Fatalf("project wins: %v %v", td, ok)
	}
	if got := PreferProject([]*TypeDescriptor{engine, project}); len(got) != 1 || got[0] != project {
		t.Fatalf("PreferProject = %v", got)
	}
	if _, err := Register[hpC](r, Name("mygame.Health")); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.ComponentNamed("Health"); ok {
		t.Fatal("two project components must stay ambiguous")
	}
}
