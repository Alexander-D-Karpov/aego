package playground

import (
	"strings"
	"testing"

	"aego/script/vm"
)

const loopKick = `package boxes

script Kick {
	func Update(dt float) {
		n := 0
		for n >= 0 {
			n++
		}
	}
}
`

const heapKick = `package boxes

script Kick {
	func Update(dt float) {
		s := "ab"
		for s != "" {
			s = s + s
		}
	}
}
`

const saveScript = `package demo

import "aego/save"

script S {
	func Ready() {
		save.Save("slot")
	}
}
`

func faultOf(t *testing.T, p *Playground, code string) Fault {
	t.Helper()
	res := p.Play(ScenePrefix+"boxes", map[string]string{"main.aes": code})
	if !res.Started {
		t.Fatalf("did not start: %s %+v", res.Error, res.Diagnostics)
	}
	if err := p.Game().Step(3); err != nil {
		t.Fatal(err)
	}
	for _, f := range p.Poll().Faults {
		if f.Code == vm.CodeBudget {
			return f
		}
	}
	t.Fatal("the script was not stopped by the sandbox")
	return Fault{}
}

func TestSandboxInstructionBudget(t *testing.T) {
	p := newHeadless(t)
	faultOf(t, p, loopKick)
	if !p.Poll().Stats.Running {
		t.Fatal("the game stopped with the script")
	}
}

func TestSandboxHeapLimit(t *testing.T) {
	p := newHeadless(t)
	if f := faultOf(t, p, heapKick); !strings.Contains(f.Message, "heap") {
		t.Fatalf("fault %+v", f)
	}
}

// The sandbox has aego/save for the Persist demo; its saves stay in memory, never in the visitor's storage.
func TestSandboxSavesStayInMemory(t *testing.T) {
	p := newHeadless(t)
	res := p.Run(map[string]string{"main.aes": saveScript}, 10)
	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			t.Fatalf("aego/save does not compile in the sandbox: %s", d.Message)
		}
	}
	for _, l := range res.Log {
		if strings.Contains(l.Text, "saves are not available") {
			t.Fatal("the sandbox has no save store")
		}
	}
}
