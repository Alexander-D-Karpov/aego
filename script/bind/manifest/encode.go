package manifest

import (
	"encoding/hex"
	"errors"
	"fmt"

	"aego/format/aetext"
	"aego/script/abi"
)

type extra struct {
	params  []string
	example string
	doc     string
	loc     string
}

func Encode(m *Manifest) []byte {
	root := &aetext.Node{Head: "manifest"}
	root.Attrs = append(root.Attrs, aetext.Field{Name: "version", Value: aetext.Int(int64(m.Version))})
	root.Fields = append(root.Fields, aetext.Field{Name: "abi", Value: aetext.String(hex.EncodeToString(m.ABIHash[:]))})
	for _, mod := range m.Modules {
		n := &aetext.Node{Head: "module", Label: mod.Path}
		for _, f := range mod.Funcs {
			n.Blocks = append(n.Blocks, callable("func", f.Name, f.Sig, extra{f.Params, f.Example, f.Doc, f.Loc}))
		}
		for _, c := range mod.Consts {
			n.Blocks = append(n.Blocks, constant(c))
		}
		root.Blocks = append(root.Blocks, n)
	}
	for _, e := range m.Enums {
		n := &aetext.Node{Head: "enum", Label: e.Name}
		n.Attrs = append(n.Attrs, aetext.Field{Name: "module", Value: aetext.String(e.Module)})
		text(n, "doc", e.Doc)
		for _, mem := range e.Members {
			b := &aetext.Node{Head: "member", Label: mem.Name}
			b.Attrs = append(b.Attrs, aetext.Field{Name: "value", Value: aetext.Int(mem.Value)})
			n.Blocks = append(n.Blocks, b)
		}
		root.Blocks = append(root.Blocks, n)
	}
	for _, s := range m.Structs {
		n := &aetext.Node{Head: "struct", Label: s.Name}
		n.Attrs = append(n.Attrs,
			aetext.Field{Name: "module", Value: aetext.String(s.Module)},
			aetext.Field{Name: "slots", Value: aetext.Int(int64(s.Slots))},
		)
		text(n, "doc", s.Doc)
		for _, f := range s.Fields {
			b := &aetext.Node{Head: "field", Label: f.Name}
			b.Fields = append(b.Fields, aetext.Field{Name: "type", Value: aetext.String(f.Type.String())})
			n.Blocks = append(n.Blocks, b)
		}
		root.Blocks = append(root.Blocks, n)
	}
	for _, e := range m.Events {
		params := make([]string, len(e.Params))
		for i, p := range e.Params {
			params[i] = p.String()
		}
		n := &aetext.Node{Head: "event", Label: e.Name}
		n.Fields = append(n.Fields, aetext.Field{Name: "params", Value: strList(params)})
		if len(e.Names) > 0 {
			n.Fields = append(n.Fields, aetext.Field{Name: "names", Value: strList(e.Names)})
		}
		text(n, "doc", e.Doc)
		root.Blocks = append(root.Blocks, n)
	}
	for _, me := range m.Methods {
		n := callable("method", me.Name, me.Sig, extra{me.Params, me.Example, me.Doc, me.Loc})
		n.Attrs = append(n.Attrs, aetext.Field{Name: "recv", Value: aetext.String(me.Recv.String())})
		root.Blocks = append(root.Blocks, n)
	}
	for _, cm := range m.CompMethods {
		n := callable("cmethod", cm.Name, cm.Sig, extra{cm.Params, cm.Example, cm.Doc, cm.Loc})
		n.Attrs = append(n.Attrs, aetext.Field{Name: "comp", Value: aetext.String(cm.Comp)})
		root.Blocks = append(root.Blocks, n)
	}
	for _, c := range m.Components {
		n := &aetext.Node{Head: "component", Label: c.Name}
		text(n, "short", c.Short)
		text(n, "category", c.Category)
		text(n, "doc", c.Doc)
		for _, f := range c.Fields {
			b := &aetext.Node{Head: "field", Label: f.Name}
			b.Fields = append(b.Fields,
				aetext.Field{Name: "type", Value: aetext.String(f.Type.String())},
				aetext.Field{Name: "readonly", Value: aetext.Bool(f.ReadOnly)},
			)
			text(b, "doc", f.Doc)
			n.Blocks = append(n.Blocks, b)
		}
		root.Blocks = append(root.Blocks, n)
	}
	return aetext.Write(root)
}

func text(n *aetext.Node, name, v string) {
	if v != "" {
		n.Fields = append(n.Fields, aetext.Field{Name: name, Value: aetext.String(v)})
	}
}

func strList(list []string) aetext.Value {
	out := make([]aetext.Value, len(list))
	for i, s := range list {
		out[i] = aetext.String(s)
	}
	return aetext.List(out...)
}

func callable(head, name string, s abi.Signature, x extra) *aetext.Node {
	n := &aetext.Node{Head: head, Label: name}
	params := make([]string, len(s.Params))
	for i, p := range s.Params {
		params[i] = p.String()
	}
	n.Fields = append(n.Fields,
		aetext.Field{Name: "params", Value: strList(params)},
		aetext.Field{Name: "result", Value: aetext.String(s.Result.String())},
	)
	if len(x.params) > 0 {
		n.Fields = append(n.Fields, aetext.Field{Name: "names", Value: strList(x.params)})
	}
	text(n, "doc", x.doc)
	text(n, "example", x.example)
	text(n, "loc", x.loc)
	return n
}

func constant(c Const) *aetext.Node {
	n := &aetext.Node{Head: "const", Label: c.Name}
	n.Fields = append(n.Fields,
		aetext.Field{Name: "type", Value: aetext.String(c.Val.Type.String())},
		aetext.Field{Name: "int", Value: aetext.Int(c.Val.Int)},
		aetext.Field{Name: "float", Value: aetext.Float64(c.Val.Float)},
		aetext.Field{Name: "bool", Value: aetext.Bool(c.Val.Bool)},
		aetext.Field{Name: "str", Value: aetext.String(c.Val.Str)},
	)
	text(n, "doc", c.Doc)
	return n
}

func str(n *aetext.Node, name string) string {
	if v, ok := n.Field(name); ok {
		return v.Str
	}
	return ""
}

func list(n *aetext.Node, name string) []string {
	v, ok := n.Field(name)
	if !ok {
		return nil
	}
	out := make([]string, len(v.List))
	for i, s := range v.List {
		out[i] = s.Str
	}
	return out
}

func typeRef(s string) (abi.TypeRef, error) {
	t, ok := abi.Parse(s)
	if !ok {
		return abi.TypeRef{}, fmt.Errorf("manifest: malformed type %q", s)
	}
	return t, nil
}

func readSig(n *aetext.Node) (abi.Signature, error) {
	var s abi.Signature
	for _, p := range list(n, "params") {
		t, err := typeRef(p)
		if err != nil {
			return s, err
		}
		s.Params = append(s.Params, t)
	}
	res := str(n, "result")
	if res == "" {
		res = "void"
	}
	t, err := typeRef(res)
	s.Result = t
	return s, err
}

func Decode(b []byte) (*Manifest, error) {
	doc, err := aetext.Parse(b, "manifest.aetext")
	if err != nil {
		return nil, err
	}
	if doc.Head != "manifest" {
		return nil, errors.New("manifest: not a bindings manifest")
	}
	m := &Manifest{Version: Version}
	if v, ok := doc.Attr("version"); ok {
		m.Version = uint32(v.AsInt())
	}
	if m.Version == 0 || m.Version > Version {
		return nil, fmt.Errorf("manifest: version %d is not supported", m.Version)
	}
	if v, ok := doc.Field("abi"); ok {
		h, err := hex.DecodeString(v.Str)
		if err != nil || len(h) != len(m.ABIHash) {
			return nil, errors.New("manifest: malformed abi hash")
		}
		copy(m.ABIHash[:], h)
	}
	for _, n := range doc.Blocks {
		var err error
		switch n.Head {
		case "module":
			err = m.readModule(n)
		case "enum":
			m.readEnum(n)
		case "struct":
			err = m.readStruct(n)
		case "event":
			err = m.readEvent(n)
		case "method":
			err = m.readMethod(n)
		case "cmethod":
			err = m.readCompMethod(n)
		case "component":
			err = m.readComponent(n)
		default:
			err = fmt.Errorf("manifest: unknown block %s", n.Head)
		}
		if err != nil {
			return nil, err
		}
	}
	if _, err := m.Table(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manifest) readModule(n *aetext.Node) error {
	mod := Module{Path: n.Label}
	for _, b := range n.Blocks {
		switch b.Head {
		case "func":
			sig, err := readSig(b)
			if err != nil {
				return err
			}
			mod.Funcs = append(mod.Funcs, Func{
				Name: b.Label, Sig: sig, Params: list(b, "names"), Example: str(b, "example"),
				Doc: str(b, "doc"), Loc: str(b, "loc"),
			})
		case "const":
			t, err := typeRef(str(b, "type"))
			if err != nil {
				return err
			}
			c := Const{Name: b.Label, Doc: str(b, "doc"), Val: abi.Const{Type: t, Str: str(b, "str")}}
			if v, ok := b.Field("int"); ok {
				c.Val.Int = v.AsInt()
			}
			if v, ok := b.Field("float"); ok {
				c.Val.Float = v.AsFloat()
			}
			if v, ok := b.Field("bool"); ok {
				c.Val.Bool = v.Bool
			}
			mod.Consts = append(mod.Consts, c)
		}
	}
	m.Modules = append(m.Modules, mod)
	return nil
}

func (m *Manifest) readEnum(n *aetext.Node) {
	e := Enum{Name: n.Label, Doc: str(n, "doc")}
	if v, ok := n.Attr("module"); ok {
		e.Module = v.Str
	}
	n.EachBlock("member", func(b *aetext.Node) {
		mem := abi.EnumMember{Name: b.Label}
		if v, ok := b.Attr("value"); ok {
			mem.Value = v.AsInt()
		}
		e.Members = append(e.Members, mem)
	})
	m.Enums = append(m.Enums, e)
}

func (m *Manifest) readStruct(n *aetext.Node) error {
	s := Struct{Name: n.Label, Doc: str(n, "doc")}
	if v, ok := n.Attr("module"); ok {
		s.Module = v.Str
	}
	if v, ok := n.Attr("slots"); ok {
		s.Slots = int(v.AsInt())
	}
	var ferr error
	n.EachBlock("field", func(b *aetext.Node) {
		t, err := typeRef(str(b, "type"))
		if err != nil {
			ferr = err
			return
		}
		s.Fields = append(s.Fields, abi.StructField{Name: b.Label, Type: t})
	})
	m.Structs = append(m.Structs, s)
	return ferr
}

func (m *Manifest) readEvent(n *aetext.Node) error {
	e := Event{Name: n.Label, Doc: str(n, "doc"), Names: list(n, "names")}
	for _, p := range list(n, "params") {
		t, err := typeRef(p)
		if err != nil {
			return err
		}
		e.Params = append(e.Params, t)
	}
	m.Events = append(m.Events, e)
	return nil
}

func (m *Manifest) readMethod(n *aetext.Node) error {
	recv := ""
	if v, ok := n.Attr("recv"); ok {
		recv = v.Str
	}
	kind, ok := abi.KindOf(recv)
	if !ok {
		return fmt.Errorf("manifest: unknown receiver %q", recv)
	}
	sig, err := readSig(n)
	if err != nil {
		return err
	}
	m.Methods = append(m.Methods, Method{
		Recv: kind, Name: n.Label, Sig: sig, Params: list(n, "names"), Example: str(n, "example"),
		Doc: str(n, "doc"), Loc: str(n, "loc"),
	})
	return nil
}

func (m *Manifest) readCompMethod(n *aetext.Node) error {
	comp := ""
	if v, ok := n.Attr("comp"); ok {
		comp = v.Str
	}
	if comp == "" {
		return fmt.Errorf("manifest: component method %s has no component", n.Label)
	}
	sig, err := readSig(n)
	if err != nil {
		return err
	}
	m.CompMethods = append(m.CompMethods, CompMethod{
		Comp: comp, Name: n.Label, Sig: sig, Params: list(n, "names"), Example: str(n, "example"),
		Doc: str(n, "doc"), Loc: str(n, "loc"),
	})
	return nil
}

func (m *Manifest) readComponent(n *aetext.Node) error {
	c := Component{Name: n.Label, Short: str(n, "short"), Category: str(n, "category"), Doc: str(n, "doc")}
	var ferr error
	n.EachBlock("field", func(b *aetext.Node) {
		t, err := typeRef(str(b, "type"))
		if err != nil {
			ferr = err
			return
		}
		f := Field{Name: b.Label, Type: t, Doc: str(b, "doc")}
		if v, ok := b.Field("readonly"); ok {
			f.ReadOnly = v.Bool
		}
		c.Fields = append(c.Fields, f)
	})
	m.Components = append(m.Components, c)
	return ferr
}
