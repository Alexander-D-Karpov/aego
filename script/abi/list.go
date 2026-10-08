package abi

import (
	"sort"
	"strings"
)

type FuncEntry struct {
	Module string
	Name   string
	Sig    Signature
}

type MethodEntry struct {
	Recv Kind
	Name string
	Sig  Signature
}

type ConstEntry struct {
	Module string
	Name   string
	Val    Const
}

func KindOf(name string) (Kind, bool) {
	for k := Void; k < kindCount; k++ {
		if kindNames[k] == name {
			return k, true
		}
	}
	return 0, false
}

func splitKey(k string) (string, string) {
	a, b, _ := strings.Cut(k, "\x00")
	return a, b
}

func (t *Table) ModuleList() []string {
	out := make([]string, 0, len(t.modules))
	for p := range t.modules {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func (t *Table) FuncList() []FuncEntry {
	out := make([]FuncEntry, 0, len(t.funcs))
	for k, s := range t.funcs {
		m, n := splitKey(k)
		out = append(out, FuncEntry{Module: m, Name: n, Sig: s})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Module != out[j].Module {
			return out[i].Module < out[j].Module
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (t *Table) MethodList() []MethodEntry {
	out := make([]MethodEntry, 0, len(t.methods))
	for k, s := range t.methods {
		r, n := splitKey(k)
		kind, _ := KindOf(r)
		out = append(out, MethodEntry{Recv: kind, Name: n, Sig: s})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Recv != out[j].Recv {
			return out[i].Recv < out[j].Recv
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (t *Table) ConstList() []ConstEntry {
	out := make([]ConstEntry, 0, len(t.consts))
	for k, c := range t.consts {
		m, n := splitKey(k)
		out = append(out, ConstEntry{Module: m, Name: n, Val: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Module != out[j].Module {
			return out[i].Module < out[j].Module
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (t *Table) EnumList() []*EnumDef {
	out := make([]*EnumDef, 0, len(t.enums))
	for _, e := range t.enums {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Module != out[j].Module {
			return out[i].Module < out[j].Module
		}
		return out[i].Name < out[j].Name
	})
	return out
}
