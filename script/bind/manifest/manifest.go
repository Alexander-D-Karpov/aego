package manifest

import (
	"errors"
	ststrings "strings"

	"aego/schema"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/runtime"
)

const Version uint32 = 6

type Manifest struct {
	Version       uint32
	ABIHash       [32]byte
	Modules       []Module
	Components    []Component
	Enums         []Enum
	Structs       []Struct
	Events        []Event
	Methods       []Method
	CompMethods   []CompMethod
	Handles       []HandleType
	HandleMethods []HandleMethod
	CompSignals   []CompSignal
	HandleSignals []HandleSignal
	Keywords      map[string]string
}

type HandleType struct {
	Name  string
	Await bool
	Doc   string
}

type HandleMethod struct {
	Handle  string
	Name    string
	Sig     abi.Signature
	Params  []string
	Example string
	Doc     string
	Loc     string
}

type CompSignal struct {
	Comp   string
	Name   string
	Params []abi.TypeRef
	Names  []string
	Doc    string
}

type HandleSignal struct {
	Handle string
	Name   string
	Params []abi.TypeRef
	Names  []string
	Doc    string
}

func (m *Manifest) collectSignals(r *bind.Registry, sr *schema.Registry) {
	t := r.Table()
	for _, c := range t.AllComponents() {
		var td *schema.TypeDescriptor
		if sr != nil {
			td, _ = sr.ByName(c.Name)
		}
		for i, s := range c.Signals {
			cs := CompSignal{Comp: c.Name, Name: s.Name, Params: s.Params}
			if td != nil && i < len(td.Signals) {
				cs.Names, cs.Doc = td.Signals[i].Names, td.Signals[i].Doc
			}
			m.CompSignals = append(m.CompSignals, cs)
		}
	}
	for _, hs := range t.HandleSignalList() {
		info := r.Info("#"+hs.Handle, hs.Def.Name)
		m.HandleSignals = append(m.HandleSignals, HandleSignal{
			Handle: hs.Handle, Name: hs.Def.Name, Params: hs.Def.Params, Names: info.Params, Doc: info.Doc,
		})
	}
}

type CompMethod struct {
	Comp    string
	Name    string
	Sig     abi.Signature
	Params  []string
	Example string
	Doc     string
	Loc     string
}

func (m *Manifest) CompMethodDoc(comp, name string) (CompMethod, bool) {
	if m == nil {
		return CompMethod{}, false
	}
	for _, cm := range m.CompMethods {
		if cm.Comp == comp && cm.Name == name {
			return cm, true
		}
	}
	return CompMethod{}, false
}

func (m *Manifest) HandleMethodDoc(handle, name string) (HandleMethod, bool) {
	if m == nil {
		return HandleMethod{}, false
	}
	for _, hm := range m.HandleMethods {
		if hm.Handle == handle && hm.Name == name {
			return hm, true
		}
	}
	return HandleMethod{}, false
}

func splitHandle(q string) (string, string) {
	i := ststrings.LastIndexByte(q, '.')
	if i < 0 {
		return "", q
	}
	return q[:i], q[i+1:]
}

func (m *Manifest) Keyword(key string) string {
	if m == nil {
		return ""
	}
	return m.Keywords[key]
}

type Module struct {
	Path   string
	Funcs  []Func
	Consts []Const
}

type Func struct {
	Name    string
	Sig     abi.Signature
	Params  []string
	Example string
	Doc     string
	Loc     string
}

type Const struct {
	Name string
	Val  abi.Const
	Doc  string
}

type Component struct {
	Name     string
	Short    string
	Category string
	Doc      string
	Builtin  bool
	Fields   []Field
}

type Field struct {
	Name     string
	Type     abi.TypeRef
	ReadOnly bool
	Doc      string
}

type Enum struct {
	Module  string
	Name    string
	Doc     string
	Members []abi.EnumMember
}

type Struct struct {
	Module string
	Name   string
	Doc    string
	Slots  int
	Fields []abi.StructField
}

type Event struct {
	Name   string
	Doc    string
	Params []abi.TypeRef
	Names  []string
}

type Method struct {
	Recv    abi.Kind
	Name    string
	Sig     abi.Signature
	Params  []string
	Example string
	Doc     string
	Loc     string
}

func FromRegistry(r *bind.Registry, sr *schema.Registry) (*Manifest, error) {
	if sr != nil && !sr.Frozen() {
		if err := sr.Freeze(); err != nil {
			return nil, err
		}
	}
	if !r.Frozen() {
		if sr != nil {
			runtime.BindComponents(r, sr)
		}
		if err := r.Freeze(); err != nil {
			return nil, err
		}
	}
	t := r.Table()
	m := &Manifest{Version: Version, ABIHash: t.Hash()}
	index := map[string]int{}
	for _, p := range t.ModuleList() {
		index[p] = len(m.Modules)
		m.Modules = append(m.Modules, Module{Path: p})
	}
	for _, f := range t.FuncList() {
		info := r.Info(f.Module, f.Name)
		mod := &m.Modules[index[f.Module]]
		mod.Funcs = append(mod.Funcs, Func{Name: f.Name, Sig: f.Sig, Params: info.Params, Example: info.Example, Doc: info.Doc, Loc: info.Loc})
	}
	for _, c := range t.ConstList() {
		mod := &m.Modules[index[c.Module]]
		mod.Consts = append(mod.Consts, Const{Name: c.Name, Val: c.Val, Doc: r.Info(c.Module, c.Name).Doc})
	}
	for _, e := range t.EnumList() {
		m.Enums = append(m.Enums, Enum{
			Module:  e.Module,
			Name:    e.Name,
			Doc:     r.Info(e.Module, e.Name).Doc,
			Members: append([]abi.EnumMember(nil), e.Members...),
		})
	}
	for _, s := range t.StructList() {
		m.Structs = append(m.Structs, Struct{
			Module: s.Module,
			Name:   s.Name,
			Doc:    r.Info(s.Module, s.Name).Doc,
			Slots:  s.Slots,
			Fields: append([]abi.StructField(nil), s.Fields...),
		})
	}
	for _, h := range t.HandleList() {
		mod, name := splitHandle(h.Name)
		m.Handles = append(m.Handles, HandleType{Name: h.Name, Await: h.Await, Doc: r.Info(mod, name).Doc})
	}
	for _, e := range t.EventList() {
		info := r.Info(bind.EventModule, e.Name)
		m.Events = append(m.Events, Event{Name: e.Name, Doc: info.Doc, Params: append([]abi.TypeRef(nil), e.Params...), Names: info.Params})
	}
	for _, me := range t.MethodList() {
		info := r.Info("#"+me.Recv.String(), me.Name)
		m.Methods = append(m.Methods, Method{Recv: me.Recv, Name: me.Name, Sig: me.Sig, Params: info.Params, Example: info.Example, Doc: info.Doc, Loc: info.Loc})
	}
	for _, cm := range t.ComponentMethodList() {
		info := r.Info("#"+cm.Comp, cm.Name)
		m.CompMethods = append(m.CompMethods, CompMethod{Comp: cm.Comp, Name: cm.Name, Sig: cm.Sig, Params: info.Params, Example: info.Example, Doc: info.Doc, Loc: info.Loc})
	}
	for _, hm := range t.HandleMethodList() {
		info := r.Info("#"+hm.Handle, hm.Name)
		m.HandleMethods = append(m.HandleMethods, HandleMethod{Handle: hm.Handle, Name: hm.Name, Sig: hm.Sig, Params: info.Params, Example: info.Example, Doc: info.Doc, Loc: info.Loc})
	}
	for _, c := range t.AllComponents() {
		m.Components = append(m.Components, component(c, sr))
	}
	m.collectSignals(r, sr)
	return m, nil
}

func component(c *abi.Component, sr *schema.Registry) Component {
	out := Component{Name: c.Name, Short: c.Short, Builtin: c.Builtin}
	out.Category, _, _ = ststrings.Cut(c.Name, ".")
	var fds []*schema.FieldDescriptor
	if sr != nil {
		if td, ok := sr.ByName(c.Name); ok {
			out.Doc = td.Doc
			fds = runtime.ComponentFields(td)
		}
	}
	for i, f := range c.Fields {
		fl := Field{Name: f.Name, Type: f.Type, ReadOnly: f.ReadOnly}
		if i < len(fds) {
			fl.Doc = fds[i].Doc
		}
		out.Fields = append(out.Fields, fl)
	}
	return out
}

func (m *Manifest) Table() (*abi.Table, error) {
	t := abi.NewTable()
	for _, mod := range m.Modules {
		if err := t.AddModule(mod.Path); err != nil {
			return nil, err
		}
	}
	for _, e := range m.Enums {
		def := &abi.EnumDef{Module: e.Module, Name: e.Name, Members: append([]abi.EnumMember(nil), e.Members...)}
		if err := t.AddEnum(def); err != nil {
			return nil, err
		}
	}
	for _, s := range m.Structs {
		def := &abi.StructDef{Module: s.Module, Name: s.Name, Slots: s.Slots, Fields: append([]abi.StructField(nil), s.Fields...)}
		if err := t.AddStruct(def); err != nil {
			return nil, err
		}
	}
	for _, h := range m.Handles {
		if err := t.AddHandle(abi.HandleDef{Name: h.Name, Await: h.Await}); err != nil {
			return nil, err
		}
	}
	for _, mod := range m.Modules {
		for _, f := range mod.Funcs {
			if err := t.AddFunc(mod.Path, f.Name, f.Sig); err != nil {
				return nil, err
			}
		}
		for _, c := range mod.Consts {
			if err := t.AddConst(mod.Path, c.Name, c.Val); err != nil {
				return nil, err
			}
		}
	}
	for _, me := range m.Methods {
		if err := t.AddMethod(me.Recv, me.Name, me.Sig); err != nil {
			return nil, err
		}
	}
	for _, cm := range m.CompMethods {
		if err := t.AddComponentMethod(cm.Comp, cm.Name, cm.Sig); err != nil {
			return nil, err
		}
	}
	for _, hm := range m.HandleMethods {
		if err := t.AddHandleMethod(hm.Handle, hm.Name, hm.Sig); err != nil {
			return nil, err
		}
	}
	for _, hs := range m.HandleSignals {
		if err := t.AddHandleSignal(hs.Handle, abi.SignalDef{Name: hs.Name, Params: hs.Params}); err != nil {
			return nil, err
		}
	}
	for _, e := range m.Events {
		if err := t.AddEvent(abi.EventDef{Name: e.Name, Params: e.Params}); err != nil {
			return nil, err
		}
	}
	for _, c := range m.Components {
		ac := &abi.Component{Name: c.Name, Short: c.Short, Builtin: c.Builtin}
		for _, f := range c.Fields {
			ac.Fields = append(ac.Fields, abi.Field{Name: f.Name, Type: f.Type, ReadOnly: f.ReadOnly})
		}
		for _, cs := range m.CompSignals {
			if cs.Comp == c.Name {
				ac.Signals = append(ac.Signals, abi.SignalDef{Name: cs.Name, Params: cs.Params})
			}
		}
		if err := t.AddComponent(ac); err != nil {
			return nil, err
		}
	}
	t.Freeze()
	if m.ABIHash != ([32]byte{}) && t.Hash() != m.ABIHash {
		return nil, errors.New("manifest: abi hash does not match its contents")
	}
	return t, nil
}

func (m *Manifest) FuncDoc(module, name string) (Func, bool) {
	if m == nil {
		return Func{}, false
	}
	for i := range m.Modules {
		if m.Modules[i].Path != module {
			continue
		}
		for _, f := range m.Modules[i].Funcs {
			if f.Name == name {
				return f, true
			}
		}
	}
	return Func{}, false
}

func (m *Manifest) ConstDoc(module, name string) (Const, bool) {
	if m == nil {
		return Const{}, false
	}
	for i := range m.Modules {
		if m.Modules[i].Path != module {
			continue
		}
		for _, c := range m.Modules[i].Consts {
			if c.Name == name {
				return c, true
			}
		}
	}
	return Const{}, false
}

func (m *Manifest) MethodDoc(recv abi.Kind, name string) (Method, bool) {
	if m == nil {
		return Method{}, false
	}
	for _, me := range m.Methods {
		if me.Recv == recv && me.Name == name {
			return me, true
		}
	}
	return Method{}, false
}

func (m *Manifest) Component(name string) (*Component, bool) {
	if m == nil {
		return nil, false
	}
	for i := range m.Components {
		if m.Components[i].Name == name {
			return &m.Components[i], true
		}
	}
	return nil, false
}

func (c *Component) Field(name string) (Field, bool) {
	for _, f := range c.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

func (m *Manifest) StructDoc(module, name string) (Struct, bool) {
	if m == nil {
		return Struct{}, false
	}
	for _, s := range m.Structs {
		if s.Module == module && s.Name == name {
			return s, true
		}
	}
	return Struct{}, false
}

func (m *Manifest) EventDoc(name string) (Event, bool) {
	if m == nil {
		return Event{}, false
	}
	for _, e := range m.Events {
		if e.Name == name {
			return e, true
		}
	}
	return Event{}, false
}
