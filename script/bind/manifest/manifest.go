package manifest

import (
	"errors"
	ststrings "strings"

	"aego/schema"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/runtime"
)

const Version uint32 = 2

type Manifest struct {
	Version    uint32
	ABIHash    [32]byte
	Modules    []Module
	Components []Component
	Enums      []Enum
	Methods    []Method
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
	for _, me := range t.MethodList() {
		info := r.Info("#"+me.Recv.String(), me.Name)
		m.Methods = append(m.Methods, Method{Recv: me.Recv, Name: me.Name, Sig: me.Sig, Params: info.Params, Example: info.Example, Doc: info.Doc, Loc: info.Loc})
	}
	for _, c := range t.AllComponents() {
		m.Components = append(m.Components, component(c, sr))
	}
	return m, nil
}

func component(c *abi.Component, sr *schema.Registry) Component {
	out := Component{Name: c.Name, Short: c.Short}
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
	for _, c := range m.Components {
		ac := &abi.Component{Name: c.Name, Short: c.Short}
		for _, f := range c.Fields {
			ac.Fields = append(ac.Fields, abi.Field{Name: f.Name, Type: f.Type, ReadOnly: f.ReadOnly})
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
