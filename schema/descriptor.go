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
)

func (f FieldFlags) Has(o FieldFlags) bool {
	return f&o == o
}

type FieldDescriptor struct {
	ID        FieldID
	Name      string
	Former    []string
	Kind      Kind
	Elem      Kind
	Struct    TypeID
	Enum      []EnumValue
	EnumName  string
	AssetKind string
	Flags     FieldFlags
	Offset    uintptr
	Size      uintptr
	Fast      bool
	Min, Max  float64
	HasMin    bool
	HasMax    bool
	Hints     EditorHints
	Doc       string
	Get       func(p unsafe.Pointer, dst *Value)
	Set       func(p unsafe.Pointer, src Value) error
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
)

type TypeDescriptor struct {
	ID       TypeID
	FullName string
	Former   []string
	Version  uint32
	Size     uintptr
	Kind     TypeKind
	Doc      string
	Fields   []FieldDescriptor
	index    map[string]int
	byID     map[FieldID]int
	New      func() unsafe.Pointer
	Copy     func(dst, src unsafe.Pointer)
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
