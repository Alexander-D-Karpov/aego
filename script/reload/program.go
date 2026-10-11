package reload

import (
	"sort"

	"aego/script/abi"
	"aego/script/bytecode"
)

type Program struct {
	ID      uint64
	Modules map[string]*bytecode.Module
	Hashes  map[string][32]byte
}

func NewProgram(id uint64, mods map[string]*bytecode.Module, hashes map[string][32]byte) Program {
	return Program{ID: id, Modules: mods, Hashes: hashes}
}

func (p Program) Ordered() []*bytecode.Module {
	keys := make([]string, 0, len(p.Modules))
	for k := range p.Modules {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*bytecode.Module, len(keys))
	for i, k := range keys {
		out[i] = p.Modules[k]
	}
	return out
}

type layout struct {
	structs map[string]*bytecode.StructDef
	enums   map[string][]string
}

func newLayout(p Program) *layout {
	l := &layout{structs: map[string]*bytecode.StructDef{}, enums: map[string][]string{}}
	for _, m := range p.Modules {
		for i := range m.Structs {
			s := &m.Structs[i]
			l.structs[s.Name] = s
		}
		for _, e := range m.Enums {
			l.enums[e.Name] = e.Members
		}
	}
	return l
}

func (l *layout) slots(t abi.TypeRef) int {
	if t.Kind == abi.Struct {
		if s := l.structs[t.Name]; s != nil {
			return int(s.Slots)
		}
	}
	return t.Slots()
}

func (l *layout) mask(t abi.TypeRef) uint32 {
	switch t.Kind {
	case abi.String, abi.Slice:
		return 1
	case abi.Struct:
		s := l.structs[t.Name]
		if s == nil {
			return 0
		}
		var m uint32
		for _, f := range s.Fields {
			if ft, ok := abi.Parse(f.Type); ok && f.Slot < 32 {
				m |= l.mask(ft) << f.Slot
			}
		}
		return m
	}
	return 0
}

func structField(s *bytecode.StructDef, name string) *bytecode.StructField {
	for i := range s.Fields {
		if s.Fields[i].Name == name {
			return &s.Fields[i]
		}
	}
	return nil
}
