package aetext

type ValueKind uint8

const (
	VInvalid ValueKind = iota
	VInt
	VFloat
	VBool
	VString
	VColor
	VIdent
	VAsset
	VNode
	VList
	VStruct
)

type Value struct {
	Kind   ValueKind
	Int    int64
	Float  float64
	Single bool
	Bool   bool
	Str    string
	Color  uint32
	Ident  string
	List   []Value
	Obj    []Field
}

func Int(v int64) Value {
	return Value{Kind: VInt, Int: v}
}

func Float32(v float32) Value {
	return Value{Kind: VFloat, Float: float64(v), Single: true}
}

func Float64(v float64) Value {
	return Value{Kind: VFloat, Float: v}
}

func Bool(v bool) Value {
	return Value{Kind: VBool, Bool: v}
}

func String(v string) Value {
	return Value{Kind: VString, Str: v}
}

func Color(v uint32) Value {
	return Value{Kind: VColor, Color: v}
}

func Ident(v string) Value {
	return Value{Kind: VIdent, Ident: v}
}

func AssetRef(id string) Value {
	return Value{Kind: VAsset, Str: id}
}

func NodeRef(id string) Value {
	return Value{Kind: VNode, Str: id}
}

func List(items ...Value) Value {
	return Value{Kind: VList, List: items}
}

func Struct(fields ...Field) Value {
	return Value{Kind: VStruct, Obj: fields}
}

func (v Value) AsFloat() float64 {
	switch v.Kind {
	case VFloat:
		return v.Float
	case VInt:
		return float64(v.Int)
	}
	return 0
}

func (v Value) AsInt() int64 {
	switch v.Kind {
	case VInt:
		return v.Int
	case VFloat:
		return int64(v.Float)
	}
	return 0
}

type Pos struct {
	Line, Col int
}

type Field struct {
	Name  string
	Value Value
	Pos   Pos
}

type NodeKind uint8

const NodeObject NodeKind = 0

type Node struct {
	Kind   NodeKind
	Head   string
	Label  string
	Attrs  []Field
	Fields []Field
	Blocks []*Node
	Pos    Pos
}

func (n *Node) Attr(name string) (Value, bool) {
	for i := range n.Attrs {
		if n.Attrs[i].Name == name {
			return n.Attrs[i].Value, true
		}
	}
	return Value{}, false
}

func (n *Node) Field(name string) (Value, bool) {
	for i := range n.Fields {
		if n.Fields[i].Name == name {
			return n.Fields[i].Value, true
		}
	}
	return Value{}, false
}

func (n *Node) SetField(name string, v Value) {
	for i := range n.Fields {
		if n.Fields[i].Name == name {
			n.Fields[i].Value = v
			return
		}
	}
	n.Fields = append(n.Fields, Field{Name: name, Value: v})
}

func (n *Node) RemoveField(name string) bool {
	for i := range n.Fields {
		if n.Fields[i].Name == name {
			n.Fields = append(n.Fields[:i], n.Fields[i+1:]...)
			return true
		}
	}
	return false
}

func (n *Node) Block(head, label string) *Node {
	for _, b := range n.Blocks {
		if b.Head == head && (label == "" || b.Label == label) {
			return b
		}
	}
	return nil
}

func (n *Node) EachBlock(head string, fn func(*Node)) {
	for _, b := range n.Blocks {
		if b.Head == head {
			fn(b)
		}
	}
}
