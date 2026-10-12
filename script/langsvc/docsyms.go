package langsvc

import (
	"sort"
	"unicode"
	"unicode/utf8"

	"aego/script/source"
	"aego/script/workspace"
)

type DocSymbol struct {
	Package   string
	Container string
	Name      string
	Kind      string
	Signature string
	Doc       string
	Path      string
	Line      int
}

func exported(name string) bool {
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r)
}

func (s *Service) ScriptDocs(snap *workspace.Snapshot) []DocSymbol {
	files := make([]source.FileID, 0, len(snap.Files))
	for id := range snap.Files {
		files = append(files, id)
	}
	sort.Slice(files, func(a, b int) bool { return snap.Sources.Path(files[a]) < snap.Sources.Path(files[b]) })
	var out []DocSymbol
	for _, id := range files {
		path := snap.Sources.Path(id)
		if workspace.IsTest(path) {
			continue
		}
		pkg := snap.Packages[id]
		var walk func(list []Symbol, container string)
		walk = func(list []Symbol, container string) {
			for _, sym := range list {
				if !exported(sym.Name) {
					continue
				}
				d := DocSymbol{Package: pkg, Container: container, Name: sym.Name, Kind: sym.Kind.String(), Path: path}
				d.Line, _ = snap.Sources.LineCol(id, sym.Selection.Start)
				if h, ok := s.HoverIn(snap, id, sym.Selection.Start); ok {
					d.Signature, d.Doc = h.Signature, h.Doc
				}
				out = append(out, d)
				next := sym.Name
				if container != "" {
					next = container + "." + sym.Name
				}
				walk(sym.Children, next)
			}
		}
		walk(s.SymbolsIn(snap, id), "")
	}
	return out
}
