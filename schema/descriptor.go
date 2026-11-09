package schema

import "unsafe"

type FieldFlags uint32

const (
	FlagSerialize FieldFlags = 1 << iota
	FlagEditor
	FlagReadOnly
	FlagTransient
	FlagScript
	FlagReplicated
	FlagLive
	FlagPersist
	// FlagOmitEmpty keeps optional zero-valued fields absent from source files.
	FlagOmitEmpty
)

func (f FieldFlags) Has(o FieldFlags) bool {
	return f&o == o
}

type FieldDescriptor struct {
	ID         FieldID
	Name       string
	Former     []string
	Kind       Kind
	Elem       Kind
	Struct     TypeID
	Enum       []EnumValue
	EnumName   string
	AssetKind  string
	Component  string
	Flags      FieldFlags
	Offset     uintptr
	Size       uintptr
	Fast       bool
	Direct     bool
	Unsigned   bool
	Min, Max   float64
	HasMin     bool
	HasMax     bool
	Hints      EditorHints
	Doc        string
	Default    Value
	HasDefault bool
	Get        func(p unsafe.Pointer, dst *Value)
	Set        func(p unsafe.Pointer, src Value) error
}

func (f *FieldDescriptor) EnumNameOf(v int64) (string, bool) {
	for i := range f.Enum {
		if f.Enum[i].Value == v {
			return f.Enum[i].Name, true
		}
	}
	return "", false
}

func (f *FieldDescriptor) EnumValueOf(name string) (int64, bool) {
	for i := range f.Enum {
		if f.Enum[i].Name == name {
			return f.Enum[i].Value, true
		}
	}
	return 0, false
}

type TypeKind uint8

const (
	TypeComponent TypeKind = iota
	TypeStruct
	TypeAsset
	TypeScript
)

type TemplateDesc struct {
	ID       string
	Name     string
	Node     string
	Category string
	Menu     string
	Icon     string
	Space    SpaceKind
	Owner    string
	Builtin  bool
	Setup    any
}

type ScriptMethod struct {
	Name string
	Fn   any
	Bind any
	Opts []any
}

type TypeDescriptor struct {
	ID          TypeID
	FullName    string
	Former      []string
	Version     uint32
	Size        uintptr
	Kind        TypeKind
	Space       SpaceKind
	Doc         string
	Hidden      bool
	DisplayName string
	// SearchAliases are display-name aliases only; they never rename serialized types.
	SearchAliases []string
	Category      string
	Icon          string
	NodeIcon      int
	Owner         string
	Builtin       bool
	SceneUnique   bool
	Requires      []string
	RequiresAny   [][]string
	Suggests      []string
	Conflicts     []string
	Default       func() unsafe.Pointer
	Template      *TemplateDesc
	ScriptFields  []FieldID
	ScriptMethods []ScriptMethod
	Signals       []SignalDesc
	Inspector     any
	OnAdd         any
	OnRemoving    any
	OnChanged     any
	Validate      any
	Fields        []FieldDescriptor
	index         map[string]int
	byID          map[FieldID]int
	New           func() unsafe.Pointer
	Copy          func(dst, src unsafe.Pointer)
	Flat          bool
	CloneInto     func(dst, src unsafe.Pointer)

	CheckedLiterals bool
}

func (t *TypeDescriptor) Scripted(fd *FieldDescriptor) bool {
	if t.ScriptFields == nil {
		return fd.Flags.Has(FlagScript)
	}
	for _, id := range t.ScriptFields {
		if id == fd.ID {
			return true
		}
	}
	return false
}

func (t *TypeDescriptor) Field(name string) (*FieldDescriptor, bool) {
	i, ok := t.index[name]
	if !ok {
		return nil, false
	}
	return &t.Fields[i], true
}

func (t *TypeDescriptor) FieldByID(id FieldID) (*FieldDescriptor, bool) {
	i, ok := t.byID[id]
	if !ok {
		return nil, false
	}
	return &t.Fields[i], true
}

func (t *TypeDescriptor) Serializable(fn func(*FieldDescriptor)) {
	for i := range t.Fields {
		if t.Fields[i].Flags.Has(FlagSerialize) {
			fn(&t.Fields[i])
		}
	}
}

type SignalDesc struct {
	Name   string
	Params []string
	Names  []string
	Doc    string
}
