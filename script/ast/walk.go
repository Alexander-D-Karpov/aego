package ast

// Inspect calls fn for n, then for each child for which fn returned true,
// recursively, in source order. It walks the tree without building a slice per
// node, so it stays cheap when run over a whole file on every keystroke.
func Inspect(n Node, fn func(Node) bool) {
	if n == nil || !fn(n) {
		return
	}
	walk(n, func(c Node) { Inspect(c, fn) })
}

// Children returns n's direct child nodes in source order.
func Children(n Node) []Node {
	var out []Node
	walk(n, func(c Node) { out = append(out, c) })
	return out
}

func walk(n Node, visit func(Node)) {
	ident := func(id *Ident) {
		if id != nil {
			visit(id)
		}
	}
	expr := func(e Expr) {
		if e != nil {
			visit(e)
		}
	}
	typ := func(t TypeExpr) {
		if t != nil {
			visit(t)
		}
	}
	stmt := func(s Stmt) {
		if s != nil {
			visit(s)
		}
	}
	attrs := func(list []*Attr) {
		for _, a := range list {
			visit(a)
		}
	}
	switch x := n.(type) {
	case *File:
		ident(x.Package)
		for _, i := range x.Imports {
			visit(i)
		}
		for _, d := range x.Decls {
			visit(d)
		}
	case *Attr:
		ident(x.Name)
		for _, a := range x.Args {
			visit(a)
		}
	case *AttrArg:
		ident(x.Key)
		expr(x.Value)
	case *CompositeLit:
		typ(x.Type)
		for _, e := range x.Elems {
			visit(e)
		}
	case *Element:
		ident(x.Key)
		expr(x.Value)
	case *ParenExpr:
		expr(x.X)
	case *SelectorExpr:
		expr(x.X)
		ident(x.Sel)
	case *IndexExpr:
		expr(x.X)
		expr(x.Index)
	case *CallExpr:
		expr(x.Fun)
		for _, t := range x.TypeArgs {
			typ(t)
		}
		for _, a := range x.Args {
			expr(a)
		}
	case *UnaryExpr:
		expr(x.X)
	case *BinaryExpr:
		expr(x.X)
		expr(x.Y)
	case *NamedType:
		ident(x.Pkg)
		ident(x.Name)
		for _, t := range x.Args {
			typ(t)
		}
	case *SliceType:
		typ(x.Elem)
	case *VarStmt:
		ident(x.Name)
		typ(x.Type)
		expr(x.Value)
	case *ShortVarStmt:
		ident(x.Name)
		expr(x.Value)
	case *AssignStmt:
		expr(x.Target)
		expr(x.Value)
	case *IncDecStmt:
		expr(x.X)
	case *ExprStmt:
		expr(x.X)
	case *ReturnStmt:
		expr(x.Value)
	case *BlockStmt:
		for _, s := range x.Stmts {
			stmt(s)
		}
	case *IfStmt:
		expr(x.Cond)
		if x.Then != nil {
			visit(x.Then)
		}
		stmt(x.Else)
	case *ForStmt:
		stmt(x.Init)
		expr(x.Cond)
		stmt(x.Post)
		if x.Body != nil {
			visit(x.Body)
		}
	case *SwitchStmt:
		expr(x.Tag)
		for _, c := range x.Cases {
			visit(c)
		}
	case *CaseClause:
		for _, v := range x.Values {
			expr(v)
		}
		for _, s := range x.Body {
			stmt(s)
		}
	case *ConstDecl:
		ident(x.Name)
		typ(x.Type)
		expr(x.Value)
	case *StructDecl:
		ident(x.Name)
		for _, f := range x.Fields {
			visit(f)
		}
	case *Field:
		ident(x.Name)
		typ(x.Type)
	case *EnumDecl:
		ident(x.Name)
		for _, m := range x.Members {
			ident(m)
		}
	case *FuncDecl:
		attrs(x.Attrs)
		ident(x.Name)
		for _, p := range x.Params {
			visit(p)
		}
		typ(x.Result)
		if x.Body != nil {
			visit(x.Body)
		}
	case *Param:
		ident(x.Name)
		typ(x.Type)
	case *ScriptDecl:
		attrs(x.Attrs)
		ident(x.Name)
		for _, v := range x.Vars {
			visit(v)
		}
		for _, f := range x.Funcs {
			visit(f)
		}
	case *ScriptVar:
		attrs(x.Attrs)
		ident(x.Name)
		typ(x.Type)
		expr(x.Value)
	}
}
