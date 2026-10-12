package context

import (
	"strings"

	"aego/core/ids"
	"aego/editor/document"
)

const (
	StubPrefix = "stub:"
	GoPrefix   = "go:"
)

var compileOnly = map[string]bool{"AES2029": true, "AES2030": true, "AES2032": true}

type Location struct {
	Path   string
	Line   int
	Col    int
	Offset int
	Node   ids.NodeID
}

func At(path string, line, col int) Location {
	return Location{Path: path, Line: line, Col: col, Offset: -1}
}

func AtOffset(path string, off int) Location {
	return Location{Path: path, Offset: off}
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
	return At(p.Path, p.Line, p.Col)
}

func (e *Editor) Open(l Location) {
	switch {
	case l.IsScript():
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
	if live == nil {
		return append(dst, built...), brev
	}
	dst = append(dst, live...)
	for _, p := range built {
		if compileOnly[p.Code] {
			dst = append(dst, p)
		}
	}
	return dst, brev ^ lrev<<32
}

func (e *Editor) FirstError(pkg string) (ScriptProblem, int, bool) {
	ps, _ := e.ScriptProblems(nil)
	var first ScriptProblem
	n := 0
	for _, p := range ps {
		if p.Error && (pkg == "" || p.Package == pkg) {
			if n == 0 {
				first = p
			}
			n++
		}
	}
	return first, n, n > 0
}
