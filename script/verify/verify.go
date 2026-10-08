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
		// Element strings must round-trip through abi; for a non-struct the
		// declared width must equal the element's slot count.
		if elem, ok := abi.Parse(t.Elem); !ok {
			v.bad("type t%d has an unparsable element %q", i, t.Elem)
		} else if elem.Kind != abi.Struct && int(t.Width) != elem.Slots() {
			v.bad("type t%d width %d does not match element %s (%d slots)", i, t.Width, t.Elem, elem.Slots())
		}
	}
	for i, imp := range m.Imports {
		if imp.Sig.Window() > 255 {
			v.bad("import h%d %s.%s needs more than 255 registers", i, imp.Module, imp.Name)
		}
	}
	v.unique()
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

// unique rejects duplicate names: the linker resolves functions and externs by
// Name, scripts by FullName, so duplicates make linking non-deterministic.
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

func (v *verifier) script(si int) {
	s := &v.m.Scripts[si]
	heap := map[uint16]bool{}
	for _, h := range s.HeapSlots {
		if h >= s.Slots {
			v.bad("script %s heap slot %d is out of %d", s.FullName, h, s.Slots)
		}
		heap[h] = true
	}
	// Slot 0 of every instance is node (D97); fields are laid out consecutively
	// from slot 1, a contiguous sequence that must end exactly at Slots.
	next := 1
	for _, f := range s.Fields {
		if f.Slots == 0 || int(f.Slot) != next {
			v.bad("script %s field %s at slot %d, expected %d", s.FullName, f.Name, f.Slot, next)
		}
		v.fieldHeap(s, f, heap)
		next += int(f.Slots)
	}
	if next != int(s.Slots) {
		v.bad("script %s fields occupy %d slots of %d", s.FullName, next, s.Slots)
	}
	v.lifecycle(si, "Ready", s.Ready, 0)
	v.lifecycle(si, "FixedUpdate", s.FixedUpdate, 1)
	v.lifecycle(si, "Update", s.Update, 1)
}

// fieldHeap checks HeapSlots against the field's type: a String/Slice field's
// slot must be a GC root; a definitely-non-heap field's slots must not be.
// A struct flattens nested string/slice slots into its range and their offsets
// are not recoverable from the type string, so struct fields are skipped.
func (v *verifier) fieldHeap(s *bytecode.ScriptDef, f bytecode.FieldDef, heap map[uint16]bool) {
	t, ok := abi.Parse(f.Type)
	if !ok {
		v.bad("script %s field %s has an unparsable type %q", s.FullName, f.Name, f.Type)
		return
	}
	switch t.Kind {
	case abi.String, abi.Slice:
		if !heap[f.Slot] {
			v.bad("script %s field %s is a heap value but slot %d is not a GC root", s.FullName, f.Name, f.Slot)
		}
	case abi.Struct:
	default:
		for sl := int(f.Slot); sl < int(f.Slot)+int(f.Slots); sl++ {
			if heap[uint16(sl)] {
				v.bad("script %s field %s is not a heap value but slot %d is a GC root", s.FullName, f.Name, sl)
			}
		}
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
	if f.Script != int32(si) || f.Params != params || f.Results != 0 {
		v.bad("script %s %s refers to %s with the wrong owner or arity", s.FullName, name, f.Name)
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
	}
}

func (v *verifier) special(f *bytecode.Function, pc int, in bytecode.Instr, bad func(int, string, ...any)) {
	m := v.m
	regs := int(f.Registers)
	a, b, c, bx := int(in.A()), int(in.B()), int(in.C()), int(in.Bx())
	window := func(n int) {
		if a+n > regs {
			bad(pc, "call window r%d..r%d exceeds %d registers", a, a+n-1, regs)
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
			window(max(int(callee.Params), int(callee.Results)))
			// A CALL passes the caller's self; calling another script's method
			// (or any method from a package function) would hand it the wrong
			// instance. CALLX's equivalent is enforced by the linker.
			if callee.Script != bytecode.NoScript && callee.Script != f.Script {
				bad(pc, "CALL to method %s of another script", callee.Name)
			}
		}
	case bytecode.CALLX:
		if bx < len(m.Externs) {
			e := m.Externs[bx]
			window(max(int(e.Params), int(e.Results)))
		}
	case bytecode.CALLH:
		if bx < len(m.Imports) {
			window(m.Imports[bx].Sig.Window())
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
