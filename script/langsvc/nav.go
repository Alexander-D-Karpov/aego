package langsvc

import (
	"slices"
	"sort"

	"aego/script/ast"
	"aego/script/source"
	"aego/script/workspace"
)

func symbolsOf(snap *workspace.Snapshot, f *ast.File) []Symbol {
	a := snap.Analysis
	detail := func(id *ast.Ident) string {
		if o := a.Defs[id]; o != nil {
			return describe(o)
		}
		return ""
	}
	var out []Symbol
	for _, d := range f.Decls {
		switch x := d.(type) {
		case *ast.ConstDecl:
			out = append(out, Symbol{Name: x.Name.Name, Kind: SymConst, Detail: detail(x.Name), Span: x.Sp, Selection: x.Name.Sp})
		case *ast.StructDecl:
			sym := Symbol{Name: x.Name.Name, Kind: SymStruct, Detail: detail(x.Name), Span: x.Sp, Selection: x.Name.Sp}
			for _, fl := range x.Fields {
				sym.Children = append(sym.Children, Symbol{Name: fl.Name.Name, Kind: SymField, Detail: detail(fl.Name), Span: fl.Sp, Selection: fl.Name.Sp})
			}
			out = append(out, sym)
		case *ast.EnumDecl:
			sym := Symbol{Name: x.Name.Name, Kind: SymEnum, Detail: detail(x.Name), Span: x.Sp, Selection: x.Name.Sp}
			for _, m := range x.Members {
				sym.Children = append(sym.Children, Symbol{Name: m.Name, Kind: SymEnumMember, Span: m.Sp, Selection: m.Sp})
			}
			out = append(out, sym)
		case *ast.FuncDecl:
			out = append(out, Symbol{Name: x.Name.Name, Kind: SymFunc, Detail: detail(x.Name), Span: x.Sp, Selection: x.Name.Sp})
		case *ast.ScriptDecl:
			sym := Symbol{Name: x.Name.Name, Kind: SymScript, Detail: detail(x.Name), Span: x.Sp, Selection: x.Name.Sp}
			for _, sg := range x.Signals {
				sym.Children = append(sym.Children, Symbol{Name: sg.Name.Name, Kind: SymSignal, Detail: detail(sg.Name), Span: sg.Sp, Selection: sg.Name.Sp})
			}
			for _, v := range x.Vars {
				sym.Children = append(sym.Children, Symbol{Name: v.Name.Name, Kind: SymField, Detail: detail(v.Name), Span: v.Sp, Selection: v.Name.Sp})
			}
			for _, fn := range x.Funcs {
				sym.Children = append(sym.Children, Symbol{Name: fn.Name.Name, Kind: SymMethod, Detail: detail(fn.Name), Span: fn.Sp, Selection: fn.Name.Sp})
			}
			out = append(out, sym)
		}
	}
	return out
}

func (s *Service) Symbols(file source.FileID) []Symbol {
	return s.SymbolsIn(s.Latest(), file)
}

func (s *Service) SymbolsIn(snap *workspace.Snapshot, file source.FileID) []Symbol {
	if snap == nil {
		return nil
	}
	f := snap.Files[file]
	if f == nil {
		return nil
	}
	return symbolsOf(snap, f)
}

func (s *Service) WorkspaceSymbols() []SymbolRef {
	snap := s.Latest()
	if snap == nil {
		return nil
	}
	var out []SymbolRef
	for id, f := range snap.Files {
		path := snap.Sources.Path(id)
		var walk func(list []Symbol, container string)
		walk = func(list []Symbol, container string) {
			for _, sym := range list {
				line, col := snap.Sources.LineCol(id, sym.Selection.Start)
				out = append(out, SymbolRef{
					Name: sym.Name, Container: container, Detail: sym.Detail,
					Kind: sym.Kind, Path: path, Line: line, Col: col,
				})
				walk(sym.Children, sym.Name)
			}
		}
		walk(symbolsOf(snap, f), "")
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Line < b.Line
	})
	return out
}

func (s *Service) RangesIn(snap *workspace.Snapshot, file source.FileID, lo, hi uint32) []source.Span {
	if snap == nil {
		return nil
	}
	f := snap.Files[file]
	if f == nil {
		return nil
	}
	var out []source.Span
	ast.Inspect(f, func(n ast.Node) bool {
		sp := n.Span()
		if sp.Start > lo || sp.End < hi {
			return false
		}
		if sp.End > sp.Start {
			out = append(out, sp)
		}
		return true
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].Len() < out[j].Len() })
	n := 0
	for i, sp := range out {
		if i > 0 && sp.Start == out[n-1].Start && sp.End == out[n-1].End {
			continue
		}
		out[n] = sp
		n++
	}
	return out[:n]
}

func scriptIdent(snap *workspace.Snapshot, typeName, field string) *ast.Ident {
	for _, st := range snap.Analysis.Scripts {
		if st.FullName != typeName && !slices.Contains(st.Former, typeName) {
			continue
		}
		if field == "" {
			return st.Decl.Name
		}
		for _, f := range st.Fields {
			if f.Name == field || slices.Contains(f.Former, field) {
				return f.Decl.Name
			}
		}
	}
	return nil
}

func (s *Service) ScriptDeclaration(typeName, field string) (Location, bool) {
	snap := s.Latest()
	if snap == nil {
		return Location{}, false
	}
	id := scriptIdent(snap, typeName, field)
	if id == nil {
		return Location{}, false
	}
	return loc(snap, id.Sp), true
}

func (s *Service) ScriptUsages(typeName, field string) ([]Location, *workspace.Snapshot) {
	snap := s.Latest()
	if snap == nil {
		return nil, nil
	}
	id := scriptIdent(snap, typeName, field)
	if id == nil {
		return nil, snap
	}
	o := snap.Analysis.Defs[id]
	if o == nil {
		return nil, snap
	}
	var out []Location
	for _, r := range snap.Analysis.References {
		if r.Object == o {
			out = append(out, loc(snap, r.Ident.Sp))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Span.Start < out[j].Span.Start
	})
	return out, snap
}
