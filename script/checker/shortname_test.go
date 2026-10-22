package checker

import (
	"testing"

	"aego/script/abi"
	"aego/script/ast"
	"aego/script/diag"
	"aego/script/parser"
	"aego/script/source"
)

func healthTable(comps ...*abi.Component) *abi.Table {
	t := abi.NewTable()
	for _, c := range comps {
		t.AddComponent(c)
	}
	t.Freeze()
	return t
}

func healthComp(name string, builtin bool, field string) *abi.Component {
	return &abi.Component{Name: name, Builtin: builtin, Fields: []abi.Field{{Name: field, Type: abi.T(abi.Int)}}}
}

func checkHealth(t *testing.T, tab *abi.Table, field string) map[string]int {
	t.Helper()
	src := "package game\n\nscript P {\n\tfunc Ready() {\n\t\th := node.Get<Health>()\n\t\tif h != nil {\n\t\t\th." + field + " = 1\n\t\t}\n\t}\n}\n"
	sm := source.NewManager()
	f, ds := parser.ParseSource(sm, "game/main.aes", []byte(src))
	a := Check(sm, map[string][]*ast.File{"game": {f}}, tab, Options{})
	return codes(append(ds, a.Diagnostics...))
}

// A project component hides an engine module component with the same short name (D93).
func TestShortNamePrefersProject(t *testing.T) {
	engine := healthComp("gameplay.Health", true, "Max")
	project := healthComp("game.Health", false, "Hits")
	if c := checkHealth(t, healthTable(engine, project), "Hits"); len(c) != 0 {
		t.Fatalf("project Health: %v", c)
	}
	if c := checkHealth(t, healthTable(healthComp("gameplay.Health", true, "Max"), healthComp("game.Health", false, "Hits")), "Max"); len(c) == 0 {
		t.Fatalf("Health resolved to the engine component: %v", c)
	}
}

func TestShortNameTwoProjectComponentsAreAmbiguous(t *testing.T) {
	tab := healthTable(healthComp("gameplay.Health", true, "Max"), healthComp("game.Health", false, "Hits"), healthComp("mygame.Health", false, "Hits"))
	if c := checkHealth(t, tab, "Hits"); c[diag.BadTypeArg] != 1 {
		t.Fatalf("two project Health: %v, want %s", c, diag.BadTypeArg)
	}
}

func TestShortNameFallsBackToEngine(t *testing.T) {
	if c := checkHealth(t, healthTable(healthComp("gameplay.Health", true, "Max")), "Max"); len(c) != 0 {
		t.Fatalf("engine Health: %v", c)
	}
	tab := healthTable(healthComp("gameplay.Health", true, "Max"), healthComp("rpg.Health", true, "Max"))
	if c := checkHealth(t, tab, "Max"); c[diag.BadTypeArg] != 1 {
		t.Fatalf("two engine Health: %v, want %s", c, diag.BadTypeArg)
	}
}

func TestShortNameFullNameAlwaysWorks(t *testing.T) {
	tab := healthTable(healthComp("gameplay.Health", true, "Max"), healthComp("game.Health", false, "Hits"))
	src := "package game\n\nscript P {\n\tfunc Ready() {\n\t\th := node.Get<gameplay.Health>()\n\t\tif h != nil {\n\t\t\th.Max = 1\n\t\t}\n\t}\n}\n"
	sm := source.NewManager()
	f, ds := parser.ParseSource(sm, "game/main.aes", []byte(src))
	a := Check(sm, map[string][]*ast.File{"game": {f}}, tab, Options{})
	if c := codes(append(ds, a.Diagnostics...)); len(c) != 0 {
		t.Fatalf("full name: %v", c)
	}
}
