package checker

import (
	"strings"

	"aego/script/ast"
	"aego/script/diag"
	"aego/script/resolver"
	"aego/script/token"
	"aego/script/types"
)

func (c *checker) stmts(list []ast.Stmt) {
	for _, s := range list {
		c.stmt(s)
	}
}

func (c *checker) block(list []ast.Stmt) {
	c.scope = resolver.NewScope(c.scope)
	c.stmts(list)
	c.scope = c.scope.Parent
}

func (c *checker) stmt(s ast.Stmt) {
	switch s := s.(type) {
	case *ast.VarStmt:
		t := c.typeExpr(s.Type)
		if s.Value != nil {
			c.assign(s.Value, c.value(s.Value), t, "variable declaration")
		}
		c.declare(s.Name, t)
	case *ast.ShortVarStmt:
		c.shortVar(s)
	case *ast.AssignStmt:
		c.assignStmt(s)
	case *ast.IncDecStmt:
		x := c.expr(s.X)
		switch {
		case x.mode == mInvalid:
		case x.mode != mValue || !x.addr:
			c.errorf(diag.NotAssignable, s.X.Span(), "cannot assign to this expression")
		case x.t != types.Int && x.t != types.Float:
			c.errorf(diag.InvalidOperation, s.Sp, "increment and decrement are not defined on %s", x.t)
		}
	case *ast.ExprStmt:
		c.exprStmt(s)
	case *ast.ReturnStmt:
		c.returnStmt(s)
	case *ast.BranchStmt:
		if s.Tok == token.Break && c.breaks == 0 {
			c.errorf(diag.MisplacedBranch, s.Sp, "break is not in a loop or switch")
		}
		if s.Tok == token.Continue && c.loops == 0 {
			c.errorf(diag.MisplacedBranch, s.Sp, "continue is not in a loop")
		}
	case *ast.BlockStmt:
		c.block(s.Stmts)
	case *ast.IfStmt:
		c.cond(s.Cond)
		if s.Then != nil {
			c.block(s.Then.Stmts)
		}
		if s.Else != nil {
			c.stmt(s.Else)
		}
	case *ast.ForStmt:
		c.forStmt(s)
	case *ast.SwitchStmt:
		c.switchStmt(s)
	}
}

func (c *checker) declare(id *ast.Ident, t *types.Type) {
	if id.Name == "_" {
		return
	}
	obj := &resolver.Object{Kind: resolver.ObjVar, Name: id.Name, Pkg: c.rp.Path, Ident: id, Decl: id, Type: t, Index: len(c.fn.Locals)}
	if prev := c.scope.Insert(obj); prev != nil {
		c.a.Diagnostics = append(c.a.Diagnostics, diag.Errorf(diag.Redeclared, id.Sp, "%s redeclared in this block", id.Name).
			WithNote(prev.Span(), "previous declaration"))
	}
	c.fn.Locals = append(c.fn.Locals, obj)
	c.def(id, obj)
}

func (c *checker) shortVar(s *ast.ShortVarStmt) {
	x := c.value(s.Value)
	t := types.Invalid
	switch {
	case x.mode == mInvalid:
	case x.t == types.Nil:
		c.errorf(diag.NilMisuse, s.Value.Span(), "use of untyped nil in a variable declaration")
	default:
		t = types.Default(x.t)
		c.assign(s.Value, x, t, "variable declaration")
	}
	c.declare(s.Name, t)
}

func (c *checker) assignStmt(s *ast.AssignStmt) {
	lhs := c.expr(s.Target)
	rhs := c.value(s.Value)
	if lhs.mode == mInvalid {
		return
	}
	if lhs.mode != mValue || !lhs.addr {
		c.errorf(diag.NotAssignable, s.Target.Span(), "cannot assign to this expression")
		return
	}
	if s.Op == token.Assign {
		c.assign(s.Value, rhs, lhs.t, "assignment")
		return
	}
	if rhs.mode == mInvalid {
		return
	}
	r := c.binop(s.Sp, s.Op.AssignOp(), lhs, rhs, s.Target, s.Value)
	if r.mode != mInvalid && r.t != lhs.t {
		c.errorf(diag.TypeMismatch, s.Sp, "cannot assign %s to %s", r.t, lhs.t)
	}
}

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

func (c *checker) exprStmt(s *ast.ExprStmt) {
	call, ok := unparen(s.X).(*ast.CallExpr)
	if !ok {
		if x := c.expr(s.X); x.mode != mInvalid {
			c.errorf(diag.Unused, s.X.Span(), "value is not used")
		}
		return
	}
	c.expr(call)
	if k := c.a.Calls[call]; k != nil {
		switch k.Kind {
		case CallLen, CallAppend, CallConvert, CallGetComponent, CallHasComponent:
			c.errorf(diag.Unused, s.X.Span(), "result of this call is not used")
		}
	}
}

func (c *checker) returnStmt(s *ast.ReturnStmt) {
	res := c.fn.Result
	switch {
	case s.Value == nil && res != types.Void:
		c.errorf(diag.ReturnMismatch, s.Sp, "missing return value of type %s", res)
	case s.Value != nil && res == types.Void:
		c.value(s.Value)
		c.errorf(diag.ReturnMismatch, s.Value.Span(), "%s returns no value", c.fn.Name)
	case s.Value != nil:
		c.assign(s.Value, c.value(s.Value), res, "return")
	}
}

func (c *checker) cond(e ast.Expr) {
	x := c.value(e)
	if x.mode != mInvalid && x.t != types.Bool {
		c.errorf(diag.TypeMismatch, e.Span(), "condition must be bool, not %s", x.t)
	}
}

func (c *checker) forStmt(s *ast.ForStmt) {
	c.scope = resolver.NewScope(c.scope)
	if s.Init != nil {
		c.stmt(s.Init)
	}
	if s.Cond != nil {
		c.cond(s.Cond)
	}
	if s.Post != nil {
		if _, ok := s.Post.(*ast.ShortVarStmt); ok {
			c.errorf(diag.InvalidOperation, s.Post.Span(), "cannot declare in the post statement of a for loop")
		}
		c.stmt(s.Post)
	}
	c.loops++
	c.breaks++
	if s.Body != nil {
		c.block(s.Body.Stmts)
	}
	c.loops--
	c.breaks--
	c.scope = c.scope.Parent
}

func (c *checker) switchStmt(s *ast.SwitchStmt) {
	tag := c.value(s.Tag)
	t := types.Invalid
	switch {
	case tag.mode == mInvalid:
	case tag.t == types.Nil:
		c.errorf(diag.NilMisuse, s.Tag.Span(), "cannot switch on nil")
	default:
		t = types.Default(tag.t)
		c.assign(s.Tag, tag, t, "switch")
		if !t.Comparable() {
			c.errorf(diag.InvalidOperation, s.Tag.Span(), "cannot switch on %s", t)
			t = types.Invalid
		}
	}
	seen := map[string]bool{}
	covered := map[int64]bool{}
	hasDefault := false
	c.breaks++
	for _, cl := range s.Cases {
		if cl.Default {
			if hasDefault {
				c.errorf(diag.DuplicateCase, cl.Sp, "multiple defaults in switch")
			}
			hasDefault = true
		}
		for _, v := range cl.Values {
			x := c.value(v)
			if t == types.Invalid || !c.assign(v, x, t, "case") {
				continue
			}
			k := c.a.Types[v].Const
			if k == nil {
				continue
			}
			if seen[k.Key()] {
				c.errorf(diag.DuplicateCase, v.Span(), "duplicate case %s", k)
			}
			seen[k.Key()] = true
			if k.Kind == types.CInt {
				covered[k.Int] = true
			}
		}
		c.block(cl.Body)
	}
	c.breaks--
	if t.Kind == types.KEnum && !hasDefault {
		var missing []string
		for i, m := range t.Members {
			if !covered[t.Values[i]] {
				missing = append(missing, m)
			}
		}
		if len(missing) > 0 {
			c.errorf(diag.NotExhaustive, s.Tag.Span(), "switch on %s is not exhaustive, missing %s; add the cases or a default", t, strings.Join(missing, ", "))
		}
	}
}

func terminating(list []ast.Stmt) bool {
	return len(list) > 0 && isTerminating(list[len(list)-1])
}

func isTerminating(s ast.Stmt) bool {
	switch s := s.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BlockStmt:
		return terminating(s.Stmts)
	case *ast.IfStmt:
		return s.Else != nil && s.Then != nil && terminating(s.Then.Stmts) && isTerminating(s.Else)
	case *ast.ForStmt:
		return s.Cond == nil && s.Body != nil && !hasBreak(s.Body.Stmts)
	case *ast.SwitchStmt:
		hasDefault := false
		for _, cl := range s.Cases {
			if cl.Default {
				hasDefault = true
			}
			if !terminating(cl.Body) || hasBreak(cl.Body) {
				return false
			}
		}
		return hasDefault
	}
	return false
}

func hasBreak(list []ast.Stmt) bool {
	for _, s := range list {
		switch s := s.(type) {
		case *ast.BranchStmt:
			if s.Tok == token.Break {
				return true
			}
		case *ast.BlockStmt:
			if hasBreak(s.Stmts) {
				return true
			}
		case *ast.IfStmt:
			if s.Then != nil && hasBreak(s.Then.Stmts) {
				return true
			}
			if s.Else != nil && hasBreak([]ast.Stmt{s.Else}) {
				return true
			}
		}
	}
	return false
}
