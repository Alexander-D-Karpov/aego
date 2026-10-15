package harness_test

import (
	scriptstdlib "aego/modules/scripting/stdlib"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/script/bind"
	"aego/script/diag"
	"aego/script/harness"
	"aego/script/stdlib"
	"aego/script/vm"
	"aego/script/workspace"
)

const calc = "package calc\n\nfunc Add(a int, b int) int {\n\treturn a + b\n}\n\nfunc TestNotRun() {\n}\n"

const calcTest = `package calc

import "aego/testing"

func TestAdd() {
	testing.Equal(Add(2, 3), 5)
}

func TestBroken() {
	testing.Equal(Add(2, 2), 5)
}

func TestSpin() {
	for {
	}
}

func helper() {
}
`

func write(t *testing.T, root, rel, src string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func project(t *testing.T) (*workspace.Workspace, *bind.Registry) {
	t.Helper()
	reg := bind.New()
	scriptstdlib.Install(reg, stdlib.Options{Seed: 1, Testing: true})
	if err := reg.Freeze(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	write(t, root, "scripts/calc/calc.aes", calc)
	write(t, root, "scripts/calc/calc_test.aes", calcTest)
	ws, err := workspace.Open(workspace.Config{Root: root, Module: "game", Bindings: reg.Table()})
	if err != nil {
		t.Fatal(err)
	}
	return ws, reg
}

func TestRun(t *testing.T) {
	ws, reg := project(t)
	g, ds := ws.BuildWith(workspace.BuildOptions{IncludeTests: true})
	if diag.HasErrors(ds) {
		var b bytes.Buffer
		diag.Print(&b, g.Sources, ds)
		t.Fatal(b.String())
	}
	rep, err := harness.Run(g, reg, &bind.Ctx{}, harness.Options{VM: vm.Options{Budget: 100_000}})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Passed != 1 || rep.Failed != 2 || len(rep.Results) != 3 {
		var b bytes.Buffer
		rep.Print(&b, true)
		t.Fatalf("report:\n%s", b.String())
	}
	for _, r := range rep.Results {
		switch r.Name {
		case "TestBroken":
			top := r.Fault.Stack[0]
			if top.File != "scripts/calc/calc_test.aes" || top.Line != 10 || r.Fault.Message != "got 4, want 5" {
				t.Fatalf("broken: %+v %+v", r.Fault, top)
			}
		case "TestSpin":
			if r.Fault == nil || r.Fault.Code != vm.CodeBudget {
				t.Fatalf("spin: %+v", r.Fault)
			}
		}
	}
	filtered, err := harness.Run(g, reg, &bind.Ctx{}, harness.Options{VM: vm.Options{Budget: 100_000}, Filter: "Add"})
	if err != nil || len(filtered.Results) != 1 || filtered.Passed != 1 {
		t.Fatalf("filter: %+v %v", filtered, err)
	}
}

func TestTestFilesStayOutOfPlay(t *testing.T) {
	ws, _ := project(t)
	before := ws.Committed().ID
	if _, ds := ws.BuildWith(workspace.BuildOptions{IncludeTests: true}); diag.HasErrors(ds) {
		t.Fatal("test build failed")
	}
	if ws.Committed().ID != before {
		t.Fatal("a test build changed the committed generation")
	}
	g, _ := ws.Build()
	if g.Modules["game/calc"].FuncIndex("TestAdd") >= 0 {
		t.Fatal("a _test.aes function reached the committed generation")
	}
}
