package verify_test

import (
	"testing"

	bc "aego/script/bytecode"
	"aego/script/diag"
	"aego/script/verify"
)

func valid() *bc.Module {
	m := bc.NewModule("v")
	m.Types = []bc.TypeDef{{Elem: "int", Width: 1}}
	m.Scripts = []bc.ScriptDef{{
		FullName: "v.S", Slots: 3, HeapSlots: []uint16{2}, // slot 0 is node (D97); fields from slot 1
		Fields: []bc.FieldDef{{Name: "A", Type: "int", Slot: 1, Slots: 1}, {Name: "B", Type: "string", Slot: 2, Slots: 1}},
		Ready:  -1, FixedUpdate: -1, Update: 1, Reloaded: -1,
	}}
	m.Functions = []bc.Function{
		{Name: "add", Params: 2, Results: 1, Registers: 3, Script: -1,
			Code:  []bc.Instr{bc.ABC(bc.ADDI, 2, 0, 1), bc.ABC(bc.RET, 2, 1, 0)},
			Lines: []bc.LineEntry{{PC: 0, Line: 1}, {PC: 1, Line: 2}}},
		{Name: "Update", Params: 1, Results: 0, Registers: 3, Script: 0,
			Code: []bc.Instr{
				bc.ABx(bc.GETSF, 1, 0),
				bc.AsBx(bc.LOADI, 2, 1),
				bc.ABx(bc.CALL, 1, 0),
				bc.ABx(bc.SETSF, 1, 0),
				bc.ABC(bc.RET, 0, 0, 0),
			}},
	}
	return m
}

func TestValidModule(t *testing.T) {
	if ds := verify.Module(valid()); len(ds) != 0 {
		t.Fatalf("%v", ds)
	}
}

func TestCorruptModules(t *testing.T) {
	cases := map[string]func(m *bc.Module){
		"register":    func(m *bc.Module) { m.Functions[0].Code[0] = bc.ABC(bc.ADDI, 9, 0, 1) },
		"jump":        func(m *bc.Module) { m.Functions[0].Code[0] = bc.AsBx(bc.JMP, 0, 50) },
		"const":       func(m *bc.Module) { m.Functions[0].Code[0] = bc.ABx(bc.LOADK, 0, 3) },
		"callWindow":  func(m *bc.Module) { m.Functions[1].Code[2] = bc.ABx(bc.CALL, 2, 0) },
		"callee":      func(m *bc.Module) { m.Functions[1].Code[2] = bc.ABx(bc.CALL, 1, 9) },
		"fallthrough": func(m *bc.Module) { m.Functions[0].Code = m.Functions[0].Code[:1]; m.Functions[0].Lines = nil },
		"selfOutside": func(m *bc.Module) { m.Functions[0].Code[0] = bc.ABx(bc.GETSF, 0, 0) },
		"selfRange":   func(m *bc.Module) { m.Functions[1].Code[0] = bc.ABx(bc.GETSF, 1, 5) },
		"retCount":    func(m *bc.Module) { m.Functions[0].Code[1] = bc.ABC(bc.RET, 2, 0, 0) },
		"lines":       func(m *bc.Module) { m.Functions[0].Lines = []bc.LineEntry{{PC: 1, Line: 1}, {PC: 0, Line: 2}} },
		"lifecycle":   func(m *bc.Module) { m.Scripts[0].Update = 0 },
		"opcode":      func(m *bc.Module) { m.Functions[0].Code[0] = bc.Instr(250) },
		"magic":       func(m *bc.Module) { m.Magic[0] = 'X' },
		"fieldSlots":  func(m *bc.Module) { m.Scripts[0].Fields[1].Slot = 1 }, // gap: A@1 then B@1
		"typeMask":    func(m *bc.Module) { m.Types[0].Mask = 2 },
		"lane":        func(m *bc.Module) { m.Functions[0].Code[0] = bc.ABC(bc.GETL, 0, 1, 7) },
		"faultConst": func(m *bc.Module) {
			m.Functions[0].Consts = []bc.Const{bc.IntK(1)}
			m.Functions[0].Code[1] = bc.ABx(bc.FAULT, 0, 0)
		},
		"heapRoot":     func(m *bc.Module) { m.Scripts[0].HeapSlots = []uint16{1} }, // marks int field A@1 as a root
		"dupFunc":      func(m *bc.Module) { m.Functions[0].Name = "Update" },
		"badFieldType": func(m *bc.Module) { m.Scripts[0].Fields[0].Type = "notatype" },
		"typeWidth":    func(m *bc.Module) { m.Types[0].Width = 2 },
		"noPackage":    func(m *bc.Module) { m.Package = "" },
		"tooLong": func(m *bc.Module) {
			code := make([]bc.Instr, (1<<15)+1)
			for i := range code {
				code[i] = bc.ABC(bc.NOP, 0, 0, 0)
			}
			code[len(code)-1] = bc.ABC(bc.RET, 0, 0, 0)
			m.Functions[1].Code = code
			m.Functions[1].Lines = nil
		},
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			m := valid()
			corrupt(m)
			ds := verify.Module(m)
			if len(ds) == 0 {
				t.Fatal("corruption was not detected")
			}
			if ds[0].Code != diag.BadBytecode {
				t.Fatalf("code = %s", ds[0].Code)
			}
		})
	}
}

func TestCrossScriptCall(t *testing.T) {
	// A cross-script method call would run B's code with A's self.
	m := bc.NewModule("v")
	m.Scripts = []bc.ScriptDef{
		{FullName: "v.A", Slots: 1, Ready: -1, FixedUpdate: -1, Update: -1, Reloaded: -1},
		{FullName: "v.B", Slots: 1, Ready: -1, FixedUpdate: -1, Update: -1, Reloaded: -1},
	}
	m.Functions = []bc.Function{
		{Name: "A.m", Registers: 1, Script: 0, Code: []bc.Instr{bc.ABx(bc.CALL, 0, 1), bc.ABC(bc.RET, 0, 0, 0)}},
		{Name: "B.m", Registers: 1, Script: 1, Code: []bc.Instr{bc.ABC(bc.RET, 0, 0, 0)}},
	}
	if ds := verify.Module(m); len(ds) == 0 {
		t.Fatal("cross-script CALL was not detected")
	}
}

func FuzzDecodeVerify(f *testing.F) {
	f.Add(bc.Encode(valid()))
	f.Add([]byte("AESB"))
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := bc.Decode(data)
		if err != nil {
			return
		}
		verify.Module(m)
	})
}

func structModule(heap []uint16) *bc.Module {
	m := valid()
	m.Structs = []bc.StructDef{{Name: "v.P", Slots: 2, Fields: []bc.StructField{
		{Name: "N", Type: "int", Slot: 0, Slots: 1},
		{Name: "S", Type: "string", Slot: 1, Slots: 1},
	}}}
	m.Scripts[0].Fields = []bc.FieldDef{{Name: "P", Type: "struct:v.P", Slot: 1, Slots: 2}}
	m.Scripts[0].HeapSlots = heap
	return m
}

func TestStructFieldRoots(t *testing.T) {
	if ds := verify.Module(structModule(nil)); len(ds) == 0 {
		t.Fatal("a string inside a struct field without a GC root was accepted")
	}
	if ds := verify.Module(structModule([]uint16{2})); len(ds) != 0 {
		t.Fatalf("correct roots rejected: %v", ds)
	}
}
