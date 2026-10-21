package types

import (
	"strings"

	"aego/script/abi"
)

type Kind uint8

const (
	KInvalid Kind = iota
	KVoid
	KBool
	KInt
	KFloat
	KString
	KVec2
	KVec3
	KColor
	KQuat
	KRect
	KTransform2D
	KTransform3D
	KNodeRef
	KAssetRef
	KComponentRef
	KEnum
	KStruct
	KSlice
	KNil
	KUntypedInt
	KUntypedFloat
	KCoroutine
	KConnection
	KScriptRef
	KCurve
	KHandle
)

type Field struct {
	Name string
	Type *Type
	Slot int
	Lane int
}

type Type struct {
	Kind      Kind
	Name      string
	Local     string
	Pkg       string
	Elem      *Type
	Fields    []*Field
	Members   []string
	Values    []int64
	Asset     string
	Component *abi.Component
	Script    string
	Handle    string
	Await     bool
	Host      bool
	slots     int
}

const (
	slotsUnknown = -1
	slotsBusy    = -2
)

func basic(k Kind, name string, slots int) *Type {
	return &Type{Kind: k, Name: name, Local: name, slots: slots}
}

func lanes(k Kind, name string, names ...string) *Type {
	t := basic(k, name, 1)
	for i, n := range names {
		t.Fields = append(t.Fields, &Field{Name: n, Type: Float, Lane: i})
	}
	return t
}

func engine(k Kind, name string, fields ...*Field) *Type {
	t := basic(k, name, 0)
	for _, f := range fields {
		f.Slot, f.Lane = t.slots, -1
		t.slots += f.Type.Slots()
		t.Fields = append(t.Fields, f)
	}
	return t
}

var (
	Invalid      = basic(KInvalid, "invalid", 1)
	Void         = basic(KVoid, "void", 0)
	Bool         = basic(KBool, "bool", 1)
	Int          = basic(KInt, "int", 1)
	Float        = basic(KFloat, "float", 1)
	String       = basic(KString, "string", 1)
	Vec2         = lanes(KVec2, "Vec2", "X", "Y")
	Vec3         = lanes(KVec3, "Vec3", "X", "Y", "Z")
	Color        = lanes(KColor, "Color", "R", "G", "B", "A")
	Quat         = lanes(KQuat, "Quat", "X", "Y", "Z", "W")
	Rect         = lanes(KRect, "Rect", "X", "Y", "W", "H")
	Transform2D  = engine(KTransform2D, "Transform2D", &Field{Name: "Position", Type: Vec2}, &Field{Name: "Rotation", Type: Float}, &Field{Name: "Scale", Type: Vec2})
	Transform3D  = engine(KTransform3D, "Transform3D", &Field{Name: "Position", Type: Vec3}, &Field{Name: "Rotation", Type: Quat}, &Field{Name: "Scale", Type: Vec3})
	NodeRef      = basic(KNodeRef, "NodeRef", 1)
	Nil          = basic(KNil, "nil", 1)
	UntypedInt   = basic(KUntypedInt, "untyped int", 1)
	UntypedFloat = basic(KUntypedFloat, "untyped float", 1)
	Coroutine    = basic(KCoroutine, "Coroutine", 1)
	Connection   = basic(KConnection, "Connection", 1)
	Curve        = basic(KCurve, "Curve", 1)
)

func shortPkg(pkg string) string {
	return pkg[strings.LastIndexByte(pkg, '/')+1:]
}

func NewStruct(pkg, name string) *Type {
	return &Type{Kind: KStruct, Name: shortPkg(pkg) + "." + name, Local: name, Pkg: pkg, slots: slotsUnknown}
}

func NewEnum(pkg, name string, members []string) *Type {
	t := &Type{Kind: KEnum, Name: shortPkg(pkg) + "." + name, Local: name, Pkg: pkg, Members: members, slots: 1}
	t.Values = make([]int64, len(members))
	for i := range members {
		t.Values[i] = int64(i)
	}
	return t
}

func (t *Type) String() string {
	return t.Name
}

func (t *Type) Qualified() string {
	if t.Pkg == "" {
		return t.Local
	}
	return t.Pkg + "." + t.Local
}

// Slots is the number of Value slots; a struct needs Layout first.
func (t *Type) Slots() int {
	if t.slots < 0 {
		return 1
	}
	return t.slots
}

// Layout assigns field slots and lanes; false means a recursive struct.
func (t *Type) Layout() bool {
	switch t.slots {
	case slotsBusy:
		return false
	case slotsUnknown:
	default:
		return true
	}
	t.slots = slotsBusy
	n := 0
	for _, f := range t.Fields {
		if f.Type.Kind == KStruct && !f.Type.Layout() {
			t.slots = 1
			return false
		}
		f.Slot, f.Lane = n, -1
		n += f.Type.Slots()
	}
	t.slots = n
	return true
}

func (t *Type) Field(name string) *Field {
	for _, f := range t.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func (t *Type) Member(name string) (int64, bool) {
	for i, m := range t.Members {
		if m == name {
			return t.Values[i], true
		}
	}
	return 0, false
}

func (t *Type) IsUntyped() bool {
	return t.Kind == KUntypedInt || t.Kind == KUntypedFloat
}

func (t *Type) IsNumeric() bool {
	switch t.Kind {
	case KInt, KFloat, KUntypedInt, KUntypedFloat:
		return true
	}
	return false
}

func (t *Type) IsInteger() bool {
	return t.Kind == KInt || t.Kind == KUntypedInt
}

func (t *Type) IsVector() bool {
	switch t.Kind {
	case KVec2, KVec3, KColor, KQuat, KRect:
		return true
	}
	return false
}

func (t *Type) IsRef() bool {
	switch t.Kind {
	case KNodeRef, KAssetRef, KComponentRef, KCoroutine, KConnection, KScriptRef, KHandle:
		return true
	}
	return false
}

func (t *Type) IsHeap() bool {
	return t.Kind == KString || t.Kind == KSlice
}

func (t *Type) Ordered() bool {
	return t.IsNumeric() || t.Kind == KString
}

func (t *Type) Comparable() bool {
	switch t.Kind {
	case KSlice, KVoid, KInvalid, KCurve:
		return false
	case KQuat, KTransform2D, KTransform3D:
		return false
	case KStruct:
		for _, f := range t.Fields {
			if !f.Type.Comparable() {
				return false
			}
		}
	}
	return true
}

func (t *Type) HeapSlots(base int, fn func(int)) {
	switch t.Kind {
	case KString, KSlice:
		fn(base)
	case KStruct, KTransform2D, KTransform3D:
		for _, f := range t.Fields {
			f.Type.HeapSlots(base+f.Slot, fn)
		}
	}
}

func Default(t *Type) *Type {
	switch t.Kind {
	case KUntypedInt:
		return Int
	case KUntypedFloat:
		return Float
	}
	return t
}

func Assignable(from, to *Type) bool {
	switch {
	case from == Invalid || to == Invalid:
		// Invalid is already reported; do not cascade.
		return true
	case from == to:
		return true
	case from == Nil:
		return to.IsRef()
	case from == UntypedInt:
		return to == Int || to == Float
	case from == UntypedFloat:
		return to == Float
	}
	return false
}

var assetKinds = map[string]string{"Texture": "texture", "Scene": "scene", "Prefab": "prefab", "Mesh": "mesh", "Clip": "audio"}

func AssetKind(name string) (string, bool) {
	k, ok := assetKinds[name]
	return k, ok
}

// Interner deduplicates derived types so Assignable can compare by pointer.
type Interner struct {
	slices  map[*Type]*Type
	assets  map[string]*Type
	comps   map[*abi.Component]*Type
	enums   map[string]*Type
	scripts map[string]*Type
	structs map[string]*Type
	handles map[string]*Type
}

func NewInterner() *Interner {
	return &Interner{
		slices:  map[*Type]*Type{},
		assets:  map[string]*Type{},
		comps:   map[*abi.Component]*Type{},
		enums:   map[string]*Type{},
		scripts: map[string]*Type{},
		structs: map[string]*Type{},
		handles: map[string]*Type{},
	}
}

func (u *Interner) Handle(d abi.HandleDef) *Type {
	if t, ok := u.handles[d.Name]; ok {
		return t
	}
	i := strings.LastIndexByte(d.Name, '.')
	mod, local := d.Name[:max(i, 0)], d.Name[i+1:]
	t := &Type{Kind: KHandle, Name: shortPkg(mod) + "." + local, Local: local, Pkg: mod, Handle: d.Name, Await: d.Await, Host: true, slots: 1}
	u.handles[d.Name] = t
	return t
}

func (u *Interner) ScriptRef(full string) *Type {
	if t, ok := u.scripts[full]; ok {
		return t
	}
	short := full[strings.LastIndexByte(full, '.')+1:]
	t := &Type{Kind: KScriptRef, Name: "ScriptRef<" + short + ">", Local: "ScriptRef", Script: full, slots: 1}
	u.scripts[full] = t
	return t
}

func (u *Interner) SliceOf(e *Type) *Type {
	if t, ok := u.slices[e]; ok {
		return t
	}
	t := &Type{Kind: KSlice, Name: "[]" + e.Name, Local: "[]" + e.Name, Elem: e, slots: 1}
	u.slices[e] = t
	return t
}

func (u *Interner) AssetRef(kind string) *Type {
	if t, ok := u.assets[kind]; ok {
		return t
	}
	local := "AssetRef"
	var name string
	switch {
	case strings.HasPrefix(kind, "data:"):
		name = "AssetRef<" + kind[len("data:"):] + ">"
	case strings.HasPrefix(kind, "table:"):
		local = "TableRef"
		name = "TableRef<" + kind[len("table:"):] + ">"
	default:
		display := kind
		for k, v := range assetKinds {
			if v == kind {
				display = k
			}
		}
		name = "AssetRef<" + display + ">"
	}
	t := &Type{Kind: KAssetRef, Name: name, Local: local, Asset: kind, slots: 1}
	u.assets[kind] = t
	return t
}

func (u *Interner) ComponentRef(c *abi.Component) *Type {
	if t, ok := u.comps[c]; ok {
		return t
	}
	t := &Type{Kind: KComponentRef, Name: "ComponentRef<" + c.Short + ">", Local: "ComponentRef", Component: c, slots: 1}
	u.comps[c] = t
	return t
}

func (u *Interner) HostEnum(e *abi.EnumDef) *Type {
	q := e.Qualified()
	if t, ok := u.enums[q]; ok {
		return t
	}
	t := &Type{Kind: KEnum, Name: shortPkg(e.Module) + "." + e.Name, Local: e.Name, Pkg: e.Module, Host: true, slots: 1}
	for _, m := range e.Members {
		t.Members = append(t.Members, m.Name)
		t.Values = append(t.Values, m.Value)
	}
	u.enums[q] = t
	return t
}

func (u *Interner) HostStruct(d *abi.StructDef, s abi.Surface) *Type {
	q := d.Qualified()
	if t, ok := u.structs[q]; ok {
		return t
	}
	t := &Type{Kind: KStruct, Name: shortPkg(d.Module) + "." + d.Name, Local: d.Name, Pkg: d.Module, Host: true, slots: slotsUnknown}
	u.structs[q] = t
	for _, f := range d.Fields {
		t.Fields = append(t.Fields, &Field{Name: f.Name, Type: u.FromABI(f.Type, s), Lane: -1})
	}
	t.Layout()
	return t
}

func (u *Interner) FromABI(r abi.TypeRef, s abi.Surface) *Type {
	switch r.Kind {
	case abi.Void:
		return Void
	case abi.Bool:
		return Bool
	case abi.Int:
		return Int
	case abi.Float:
		return Float
	case abi.String:
		return String
	case abi.Vec2:
		return Vec2
	case abi.Vec3:
		return Vec3
	case abi.Color:
		return Color
	case abi.Quat:
		return Quat
	case abi.Rect:
		return Rect
	case abi.Transform2D:
		return Transform2D
	case abi.Transform3D:
		return Transform3D
	case abi.NodeRef:
		return NodeRef
	case abi.Coroutine:
		return Coroutine
	case abi.Connection:
		return Connection
	case abi.Curve:
		return Curve
	case abi.ScriptRef:
		return u.ScriptRef(r.Name)
	case abi.AssetRef:
		return u.AssetRef(r.Name)
	case abi.Handle:
		if s != nil {
			if d, ok := s.Handle(r.Name); ok {
				return u.Handle(d)
			}
		}
	case abi.ComponentRef:
		if s != nil {
			if l := s.Components(r.Name); len(l) == 1 {
				return u.ComponentRef(l[0])
			}
		}
	case abi.Enum:
		if i := strings.LastIndexByte(r.Name, '.'); s != nil && i > 0 {
			if e, ok := s.Enum(r.Name[:i], r.Name[i+1:]); ok {
				return u.HostEnum(e)
			}
		}
	case abi.Struct:
		if i := strings.LastIndexByte(r.Name, '.'); s != nil && i > 0 {
			if d, ok := s.Struct(r.Name[:i], r.Name[i+1:]); ok {
				return u.HostStruct(d, s)
			}
		}
	case abi.Slice:
		if r.Elem != nil {
			return u.SliceOf(u.FromABI(*r.Elem, s))
		}
	}
	return Invalid
}

func ToABI(t *Type) abi.TypeRef {
	switch t.Kind {
	case KBool:
		return abi.T(abi.Bool)
	case KInt, KUntypedInt:
		return abi.T(abi.Int)
	case KFloat, KUntypedFloat:
		return abi.T(abi.Float)
	case KString:
		return abi.T(abi.String)
	case KVec2:
		return abi.T(abi.Vec2)
	case KVec3:
		return abi.T(abi.Vec3)
	case KColor:
		return abi.T(abi.Color)
	case KQuat:
		return abi.T(abi.Quat)
	case KRect:
		return abi.T(abi.Rect)
	case KTransform2D:
		return abi.T(abi.Transform2D)
	case KTransform3D:
		return abi.T(abi.Transform3D)
	case KNodeRef:
		return abi.T(abi.NodeRef)
	case KAssetRef:
		return abi.AssetOf(t.Asset)
	case KComponentRef:
		return abi.ComponentOf(t.Component.Name)
	case KCoroutine:
		return abi.T(abi.Coroutine)
	case KConnection:
		return abi.T(abi.Connection)
	case KScriptRef:
		return abi.ScriptRefOf(t.Script)
	case KCurve:
		return abi.T(abi.Curve)
	case KHandle:
		return abi.HandleOf(t.Handle)
	case KEnum:
		return abi.EnumOf(t.Qualified())
	case KStruct:
		return abi.StructOf(t.Qualified())
	case KSlice:
		return abi.SliceOf(ToABI(t.Elem))
	}
	return abi.T(abi.Void)
}
