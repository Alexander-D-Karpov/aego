package abi

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// errFrozen is returned by every Add* once Freeze has sealed the table.
var errFrozen = errors.New("abi: table is frozen")

const MaxStructSlots = 32

type Table struct {
	modules  map[string]bool
	funcs    map[string]Signature
	methods  map[string]Signature
	cmethods map[string]Signature
	hmethods map[string]Signature
	hsignals map[string][]SignalDef
	handles  map[string]HandleDef
	consts   map[string]Const
	enums    map[string]*EnumDef
	structs  map[string]*StructDef
	events   []EventDef
	eventIdx map[string]int
	comps    []*Component
	hash     [32]byte
	frozen   bool
}

func NewTable() *Table {
	return &Table{
		modules:  map[string]bool{},
		funcs:    map[string]Signature{},
		methods:  map[string]Signature{},
		cmethods: map[string]Signature{},
		hmethods: map[string]Signature{},
		hsignals: map[string][]SignalDef{},
		handles:  map[string]HandleDef{},
		consts:   map[string]Const{},
		enums:    map[string]*EnumDef{},
		structs:  map[string]*StructDef{},
		eventIdx: map[string]int{},
	}
}

func (t *Table) AddHandle(h HandleDef) error {
	if t.frozen {
		return errFrozen
	}
	i := strings.LastIndexByte(h.Name, '.')
	if i <= 0 || i == len(h.Name)-1 || !Exported(h.Name[i+1:]) {
		return fmt.Errorf("handle type %q needs the form module.Name", h.Name)
	}
	if _, dup := t.handles[h.Name]; dup {
		return fmt.Errorf("handle type %s already registered", h.Name)
	}
	t.modules[h.Name[:i]] = true
	t.handles[h.Name] = h
	return nil
}

func (t *Table) hasHandleSignal(handle, name string) bool {
	for _, s := range t.hsignals[handle] {
		if s.Name == name {
			return true
		}
	}
	return false
}

func (t *Table) AddHandleMethod(handle, name string, sig Signature) error {
	if t.frozen {
		return errFrozen
	}
	if err := sig.Validate(); err != nil {
		return fmt.Errorf("method %s.%s: %w", handle, name, err)
	}
	k := key(handle, name)
	if _, dup := t.hmethods[k]; dup || t.hasHandleSignal(handle, name) {
		return fmt.Errorf("method %s.%s already registered", handle, name)
	}
	t.hmethods[k] = sig
	return nil
}

func (t *Table) AddHandleSignal(handle string, s SignalDef) error {
	if t.frozen {
		return errFrozen
	}
	if !Exported(s.Name) {
		return fmt.Errorf("signal %s.%s needs an exported name", handle, s.Name)
	}
	if _, dup := t.hmethods[key(handle, s.Name)]; dup || t.hasHandleSignal(handle, s.Name) {
		return fmt.Errorf("signal %s.%s already registered", handle, s.Name)
	}
	for i, p := range s.Params {
		if !Plain(p) {
			return fmt.Errorf("signal %s.%s param %d: %s cannot be a signal parameter", handle, s.Name, i, p)
		}
	}
	t.hsignals[handle] = append(t.hsignals[handle], SignalDef{Name: s.Name, Params: append([]TypeRef(nil), s.Params...)})
	return nil
}

func (t *Table) HandleSignals(handle string) []SignalDef {
	return t.hsignals[handle]
}

func (t *Table) HandleSignal(handle, name string) (int, SignalDef, bool) {
	for i, s := range t.hsignals[handle] {
		if s.Name == name {
			return i, s, true
		}
	}
	return -1, SignalDef{}, false
}

type HandleSignalEntry struct {
	Handle string
	Index  int
	Def    SignalDef
}

func (t *Table) HandleSignalList() []HandleSignalEntry {
	var out []HandleSignalEntry
	for h, list := range t.hsignals {
		for i, s := range list {
			out = append(out, HandleSignalEntry{Handle: h, Index: i, Def: s})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Handle != out[j].Handle {
			return out[i].Handle < out[j].Handle
		}
		return out[i].Index < out[j].Index
	})
	return out
}

func (t *Table) Handle(name string) (HandleDef, bool) {
	h, ok := t.handles[name]
	return h, ok
}

func (t *Table) HandleMethod(handle, name string) (Signature, bool) {
	s, ok := t.hmethods[key(handle, name)]
	return s, ok
}

type HandleMethodEntry struct {
	Handle string
	Name   string
	Sig    Signature
}

func (t *Table) HandleList() []HandleDef {
	out := make([]HandleDef, 0, len(t.handles))
	for _, h := range t.handles {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (t *Table) HandleMethodList() []HandleMethodEntry {
	out := make([]HandleMethodEntry, 0, len(t.hmethods))
	for k, s := range t.hmethods {
		h, n, _ := strings.Cut(k, "\x00")
		out = append(out, HandleMethodEntry{Handle: h, Name: n, Sig: s})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Handle != out[j].Handle {
			return out[i].Handle < out[j].Handle
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func key(a, b string) string {
	return a + "\x00" + b
}

func (t *Table) AddModule(path string) error {
	if t.frozen {
		return errFrozen
	}
	t.modules[path] = true
	return nil
}

func (t *Table) AddFunc(module, name string, sig Signature) error {
	if t.frozen {
		return errFrozen
	}
	if err := sig.Validate(); err != nil {
		return fmt.Errorf("func %s.%s: %w", module, name, err)
	}
	k := key(module, name)
	if _, dup := t.funcs[k]; dup {
		return fmt.Errorf("func %s.%s already registered", module, name)
	}
	t.modules[module] = true
	t.funcs[k] = sig
	return nil
}

func (t *Table) AddMethod(recv Kind, name string, sig Signature) error {
	if t.frozen {
		return errFrozen
	}
	if err := sig.Validate(); err != nil {
		return fmt.Errorf("method %s.%s: %w", recv, name, err)
	}
	k := key(recv.String(), name)
	if _, dup := t.methods[k]; dup {
		return fmt.Errorf("method %s.%s already registered", recv, name)
	}
	t.methods[k] = sig
	return nil
}

func (t *Table) AddComponentMethod(comp, name string, sig Signature) error {
	if t.frozen {
		return errFrozen
	}
	if err := sig.Validate(); err != nil {
		return fmt.Errorf("method %s.%s: %w", comp, name, err)
	}
	k := key(comp, name)
	if _, dup := t.cmethods[k]; dup {
		return fmt.Errorf("method %s.%s already registered", comp, name)
	}
	t.cmethods[k] = sig
	return nil
}

func (t *Table) AddConst(module, name string, c Const) error {
	if t.frozen {
		return errFrozen
	}
	k := key(module, name)
	if _, dup := t.consts[k]; dup {
		return fmt.Errorf("const %s.%s already registered", module, name)
	}
	t.modules[module] = true
	t.consts[k] = c
	return nil
}

func (t *Table) AddEnum(e *EnumDef) error {
	if t.frozen {
		return errFrozen
	}
	k := key(e.Module, e.Name)
	if _, dup := t.enums[k]; dup {
		return fmt.Errorf("enum %s already registered", e.Qualified())
	}
	t.modules[e.Module] = true
	t.enums[k] = e
	return nil
}

func (t *Table) AddStruct(s *StructDef) error {
	if t.frozen {
		return errFrozen
	}
	q := s.Qualified()
	if _, dup := t.structs[q]; dup {
		return fmt.Errorf("struct %s already registered", q)
	}
	if s.Slots <= 0 || s.Slots > MaxStructSlots {
		return fmt.Errorf("struct %s has %d value slots, the limit is %d", q, s.Slots, MaxStructSlots)
	}
	for _, f := range s.Fields {
		if !Plain(f.Type) {
			return fmt.Errorf("struct %s field %s has type %s, which cannot live in a host struct", q, f.Name, f.Type)
		}
	}
	t.modules[s.Module] = true
	t.structs[q] = s
	return nil
}

func (t *Table) AddEvent(e EventDef) error {
	if t.frozen {
		return errFrozen
	}
	if !Exported(e.Name) {
		return fmt.Errorf("event %q must have an exported name", e.Name)
	}
	if _, dup := t.eventIdx[e.Name]; dup {
		return fmt.Errorf("event %s already registered", e.Name)
	}
	for i, p := range e.Params {
		if !EventParam(p) {
			return fmt.Errorf("event %s param %d: %s cannot be an event parameter", e.Name, i, p)
		}
	}
	t.eventIdx[e.Name] = len(t.events)
	t.events = append(t.events, EventDef{Name: e.Name, Params: append([]TypeRef(nil), e.Params...)})
	return nil
}

func (t *Table) AddComponent(c *Component) error {
	if t.frozen {
		return errFrozen
	}
	if c.Short == "" {
		c.Short = c.Name[strings.LastIndexByte(c.Name, '.')+1:]
	}
	for _, e := range t.comps {
		if e.Name == c.Name {
			return fmt.Errorf("component %s already registered", c.Name)
		}
	}
	for i := range c.Fields {
		c.Fields[i].Index = i
	}
	c.Index = len(t.comps)
	t.comps = append(t.comps, c)
	return nil
}

func (t *Table) Module(path string) bool {
	return t.modules[path]
}

func (t *Table) Func(module, name string) (Signature, bool) {
	s, ok := t.funcs[key(module, name)]
	return s, ok
}

func (t *Table) Method(recv Kind, name string) (Signature, bool) {
	s, ok := t.methods[key(recv.String(), name)]
	return s, ok
}

func (t *Table) ComponentMethod(comp, name string) (Signature, bool) {
	s, ok := t.cmethods[key(comp, name)]
	return s, ok
}

func (t *Table) Const(module, name string) (Const, bool) {
	c, ok := t.consts[key(module, name)]
	return c, ok
}

func (t *Table) Enum(module, name string) (*EnumDef, bool) {
	e, ok := t.enums[key(module, name)]
	return e, ok
}

func (t *Table) Struct(module, name string) (*StructDef, bool) {
	s, ok := t.structs[module+"."+name]
	return s, ok
}

func (t *Table) Event(name string) (Signature, bool) {
	i, ok := t.eventIdx[name]
	if !ok {
		return Signature{}, false
	}
	return t.events[i].Sig(), true
}

func (t *Table) EventIndex(name string) (int, bool) {
	i, ok := t.eventIdx[name]
	return i, ok
}

func (t *Table) Slots(r TypeRef) int {
	if r.Kind == Struct {
		if s, ok := t.structs[r.Name]; ok {
			return s.Slots
		}
	}
	return r.Slots()
}

func (t *Table) Components(name string) []*Component {
	for _, c := range t.comps {
		if c.Name == name {
			return []*Component{c}
		}
	}
	var out []*Component
	for _, c := range t.comps {
		if c.Short == name {
			out = append(out, c)
		}
	}
	return out
}

func (t *Table) AllComponents() []*Component {
	return t.comps
}

func (t *Table) Hash() [32]byte {
	if !t.frozen {
		panic("abi: Hash called on an unfrozen table; call Freeze first")
	}
	return t.hash
}

// Freeze seals the table and computes its hash; later Add* calls return errFrozen.
func (t *Table) Freeze() {
	if t.frozen {
		return
	}
	var lines []string
	for p := range t.modules {
		lines = append(lines, "p|"+p)
	}
	for k, s := range t.funcs {
		lines = append(lines, "f|"+k+"|"+s.String())
	}
	for k, s := range t.methods {
		lines = append(lines, "m|"+k+"|"+s.String())
	}
	for k, s := range t.cmethods {
		lines = append(lines, "q|"+k+"|"+s.String())
	}
	for k, s := range t.hmethods {
		lines = append(lines, "y|"+k+"|"+s.String())
	}
	for h, list := range t.hsignals {
		for i, s := range list {
			lines = append(lines, "z|"+h+"|"+strconv.Itoa(i)+"|"+s.Name+"|"+s.Sig().String())
		}
	}
	for k, h := range t.handles {
		lines = append(lines, "h|"+k+"|"+strconv.FormatBool(h.Await))
	}
	for k, c := range t.consts {
		lines = append(lines, "c|"+k+"|"+c.Type.String()+"|"+strconv.FormatInt(c.Int, 10)+"|"+
			strconv.FormatFloat(c.Float, 'g', -1, 64)+"|"+strconv.FormatBool(c.Bool)+"|"+c.Str)
	}
	for k, e := range t.enums {
		var b strings.Builder
		b.WriteString("e|" + k)
		for _, m := range e.Members {
			b.WriteString("|" + m.Name + "=" + strconv.FormatInt(m.Value, 10))
		}
		lines = append(lines, b.String())
	}
	for k, s := range t.structs {
		var b strings.Builder
		b.WriteString("t|" + k + "|" + strconv.Itoa(s.Slots))
		for _, f := range s.Fields {
			b.WriteString("|" + f.Name + ":" + f.Type.String())
		}
		lines = append(lines, b.String())
	}
	for i, e := range t.events {
		lines = append(lines, "v|"+strconv.Itoa(i)+"|"+e.Name+"|"+e.Sig().String())
	}
	for _, c := range t.comps {
		var b strings.Builder
		b.WriteString("k|" + c.Name)
		for _, f := range c.Fields {
			b.WriteString("|" + f.Name + ":" + f.Type.String() + ":" + strconv.FormatBool(f.ReadOnly))
		}
		for _, s := range c.Signals {
			b.WriteString("|!" + s.Name + s.Sig().String())
		}
		lines = append(lines, b.String())
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{'\n'})
	}
	copy(t.hash[:], h.Sum(nil))
	t.frozen = true
}
