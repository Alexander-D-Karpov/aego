package abi

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Kind uint8

const (
	Void Kind = iota
	Bool
	Int
	Float
	String
	Vec2
	Vec3
	Color
	Quat
	Rect
	Transform2D
	Transform3D
	NodeRef
	AssetRef
	ComponentRef
	Enum
	Struct
	Slice
	Coroutine
	Connection
	ScriptRef
	kindCount
)

var kindNames = [kindCount]string{
	"void", "bool", "int", "float", "string", "Vec2", "Vec3", "Color", "Quat", "Rect",
	"Transform2D", "Transform3D", "NodeRef", "AssetRef", "ComponentRef", "enum", "struct", "slice",
	"Coroutine", "Connection", "ScriptRef",
}

func (k Kind) String() string {
	if k < kindCount {
		return kindNames[k]
	}
	return "kind" + strconv.Itoa(int(k))
}

type TypeRef struct {
	Kind Kind
	Name string
	Elem *TypeRef
}

func T(k Kind) TypeRef {
	return TypeRef{Kind: k}
}

func SliceOf(e TypeRef) TypeRef {
	return TypeRef{Kind: Slice, Elem: &e}
}

func AssetOf(kind string) TypeRef {
	return TypeRef{Kind: AssetRef, Name: kind}
}

func ComponentOf(name string) TypeRef {
	return TypeRef{Kind: ComponentRef, Name: name}
}

func EnumOf(qualified string) TypeRef {
	return TypeRef{Kind: Enum, Name: qualified}
}

func StructOf(qualified string) TypeRef {
	return TypeRef{Kind: Struct, Name: qualified}
}

func ScriptRefOf(script string) TypeRef {
	return TypeRef{Kind: ScriptRef, Name: script}
}

func (t TypeRef) String() string {
	switch t.Kind {
	case Slice:
		if t.Elem == nil {
			return "[]void"
		}
		return "[]" + t.Elem.String()
	case AssetRef:
		return "AssetRef<" + t.Name + ">"
	case ComponentRef:
		return "ComponentRef<" + t.Name + ">"
	case ScriptRef:
		return "ScriptRef<" + t.Name + ">"
	case Enum:
		return "enum:" + t.Name
	case Struct:
		return "struct:" + t.Name
	}
	return t.Kind.String()
}

func Parse(s string) (TypeRef, bool) {
	switch {
	case strings.HasPrefix(s, "[]"):
		e, ok := Parse(s[2:])
		if !ok || e.Kind == Void {
			return TypeRef{}, false
		}
		return SliceOf(e), true
	case strings.HasPrefix(s, "AssetRef<") && strings.HasSuffix(s, ">"):
		return AssetOf(s[len("AssetRef<") : len(s)-1]), true
	case strings.HasPrefix(s, "ComponentRef<") && strings.HasSuffix(s, ">"):
		return ComponentOf(s[len("ComponentRef<") : len(s)-1]), true
	case strings.HasPrefix(s, "ScriptRef<") && strings.HasSuffix(s, ">"):
		return ScriptRefOf(s[len("ScriptRef<") : len(s)-1]), len(s) > len("ScriptRef<>")
	case strings.HasPrefix(s, "enum:"):
		return EnumOf(s[5:]), len(s) > 5
	case strings.HasPrefix(s, "struct:"):
		return StructOf(s[7:]), len(s) > 7
	case s == "Coroutine":
		return T(Coroutine), true
	case s == "Connection":
		return T(Connection), true
	}
	for k := Void; k < Enum; k++ {
		if kindNames[k] == s && k != AssetRef && k != ComponentRef {
			return T(k), true
		}
	}
	return TypeRef{}, false
}

func (t TypeRef) Equal(o TypeRef) bool {
	if t.Kind != o.Kind || t.Name != o.Name {
		return false
	}
	if t.Kind == Slice {
		if t.Elem == nil || o.Elem == nil {
			return t.Elem == o.Elem
		}
		return t.Elem.Equal(*o.Elem)
	}
	return true
}

func (t TypeRef) Slots() int {
	switch t.Kind {
	case Void:
		return 0
	case Transform2D, Transform3D:
		return 3
	}
	return 1
}

func (t TypeRef) Heap() bool {
	return t.Kind == String || t.Kind == Slice
}

type Signature struct {
	Params []TypeRef
	Result TypeRef
}

func Sig(result TypeRef, params ...TypeRef) Signature {
	return Signature{Params: params, Result: result}
}

func (s Signature) String() string {
	var b strings.Builder
	b.WriteByte('(')
	for i, p := range s.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.String())
	}
	b.WriteByte(')')
	if s.Result.Kind != Void {
		b.WriteByte(' ')
		b.WriteString(s.Result.String())
	}
	return b.String()
}

func (s Signature) ParamSlots() int {
	n := 0
	for _, p := range s.Params {
		n += p.Slots()
	}
	return n
}

func (s Signature) ResultSlots() int {
	return s.Result.Slots()
}

func (s Signature) Window() int {
	return max(s.ParamSlots(), s.ResultSlots())
}

func (s Signature) Equal(o Signature) bool {
	if len(s.Params) != len(o.Params) || !s.Result.Equal(o.Result) {
		return false
	}
	for i := range s.Params {
		if !s.Params[i].Equal(o.Params[i]) {
			return false
		}
	}
	return true
}

// Validate rejects signatures that cannot cross the host boundary. Per D97 a
// host function takes and returns only scalars, vectors, Transform, references
// and enums: structs and slices have no stable Go-side read API yet, and their
// Slots() would silently report 1 and corrupt the call window. Strings are
// allowed — they cross as a heap handle the adapter copies.
func (s Signature) Validate() error {
	for i, p := range s.Params {
		if !hostable(p) {
			return fmt.Errorf("param %d: %s cannot cross the host boundary", i, p)
		}
	}
	if s.Result.Kind != Void && !hostable(s.Result) {
		return fmt.Errorf("result: %s cannot cross the host boundary", s.Result)
	}
	return nil
}

func hostable(t TypeRef) bool {
	switch t.Kind {
	case Void, Struct, Slice, Coroutine, Connection, ScriptRef:
		return false
	}
	return true
}

type EnumMember struct {
	Name  string
	Value int64
}

type EnumDef struct {
	Module  string
	Name    string
	Members []EnumMember
}

func (e *EnumDef) Qualified() string {
	return e.Module + "." + e.Name
}

type Const struct {
	Type  TypeRef
	Int   int64
	Float float64
	Bool  bool
	Str   string
}

type Field struct {
	Name     string
	Type     TypeRef
	ReadOnly bool
	Index    int
}

type Component struct {
	Name   string
	Short  string
	Index  int
	Fields []Field
}

func (c *Component) Field(name string) (*Field, bool) {
	for i := range c.Fields {
		if c.Fields[i].Name == name {
			return &c.Fields[i], true
		}
	}
	return nil, false
}

type Surface interface {
	Module(path string) bool
	Func(module, name string) (Signature, bool)
	Method(recv Kind, name string) (Signature, bool)
	Const(module, name string) (Const, bool)
	Enum(module, name string) (*EnumDef, bool)
	Components(name string) []*Component
	Hash() [32]byte
}

func Exported(name string) bool {
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r)
}
