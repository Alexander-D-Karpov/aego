package bytecode

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

func Disassemble(w io.Writer, m *Module) {
	DisassembleSource(w, m, nil)
}

func short(name string) string {
	return name[strings.LastIndexByte(name, '.')+1:]
}

func DisassembleSource(w io.Writer, m *Module, source func(file string, line int) string) {
	var b strings.Builder
	fmt.Fprintf(&b, "module %s bytecode=%d lang=%d\n", m.Package, m.Version, m.LangVersion)
	for i, imp := range m.Imports {
		fmt.Fprintf(&b, "import h%d %s.%s %s\n", i, imp.Module, imp.Name, imp.Sig)
	}
	for i, e := range m.Externs {
		fmt.Fprintf(&b, "extern x%d %s.%s params=%d results=%d\n", i, e.Package, e.Name, e.Params, e.Results)
	}
	for i, t := range m.Types {
		fmt.Fprintf(&b, "type t%d []%s width=%d mask=%#x\n", i, t.Elem, t.Width, t.Mask)
	}
	for _, e := range m.Enums {
		fmt.Fprintf(&b, "enum %s %s\n", e.Name, strings.Join(e.Members, " "))
	}
	for i, r := range m.ScriptRefs {
		fmt.Fprintf(&b, "scriptref s%d %s\n", i, r)
	}
	for i, x := range m.XFields {
		fmt.Fprintf(&b, "xfield f%d s%d.%s %s slots=%d\n", i, x.Ref, x.Name, x.Type, x.Slots)
	}
	for i, x := range m.XMethods {
		fmt.Fprintf(&b, "xmethod m%d s%d.%s params=%d results=%d\n", i, x.Ref, x.Name, x.Params, x.Results)
	}
	for i, c := range m.Connects {
		fmt.Fprintf(&b, "connect c%d %s.%s -> %s.%s\n", i, c.Source, c.Signal, c.Target, c.Handler)
	}
	for i := range m.Scripts {
		s := &m.Scripts[i]
		fmt.Fprintf(&b, "script %s slots=%d ready=%d fixed=%d update=%d reloaded=%d\n", s.FullName, s.Slots, s.Ready, s.FixedUpdate, s.Update, s.Reloaded)
		for _, sg := range s.Signals {
			params := make([]string, len(sg.Params))
			for j, p := range sg.Params {
				params[j] = p.String()
			}
			fmt.Fprintf(&b, "  signal %s(%s) slots=%d\n", sg.Name, strings.Join(params, ", "), sg.Slots)
		}
		for _, h := range s.Events {
			fmt.Fprintf(&b, "  event %s f%d\n", h.Name, h.Func)
		}
		for _, f := range s.Fields {
			exp := ""
			if f.Export {
				exp = " export"
			}
			fmt.Fprintf(&b, "  field %s %s slot=%d+%d%s\n", f.Name, f.Type, f.Slot, f.Slots, exp)
		}
	}
	for i := range m.Functions {
		disasmFunc(&b, m, i, source)
	}
	io.WriteString(w, b.String())
}

func disasmFunc(b *strings.Builder, m *Module, fi int, source func(string, int) string) {
	f := &m.Functions[fi]
	fmt.Fprintf(b, "\nfunc f%d %s params=%d results=%d regs=%d", fi, f.Name, f.Params, f.Results, f.Registers)
	if f.Script >= 0 {
		fmt.Fprintf(b, " script=%d", f.Script)
	}
	if f.Coroutine {
		b.WriteString(" coroutine")
	}
	b.WriteByte('\n')
	last := -1
	for pc, in := range f.Code {
		if line := f.Line(pc); line > 0 && line != last {
			last = line
			text := ""
			if source != nil {
				text = strings.TrimSpace(source(f.File, line))
			}
			fmt.Fprintf(b, "  ; %s:%d %s\n", f.File, line, text)
		}
		fmt.Fprintf(b, "  %04d  %-8s %s\n", pc, in.Op(), operands(m, f, pc, in))
	}
}

func operands(m *Module, f *Function, pc int, in Instr) string {
	info := in.Op().Info()
	var parts []string
	add := func(k Operand, v int) {
		if s := operand(m, f, pc, k, v); s != "" {
			parts = append(parts, s)
		}
	}
	add(info.A, int(in.A()))
	switch info.Format {
	case FormatABC:
		add(info.B, int(in.B()))
		add(info.C, int(in.C()))
	case FormatABx:
		add(info.B, int(in.Bx()))
	default:
		add(info.B, int(in.SBx()))
	}
	return strings.Join(parts, " ")
}

func operand(m *Module, f *Function, pc int, k Operand, v int) string {
	n := strconv.Itoa(v)
	switch k {
	case OpReg:
		return "r" + n
	case OpInt, OpCount, OpField:
		return n
	case OpBool:
		return strconv.FormatBool(v != 0)
	case OpLane:
		return "." + "xyzw"[v&3:v&3+1]
	case OpConst:
		if v < len(f.Consts) {
			return "k" + n + "(" + f.Consts[v].String() + ")"
		}
		return "k" + n + "(?)"
	case OpJump:
		return "-> " + strconv.Itoa(pc+1+v)
	case OpFunc:
		if v < len(m.Functions) {
			return "f" + n + "(" + m.Functions[v].Name + ")"
		}
		return "f" + n
	case OpExtern:
		if v < len(m.Externs) {
			return "x" + n + "(" + m.Externs[v].Package + "." + m.Externs[v].Name + ")"
		}
		return "x" + n
	case OpHost:
		if v < len(m.Imports) {
			return "h" + n + "(" + m.Imports[v].Module + "." + m.Imports[v].Name + ")"
		}
		return "h" + n
	case OpSelf:
		return "self[" + n + "]"
	case OpType:
		return "t" + n
	case OpYield:
		if v < len(WaitNames) {
			return WaitNames[v]
		}
		return "wait" + n
	case OpSignal:
		if f.Script >= 0 && int(f.Script) < len(m.Scripts) && v < len(m.Scripts[f.Script].Signals) {
			return "g" + n + "(" + m.Scripts[f.Script].Signals[v].Name + ")"
		}
		return "g" + n
	case OpConnect:
		if v < len(m.Connects) {
			c := m.Connects[v]
			return "c" + n + "(" + short(c.Source) + "." + c.Signal + " -> " + short(c.Target) + "." + c.Handler + ")"
		}
		return "c" + n
	case OpScriptRef:
		if v < len(m.ScriptRefs) {
			return "s" + n + "(" + m.ScriptRefs[v] + ")"
		}
		return "s" + n
	case OpXField:
		if v < len(m.XFields) {
			x := m.XFields[v]
			if int(x.Ref) < len(m.ScriptRefs) {
				return "f" + n + "(" + short(m.ScriptRefs[x.Ref]) + "." + x.Name + ")"
			}
		}
		return "f" + n
	case OpXMethod:
		if v < len(m.XMethods) {
			x := m.XMethods[v]
			if int(x.Ref) < len(m.ScriptRefs) {
				return "m" + n + "(" + short(m.ScriptRefs[x.Ref]) + "." + x.Name + ")"
			}
		}
		return "m" + n
	}
	return ""
}
