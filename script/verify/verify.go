package verify

import (
	"fmt"

	"aego/script/abi"
	"aego/script/bytecode"
	"aego/script/diag"
	"aego/script/source"
)

type verifier struct {
	m     *bytecode.Module
	diags []diag.Diagnostic
}

func Module(m *bytecode.Module) []diag.Diagnostic {
	v := &verifier{m: m}
	if string(m.Magic[:]) != bytecode.Magic {
		v.bad("bad magic %q", m.Magic[:])
	}
	if m.Version != bytecode.Version {
		v.bad("bytecode version %d is not supported", m.Version)
	}
	if m.Package == "" {
		v.bad("module has no package name")
	}
	if m.LangVersion > bytecode.LangVersion {
		v.bad("language version %d is newer than %d", m.LangVersion, bytecode.LangVersion)
	}
	for i, t := range m.Types {
		if t.Width == 0 || t.Width > 32 || t.Mask>>t.Width != 0 {
			v.bad("type t%d has width %d and mask %#x", i, t.Width, t.Mask)
		}
		if elem, ok := abi.Parse(t.Elem); !ok {
			v.bad("type t%d has an unparsable element %q", i, t.Elem)
		} else if elem.Kind != abi.Struct && int(t.Width) != elem.Slots() {
			v.bad("type t%d width %d does not match element %s (%d slots)", i, t.Width, t.Elem, elem.Slots())
		}
	}
	for i, imp := range m.Imports {
		for _, t := range imp.Sig.Params {
			v.hostType(i, imp, t)
		}
		v.hostType(i, imp, imp.Sig.Result)
		if m.Window(imp.Sig) > bytecode.MaxRegisters {
			v.bad("import h%d %s.%s needs more than 255 registers", i, imp.Module, imp.Name)
		}
	}
	v.unique()
	v.tables()
	for i := range m.Scripts {
		v.script(i)
	}
	for i := range m.Functions {
		v.function(i)
	}
	return v.diags
}

func (v *verifier) bad(format string, args ...any) {
	args = append([]any{v.m.Package}, args...)
	v.diags = append(v.diags, diag.Errorf(diag.BadBytecode, source.Span{}, "module %s: "+format, args...))
}

func (v *verifier) unique() {
	m := v.m
	fn := map[string]bool{}
	for i := range m.Functions {
		if n := m.Functions[i].Name; fn[n] {
			v.bad("function %s is defined more than once", n)
		} else {
			fn[n] = true
		}
	}
	sc := map[string]bool{}
	for i := range m.Scripts {
		if n := m.Scripts[i].FullName; sc[n] {
			v.bad("script %s is defined more than once", n)
		} else {
			sc[n] = true
		}
	}
	en := map[string]bool{}
	for i := range m.Enums {
		e := &m.Enums[i]
		if en[e.Name] {
			v.bad("enum %s is defined more than once", e.Name)
		}
		en[e.Name] = true
		mem := map[string]bool{}
		for _, name := range e.Members {
			if mem[name] {
				v.bad("enum %s has a duplicate member %s", e.Name, name)
			}
			mem[name] = true
		}
	}
}

func (v *verifier) tables() {
	m := v.m
	for i, r := range m.ScriptRefs {
		if r == "" {
			v.bad("script ref s%d is empty", i)
		}
	}
	if len(m.XFields) > bytecode.MaxScriptFieldRefs {
		v.bad("module references %d script fields, more than %d", len(m.XFields), bytecode.MaxScriptFieldRefs)
	}
	for i, x := range m.XFields {
		if int(x.Ref) >= len(m.ScriptRefs) {
			v.bad("xfield f%d refers to missing script ref s%d", i, x.Ref)
		}
		if x.Name == "" || x.Slots == 0 {
			v.bad("xfield f%d is malformed", i)
		}
		if _, ok := abi.Parse(x.Type); !ok {
			v.bad("xfield f%d has an unparsable type %q", i, x.Type)
		}
	}
	for i, x := range m.XMethods {
		if int(x.Ref) >= len(m.ScriptRefs) {
			v.bad("xmethod m%d refers to missing script ref s%d", i, x.Ref)
		}
		if x.Name == "" || int(x.Params)+1 > bytecode.MaxRegisters {
			v.bad("xmethod m%d is malformed", i)
		}
	}
	for i, c := range m.Connects {
		if c.Source == "" || c.Signal == "" || c.Target == "" || c.Handler == "" {
			v.bad("connect c%d is malformed", i)
		}
	}
}

func (v *verifier) script(si int) {
	s := &v.m.Scripts[si]
	if s.Pause > bytecode.PauseWhenPaused {
		v.bad("script %s has an unknown pause mode %d", s.FullName, s.Pause)
	}
	for _, h := range s.HeapSlots {
		if h >= s.Slots {
			v.bad("script %s heap slot %d is out of %d", s.FullName, h, s.Slots)
		}
	}
	next := 1
	for _, f := range s.Fields {
		if f.Slots == 0 || int(f.Slot) != next {
			v.bad("script %s field %s at slot %d, expected %d", s.FullName, f.Name, f.Slot, next)
		}
		next += int(f.Slots)
	}
	if next != int(s.Slots) {
		v.bad("script %s fields occupy %d slots of %d", s.FullName, next, s.Slots)
	}
	local := make(map[string]*bytecode.StructDef, len(v.m.Structs))
	for i := range v.m.Structs {
		local[v.m.Structs[i].Name] = &v.m.Structs[i]
	}
	for _, msg := range ScriptRoots(s, local, false) {
		v.bad("%s", msg)
	}
	seen := map[string]bool{}
	for _, sg := range s.Signals {
		if sg.Name == "" || seen[sg.Name] {
			v.bad("script %s has an empty or duplicate signal %q", s.FullName, sg.Name)
		}
		seen[sg.Name] = true
		if int(sg.Slots) < len(sg.Params) {
			v.bad("script %s signal %s has %d params in %d slots", s.FullName, sg.Name, len(sg.Params), sg.Slots)
		}
		for _, p := range sg.Params {
			if p.Kind == abi.Void {
				v.bad("script %s signal %s has a void parameter", s.FullName, sg.Name)
			}
		}
	}
	v.lifecycle(si, "Ready", s.Ready, 0)
	v.lifecycle(si, "FixedUpdate", s.FixedUpdate, 1)
	v.lifecycle(si, "Update", s.Update, 1)
	v.lifecycle(si, "Reloaded", s.Reloaded, 0)
	events := map[string]bool{}
	for _, h := range s.Events {
		if h.Name == "" || events[h.Name] {
			v.bad("script %s has an empty or duplicate event handler %q", s.FullName, h.Name)
		}
		events[h.Name] = true
		if h.Func < 0 || int(h.Func) >= len(v.m.Functions) {
			v.bad("script %s event %s refers to missing function %d", s.FullName, h.Name, h.Func)
			continue
		}
		f := &v.m.Functions[h.Func]
		if f.Script != int32(si) || f.Results != 0 || f.Coroutine {
			v.bad("script %s event %s refers to %s with the wrong owner, result or kind", s.FullName, h.Name, f.Name)
		}
	}
}

func (v *verifier) hostType(i int, imp bytecode.HostImport, t abi.TypeRef) {
	if t.Kind == abi.Slice && t.Elem != nil {
		t = *t.Elem
	}
	if t.Kind == abi.Struct && v.m.Struct(t.Name) == nil {
		v.bad("import h%d %s.%s uses struct %s, which has no layout in the module", i, imp.Module, imp.Name, t.Name)
	}
}

func (v *verifier) lifecycle(si int, name string, idx int32, params uint8) {
	if idx == bytecode.NoScript {
		return
	}
	s := &v.m.Scripts[si]
	if idx < 0 || int(idx) >= len(v.m.Functions) {
		v.bad("script %s %s refers to missing function %d", s.FullName, name, idx)
		return
	}
	f := &v.m.Functions[idx]
	if f.Script != int32(si) || f.Params != params || f.Results != 0 || f.Coroutine {
		v.bad("script %s %s refers to %s with the wrong owner, arity or kind", s.FullName, name, f.Name)
	}
}

func (v *verifier) function(fi int) {
	m := v.m
	f := &m.Functions[fi]
	bad := func(pc int, format string, args ...any) {
		v.bad("%s pc %d: %s", f.Name, pc, fmt.Sprintf(format, args...))
	}
	regs := int(f.Registers)
	if int(f.Params) > regs || int(f.Results) > regs {
		bad(-1, "params %d or results %d exceed %d registers", f.Params, f.Results, regs)
	}
	if f.Script < bytecode.NoScript || int(f.Script) >= len(m.Scripts) {
		bad(-1, "script index %d is out of range", f.Script)
	}
	if f.Coroutine && (f.Script == bytecode.NoScript || f.Results != 0) {
		bad(-1, "a coroutine must be a script method without results")
	}
	if len(f.Code) == 0 {
		bad(-1, "function has no code")
		return
	}
	if len(f.Code) > 1<<15 {
		bad(-1, "function has %d instructions, more than %d", len(f.Code), 1<<15)
	}
	for pc, in := range f.Code {
		op := in.Op()
		if !op.Valid() {
			bad(pc, "invalid opcode %d", uint8(op))
			continue
		}
		info := op.Info()
		v.operand(f, pc, info.A, int(in.A()), bad)
		switch info.Format {
		case bytecode.FormatABC:
			v.operand(f, pc, info.B, int(in.B()), bad)
			v.operand(f, pc, info.C, int(in.C()), bad)
		case bytecode.FormatABx:
			v.operand(f, pc, info.B, int(in.Bx()), bad)
		default:
			v.operand(f, pc, info.B, int(in.SBx()), bad)
		}
		v.special(f, pc, in, bad)
	}
	switch f.Code[len(f.Code)-1].Op() {
	case bytecode.RET, bytecode.JMP, bytecode.FAULT:
	default:
		bad(len(f.Code)-1, "execution can fall off the end of the function")
	}
	prev := -1
	for _, l := range f.Lines {
		if int(l.PC) <= prev || int(l.PC) >= len(f.Code) {
			bad(int(l.PC), "line table is not increasing inside the code")
			break
		}
		prev = int(l.PC)
	}
}

func (v *verifier) operand(f *bytecode.Function, pc int, k bytecode.Operand, x int, bad func(int, string, ...any)) {
	m := v.m
	switch k {
	case bytecode.OpReg:
		if x >= int(f.Registers) {
			bad(pc, "register r%d is out of %d", x, f.Registers)
		}
	case bytecode.OpConst:
		if x >= len(f.Consts) {
			bad(pc, "constant k%d is out of %d", x, len(f.Consts))
		}
	case bytecode.OpJump:
		if t := pc + 1 + x; t < 0 || t >= len(f.Code) {
			bad(pc, "jump target %d is outside the function", t)
		}
	case bytecode.OpFunc:
		if x >= len(m.Functions) {
			bad(pc, "function f%d does not exist", x)
		}
	case bytecode.OpExtern:
		if x >= len(m.Externs) {
			bad(pc, "extern x%d does not exist", x)
		}
	case bytecode.OpHost:
		if x >= len(m.Imports) {
			bad(pc, "host import h%d does not exist", x)
		}
	case bytecode.OpType:
		if x >= len(m.Types) {
			bad(pc, "type t%d does not exist", x)
		}
	case bytecode.OpSelf:
		switch {
		case f.Script < 0 || int(f.Script) >= len(m.Scripts):
			bad(pc, "self field access outside a script method")
		case x >= int(m.Scripts[f.Script].Slots):
			bad(pc, "self slot %d is out of %d", x, m.Scripts[f.Script].Slots)
		}
	case bytecode.OpLane:
		if x > 3 {
			bad(pc, "lane %d does not exist", x)
		}
	case bytecode.OpBool:
		if x > 1 {
			bad(pc, "bool operand %d", x)
		}
	case bytecode.OpYield:
		if x > int(bytecode.MaxWait) {
			bad(pc, "yield kind %d does not exist", x)
		}
	case bytecode.OpSignal:
		switch {
		case f.Script < 0 || int(f.Script) >= len(m.Scripts):
			bad(pc, "signal access outside a script method")
		case x >= len(m.Scripts[f.Script].Signals):
			bad(pc, "signal g%d is out of %d", x, len(m.Scripts[f.Script].Signals))
		}
	case bytecode.OpConnect:
		if x >= len(m.Connects) {
			bad(pc, "connect c%d does not exist", x)
		}
	case bytecode.OpScriptRef:
		if x >= len(m.ScriptRefs) {
			bad(pc, "script ref s%d does not exist", x)
		}
	case bytecode.OpXField:
		if x >= len(m.XFields) {
			bad(pc, "xfield f%d does not exist", x)
		}
	case bytecode.OpXMethod:
		if x >= len(m.XMethods) {
			bad(pc, "xmethod m%d does not exist", x)
		}
	}
}

func (v *verifier) special(f *bytecode.Function, pc int, in bytecode.Instr, bad func(int, string, ...any)) {
	m := v.m
	regs := int(f.Registers)
	a, b, c, bx := int(in.A()), int(in.B()), int(in.C()), int(in.Bx())
	window := func(at, n int) {
		if at+n > regs {
			bad(pc, "window r%d..r%d exceeds %d registers", at, at+n-1, regs)
		}
	}
	switch in.Op() {
	case bytecode.MOVEN:
		if a+c > regs || b+c > regs {
			bad(pc, "MOVEN of %d registers exceeds %d", c, regs)
		}
	case bytecode.VEC2, bytecode.VEC3, bytecode.VEC4:
		n := int(in.Op()-bytecode.VEC2) + 2
		if b+n > regs {
			bad(pc, "vector source r%d..r%d exceeds %d registers", b, b+n-1, regs)
		}
	case bytecode.CALL:
		if bx < len(m.Functions) {
			callee := &m.Functions[bx]
			window(a, max(int(callee.Params), int(callee.Results)))
			if callee.Script != bytecode.NoScript && callee.Script != f.Script {
				bad(pc, "CALL to method %s of another script", callee.Name)
			}
			if callee.Coroutine {
				bad(pc, "CALL to coroutine %s, use START", callee.Name)
			}
		}
	case bytecode.CALLX:
		if bx < len(m.Externs) {
			e := m.Externs[bx]
			window(a, max(int(e.Params), int(e.Results)))
		}
	case bytecode.CALLH:
		if bx < len(m.Imports) {
			window(a, m.Window(m.Imports[bx].Sig))
		}
	case bytecode.START:
		if bx < len(m.Functions) {
			callee := &m.Functions[bx]
			if !callee.Coroutine || callee.Script != f.Script || f.Script == bytecode.NoScript {
				bad(pc, "START of %s, which is not a coroutine of this script", callee.Name)
			}
			window(a, max(int(callee.Params), 1))
		}
	case bytecode.YIELD:
		if !f.Coroutine {
			bad(pc, "YIELD outside a coroutine")
		}
	case bytecode.EMIT:
		if f.Script >= 0 && int(f.Script) < len(m.Scripts) && bx < len(m.Scripts[f.Script].Signals) {
			window(a, int(m.Scripts[f.Script].Signals[bx].Slots))
		}
	case bytecode.CONNECT:
		window(a, 3)
	case bytecode.GETXF:
		if c < len(m.XFields) {
			window(a, int(m.XFields[c].Slots))
		}
	case bytecode.SETXF:
		if c < len(m.XFields) {
			window(b, int(m.XFields[c].Slots))
		}
	case bytecode.CALLR:
		if bx < len(m.XMethods) {
			x := m.XMethods[bx]
			window(a, max(int(x.Params)+1, int(x.Results)))
		}
	case bytecode.SELFREF:
		if f.Script == bytecode.NoScript {
			bad(pc, "SELFREF outside a script method")
		}
	case bytecode.RET:
		if b != int(f.Results) || a+b > regs {
			bad(pc, "RET returns %d slots from r%d, function returns %d", b, a, f.Results)
		}
	case bytecode.FAULT:
		if bx < len(f.Consts) && f.Consts[bx].Kind != bytecode.KString {
			bad(pc, "FAULT message must be a string constant")
		}
	}
}
