package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
)

type colorHit struct {
	file string
	pos  string
	what string
}

// colorScope is where colours come only from theme tokens (D345).
func colorScope(p string) bool {
	if p == "ui/theme" || strings.HasPrefix(p, "ui/theme/") {
		return false
	}
	return strings.HasPrefix(p, "editor/") || strings.HasPrefix(p, "ui/")
}

func isMathx(e ast.Expr, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == "mathx"
}

// colorElem reports whether a collection's elements are colours, so their untyped literals count too.
func colorElem(e ast.Expr) bool {
	switch t := e.(type) {
	case *ast.ArrayType:
		return isMathx(t.Elt, "Color")
	case *ast.MapType:
		return isMathx(t.Value, "Color")
	}
	return false
}

// constant reports whether a colour literal sets a channel to a number, not to an expression or a token.
func constant(cl *ast.CompositeLit) bool {
	for _, el := range cl.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			el = kv.Value
		}
		switch v := el.(type) {
		case *ast.BasicLit:
			return true
		case *ast.UnaryExpr:
			if _, ok := v.X.(*ast.BasicLit); ok {
				return true
			}
		case *ast.BinaryExpr:
			_, l := v.X.(*ast.BasicLit)
			_, r := v.Y.(*ast.BasicLit)
			if l && r {
				return true
			}
		}
	}
	return false
}

func scanColors(root string, g *graph) ([]colorHit, error) {
	var out []colorHit
	fset := token.NewFileSet()
	for _, p := range g.order {
		if !colorScope(p) {
			continue
		}
		info := g.pkgs[p]
		for _, name := range info.files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, filepath.Join(info.dir, name), nil, parser.SkipObjectResolution)
			if err != nil {
				return nil, err
			}
			add := func(n ast.Node, what string) {
				pos := fset.Position(n.Pos())
				rel, _ := filepath.Rel(root, pos.Filename)
				rel = filepath.ToSlash(rel)
				out = append(out, colorHit{file: rel, pos: rel + ":" + strconv.Itoa(pos.Line), what: what})
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					if isMathx(x.Fun, "ColorRGB8") || isMathx(x.Fun, "ColorRGBA8") {
						add(x, "calls mathx."+x.Fun.(*ast.SelectorExpr).Sel.Name)
					}
				case *ast.CompositeLit:
					if isMathx(x.Type, "Color") && constant(x) {
						add(x, "writes a mathx.Color literal")
						return false
					}
					if colorElem(x.Type) {
						for _, el := range x.Elts {
							if kv, ok := el.(*ast.KeyValueExpr); ok {
								el = kv.Value
							}
							if cl, ok := el.(*ast.CompositeLit); ok && cl.Type == nil && constant(cl) {
								add(cl, "writes a mathx.Color literal")
							}
						}
					}
				}
				return true
			})
		}
	}
	return out, nil
}
