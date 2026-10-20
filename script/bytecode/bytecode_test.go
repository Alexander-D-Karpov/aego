package bytecode

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"strings"
	"testing"

	"aego/script/abi"
)

func sample() *Module {
	m := NewModule("game/player")
	m.LangVersion = 1
	m.Hash[0], m.ABIHash[31] = 7, 9
	m.Imports = []HostImport{{Module: "aego/log", Name: "Info", Sig: abi.Sig(abi.T(abi.Void), abi.T(abi.String))},
		{Module: "aego/math", Name: "Sum", Sig: abi.Sig(abi.T(abi.Int), abi.SliceOf(abi.T(abi.Int)))}}
	m.Externs = []Extern{{Package: "game/combat", Name: "Hit", Params: 2, Results: 1}}
	m.Types = []TypeDef{{Elem: "string", Width: 1, Mask: 1}}
	m.Enums = []EnumDef{{Name: "game/player.State", Members: []string{"Idle", "Run"}}}
	m.Scripts = []ScriptDef{{
		FullName: "game.player.P", Former: []string{"Old"}, Hidden: true, Slots: 4, HeapSlots: []uint16{3},
		Fields: []FieldDef{
			{Name: "Speed", Type: "float", Slot: 0, Slots: 1, Export: true, Persist: true,
				Hints:      ExportHints{Min: 0, Max: 600, HasMin: true, HasMax: true, Unit: "px/s"},
				HasDefault: true, Default: FloatK(220)},
			{Name: "Pose", Type: "Transform2D", Slot: 1, Slots: 3},
		},
		Ready: -1, FixedUpdate: -1, Update: 0,
	}}
	m.Functions = []Function{{
		Name: "Update", File: "player.aes", Params: 1, Results: 0, Registers: 4, Script: 0,
		Code:   []Instr{ABx(LOADK, 1, 0), AsBx(LOADI, 2, -5), ABC(RET, 0, 0, 0)},
		Consts: []Const{StringK("hi"), IntK(-3), RawK(1, 2)},
		Lines:  []LineEntry{{PC: 0, Line: 10}, {PC: 2, Line: 12}},
	}}
	return m
}

func TestInstrEncoding(t *testing.T) {
	in := ABC(ADDI, 1, 2, 3)
	if in.Op() != ADDI || in.A() != 1 || in.B() != 2 || in.C() != 3 {
		t.Fatal("abc")
	}
	j := AsBx(JMP, 0, -300)
	if j.SBx() != -300 || j.Op() != JMP {
		t.Fatalf("sbx %d", j.SBx())
	}
	if ABx(CALL, 4, 65535).Bx() != 65535 {
		t.Fatal("bx")
	}
}

func TestRoundTrip(t *testing.T) {
	m := sample()
	data := Encode(m)
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, got) {
		t.Fatalf("round trip differs\n%+v\n%+v", m, got)
	}
	if !bytes.Equal(Encode(got), data) {
		t.Fatal("encoding is not canonical")
	}
}

func TestDecodeRejectsDamage(t *testing.T) {
	data := Encode(sample())
	for n := 0; n < len(data); n++ {
		if _, err := Decode(data[:n]); err == nil {
			t.Fatalf("prefix of %d bytes decoded", n)
		}
	}
	bad := append([]byte(nil), data...)
	bad[0] = 'X'
	if _, err := Decode(bad); !errors.Is(err, ErrMagic) {
		t.Fatalf("magic: %v", err)
	}
	if _, err := Decode(append(data, 0)); !errors.Is(err, ErrTrailing) {
		t.Fatalf("trailing: %v", err)
	}
}

func TestDisassemble(t *testing.T) {
	var b bytes.Buffer
	DisassembleSource(&b, sample(), func(file string, line int) string { return "src line" })
	out := b.String()
	for _, want := range []string{"import h0 aego/log.Info (string)", "script game.player.P", "LOADK    r1 k0(\"hi\")", "LOADI    r2 -5", "; player.aes:12 src line"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
}

func TestDecodeRejectsOldVersion(t *testing.T) {
	data := Encode(sample())
	binary.LittleEndian.PutUint16(data[4:], 2)
	if _, err := Decode(data); !errors.Is(err, ErrVersion) || !strings.Contains(err.Error(), "2") {
		t.Fatalf("err = %v", err)
	}
}
