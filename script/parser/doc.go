package parser

import (
	"sort"
	"strings"

	"aego/script/ast"
)

func (p *parser) docBefore(start uint32) string {
	i := sort.Search(len(p.comments), func(i int) bool { return p.comments[i].Start >= start }) - 1
	var parts []string
	next := start
	for ; i >= 0; i-- {
		c := p.comments[i]
		if !c.Line || !p.adjacent(c.End, next) || !p.lineStart(c.Start) {
			break
		}
		parts = append(parts, docText(p.src[c.Start:c.End]))
		next = c.Start
	}
	for l, r := 0, len(parts)-1; l < r; l, r = l+1, r-1 {
		parts[l], parts[r] = parts[r], parts[l]
	}
	return strings.Join(parts, "\n")
}

func (p *parser) adjacent(end, next uint32) bool {
	lines := 0
	for i := end; i < next && int(i) < len(p.src); i++ {
		switch p.src[i] {
		case '\n':
			lines++
		case ' ', '\t', '\r':
		default:
			return false
		}
	}
	return lines == 1
}

func (p *parser) lineStart(off uint32) bool {
	for i := int(off) - 1; i >= 0; i-- {
		switch p.src[i] {
		case '\n':
			return true
		case ' ', '\t', '\r':
		default:
			return false
		}
	}
	return true
}

func docText(b []byte) string {
	s := strings.TrimPrefix(string(b), "//")
	s = strings.TrimPrefix(s, " ")
	return strings.TrimRight(s, " \t\r")
}

func setDoc(d ast.Decl, doc string) {
	if doc == "" {
		return
	}
	switch x := d.(type) {
	case *ast.ConstDecl:
		x.Doc = doc
	case *ast.StructDecl:
		x.Doc = doc
	case *ast.EnumDecl:
		x.Doc = doc
	case *ast.FuncDecl:
		x.Doc = doc
	case *ast.ScriptDecl:
		x.Doc = doc
	}
}
