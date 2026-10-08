package asm

import (
	"testing"

	"aego/script/bytecode"
)

func TestLabelsConstsLines(t *testing.T) {
	b := NewBuilder()
	top, end := b.NewLabel(), b.NewLabel()
	b.SetLine(3)
	b.Mark(top)
	k1 := b.Const(bytecode.IntK(70000))
	k2 := b.Const(bytecode.IntK(70000))
	b.ABx(bytecode.LOADK, 0, k1)
	b.Jump(bytecode.JMPIF, 0, end)
	b.SetLine(4)
	b.Jump(bytecode.JMP, 0, top)
	b.Mark(end)
	b.ABC(bytecode.RET, 0, 0, 0)
	code, consts, lines, err := b.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if k1 != k2 || len(consts) != 1 {
		t.Fatal("constants are not deduplicated")
	}
	if code[1].SBx() != 1 || code[2].SBx() != -3 {
		t.Fatalf("jumps %d %d", code[1].SBx(), code[2].SBx())
	}
	if len(lines) != 2 || lines[1].PC != 2 || lines[1].Line != 4 {
		t.Fatalf("lines %+v", lines)
	}
}

func TestBuilderErrors(t *testing.T) {
	b := NewBuilder()
	b.ABC(bytecode.MOVE, 300, 0, 0)
	if _, _, _, err := b.Finish(); err == nil {
		t.Fatal("register overflow not reported")
	}
	b = NewBuilder()
	b.Jump(bytecode.JMP, 0, b.NewLabel())
	if _, _, _, err := b.Finish(); err == nil {
		t.Fatal("unplaced label not reported")
	}
	b = NewBuilder()
	l := b.NewLabel()
	b.Mark(l)
	b.Mark(l)
	if _, _, _, err := b.Finish(); err == nil {
		t.Fatal("label marked twice not reported")
	}
	b = NewBuilder()
	end := b.NewLabel()
	b.Jump(bytecode.JMP, 0, end)
	b.Mark(end) // points at len(code): a jump target past the last instruction
	if code, _, _, err := b.Finish(); err == nil || code != nil {
		t.Fatal("label past the end not reported, or code not discarded")
	}
}
