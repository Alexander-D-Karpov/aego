package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Catalog lists built-in module names that must not appear as literals in kernel, editor and SDK.
type Catalog struct {
	Modules []CatalogModule
}

type CatalogModule struct {
	ID     string   // module ID; "<ID>" and "<ID>.*" are module literals
	Dir    string   // package directory, module-relative
	Tokens []string // further exact literals: component and script module names
	codes  []string // diagnostic code prefixes found in Dir, e.g. "PHY"
}

// semanticScope lists the groups whose literals are checked.
func semanticScope(g group) bool {
	switch g.kind {
	case kLow, kKernel, kEdCore, kEdExt, kPanel, kSDK:
		return true
	}
	return false
}

var codeConst = regexp.MustCompile(`^([A-Z]{2,4})\d{3}$`)

// diagPrefixes collects prefixes of diagnostic-code constants ("PHY001") under dir.
func diagPrefixes(root, dir string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				for _, v := range spec.(*ast.ValueSpec).Values {
					lit, ok := v.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					s, _ := strconv.Unquote(lit.Value)
					if m := codeConst.FindStringSubmatch(s); m != nil && !seen[m[1]] {
						seen[m[1]] = true
						out = append(out, m[1])
					}
				}
			}
		}
		return nil
	})
	return out, err
}

// moduleLiteral returns the module a literal names, or "".
func (c *Catalog) moduleLiteral(s string) string {
	for _, m := range c.Modules {
		if s == m.ID || strings.HasPrefix(s, m.ID+".") {
			return m.ID
		}
		for _, t := range m.Tokens {
			if s == t {
				return m.ID
			}
		}
		for _, code := range m.codes {
			if rest, ok := strings.CutPrefix(s, code); ok && rest != "" && rest[0] >= '0' && rest[0] <= '9' {
				return m.ID
			}
		}
	}
	return ""
}

type literalHit struct {
	file   string // module-relative file path
	pos    string
	lit    string
	module string
}

func scanLiterals(root string, g *graph, c *Catalog) ([]literalHit, error) {
	var out []literalHit
	fset := token.NewFileSet()
	for _, p := range g.order {
		gr, ok := groupOf(p)
		if !ok || !semanticScope(gr) {
			continue
		}
		info := g.pkgs[p]
		for _, name := range info.files {
			full := filepath.Join(info.dir, name)
			f, err := parser.ParseFile(fset, full, nil, parser.SkipObjectResolution)
			if err != nil {
				return nil, err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.ImportSpec:
					return false
				case *ast.BasicLit:
					if x.Kind != token.STRING {
						return true
					}
					s, err := strconv.Unquote(x.Value)
					if err != nil {
						return true
					}
					if id := c.moduleLiteral(s); id != "" {
						pos := fset.Position(x.Pos())
						rel, _ := filepath.Rel(root, pos.Filename)
						out = append(out, literalHit{
							file:   filepath.ToSlash(rel),
							pos:    filepath.ToSlash(rel) + ":" + strconv.Itoa(pos.Line),
							lit:    strconv.Quote(s),
							module: id,
						})
					}
				}
				return true
			})
		}
	}
	return out, nil
}
