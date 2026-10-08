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
	for i := range m.Scripts {
		s := &m.Scripts[i]
		fmt.Fprintf(&b, "script %s slots=%d ready=%d fixed=%d update=%d\n", s.FullName, s.Slots, s.Ready, s.FixedUpdate, s.Update)
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
	}
	return ""
}
