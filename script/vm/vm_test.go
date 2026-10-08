package vm_test

import (
	"strings"
	"testing"

	"aego/script/abi"
	bc "aego/script/bytecode"
	"aego/script/vm"
)

func fn(name string, params, results, regs int, code ...bc.Instr) bc.Function {
	lines := make([]bc.LineEntry, len(code))
	for i := range code {
		lines[i] = bc.LineEntry{PC: uint32(i), Line: uint32(i + 1)}
	}
	return bc.Function{Name: name, File: "test.aes", Params: uint8(params), Results: uint8(results), Registers: uint8(regs), Script: -1, Code: code, Lines: lines}
}

func module(funcs ...bc.Function) *bc.Module {
	m := bc.NewModule("test")
	m.Functions = funcs
	return m
}

func load(t *testing.T, mod *bc.Module, hosts map[string]vm.HostFunc) (*vm.Machine, *vm.Module) {
	t.Helper()
	m := vm.New(vm.Options{Budget: 10_000_000})
	lm, err := m.Load(mod, func(imp bc.HostImport) (vm.HostFunc, bool) {
		h, ok := hosts[imp.Module+"."+imp.Name]
		return h, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	return m, lm
}

func run(t *testing.T, mod *bc.Module, name string, args ...vm.Value) ([]vm.Value, *vm.Fault, *vm.Machine) {
	t.Helper()
	m, lm := load(t, mod, nil)
	out, fl := m.Call(lm.Func(name), nil, args)
	return append([]vm.Value(nil), out...), fl, m
}

func want(t *testing.T, out []vm.Value, fl *vm.Fault, v int64) {
	t.Helper()
	if fl != nil {
		t.Fatalf("fault: %v", fl)
	}
	if len(out) != 1 || out[0].Int() != v {
		t.Fatalf("got %v want %d", out, v)
	}
}

func sumFunc() bc.Function {
	return fn("sum", 1, 1, 5,
		bc.AsBx(bc.LOADI, 1, 1),
		bc.AsBx(bc.LOADI, 2, 0),
		bc.AsBx(bc.LOADI, 3, 1),
		bc.ABC(bc.LEI, 4, 1, 0),
		bc.AsBx(bc.JMPNOT, 4, 3),
		bc.ABC(bc.ADDI, 2, 2, 1),
		bc.ABC(bc.ADDI, 1, 1, 3),
		bc.AsBx(bc.JMP, 0, -5),
		bc.ABC(bc.RET, 2, 1, 0),
	)
}

func fibFunc() bc.Function {
	return fn("fib", 1, 1, 5,
		bc.AsBx(bc.LOADI, 1, 2),
		bc.ABC(bc.LTI, 2, 0, 1),
		bc.AsBx(bc.JMPNOT, 2, 1),
		bc.ABC(bc.RET, 0, 1, 0),
		bc.AsBx(bc.LOADI, 1, 1),
		bc.ABC(bc.SUBI, 3, 0, 1),
		bc.ABx(bc.CALL, 3, 0),
		bc.AsBx(bc.LOADI, 1, 2),
		bc.ABC(bc.SUBI, 4, 0, 1),
		bc.ABx(bc.CALL, 4, 0),
		bc.ABC(bc.ADDI, 3, 3, 4),
		bc.ABC(bc.RET, 3, 1, 0),
	)
}

func TestIntegerArithmetic(t *testing.T) {
	f := fn("f", 2, 1, 6,
		bc.ABC(bc.ADDI, 2, 0, 1),
		bc.ABC(bc.SUBI, 3, 0, 1),
		bc.ABC(bc.MULI, 2, 2, 3),
		bc.AsBx(bc.LOADI, 4, 3),
		bc.ABC(bc.DIVI, 2, 2, 4),
		bc.AsBx(bc.LOADI, 5, 5),
		bc.ABC(bc.MODI, 2, 2, 5),
		bc.ABC(bc.RET, 2, 1, 0),
	)
	out, fl, _ := run(t, module(f), "f", vm.Int(10), vm.Int(4))
	want(t, out, fl, 3)
}

func TestFloatsAndConversions(t *testing.T) {
	f := fn("f", 1, 1, 3,
		bc.ABC(bc.ITOF, 1, 0, 0),
		bc.ABx(bc.LOADK, 2, 0),
		bc.ABC(bc.MULF, 1, 1, 2),
		bc.ABx(bc.LOADK, 2, 1),
		bc.ABC(bc.ADDF, 1, 1, 2),
		bc.ABC(bc.RET, 1, 1, 0),
	)
	f.Consts = []bc.Const{bc.FloatK(0.5), bc.FloatK(1.25)}
	out, fl, _ := run(t, module(f), "f", vm.Int(3))
	if fl != nil || out[0].Float() != 2.75 {
		t.Fatalf("%v %v", out, fl)
	}
}

func TestLoopAndRecursion(t *testing.T) {
	out, fl, _ := run(t, module(sumFunc()), "sum", vm.Int(100))
	want(t, out, fl, 5050)
	out, fl, _ = run(t, module(fibFunc()), "fib", vm.Int(20))
	want(t, out, fl, 6765)
}

func TestFaults(t *testing.T) {
	_, fl, _ := run(t, module(fn("loop", 0, 0, 1, bc.AsBx(bc.JMP, 0, -1))), "loop")
	if fl == nil || fl.Code != vm.CodeBudget {
		t.Fatalf("budget: %v", fl)
	}
	_, fl, _ = run(t, module(fn("deep", 0, 0, 1, bc.ABx(bc.CALL, 0, 0), bc.ABC(bc.RET, 0, 0, 0))), "deep")
	if fl == nil || fl.Code != vm.CodeCallDepth {
		t.Fatalf("depth: %v", fl)
	}
	_, fl, _ = run(t, module(fn("div", 2, 1, 3, bc.ABC(bc.DIVI, 2, 0, 1), bc.ABC(bc.RET, 2, 1, 0))), "div", vm.Int(1), vm.Int(0))
	if fl == nil || fl.Code != vm.CodeDivZero {
		t.Fatalf("div: %v", fl)
	}
	bang := fn("bang", 0, 0, 1, bc.ABx(bc.FAULT, 0, 0))
	bang.Consts = []bc.Const{bc.StringK("boom")}
	_, fl, _ = run(t, module(bang), "bang")
	if fl == nil || fl.Code != vm.CodeExplicit || fl.Message != "boom" {
		t.Fatalf("explicit: %v", fl)
	}
}

func TestFaultStack(t *testing.T) {
	outer := fn("outer", 0, 1, 1, bc.AsBx(bc.LOADI, 0, 0), bc.ABx(bc.CALL, 0, 1), bc.ABC(bc.RET, 0, 1, 0))
	inner := fn("inner", 0, 1, 2, bc.AsBx(bc.LOADI, 0, 1), bc.AsBx(bc.LOADI, 1, 0), bc.ABC(bc.DIVI, 0, 0, 1), bc.ABC(bc.RET, 0, 1, 0))
	_, fl, _ := run(t, module(outer, inner), "outer")
	if fl == nil || len(fl.Stack) != 2 {
		t.Fatalf("fault %v", fl)
	}
	if fl.Stack[0].Function != "inner" || fl.Stack[0].Line != 3 || fl.Stack[1].Function != "outer" || fl.Stack[1].Line != 2 {
		t.Fatalf("stack %+v", fl.Stack)
	}
	if !strings.Contains(fl.Error(), "inner at test.aes:3") {
		t.Fatal(fl.Error())
	}
}

func TestStrings(t *testing.T) {
	f := fn("s", 0, 1, 6,
		bc.ABx(bc.LOADK, 0, 0),
		bc.ABx(bc.LOADK, 1, 1),
		bc.ABC(bc.ADDS, 2, 0, 1),
		bc.ABx(bc.LOADK, 3, 2),
		bc.ABC(bc.EQS, 4, 2, 3),
		bc.ABC(bc.LENS, 5, 2, 0),
		bc.AsBx(bc.JMPNOT, 4, 1),
		bc.ABC(bc.RET, 5, 1, 0),
		bc.AsBx(bc.LOADI, 5, -1),
		bc.ABC(bc.RET, 5, 1, 0),
	)
	f.Consts = []bc.Const{bc.StringK("ab"), bc.StringK("cd"), bc.StringK("abcd")}
	out, fl, _ := run(t, module(f), "s")
	want(t, out, fl, 4)
}

func sliceModule() *bc.Module {
	f := fn("sl", 1, 1, 6,
		bc.ABC(bc.LOADNIL, 1, 0, 0),
		bc.ABx(bc.ENSURE, 1, 0),
		bc.AsBx(bc.LOADI, 2, 10),
		bc.ABC(bc.APPEND, 1, 1, 2),
		bc.AsBx(bc.LOADI, 2, 20),
		bc.ABC(bc.APPEND, 1, 1, 2),
		bc.AsBx(bc.LOADI, 3, 5),
		bc.AsBx(bc.LOADI, 4, 1),
		bc.ABC(bc.SETINDEX, 1, 4, 3),
		bc.ABC(bc.INDEX, 2, 1, 0),
		bc.ABC(bc.LEN, 5, 1, 0),
		bc.ABC(bc.ADDI, 2, 2, 5),
		bc.ABC(bc.RET, 2, 1, 0),
	)
	m := module(f)
	m.Types = []bc.TypeDef{{Elem: "int", Width: 1}}
	return m
}

func TestSlices(t *testing.T) {
	out, fl, _ := run(t, sliceModule(), "sl", vm.Int(1))
	want(t, out, fl, 7)
	_, fl, _ = run(t, sliceModule(), "sl", vm.Int(2))
	if fl == nil || fl.Code != vm.CodeIndex {
		t.Fatalf("bounds: %v", fl)
	}
}

func TestAppendCopyOnWrite(t *testing.T) {
	f := fn("cow", 0, 1, 6,
		bc.ABx(bc.NEWSLICE, 0, 0),
		bc.AsBx(bc.LOADI, 1, 1),
		bc.ABC(bc.APPEND, 0, 0, 1),
		bc.ABC(bc.MOVE, 2, 0, 0),
		bc.AsBx(bc.LOADI, 1, 2),
		bc.ABC(bc.APPEND, 2, 2, 1),
		bc.AsBx(bc.LOADI, 1, 3),
		bc.ABC(bc.APPEND, 3, 0, 1),
		bc.AsBx(bc.LOADI, 4, 1),
		bc.ABC(bc.INDEX, 5, 2, 4),
		bc.ABC(bc.RET, 5, 1, 0),
	)
	m := module(f)
	m.Types = []bc.TypeDef{{Elem: "int", Width: 1}}
	out, fl, _ := run(t, m, "cow")
	want(t, out, fl, 2)
}

func TestVectors(t *testing.T) {
	f := fn("v", 0, 1, 6,
		bc.ABx(bc.LOADK, 0, 0),
		bc.ABx(bc.LOADK, 1, 1),
		bc.ABC(bc.VEC2, 2, 0, 0),
		bc.ABC(bc.VADD, 3, 2, 2),
		bc.ABx(bc.LOADK, 4, 2),
		bc.ABC(bc.VMULS, 3, 3, 4),
		bc.ABC(bc.SETL, 3, 0, 0),
		bc.ABC(bc.GETL, 5, 3, 1),
		bc.ABC(bc.GETL, 4, 3, 0),
		bc.ABC(bc.ADDF, 5, 5, 4),
		bc.ABC(bc.RET, 5, 1, 0),
	)
	f.Consts = []bc.Const{bc.FloatK(1), bc.FloatK(2), bc.FloatK(0.5)}
	out, fl, _ := run(t, module(f), "v")
	if fl != nil || out[0].Float() != 3 {
		t.Fatalf("%v %v", out, fl)
	}
}

func TestSelfFields(t *testing.T) {
	f := fn("Tick", 0, 1, 2, bc.ABx(bc.GETSF, 0, 1), bc.AsBx(bc.LOADI, 1, 1), bc.ABC(bc.ADDI, 0, 0, 1), bc.ABx(bc.SETSF, 0, 1), bc.ABC(bc.RET, 0, 1, 0))
	f.Script = 0
	mod := module(f)
	mod.Scripts = []bc.ScriptDef{{FullName: "test.S", Slots: 2, Fields: []bc.FieldDef{{Name: "N", Type: "int", Slot: 1, Slots: 1}}, Ready: -1, FixedUpdate: -1, Update: -1}}
	m, lm := load(t, mod, nil)
	self := []vm.Value{{}, vm.Int(41)}
	out, fl := m.Call(lm.Func("Tick"), self, nil)
	want(t, out, fl, 42)
	if self[1].Int() != 42 {
		t.Fatal("self field was not written")
	}
}

func hostModule() *bc.Module {
	f := fn("h", 0, 1, 3, bc.AsBx(bc.LOADI, 1, 2), bc.AsBx(bc.LOADI, 2, 3), bc.ABx(bc.CALLH, 1, 0), bc.ABC(bc.RET, 1, 1, 0))
	m := module(f)
	m.Imports = []bc.HostImport{{Module: "t", Name: "add", Sig: abi.Sig(abi.T(abi.Int), abi.T(abi.Int), abi.T(abi.Int))}}
	return m
}

func TestHostCalls(t *testing.T) {
	m, lm := load(t, hostModule(), map[string]vm.HostFunc{"t.add": func(m *vm.Machine, w []vm.Value) error {
		w[0] = vm.Int(w[0].Int() + w[1].Int())
		return nil
	}})
	out, fl := m.Call(lm.Func("h"), nil, nil)
	want(t, out, fl, 5)
	m, lm = load(t, hostModule(), map[string]vm.HostFunc{"t.add": func(*vm.Machine, []vm.Value) error { panic("bad binding") }})
	_, fl = m.Call(lm.Func("h"), nil, nil)
	if fl == nil || fl.Code != vm.CodeNativePanic || !strings.Contains(fl.Message, "bad binding") || fl.Detail == "" {
		t.Fatalf("panic: %+v", fl)
	}
	if _, err := vm.New(vm.Options{}).Load(hostModule(), nil); err == nil {
		t.Fatal("missing host import must fail at load")
	}
}

func TestLoadVerifies(t *testing.T) {
	mod := module(fn("bad", 0, 0, 1, bc.ABC(bc.ADDI, 9, 0, 0), bc.ABC(bc.RET, 0, 0, 0)))
	if _, err := vm.New(vm.Options{}).Load(mod, nil); err == nil {
		t.Fatal("corrupt module was loaded")
	}
}

func TestExterns(t *testing.T) {
	a := bc.NewModule("game/a")
	a.Functions = []bc.Function{fn("Double", 1, 1, 1, bc.ABC(bc.ADDI, 0, 0, 0), bc.ABC(bc.RET, 0, 1, 0))}
	b := bc.NewModule("game/b")
	b.Externs = []bc.Extern{{Package: "game/a", Name: "Double", Params: 1, Results: 1}}
	b.Functions = []bc.Function{fn("run", 0, 1, 1, bc.AsBx(bc.LOADI, 0, 21), bc.ABx(bc.CALLX, 0, 0), bc.ABC(bc.RET, 0, 1, 0))}
	m := vm.New(vm.Options{})
	ma, err := m.Load(a, nil)
	if err != nil {
		t.Fatal(err)
	}
	mb, err := m.Load(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := vm.Link([]*vm.Module{ma, mb}); err != nil {
		t.Fatal(err)
	}
	out, fl := m.Call(mb.Func("run"), nil, nil)
	want(t, out, fl, 42)
}

type roots []vm.Value

func (r roots) MarkRoots(mk *vm.Marker) {
	for _, v := range r {
		mk.Mark(v)
	}
}

func TestGCKeepsLiveStrings(t *testing.T) {
	f := fn("gen", 1, 1, 5,
		bc.AsBx(bc.LOADI, 1, 0),
		bc.AsBx(bc.LOADI, 2, 1),
		bc.ABC(bc.LTI, 3, 1, 0),
		bc.AsBx(bc.JMPNOT, 3, 3),
		bc.ABC(bc.ITOS, 4, 1, 0),
		bc.ABC(bc.ADDI, 1, 1, 2),
		bc.AsBx(bc.JMP, 0, -5),
		bc.ABC(bc.RET, 4, 1, 0),
	)
	m, lm := load(t, module(f), nil)
	out, fl := m.Call(lm.Func("gen"), nil, []vm.Value{vm.Int(1000)})
	if fl != nil {
		t.Fatal(fl)
	}
	last := out[0]
	if n := m.Stats().HeapObjects; n != 1000 {
		t.Fatalf("heap objects before gc %d", n)
	}
	m.Collect(roots{last})
	if s, ok := m.Str(last); !ok || s != "999" || m.Stats().HeapObjects != 1 {
		t.Fatalf("after gc %q %v %d", s, ok, m.Stats().HeapObjects)
	}
	m.Collect(nil)
	if _, ok := m.Str(last); ok || m.Stats().HeapObjects != 0 {
		t.Fatal("unreachable string survived")
	}
}

func TestGCTracesSlices(t *testing.T) {
	f := fn("tags", 0, 1, 3,
		bc.ABx(bc.NEWSLICE, 0, 0),
		bc.AsBx(bc.LOADI, 1, 7),
		bc.ABC(bc.ITOS, 1, 1, 0),
		bc.ABC(bc.APPEND, 0, 0, 1),
		bc.AsBx(bc.LOADI, 1, 8),
		bc.ABC(bc.ITOS, 1, 1, 0),
		bc.ABC(bc.APPEND, 0, 0, 1),
		bc.AsBx(bc.LOADI, 2, 9),
		bc.ABC(bc.ITOS, 2, 2, 0),
		bc.ABC(bc.RET, 0, 1, 0),
	)
	mod := module(f)
	mod.Types = []bc.TypeDef{{Elem: "string", Width: 1, Mask: 1}}
	m, lm := load(t, mod, nil)
	out, fl := m.Call(lm.Func("tags"), nil, nil)
	if fl != nil {
		t.Fatal(fl)
	}
	s := out[0]
	m.Collect(roots{s})
	if n := m.Stats().HeapObjects; n != 3 {
		t.Fatalf("live objects %d, want slice and two strings", n)
	}
}

func TestCallDoesNotAllocate(t *testing.T) {
	m, lm := load(t, module(sumFunc()), nil)
	f := lm.Func("sum")
	args := []vm.Value{vm.Int(50)}
	m.Call(f, nil, args)
	if n := testing.AllocsPerRun(200, func() { m.Call(f, nil, args) }); n != 0 {
		t.Fatalf("call allocates %.1f objects", n)
	}
}
