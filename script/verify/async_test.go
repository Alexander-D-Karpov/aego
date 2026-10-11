package verify_test

import (
	"testing"

	"aego/script/abi"
	bc "aego/script/bytecode"
	"aego/script/verify"
)

func asyncModule() *bc.Module {
	m := valid()
	m.Scripts[0].Signals = []bc.SignalDef{{Name: "Hit", Params: []abi.TypeRef{abi.T(abi.Int)}, Slots: 1}}
	m.ScriptRefs = []string{"v.S"}
	m.XFields = []bc.XFieldDef{{Ref: 0, Name: "A", Type: "int", Slots: 1}}
	m.XMethods = []bc.XMethodDef{{Ref: 0, Name: "Poke"}}
	m.Connects = []bc.ConnectDef{{Source: "v.S", Signal: "Hit", Target: "v.S", Handler: "Poke"}}
	m.Functions = append(m.Functions, bc.Function{
		Name: "S.Blink", Registers: 1, Script: 0, Coroutine: true,
		Code: []bc.Instr{bc.ABC(bc.YIELD, 0, 0, 0), bc.ABC(bc.RET, 0, 0, 0)},
	})
	return m
}

func TestAsyncModuleValid(t *testing.T) {
	if ds := verify.Module(asyncModule()); len(ds) != 0 {
		t.Fatalf("%v", ds)
	}
}

func TestAsyncCorrupt(t *testing.T) {
	cases := map[string]func(m *bc.Module){
		"startPlain":    func(m *bc.Module) { m.Functions[1].Code[2] = bc.ABx(bc.START, 1, 0) },
		"callCoroutine": func(m *bc.Module) { m.Functions[1].Code[2] = bc.ABx(bc.CALL, 1, 2) },
		"yieldOutside":  func(m *bc.Module) { m.Functions[0].Code[0] = bc.ABC(bc.YIELD, 0, 0, 0) },
		"yieldKind":     func(m *bc.Module) { m.Functions[2].Code[0] = bc.ABC(bc.YIELD, 0, 0, 9) },
		"scriptRefIdx":  func(m *bc.Module) { m.Functions[1].Code[0] = bc.ABx(bc.GETSCRIPT, 1, 4) },
		"xfieldRef":     func(m *bc.Module) { m.XFields[0].Ref = 5 },
		"xfieldIdx":     func(m *bc.Module) { m.Functions[1].Code[0] = bc.ABC(bc.GETXF, 1, 1, 7) },
		"xmethodRef":    func(m *bc.Module) { m.XMethods[0].Ref = 5 },
		"xmethodIdx":    func(m *bc.Module) { m.Functions[1].Code[0] = bc.ABx(bc.CALLR, 1, 7) },
		"connectIdx":    func(m *bc.Module) { m.Functions[1].Code[0] = bc.ABx(bc.CONNECT, 0, 3) },
		"emitIdx":       func(m *bc.Module) { m.Functions[1].Code[0] = bc.ABx(bc.EMIT, 1, 7) },
		"signalVoid":    func(m *bc.Module) { m.Scripts[0].Signals[0].Params[0] = abi.T(abi.Void) },
		"emptyConnect":  func(m *bc.Module) { m.Connects[0].Handler = "" },
		"selfrefOut":    func(m *bc.Module) { m.Functions[0].Code[0] = bc.ABC(bc.SELFREF, 0, 0, 0) },
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			m := asyncModule()
			corrupt(m)
			if ds := verify.Module(m); len(ds) == 0 {
				t.Fatal("corruption was not detected")
			}
		})
	}
}

func FuzzDecodeVerifyV3(f *testing.F) {
	f.Add(bc.Encode(asyncModule()))
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := bc.Decode(data)
		if err != nil {
			return
		}
		verify.Module(m)
	})
}
