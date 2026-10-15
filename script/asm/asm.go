// Package asm is the bytecode assembler: instructions, deduped constants, labels and the line table.
package asm

import (
	"fmt"

	"aego/script/bytecode"
)

type Label int

type fixup struct {
	pc    int
	label Label
}

type Builder struct {
	code   []bytecode.Instr
	consts []bytecode.Const
	index  map[bytecode.Const]int
	lines  []bytecode.LineEntry
	labels []int
	fixups []fixup
	line   int
	maxReg int
	err    error
}

func NewBuilder() *Builder {
	return &Builder{index: map[bytecode.Const]int{}}
}

func (b *Builder) fail(format string, args ...any) {
	if b.err == nil {
		b.err = fmt.Errorf(format, args...)
	}
}

func (b *Builder) Use(top int) {
	if top > b.maxReg {
		b.maxReg = top
	}
}

func (b *Builder) MaxReg() int {
	return b.maxReg
}

func (b *Builder) PC() int {
	return len(b.code)
}

func (b *Builder) SetLine(line int) {
	b.line = line
}

func (b *Builder) emit(in bytecode.Instr) int {
	pc := len(b.code)
	if b.line > 0 && (len(b.lines) == 0 || int(b.lines[len(b.lines)-1].Line) != b.line) {
		b.lines = append(b.lines, bytecode.LineEntry{PC: uint32(pc), Line: uint32(b.line)})
	}
	b.code = append(b.code, in)
	return pc
}

func (b *Builder) u8(v int, what string) uint8 {
	if v < 0 || v > 255 {
		b.fail("%s %d does not fit in 8 bits", what, v)
		return 0
	}
	return uint8(v)
}

func (b *Builder) ABC(op bytecode.Opcode, a, bb, c int) {
	b.emit(bytecode.ABC(op, b.u8(a, "register"), b.u8(bb, "operand"), b.u8(c, "operand")))
}

func (b *Builder) ABx(op bytecode.Opcode, a, bx int) {
	if bx < 0 || bx > 65535 {
		b.fail("operand %d does not fit in 16 bits", bx)
		bx = 0
	}
	b.emit(bytecode.ABx(op, b.u8(a, "register"), uint16(bx)))
}

func (b *Builder) AsBx(op bytecode.Opcode, a, sbx int) {
	if sbx < -32768 || sbx > 32767 {
		b.fail("operand %d does not fit in 16 bits", sbx)
		sbx = 0
	}
	b.emit(bytecode.AsBx(op, b.u8(a, "register"), int16(sbx)))
}

func (b *Builder) Const(k bytecode.Const) int {
	if i, ok := b.index[k]; ok {
		return i
	}
	i := len(b.consts)
	b.consts = append(b.consts, k)
	b.index[k] = i
	return i
}

func (b *Builder) NewLabel() Label {
	b.labels = append(b.labels, -1)
	return Label(len(b.labels) - 1)
}

func (b *Builder) Mark(l Label) {
	if b.labels[l] >= 0 {
		b.fail("label %d is placed twice", l)
		return
	}
	b.labels[l] = len(b.code)
}

func (b *Builder) Jump(op bytecode.Opcode, a int, l Label) {
	pc := b.emit(bytecode.AsBx(op, b.u8(a, "register"), 0))
	b.fixups = append(b.fixups, fixup{pc: pc, label: l})
}

func (b *Builder) Finish() ([]bytecode.Instr, []bytecode.Const, []bytecode.LineEntry, error) {
	for _, f := range b.fixups {
		target := b.labels[f.label]
		if target < 0 {
			b.fail("label %d is never placed", f.label)
			continue
		}
		if target >= len(b.code) {
			b.fail("label %d points past the end of the function; emit RET before Finish", f.label)
			continue
		}
		off := target - (f.pc + 1)
		if off < -32768 || off > 32767 {
			b.fail("jump of %d instructions is too far", off)
			continue
		}
		in := b.code[f.pc]
		b.code[f.pc] = bytecode.AsBx(in.Op(), in.A(), int16(off))
	}
	if b.err != nil {
		return nil, nil, nil, b.err
	}
	return b.code, b.consts, b.lines, nil
}
