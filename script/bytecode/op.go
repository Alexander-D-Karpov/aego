package bytecode

import "strconv"

type Opcode uint8

const (
	NOP Opcode = iota
	MOVE
	MOVEN
	LOADK
	LOADI
	LOADNIL
	LOADBOOL
	ADDI
	SUBI
	MULI
	DIVI
	MODI
	NEGI
	ADDF
	SUBF
	MULF
	DIVF
	NEGF
	ITOF
	FTOI
	ITOS
	FTOS
	BTOS
	ADDS
	LENS
	EQ
	EQF
	EQS
	EQV
	LTI
	LEI
	LTF
	LEF
	LTS
	LES
	NOT
	JMP
	JMPIF
	JMPNOT
	CALL
	CALLX
	CALLH
	RET
	GETSF
	SETSF
	GETCF
	SETCF
	NEWSLICE
	ENSURE
	LEN
	INDEX
	SETINDEX
	APPEND
	VEC2
	VEC3
	VEC4
	GETL
	SETL
	VADD
	VSUB
	VMULS
	VDIVS
	VNEG
	ISNIL
	FAULT
	START
	YIELD
	COSTOP
	CORUN
	EMIT
	CONNECT
	DISCONNECT
	CONNECTED
	GETSCRIPT
	GETXF
	SETXF
	CALLR
	SELFREF
	opCount
)

type Format uint8

const (
	FormatABC Format = iota
	FormatABx
	FormatAsBx
)

type Operand uint8

const (
	OpNone Operand = iota
	OpReg
	OpInt
	OpBool
	OpCount
	OpLane
	OpConst
	OpJump
	OpFunc
	OpExtern
	OpHost
	OpSelf
	OpType
	OpField
	OpYield
	OpSignal
	OpConnect
	OpScriptRef
	OpXField
	OpXMethod
)

type OpInfo struct {
	Name    string
	Format  Format
	A, B, C Operand
}

func abc(name string, a, b, c Operand) OpInfo {
	return OpInfo{Name: name, Format: FormatABC, A: a, B: b, C: c}
}

func abx(name string, a, b Operand) OpInfo {
	return OpInfo{Name: name, Format: FormatABx, A: a, B: b}
}

func asbx(name string, a, b Operand) OpInfo {
	return OpInfo{Name: name, Format: FormatAsBx, A: a, B: b}
}

func bin(name string) OpInfo {
	return abc(name, OpReg, OpReg, OpReg)
}

func un(name string) OpInfo {
	return abc(name, OpReg, OpReg, OpNone)
}

func reg(name string) OpInfo {
	return abc(name, OpReg, OpNone, OpNone)
}

var opInfo = [opCount]OpInfo{
	NOP:        abc("NOP", OpNone, OpNone, OpNone),
	MOVE:       un("MOVE"),
	MOVEN:      abc("MOVEN", OpReg, OpReg, OpCount),
	LOADK:      abx("LOADK", OpReg, OpConst),
	LOADI:      asbx("LOADI", OpReg, OpInt),
	LOADNIL:    reg("LOADNIL"),
	LOADBOOL:   abc("LOADBOOL", OpReg, OpBool, OpNone),
	ADDI:       bin("ADDI"),
	SUBI:       bin("SUBI"),
	MULI:       bin("MULI"),
	DIVI:       bin("DIVI"),
	MODI:       bin("MODI"),
	NEGI:       un("NEGI"),
	ADDF:       bin("ADDF"),
	SUBF:       bin("SUBF"),
	MULF:       bin("MULF"),
	DIVF:       bin("DIVF"),
	NEGF:       un("NEGF"),
	ITOF:       un("ITOF"),
	FTOI:       un("FTOI"),
	ITOS:       un("ITOS"),
	FTOS:       un("FTOS"),
	BTOS:       un("BTOS"),
	ADDS:       bin("ADDS"),
	LENS:       un("LENS"),
	EQ:         bin("EQ"),
	EQF:        bin("EQF"),
	EQS:        bin("EQS"),
	EQV:        bin("EQV"),
	LTI:        bin("LTI"),
	LEI:        bin("LEI"),
	LTF:        bin("LTF"),
	LEF:        bin("LEF"),
	LTS:        bin("LTS"),
	LES:        bin("LES"),
	NOT:        un("NOT"),
	JMP:        asbx("JMP", OpNone, OpJump),
	JMPIF:      asbx("JMPIF", OpReg, OpJump),
	JMPNOT:     asbx("JMPNOT", OpReg, OpJump),
	CALL:       abx("CALL", OpReg, OpFunc),
	CALLX:      abx("CALLX", OpReg, OpExtern),
	CALLH:      abx("CALLH", OpReg, OpHost),
	RET:        abc("RET", OpReg, OpCount, OpNone),
	GETSF:      abx("GETSF", OpReg, OpSelf),
	SETSF:      abx("SETSF", OpReg, OpSelf),
	GETCF:      abc("GETCF", OpReg, OpReg, OpField),
	SETCF:      abc("SETCF", OpReg, OpReg, OpField),
	NEWSLICE:   abx("NEWSLICE", OpReg, OpType),
	ENSURE:     abx("ENSURE", OpReg, OpType),
	LEN:        un("LEN"),
	INDEX:      bin("INDEX"),
	SETINDEX:   bin("SETINDEX"),
	APPEND:     bin("APPEND"),
	VEC2:       un("VEC2"),
	VEC3:       un("VEC3"),
	VEC4:       un("VEC4"),
	GETL:       abc("GETL", OpReg, OpReg, OpLane),
	SETL:       abc("SETL", OpReg, OpReg, OpLane),
	VADD:       bin("VADD"),
	VSUB:       bin("VSUB"),
	VMULS:      bin("VMULS"),
	VDIVS:      bin("VDIVS"),
	VNEG:       un("VNEG"),
	ISNIL:      un("ISNIL"),
	FAULT:      abx("FAULT", OpNone, OpConst),
	START:      abx("START", OpReg, OpFunc),
	YIELD:      abc("YIELD", OpReg, OpNone, OpYield),
	COSTOP:     reg("COSTOP"),
	CORUN:      un("CORUN"),
	EMIT:       abx("EMIT", OpReg, OpSignal),
	CONNECT:    abx("CONNECT", OpReg, OpConnect),
	DISCONNECT: reg("DISCONNECT"),
	CONNECTED:  un("CONNECTED"),
	GETSCRIPT:  abx("GETSCRIPT", OpReg, OpScriptRef),
	GETXF:      abc("GETXF", OpReg, OpReg, OpXField),
	SETXF:      abc("SETXF", OpReg, OpReg, OpXField),
	CALLR:      abx("CALLR", OpReg, OpXMethod),
	SELFREF:    reg("SELFREF"),
}

func (o Opcode) Valid() bool {
	return o < opCount
}

func (o Opcode) Info() OpInfo {
	if o < opCount {
		return opInfo[o]
	}
	return OpInfo{Name: "OP" + strconv.Itoa(int(o))}
}

func (o Opcode) String() string {
	return o.Info().Name
}

type Instr uint32

func ABC(op Opcode, a, b, c uint8) Instr {
	return Instr(op) | Instr(a)<<8 | Instr(b)<<16 | Instr(c)<<24
}

func ABx(op Opcode, a uint8, bx uint16) Instr {
	return Instr(op) | Instr(a)<<8 | Instr(bx)<<16
}

func AsBx(op Opcode, a uint8, sbx int16) Instr {
	return ABx(op, a, uint16(sbx))
}

func (i Instr) Op() Opcode {
	return Opcode(i)
}

func (i Instr) A() uint8 {
	return uint8(i >> 8)
}

func (i Instr) B() uint8 {
	return uint8(i >> 16)
}

func (i Instr) C() uint8 {
	return uint8(i >> 24)
}

func (i Instr) Bx() uint16 {
	return uint16(i >> 16)
}

func (i Instr) SBx() int16 {
	return int16(i.Bx())
}

const (
	WaitNext uint8 = iota
	WaitTime
	WaitFrames
	WaitFixed
	WaitAwait
	WaitUnscaled
	WaitHandle
	MaxWait = WaitHandle
)

var WaitNames = [...]string{"next", "wait", "frames", "fixed", "await", "waitUnscaled", "handle"}

const (
	MaxRegisters       = 255
	MaxJumpOffset      = 32767
	MaxSliceElemSlots  = 32
	MaxComponentFields = 255
	MaxScriptFieldRefs = 256
)
