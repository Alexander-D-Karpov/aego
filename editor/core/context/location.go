package context

import (
	"strings"

	"aego/core/ids"
	"aego/editor/core/document"
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

func EventsAt(frame uint64) Location {
	return Location{Target: TargetEvents, Frame: frame, Offset: -1}
}

func (l Location) IsScript() bool {
	return strings.HasSuffix(l.Path, document.ScriptExt) ||
		strings.HasPrefix(l.Path, StubPrefix) ||
		strings.HasPrefix(l.Path, GoPrefix)
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
	case strings.HasSuffix(l.Path, document.SceneExt):
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
		dst = append(dst, e.row(&list[i]))
	}
	return dst, rev
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
