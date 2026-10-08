package harness_test

import (
	"bytes"
	"testing"

	"aego/script/bind"
	"aego/script/build"
	"aego/script/diag"
	"aego/script/harness"
	"aego/script/stdlib"
	"aego/script/vm"
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

func TestRun(t *testing.T) {
	reg := bind.New()
	stdlib.Install(reg, stdlib.Options{Seed: 1, Testing: true})
	if err := reg.Freeze(); err != nil {
		t.Fatal(err)
	}
	res := build.Build(map[string][]build.File{"game/calc": {
		{Path: "scripts/calc/calc.aes", Src: []byte(calc)},
		{Path: "scripts/calc/calc_test.aes", Src: []byte(calcTest)},
	}}, reg.Surface(), build.Options{Tests: true})
	if res.HasErrors() {
		var b bytes.Buffer
		diag.Print(&b, res.Sources, res.Diagnostics)
		t.Fatal(b.String())
	}
	rep, err := harness.Run(res, reg, &bind.Ctx{}, harness.Options{VM: vm.Options{Budget: 100_000}})
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
}
