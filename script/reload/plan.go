package reload

import (
	"fmt"
	"slices"
	"sort"

	"aego/script/abi"
	"aego/script/bytecode"
	"aego/script/diag"
	"aego/script/source"
)

type Conversion uint8

const (
	Same Conversion = iota
	Default
	EnumByName
	Struct
	Slice
	Reresolve
)

type FieldMap struct {
	Name     string
	OldSlot  int
	NewSlot  int
	OldSlots int
	NewSlots int
	Old      abi.TypeRef
	New      abi.TypeRef
	Conv     Conversion
}

type TypeChange struct {
	Name    string
	Old     *bytecode.ScriptDef
	New     *bytecode.ScriptDef
	Fields  []FieldMap
	Reset   []string
	Removed []string
	Changed bool
}

type Plan struct {
	From   Program
	To     Program
	Types  []TypeChange
	Diags  []diag.Diagnostic
	from   *layout
	to     *layout
	byName map[string]*TypeChange
}

func (p *Plan) warn(format string, args ...any) {
	p.Diags = append(p.Diags, diag.Warnf("", source.Span{}, format, args...))
}

func (p *Plan) changed(name string) bool {
	tc := p.byName[name]
	return tc == nil || tc.Changed
}

func sameLayout(a, b abi.TypeRef, from, to *layout) bool {
	if !a.Equal(b) {
		return false
	}
	switch a.Kind {
	case abi.Struct:
		os, ns := from.structs[a.Name], to.structs[b.Name]
		if os == nil || ns == nil {
			return os == nil && ns == nil
		}
		if os.Slots != ns.Slots || len(os.Fields) != len(ns.Fields) {
			return false
		}
		for i := range os.Fields {
			of, nf := os.Fields[i], ns.Fields[i]
			if of.Name != nf.Name || of.Slot != nf.Slot || of.Slots != nf.Slots {
				return false
			}
			ot, ok1 := abi.Parse(of.Type)
			nt, ok2 := abi.Parse(nf.Type)
			if !ok1 || !ok2 || !sameLayout(ot, nt, from, to) {
				return false
			}
		}
	case abi.Slice:
		return a.Elem != nil && b.Elem != nil && sameLayout(*a.Elem, *b.Elem, from, to)
	case abi.Enum:
		return slices.Equal(from.enums[a.Name], to.enums[b.Name])
	}
	return true
}

func classify(a, b abi.TypeRef, from, to *layout) Conversion {
	switch {
	case sameLayout(a, b, from, to):
		if a.Kind == abi.ComponentRef {
			return Reresolve
		}
		return Same
	case a.Kind != b.Kind || a.Name != b.Name:
		return Default
	}
	switch a.Kind {
	case abi.Struct:
		if from.structs[a.Name] != nil && to.structs[b.Name] != nil {
			return Struct
		}
	case abi.Enum:
		if from.enums[a.Name] != nil && to.enums[b.Name] != nil {
			return EnumByName
		}
	case abi.Slice:
		if a.Elem != nil && b.Elem != nil && classify(*a.Elem, *b.Elem, from, to) != Default {
			return Slice
		}
	}
	return Default
}

func findField(od *bytecode.ScriptDef, name string, former []string) *bytecode.FieldDef {
	for _, n := range append([]string{name}, former...) {
		for i := range od.Fields {
			if od.Fields[i].Name == n {
				return &od.Fields[i]
			}
		}
	}
	return nil
}

func Build(from, to Program) (*Plan, error) {
	p := &Plan{From: from, To: to, from: newLayout(from), to: newLayout(to), byName: map[string]*TypeChange{}}
	olds := map[string]*bytecode.ScriptDef{}
	oldPkg := map[string]string{}
	for pkg, m := range from.Modules {
		for i := range m.Scripts {
			d := &m.Scripts[i]
			olds[d.FullName] = d
			oldPkg[d.FullName] = pkg
		}
	}
	matched := map[string]bool{}
	for _, nm := range to.Ordered() {
		pkg := nm.Package
		for i := range nm.Scripts {
			nd := &nm.Scripts[i]
			od := olds[nd.FullName]
			if od == nil {
				for _, f := range nd.Former {
					if od = olds[f]; od != nil {
						break
					}
				}
			}
			tc := TypeChange{Name: nd.FullName, New: nd, Old: od, Changed: true}
			if od != nil {
				matched[od.FullName] = true
				tc.Changed = od.FullName != nd.FullName || from.Hashes[oldPkg[od.FullName]] != to.Hashes[pkg]
				if err := p.fields(&tc); err != nil {
					return nil, err
				}
			}
			p.Types = append(p.Types, tc)
		}
	}
	for i := range p.Types {
		p.byName[p.Types[i].Name] = &p.Types[i]
	}
	names := make([]string, 0, len(olds))
	for n := range olds {
		if !matched[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		p.warn("script %s was removed, its instances stop", n)
	}
	return p, nil
}

func (p *Plan) fields(tc *TypeChange) error {
	od, nd := tc.Old, tc.New
	used := map[string]bool{}
	for _, nf := range nd.Fields {
		of := findField(od, nf.Name, nf.Former)
		if of == nil {
			continue
		}
		used[of.Name] = true
		ot, ok1 := abi.Parse(of.Type)
		nt, ok2 := abi.Parse(nf.Type)
		if !ok1 || !ok2 {
			return fmt.Errorf("script %s field %s has a malformed type", nd.FullName, nf.Name)
		}
		conv := classify(ot, nt, p.from, p.to)
		if conv == Default {
			tc.Reset = append(tc.Reset, nf.Name)
			p.warn("field %s.%s changed from %s to %s, reset to its default", nd.FullName, nf.Name, of.Type, nf.Type)
		}
		tc.Fields = append(tc.Fields, FieldMap{
			Name:     nf.Name,
			OldSlot:  int(of.Slot),
			NewSlot:  int(nf.Slot),
			OldSlots: int(of.Slots),
			NewSlots: int(nf.Slots),
			Old:      ot,
			New:      nt,
			Conv:     conv,
		})
	}
	for _, of := range od.Fields {
		if !used[of.Name] {
			tc.Removed = append(tc.Removed, of.Name)
		}
	}
	return nil
}
