package verify

import (
	"fmt"

	"aego/script/abi"
	"aego/script/bytecode"
)

const maxStructDepth = 32

type roots struct {
	s       *bytecode.ScriptDef
	structs map[string]*bytecode.StructDef
	strict  bool
	heap    map[uint16]bool
	out     []string
}

func ScriptRoots(s *bytecode.ScriptDef, structs map[string]*bytecode.StructDef, strict bool) []string {
	r := &roots{s: s, structs: structs, strict: strict, heap: map[uint16]bool{}}
	for _, h := range s.HeapSlots {
		r.heap[h] = true
	}
	for _, f := range s.Fields {
		t, ok := abi.Parse(f.Type)
		if !ok {
			r.bad("field %s has an unparsable type %q", f.Name, f.Type)
			continue
		}
		r.slot(f.Name, t, int(f.Slot), int(f.Slots), 0)
	}
	return r.out
}

func (r *roots) bad(format string, args ...any) {
	r.out = append(r.out, fmt.Sprintf("script %s ", r.s.FullName)+fmt.Sprintf(format, args...))
}

func (r *roots) slot(field string, t abi.TypeRef, at, n, depth int) {
	switch t.Kind {
	case abi.String, abi.Slice:
		if !r.heap[uint16(at)] {
			r.bad("field %s is a heap value but slot %d is not a GC root", field, at)
		}
	case abi.Struct:
		sd := r.structs[t.Name]
		switch {
		case sd == nil && r.strict:
			r.bad("field %s uses struct %s, which is not loaded", field, t.Name)
		case sd == nil:
		case depth > maxStructDepth:
			r.bad("field %s nests structs deeper than %d", field, maxStructDepth)
		default:
			for _, sf := range sd.Fields {
				st, ok := abi.Parse(sf.Type)
				if !ok {
					r.bad("struct %s field %s has an unparsable type %q", t.Name, sf.Name, sf.Type)
					continue
				}
				r.slot(field+"."+sf.Name, st, at+int(sf.Slot), int(sf.Slots), depth+1)
			}
		}
	default:
		for sl := at; sl < at+n; sl++ {
			if r.heap[uint16(sl)] {
				r.bad("field %s is not a heap value but slot %d is a GC root", field, sl)
			}
		}
	}
}
