package context

import (
	"path"
	"strings"

	"aego/core/ids"
	"aego/editor/core/document"
	"aego/engine/problems"
)

const (
	StubPrefix = "stub:"
	GoPrefix   = "go:"
)

var compileOnly = map[string]bool{"AES2029": true, "AES2030": true, "AES2032": true}

type Target uint8

const (
	TargetAuto Target = iota
	TargetDocs
	TargetProblems
	TargetSettings
	TargetAsset
	TargetEvents
	TargetAssetDocument
)

type Location struct {
	Path   string
	Line   int
	Col    int
	Offset int
	Node   ids.NodeID
	Symbol string
	Target Target
	Ref    string
	Query  string
	Frame  uint64
}

func At(path string, line, col int) Location {
	return Location{Path: path, Line: line, Col: col, Offset: -1}
}

func AtNode(scenePath string, id ids.NodeID) Location {
	return Location{Path: scenePath, Node: id, Offset: -1}
}

func AtOffset(path string, off int) Location {
	return Location{Path: path, Offset: off}
}

func DocsAt(ref, query string) Location {
	return Location{Target: TargetDocs, Ref: ref, Query: query, Offset: -1}
}

func ProblemsAt(filter string) Location {
	return Location{Target: TargetProblems, Ref: filter, Offset: -1}
}

func SettingsAt(page string) Location {
	return Location{Target: TargetSettings, Ref: page, Offset: -1}
}

func AssetAt(path string) Location {
	return Location{Target: TargetAsset, Path: path, Offset: -1}
}

// AssetDocumentAt opens the asset's registered document; AssetAt remains an
// explicit reveal-in-Assets location used by references and diagnostic links.
func AssetDocumentAt(path string) Location {
	return Location{Target: TargetAssetDocument, Path: path, Offset: -1}
}

func EventsAt(frame uint64) Location {
	return Location{Target: TargetEvents, Frame: frame, Offset: -1}
}

func (l Location) IsScript() bool {
	if strings.HasSuffix(l.Path, document.ScriptExt) ||
		strings.HasPrefix(l.Path, StubPrefix) ||
		strings.HasPrefix(l.Path, GoPrefix) {
		return true
	}
	// These source assets share the code editor, while their own importers
	// provide diagnostics instead of the Aego script language service.
	switch strings.ToLower(path.Ext(l.Path)) {
	case ".aeshader", ".aemat", ".glsl", ".vert", ".frag":
		return true
	}
	return false
}

type SearchItem struct {
	Label  string
	Detail string
	Kind   string
	Loc    Location
}

type ScriptNav interface {
	Goto(l Location)
	Declaration(typeName, field string) (Location, bool)
	FindUsages(typeName, field string)
	LiveProblems() ([]ScriptProblem, uint64)
	Symbols() []SearchItem
	Renamed(oldRel, newRel string)
}

func (p ScriptProblem) Location() Location {
	if !p.Node.IsZero() {
		return AtNode(p.Path, p.Node)
	}
	return At(p.Path, p.Line, p.Col)
}

func (e *Editor) Open(l Location) {
	switch l.Target {
	case TargetDocs:
		if e.ShowDocs != nil {
			e.ShowDocs(l.Ref, l.Query)
		}
		return
	case TargetProblems:
		if e.ShowProblems != nil {
			e.ShowProblems(l.Ref)
		}
		return
	case TargetSettings:
		if e.ShowSettings != nil {
			e.ShowSettings(l.Ref)
		}
		return
	case TargetAsset:
		if e.RevealPath != nil {
			e.RevealPath(l.Path)
		}
		return
	case TargetEvents:
		if e.ShowEvents != nil {
			e.ShowEvents(l.Frame)
		}
		return
	case TargetAssetDocument:
		if _, err := e.OpenAssetPath(l.Path); err != nil {
			e.Notify(LevelError, err.Error())
		}
		return
	}
	if l.Path != "" && e.Pipeline != nil && e.Docs != nil {
		if id, ok := e.Pipeline.Lookup(document.Clean(l.Path)); ok {
			if l.Node.IsZero() && l.Line <= 0 && l.Offset < 0 && l.Symbol == "" {
				if _, err := e.OpenAsset(id); err != nil {
					e.Notify(LevelError, err.Error())
				}
				return
			}
			// A diagnostic/source location shares the model, while retaining its
			// exact line or node destination instead of opening the default view.
			_, _ = e.DocumentForAsset(id)
		}
	}
	switch {
	case l.IsScript():
		if l.Symbol != "" {
			if g, ok := e.Code.(interface{ GotoSymbol(stub, symbol string) }); ok {
				g.GotoSymbol(l.Path, l.Symbol)
				return
			}
		}
		if e.Code != nil {
			e.Code.Goto(l)
			return
		}
		if e.OpenScript != nil {
			e.OpenScript(l.Path, l.Line)
			return
		}
	case strings.HasSuffix(l.Path, document.SceneExt), strings.HasSuffix(l.Path, document.PrefabExt):
		if _, err := e.Docs.Open(l.Path); err != nil {
			e.Notify(LevelError, err.Error())
			return
		}
		if !l.Node.IsZero() {
			e.Select(l.Node)
		}
		return
	}
	if l.Path != "" && e.RevealPath != nil {
		e.RevealPath(l.Path)
	}
}

func (e *Editor) ScriptProblems(dst []ScriptProblem) ([]ScriptProblem, uint64) {
	var built []ScriptProblem
	var brev uint64
	if e.Scripts != nil {
		built, brev = e.Scripts.Problems()
	}
	var live []ScriptProblem
	var lrev uint64
	if e.Code != nil {
		live, lrev = e.Code.LiveProblems()
	}
	dst = dst[:0]
	rev := brev ^ lrev<<32
	if live == nil {
		dst = append(dst, built...)
	} else {
		dst = append(dst, live...)
		for _, p := range built {
			if compileOnly[p.Code] {
				dst = append(dst, p)
			}
		}
	}
	list, prev := e.Problems.Collect()
	rev = rev*1099511628211 ^ prev
	for i := range list {
		if !sourceAlreadyExplainsPreview(&list[i], list) {
			dst = append(dst, e.row(&list[i]))
		}
	}
	return dst, rev
}

// Keep a forwarded import error on its thumbnail, but show the source location only once in Problems when the source provider reports the same error.
func sourceAlreadyExplainsPreview(v *problems.Diagnostic, all []problems.Diagnostic) bool {
	if v.Owner != "editor.preview" || v.Code != "THUMB001" || v.Severity != problems.Warning || v.Subject.Asset.IsZero() {
		return false
	}
	for i := range all {
		d := &all[i]
		if d.Severity == problems.Error && d.Owner != v.Owner && d.Subject.Asset == v.Subject.Asset && d.Message == v.Message {
			return true
		}
	}
	return false
}

func (e *Editor) FirstError(pkg string) (ScriptProblem, int, bool) {
	ps, _ := e.ScriptProblems(nil)
	var first ScriptProblem
	n := 0
	for _, p := range ps {
		if p.Error && p.Script() && (pkg == "" || p.Package == pkg) {
			if n == 0 {
				first = p
			}
			n++
		}
	}
	return first, n, n > 0
}

func (e *Editor) Errors(category string) int {
	ps, _ := e.ScriptProblems(nil)
	n := 0
	for _, p := range ps {
		if p.Error && p.Category == category {
			n++
		}
	}
	return n
}
