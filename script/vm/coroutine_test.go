package vm_test

import (
	"testing"

	"aego/script/abi"
	bc "aego/script/bytecode"
	"aego/script/vm"
)

func coFn(name string, regs int, code ...bc.Instr) bc.Function {
	f := fn(name, 0, 0, regs, code...)
	f.Script, f.Coroutine = 0, true
	return f
}

func yieldNext() bc.Instr {
	return bc.ABC(bc.YIELD, 0, 0, 0)
}

func coModule() *bc.Module {
	ret := bc.ABC(bc.RET, 0, 0, 0)
	m := bc.NewModule("co")
	m.Imports = []bc.HostImport{{Module: "t", Name: "hit", Sig: abi.Sig(abi.T(abi.Void), abi.T(abi.Int))}}
	m.Scripts = []bc.ScriptDef{{
		FullName: "co.S", Slots: 2,
		Fields: []bc.FieldDef{{Name: "Id", Type: "int", Slot: 1, Slots: 1}},
		Ready:  -1, FixedUpdate: -1, Update: -1, Reloaded: -1,
	}}
	m.Functions = []bc.Function{
		coFn("S.Next", 1, yieldNext(), bc.ABx(bc.GETSF, 0, 1), bc.ABx(bc.CALLH, 0, 0), ret),
		coFn("S.Spin", 1, yieldNext(), bc.AsBx(bc.JMP, 0, -1)),
		coFn("S.Div", 2, yieldNext(), bc.AsBx(bc.LOADI, 0, 1), bc.AsBx(bc.LOADI, 1, 0), bc.ABC(bc.DIVI, 0, 0, 1), ret),
		coFn("S.Nested", 1, yieldNext(), bc.AsBx(bc.LOADI, 0, 20), bc.ABx(bc.CALL, 0, 4), bc.ABx(bc.CALLH, 0, 0), ret),
		fn("double", 1, 1, 1, bc.ABC(bc.ADDI, 0, 0, 0), bc.ABC(bc.RET, 0, 1, 0)),
	}
	return m
}

func coMachine(t *testing.T, budget uint64) (*vm.Machine, *vm.Module, *[]int64) {
	t.Helper()
	hits := &[]int64{}
	m := vm.New(vm.Options{Budget: budget})
	lm, err := m.Load(coModule(), func(bc.HostImport) (vm.HostFunc, bool) {
		return func(_ *vm.Machine, w []vm.Value) error {
			*hits = append(*hits, w[0].Int())
			return nil
		}, true
	})
	if err != nil {
		t.Fatal(err)
	}
	return m, lm, hits
}

func self(id int64) []vm.Value {
	return []vm.Value{{}, vm.Int(id)}
}

func TestCoroutineRunsToFirstYield(t *testing.T) {
	m, lm, hits := coMachine(t, 10_000)
	h, fl := m.Start(lm.Func("S.Next"), self(7), nil, 1)
	if fl != nil {
		t.Fatal(fl)
	}
	if len(*hits) != 0 {
		t.Fatal("coroutine ran past its first yield")
	}
	if st, ok := m.CoState(h); !ok || st != vm.CoSuspended {
		t.Fatalf("state %v %v", st, ok)
	}
	done, fl := m.Resume(h, self(7))
	if fl != nil || !done || len(*hits) != 1 || (*hits)[0] != 7 {
		t.Fatalf("resume %v %v %v", done, fl, *hits)
	}
	if _, ok := m.CoState(h); ok {
		t.Fatal("finished coroutine is still live")
	}
}

func TestCoroutineResumeSeesNewSelf(t *testing.T) {
	m, lm, hits := coMachine(t, 10_000)
	h, _ := m.Start(lm.Func("S.Next"), self(1), nil, 1)
	m.Resume(h, self(2))
	if len(*hits) != 1 || (*hits)[0] != 2 {
		t.Fatalf("hits %v", *hits)
	}
}

func TestCoroutineFaultBudgetCancelNested(t *testing.T) {
	m, lm, hits := coMachine(t, 1000)
	h, _ := m.Start(lm.Func("S.Div"), self(1), nil, 1)
	if _, fl := m.Resume(h, self(1)); fl == nil || fl.Code != vm.CodeDivZero {
		t.Fatalf("div %v", fl)
	}
	if _, ok := m.CoState(h); ok {
		t.Fatal("faulted coroutine is still live")
	}
	h, _ = m.Start(lm.Func("S.Spin"), self(1), nil, 1)
	if _, fl := m.Resume(h, self(1)); fl == nil || fl.Code != vm.CodeBudget {
		t.Fatalf("budget %v", fl)
	}
	h, _ = m.Start(lm.Func("S.Next"), self(1), nil, 1)
	m.Cancel(h)
	if done, fl := m.Resume(h, self(1)); !done || fl != nil || len(*hits) != 0 {
		t.Fatal("cancelled coroutine resumed")
	}
	if m.CoRunning(vm.CoValue(h)) {
		t.Fatal("cancelled coroutine reports running")
	}
	h, _ = m.Start(lm.Func("S.Nested"), self(1), nil, 1)
	if _, fl := m.Resume(h, self(1)); fl != nil || len(*hits) != 1 || (*hits)[0] != 40 {
		t.Fatalf("nested %v %v", fl, *hits)
	}
}

type deadScripts struct{}

func (deadScripts) Find(vm.Value, string) (vm.Value, error) {
	return vm.Value{}, nil
}

func (deadScripts) Instance(vm.Value, string) ([]vm.Value, vm.InstanceID, bool) {
	return nil, vm.NoInstance, false
}

func refModule() *bc.Module {
	m := bc.NewModule("v")
	m.Scripts = []bc.ScriptDef{{
		FullName: "v.S", Slots: 2,
		Fields: []bc.FieldDef{{Name: "N", Type: "int", Slot: 1, Slots: 1}},
		Ready:  1, FixedUpdate: -1, Update: -1, Reloaded: -1,
	}}
	m.ScriptRefs = []string{"v.S"}
	m.XFields = []bc.XFieldDef{{Ref: 0, Name: "N", Type: "int", Slots: 1}}
	ready := fn("S.Ready", 0, 0, 1, bc.ABC(bc.RET, 0, 0, 0))
	ready.Script = 0
	m.Functions = []bc.Function{
		fn("get", 1, 1, 1, bc.ABC(bc.GETXF, 0, 0, 0), bc.ABC(bc.RET, 0, 1, 0)),
		ready,
	}
	return m
}

func TestScriptRefOnDeadInstance(t *testing.T) {
	m := vm.New(vm.Options{})
	lm, err := m.Load(refModule(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := vm.Link([]*vm.Module{lm}); err != nil {
		t.Fatal(err)
	}
	m.Scripts = deadScripts{}
	if _, fl := m.Call(lm.Func("get"), nil, []vm.Value{{A: 1, B: 1}}); fl == nil || fl.Code != vm.CodeNil {
		t.Fatalf("fault %v", fl)
	}
}

func TestCallRLifecycleIsLinkError(t *testing.T) {
	mod := refModule()
	mod.XMethods = []bc.XMethodDef{{Ref: 0, Name: "Ready"}}
	m := vm.New(vm.Options{})
	lm, err := m.Load(mod, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := vm.Link([]*vm.Module{lm}); err == nil {
		t.Fatal("CALLR to a lifecycle method linked")
	}
}
