package bytecode

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"aego/script/abi"
)

var (
	ErrTruncated = errors.New("bytecode: module is truncated")
	ErrMagic     = errors.New("bytecode: not an Aego Script module")
	ErrVersion   = errors.New("bytecode: unsupported bytecode version")
	ErrType      = errors.New("bytecode: malformed type reference")
	ErrTrailing  = errors.New("bytecode: trailing bytes after the module")
)

type writer struct {
	b []byte
}

func (w *writer) u8(v uint8) {
	w.b = append(w.b, v)
}

func (w *writer) u16(v uint16) {
	w.b = binary.LittleEndian.AppendUint16(w.b, v)
}

func (w *writer) u32(v uint32) {
	w.b = binary.LittleEndian.AppendUint32(w.b, v)
}

func (w *writer) u64(v uint64) {
	w.b = binary.LittleEndian.AppendUint64(w.b, v)
}

func (w *writer) uv(v int) {
	w.b = binary.AppendUvarint(w.b, uint64(v))
}

func (w *writer) str(s string) {
	w.uv(len(s))
	w.b = append(w.b, s...)
}

func (w *writer) strs(list []string) {
	w.uv(len(list))
	for _, s := range list {
		w.str(s)
	}
}

func (w *writer) f64(v float64) {
	w.u64(math.Float64bits(v))
}

func (w *writer) flag(v bool) {
	if v {
		w.u8(1)
		return
	}
	w.u8(0)
}

func (w *writer) konst(k Const) {
	w.u8(uint8(k.Kind))
	w.u64(k.A)
	w.u64(k.B)
	w.str(k.Str)
}

func (w *writer) sig(s abi.Signature) {
	w.uv(len(s.Params))
	for _, p := range s.Params {
		w.str(p.String())
	}
	w.str(s.Result.String())
}

func Encode(m *Module) []byte {
	w := &writer{b: make([]byte, 0, 4096)}
	w.b = append(w.b, m.Magic[:]...)
	w.u16(m.Version)
	w.u16(m.LangVersion)
	w.str(m.Package)
	w.b = append(w.b, m.Hash[:]...)
	w.b = append(w.b, m.ABIHash[:]...)
	w.uv(len(m.Imports))
	for _, imp := range m.Imports {
		w.str(imp.Module)
		w.str(imp.Name)
		w.sig(imp.Sig)
	}
	w.uv(len(m.Externs))
	for _, e := range m.Externs {
		w.str(e.Package)
		w.str(e.Name)
		w.u8(e.Params)
		w.u8(e.Results)
	}
	w.uv(len(m.Types))
	for _, t := range m.Types {
		w.str(t.Elem)
		w.u8(t.Width)
		w.u32(t.Mask)
	}
	w.uv(len(m.Enums))
	for _, e := range m.Enums {
		w.str(e.Name)
		w.strs(e.Members)
	}
	w.uv(len(m.Structs))
	for _, s := range m.Structs {
		w.str(s.Name)
		w.u16(s.Slots)
		w.uv(len(s.Fields))
		for _, f := range s.Fields {
			w.str(f.Name)
			w.str(f.Type)
			w.u16(f.Slot)
			w.u8(f.Slots)
		}
	}
	w.uv(len(m.Scripts))
	for i := range m.Scripts {
		w.script(&m.Scripts[i])
	}
	w.strs(m.ScriptRefs)
	w.uv(len(m.XFields))
	for _, x := range m.XFields {
		w.u16(x.Ref)
		w.str(x.Name)
		w.str(x.Type)
		w.u8(x.Slots)
	}
	w.uv(len(m.XMethods))
	for _, x := range m.XMethods {
		w.u16(x.Ref)
		w.str(x.Name)
		w.u8(x.Params)
		w.u8(x.Results)
	}
	w.uv(len(m.Connects))
	for _, c := range m.Connects {
		w.str(c.Source)
		w.str(c.Signal)
		w.str(c.Target)
		w.str(c.Handler)
	}
	w.uv(len(m.Functions))
	for i := range m.Functions {
		w.function(&m.Functions[i])
	}
	return w.b
}

func (w *writer) script(s *ScriptDef) {
	w.str(s.FullName)
	w.strs(s.Former)
	w.flag(s.Hidden)
	w.u16(s.Slots)
	w.uv(len(s.HeapSlots))
	for _, h := range s.HeapSlots {
		w.u16(h)
	}
	w.uv(len(s.Fields))
	for i := range s.Fields {
		f := &s.Fields[i]
		w.str(f.Name)
		w.strs(f.Former)
		w.str(f.Type)
		w.u16(f.Slot)
		w.u8(f.Slots)
		w.flag(f.Export)
		h := &f.Hints
		w.f64(h.Min)
		w.f64(h.Max)
		w.f64(h.Step)
		w.flag(h.HasMin)
		w.flag(h.HasMax)
		w.str(h.Unit)
		w.str(h.Display)
		w.str(h.Tip)
		w.str(h.Category)
		w.str(h.Gizmo)
		w.str(h.Space)
		w.flag(f.HasDefault)
		w.konst(f.Default)
	}
	w.u32(uint32(s.Ready))
	w.u32(uint32(s.FixedUpdate))
	w.u32(uint32(s.Update))
	w.u32(uint32(s.Reloaded))
	w.uv(len(s.Signals))
	for _, sg := range s.Signals {
		w.str(sg.Name)
		w.u8(sg.Slots)
		w.uv(len(sg.Params))
		for _, p := range sg.Params {
			w.str(p.String())
		}
	}
	w.uv(len(s.Events))
	for _, h := range s.Events {
		w.str(h.Name)
		w.u32(uint32(h.Func))
	}
}

func Decode(b []byte) (*Module, error) {
	r := &reader{b: b}
	m := &Module{}
	copy(m.Magic[:], r.take(4))
	if r.err == nil && string(m.Magic[:]) != Magic {
		return nil, ErrMagic
	}
	m.Version = r.u16()
	if r.err == nil && m.Version != Version {
		return nil, fmt.Errorf("%w: %d", ErrVersion, m.Version)
	}
	m.LangVersion = r.u16()
	m.Package = r.str()
	copy(m.Hash[:], r.take(32))
	copy(m.ABIHash[:], r.take(32))
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		m.Imports = append(m.Imports, HostImport{Module: r.str(), Name: r.str(), Sig: r.sig()})
	}
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		m.Externs = append(m.Externs, Extern{Package: r.str(), Name: r.str(), Params: r.u8(), Results: r.u8()})
	}
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		m.Types = append(m.Types, TypeDef{Elem: r.str(), Width: r.u8(), Mask: r.u32()})
	}
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		m.Enums = append(m.Enums, EnumDef{Name: r.str(), Members: r.strs()})
	}
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		s := StructDef{Name: r.str(), Slots: r.u16()}
		for k, j := r.count(), 0; j < k && r.err == nil; j++ {
			s.Fields = append(s.Fields, StructField{Name: r.str(), Type: r.str(), Slot: r.u16(), Slots: r.u8()})
		}
		m.Structs = append(m.Structs, s)
	}
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		m.Scripts = append(m.Scripts, r.script())
	}
	m.ScriptRefs = r.strs()
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		m.XFields = append(m.XFields, XFieldDef{Ref: r.u16(), Name: r.str(), Type: r.str(), Slots: r.u8()})
	}
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		m.XMethods = append(m.XMethods, XMethodDef{Ref: r.u16(), Name: r.str(), Params: r.u8(), Results: r.u8()})
	}
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		m.Connects = append(m.Connects, ConnectDef{Source: r.str(), Signal: r.str(), Target: r.str(), Handler: r.str()})
	}
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		m.Functions = append(m.Functions, r.function())
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.off != len(b) {
		return nil, ErrTrailing
	}
	return m, nil
}

func (r *reader) script() ScriptDef {
	s := ScriptDef{FullName: r.str(), Former: r.strs(), Hidden: r.flag(), Slots: r.u16()}
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		s.HeapSlots = append(s.HeapSlots, r.u16())
	}
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		f := FieldDef{Name: r.str(), Former: r.strs(), Type: r.str(), Slot: r.u16(), Slots: r.u8(), Export: r.flag()}
		f.Hints = ExportHints{
			Min: r.f64(), Max: r.f64(), Step: r.f64(), HasMin: r.flag(), HasMax: r.flag(),
			Unit: r.str(), Display: r.str(), Tip: r.str(), Category: r.str(),
			Gizmo: r.str(), Space: r.str(),
		}
		f.HasDefault = r.flag()
		f.Default = r.konst()
		s.Fields = append(s.Fields, f)
	}
	s.Ready = int32(r.u32())
	s.FixedUpdate = int32(r.u32())
	s.Update = int32(r.u32())
	s.Reloaded = int32(r.u32())
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		sg := SignalDef{Name: r.str(), Slots: r.u8()}
		for k, j := r.count(), 0; j < k && r.err == nil; j++ {
			sg.Params = append(sg.Params, r.typeRef())
		}
		s.Signals = append(s.Signals, sg)
	}
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		s.Events = append(s.Events, HandlerDef{Name: r.str(), Func: int32(r.u32())})
	}
	return s
}

func (w *writer) function(f *Function) {
	w.str(f.Name)
	w.str(f.File)
	w.u8(f.Params)
	w.u8(f.Results)
	w.u8(f.Registers)
	w.u32(uint32(f.Script))
	w.flag(f.Coroutine)
	w.uv(len(f.Code))
	for _, in := range f.Code {
		w.u32(uint32(in))
	}
	w.uv(len(f.Consts))
	for _, k := range f.Consts {
		w.konst(k)
	}
	w.uv(len(f.Lines))
	for _, l := range f.Lines {
		w.uv(int(l.PC))
		w.uv(int(l.Line))
	}
}

type reader struct {
	b   []byte
	off int
	err error
}

func (r *reader) fail(err error) {
	if r.err == nil {
		r.err = err
	}
}

func (r *reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || len(r.b)-r.off < n {
		r.fail(ErrTruncated)
		return nil
	}
	s := r.b[r.off : r.off+n]
	r.off += n
	return s
}

func (r *reader) u8() uint8 {
	if s := r.take(1); s != nil {
		return s[0]
	}
	return 0
}

func (r *reader) u16() uint16 {
	if s := r.take(2); s != nil {
		return binary.LittleEndian.Uint16(s)
	}
	return 0
}

func (r *reader) u32() uint32 {
	if s := r.take(4); s != nil {
		return binary.LittleEndian.Uint32(s)
	}
	return 0
}

func (r *reader) u64() uint64 {
	if s := r.take(8); s != nil {
		return binary.LittleEndian.Uint64(s)
	}
	return 0
}

func (r *reader) uv() int {
	if r.err != nil {
		return 0
	}
	v, n := binary.Uvarint(r.b[r.off:])
	if n <= 0 || v > uint64(len(r.b)) {
		r.fail(ErrTruncated)
		return 0
	}
	r.off += n
	return int(v)
}

func (r *reader) count() int {
	n := r.uv()
	if n > len(r.b)-r.off {
		r.fail(ErrTruncated)
		return 0
	}
	return n
}

func (r *reader) str() string {
	return string(r.take(r.uv()))
}

func (r *reader) strs() []string {
	var out []string
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		out = append(out, r.str())
	}
	return out
}

func (r *reader) f64() float64 {
	return math.Float64frombits(r.u64())
}

func (r *reader) flag() bool {
	return r.u8() != 0
}

func (r *reader) konst() Const {
	return Const{Kind: ConstKind(r.u8()), A: r.u64(), B: r.u64(), Str: r.str()}
}

func (r *reader) typeRef() abi.TypeRef {
	s := r.str()
	if r.err != nil {
		return abi.TypeRef{}
	}
	t, ok := abi.Parse(s)
	if !ok {
		r.fail(ErrType)
	}
	return t
}

func (r *reader) sig() abi.Signature {
	var s abi.Signature
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		s.Params = append(s.Params, r.typeRef())
	}
	s.Result = r.typeRef()
	return s
}

func (r *reader) function() Function {
	f := Function{Name: r.str(), File: r.str(), Params: r.u8(), Results: r.u8(), Registers: r.u8(), Script: int32(r.u32()), Coroutine: r.flag()}
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		f.Code = append(f.Code, Instr(r.u32()))
	}
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		f.Consts = append(f.Consts, r.konst())
	}
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		f.Lines = append(f.Lines, LineEntry{PC: uint32(r.uv()), Line: uint32(r.uv())})
	}
	return f
}
