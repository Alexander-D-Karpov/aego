package bytecode

import (
	"math"
	"sort"
	"strconv"

	"aego/script/abi"
)

const (
	Magic              = "AESB"
	Version     uint16 = 7
	LangVersion uint16 = 2
	NoScript           = -1
)

const (
	PauseNormal uint8 = iota
	PauseAlways
	PauseWhenPaused
)

type ConstKind uint8

const (
	KInt ConstKind = iota
	KFloat
	KString
	KRaw
)

type Const struct {
	Kind ConstKind
	A, B uint64
	Str  string
}

func IntK(v int64) Const {
	return Const{Kind: KInt, A: uint64(v)}
}

func FloatK(v float32) Const {
	return Const{Kind: KFloat, A: uint64(math.Float32bits(v))}
}

func StringK(s string) Const {
	return Const{Kind: KString, Str: s}
}

func RawK(a, b uint64) Const {
	return Const{Kind: KRaw, A: a, B: b}
}

func LanesK(l [4]float32) Const {
	return RawK(uint64(math.Float32bits(l[0]))|uint64(math.Float32bits(l[1]))<<32,
		uint64(math.Float32bits(l[2]))|uint64(math.Float32bits(l[3]))<<32)
}

func (k Const) Lanes() [4]float32 {
	return [4]float32{
		math.Float32frombits(uint32(k.A)), math.Float32frombits(uint32(k.A >> 32)),
		math.Float32frombits(uint32(k.B)), math.Float32frombits(uint32(k.B >> 32)),
	}
}

func (k Const) String() string {
	switch k.Kind {
	case KInt:
		return strconv.FormatInt(int64(k.A), 10)
	case KFloat:
		return strconv.FormatFloat(float64(math.Float32frombits(uint32(k.A))), 'g', -1, 32)
	case KString:
		return strconv.Quote(k.Str)
	}
	return "0x" + strconv.FormatUint(k.A, 16) + ":0x" + strconv.FormatUint(k.B, 16)
}

type LineEntry struct {
	PC   uint32
	Line uint32
}

type Function struct {
	Name      string
	File      string
	Params    uint8
	Results   uint8
	Registers uint8
	Script    int32
	Coroutine bool
	Code      []Instr
	Consts    []Const
	Lines     []LineEntry
}

func (f *Function) Line(pc int) int {
	i := sort.Search(len(f.Lines), func(i int) bool { return int(f.Lines[i].PC) > pc })
	if i == 0 {
		return 0
	}
	return int(f.Lines[i-1].Line)
}

type HostImport struct {
	Module string
	Name   string
	Sig    abi.Signature
}

type Extern struct {
	Package string
	Name    string
	Params  uint8
	Results uint8
}

type TypeDef struct {
	Elem  string
	Width uint8
	Mask  uint32
}

type EnumDef struct {
	Name    string
	Members []string
}

type ExportHints struct {
	Min, Max, Step float64
	HasMin, HasMax bool
	Unit           string
	Display        string
	Tip            string
	Category       string
	Gizmo          string
	Space          string
}

type FieldDef struct {
	Name       string
	Former     []string
	Type       string
	Slot       uint16
	Slots      uint8
	Export     bool
	Persist    bool
	Hints      ExportHints
	HasDefault bool
	Default    Const
}

type StructField struct {
	Name  string
	Type  string
	Slot  uint16
	Slots uint8
}

type StructDef struct {
	Name   string
	Slots  uint16
	Fields []StructField
}

type SignalDef struct {
	Name   string
	Params []abi.TypeRef
	Slots  uint8
}

type HandlerDef struct {
	Name string
	Func int32
}

type ScriptDef struct {
	FullName    string
	Former      []string
	Hidden      bool
	Pause       uint8
	Slots       uint16
	HeapSlots   []uint16
	Fields      []FieldDef
	Signals     []SignalDef
	Events      []HandlerDef
	Ready       int32
	FixedUpdate int32
	Update      int32
	Reloaded    int32
}

func (s *ScriptDef) Field(name string) *FieldDef {
	for i := range s.Fields {
		if s.Fields[i].Name == name {
			return &s.Fields[i]
		}
	}
	return nil
}

func (s *ScriptDef) Signal(name string) int {
	for i := range s.Signals {
		if s.Signals[i].Name == name {
			return i
		}
	}
	return -1
}

type XFieldDef struct {
	Ref   uint16
	Name  string
	Type  string
	Slots uint8
}

type XMethodDef struct {
	Ref     uint16
	Name    string
	Params  uint8
	Results uint8
}

type ConnectDef struct {
	Source  string
	Signal  string
	Target  string
	Handler string
}

type Module struct {
	Magic       [4]byte
	Version     uint16
	LangVersion uint16
	Package     string
	Hash        [32]byte
	ABIHash     [32]byte
	Imports     []HostImport
	Externs     []Extern
	Types       []TypeDef
	Enums       []EnumDef
	Structs     []StructDef
	Scripts     []ScriptDef
	ScriptRefs  []string
	XFields     []XFieldDef
	XMethods    []XMethodDef
	Connects    []ConnectDef
	Functions   []Function
}

func NewModule(pkg string) *Module {
	m := &Module{Version: Version, Package: pkg}
	copy(m.Magic[:], Magic)
	return m
}

func (m *Module) Struct(name string) *StructDef {
	for i := range m.Structs {
		if m.Structs[i].Name == name {
			return &m.Structs[i]
		}
	}
	return nil
}

func (m *Module) Slots(t abi.TypeRef) int {
	if t.Kind == abi.Struct {
		if s := m.Struct(t.Name); s != nil {
			return int(s.Slots)
		}
	}
	return t.Slots()
}

func (m *Module) Window(s abi.Signature) int {
	n := 0
	for _, p := range s.Params {
		n += m.Slots(p)
	}
	return max(n, m.Slots(s.Result))
}

func (m *Module) FuncIndex(name string) int {
	for i := range m.Functions {
		if m.Functions[i].Name == name {
			return i
		}
	}
	return -1
}
