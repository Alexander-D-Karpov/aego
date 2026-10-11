package langsvc

import (
	"aego/script/ast"
	"aego/script/checker"
	"aego/script/resolver"
	"aego/script/source"
	"aego/script/types"
	"aego/script/workspace"
)

func contains(sp source.Span, off uint32) bool {
	return sp.Start <= off && off <= sp.End
}

func pathTo(n ast.Node, off uint32, stack []ast.Node, fn func(ast.Node, []ast.Node)) {
	if n == nil || !contains(n.Span(), off) {
		return
	}
	fn(n, stack)
	stack = append(stack, n)
	for _, c := range ast.Children(n) {
		pathTo(c, off, stack, fn)
	}
}

func identAt(f *ast.File, off uint32) (*ast.Ident, []ast.Node) {
	var best *ast.Ident
	var bestStack []ast.Node
	score := 0
	pathTo(f, off, nil, func(n ast.Node, stack []ast.Node) {
		id, ok := n.(*ast.Ident)
		if !ok || id.Name == "_" || id.Sp.End == id.Sp.Start {
			return
		}
		s := 1
		if off < id.Sp.End {
			s = 2
		}
		if s > score {
			best, score = id, s
			bestStack = append([]ast.Node(nil), stack...)
		}
	})
	return best, bestStack
}

func object(a *checker.Analysis, id *ast.Ident) *resolver.Object {
	if o := a.Uses[id]; o != nil {
		return o
	}
	return a.Defs[id]
}

func selectorWith(stack []ast.Node, id *ast.Ident) *ast.SelectorExpr {
	if n := len(stack); n > 0 {
		if sel, ok := stack[n-1].(*ast.SelectorExpr); ok && sel.Sel == id {
			return sel
		}
	}
	return nil
}

func callOf(stack []ast.Node, sel *ast.SelectorExpr) *ast.CallExpr {
	if n := len(stack); n > 1 {
		if call, ok := stack[n-2].(*ast.CallExpr); ok && call.Fun == ast.Expr(sel) {
			return call
		}
	}
	return nil
}

func componentArg(a *checker.Analysis, id *ast.Ident, stack []ast.Node) *checker.Call {
	n := len(stack)
	if n < 2 {
		return nil
	}
	nt, ok := stack[n-1].(*ast.NamedType)
	if !ok || nt.Name != id {
		return nil
	}
	call, ok := stack[n-2].(*ast.CallExpr)
	if !ok {
		return nil
	}
	k := a.Calls[call]
	if k == nil || (k.Kind != checker.CallGetComponent && k.Kind != checker.CallHasComponent) {
		return nil
	}
	return k
}

func loc(snap *workspace.Snapshot, sp source.Span) Location {
	return Location{File: sp.File, Path: snap.Sources.Path(sp.File), Span: sp}
}

func structDecls(a *checker.Analysis, fn func(t *types.Type, d *ast.StructDecl) bool) {
	for _, p := range a.Order {
		cp := a.Packages[p]
		if cp == nil || cp.Res == nil {
			continue
		}
		for _, o := range cp.Res.Scope.Objects() {
			d, ok := o.Decl.(*ast.StructDecl)
			if ok && o.Type != nil && !fn(o.Type, d) {
				return
			}
		}
	}
}

func fieldDecl(a *checker.Analysis, f *types.Field) *ast.Field {
	var out *ast.Field
	structDecls(a, func(t *types.Type, d *ast.StructDecl) bool {
		for _, tf := range t.Fields {
			if tf != f {
				continue
			}
			for _, df := range d.Fields {
				if df.Name.Name == f.Name {
					out = df
					return false
				}
			}
		}
		return true
	})
	return out
}

func structField(a *checker.Analysis, o *resolver.Object) *types.Field {
	fd, ok := o.Decl.(*ast.Field)
	if o.Kind != resolver.ObjField || !ok {
		return nil
	}
	var out *types.Field
	structDecls(a, func(t *types.Type, d *ast.StructDecl) bool {
		for _, df := range d.Fields {
			if df == fd {
				out = t.Field(o.Name)
				return false
			}
		}
		return true
	})
	return out
}
