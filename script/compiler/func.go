package compiler

import (
	"fmt"

	"aego/script/asm"
	"aego/script/ast"
	"aego/script/bytecode"
	"aego/script/checker"
	"aego/script/diag"
	"aego/script/resolver"
	"aego/script/source"
	"aego/script/token"
	"aego/script/types"
)

type target struct {
	brk  asm.Label
	cont asm.Label
	loop bool
}

type fn struct {
	u       *unit
	f       *checker.Func
	b       *asm.Builder
	regs    map[*resolver.Object]int
	top     int
	targets []target
}

func (u *unit) function(f *checker.Func, idx int) {
	c := &fn{u: u, f: f, b: asm.NewBuilder(), regs: map[*resolver.Object]int{}}
	for _, p := range f.Params {
		c.regs[p] = c.alloc(p.Type.Slots())
	}
	params := c.top
	c.line(f.Decl.Sp)
	if f.Decl.Body != nil {
		c.stmts(f.Decl.Body.Stmts)
		end := f.Decl.Body.Sp
		if end.End > end.Start {
			end.Start = end.End - 1
		}
		c.line(end)
	}
	if f.Result == types.Void {
		c.b.ABC(bytecode.RET, 0, 0, 0)
	} else {
		c.b.ABx(bytecode.FAULT, 0, c.b.Const(bytecode.StringK("missing return in "+f.FullName())))
	}
	code, consts, lines, err := c.b.Finish()
	regs := max(c.b.MaxReg(), 1)
	if err == nil && regs > bytecode.MaxRegisters {
		err = fmt.Errorf("it needs %d registers, the limit is %d", regs, bytecode.MaxRegisters)
	}
	if err != nil {
		u.diags = append(u.diags, diag.Errorf(diag.RegisterLimit, f.Decl.Name.Sp, "%s cannot be compiled: %v", f.FullName(), err))
		return
	}
	script := int32(bytecode.NoScript)
	if f.Script != nil {
		script = int32(u.scripts[f.Script])
	}
	file := ""
	if u.opt.Sources != nil {
		file = u.opt.Sources.Path(f.Decl.Sp.File)
	}
	u.mod.Functions[idx] = bytecode.Function{
		Name: funcName(f), File: file, Params: uint8(params), Results: uint8(f.Result.Slots()),
		Registers: uint8(regs), Script: script, Coroutine: f.Decl.Coroutine,
		Code: code, Consts: consts, Lines: lines,
	}
}

func (c *fn) alloc(n int) int {
	r := c.top
	c.top += n
	c.b.Use(c.top)
	return r
}

func (c *fn) line(sp source.Span) {
	if c.u.opt.Sources == nil || !sp.Valid() {
		return
	}
	l, _ := c.u.opt.Sources.LineCol(sp.File, sp.Start)
	c.b.SetLine(l)
}

func (c *fn) stmts(list []ast.Stmt) {
	for _, s := range list {
		c.stmt(s)
	}
}

func (c *fn) block(list []ast.Stmt) {
	mark := c.top
	c.stmts(list)
	c.top = mark
}

func (c *fn) declare(id *ast.Ident, value ast.Expr, t *types.Type) {
	n := t.Slots()
	r := c.alloc(n)
	if obj := c.u.a.Defs[id]; obj != nil {
		c.regs[obj] = r
	}
	if value != nil {
		c.exprTo(value, r)
	} else {
		c.zero(r, n)
	}
	c.top = r + n
}

func (c *fn) stmt(s ast.Stmt) {
	c.line(s.Span())
	mark := c.top
	switch s := s.(type) {
	case *ast.VarStmt:
		c.declare(s.Name, s.Value, c.declType(s.Name, s.Value))
		return
	case *ast.ShortVarStmt:
		c.declare(s.Name, s.Value, c.declType(s.Name, s.Value))
		return
	case *ast.AssignStmt:
		c.assign(s)
	case *ast.IncDecStmt:
		c.incDec(s)
	case *ast.ExprStmt:
		switch x := unparen(s.X).(type) {
		case *ast.CallExpr:
			c.callTo(x, 0, false)
		case *ast.StartExpr:
			c.startTo(x, c.alloc(1))
		}
	case *ast.YieldStmt:
		c.yield(s)
	case *ast.ReturnStmt:
		if s.Value == nil {
			c.b.ABC(bytecode.RET, 0, 0, 0)
		} else {
			r := c.operand(s.Value)
			c.b.ABC(bytecode.RET, r, c.f.Result.Slots(), 0)
		}
	case *ast.BranchStmt:
		c.branch(s)
	case *ast.BlockStmt:
		c.block(s.Stmts)
	case *ast.IfStmt:
		c.ifStmt(s)
	case *ast.ForStmt:
		c.forStmt(s)
	case *ast.SwitchStmt:
		c.switchStmt(s)
	}
	c.top = mark
}

func (c *fn) declType(id *ast.Ident, value ast.Expr) *types.Type {
	if obj := c.u.a.Defs[id]; obj != nil && obj.Type != nil {
		return obj.Type
	}
	if value != nil {
		return c.typ(value)
	}
	return types.Invalid
}

func logical(e ast.Expr) bool {
	b, ok := unparen(e).(*ast.BinaryExpr)
	return ok && (b.Op == token.AndAnd || b.Op == token.OrOr)
}

func (c *fn) assign(s *ast.AssignStmt) {
	t := c.typ(s.Target)
	n := t.Slots()
	p, ok := c.placeOf(s.Target)
	if !ok {
		return
	}
	if s.Op == token.Assign {
		if p.kind == pReg && n == 1 && !logical(s.Value) {
			c.exprTo(s.Value, p.reg)
			return
		}
		v := c.alloc(n)
		c.exprTo(s.Value, v)
		c.store(p, v)
		return
	}
	op := s.Op.AssignOp()
	if p.kind == pReg {
		y := c.operand(s.Value)
		c.arith(p.reg, op, p.reg, y, t, c.typ(s.Value))
		return
	}
	cur := c.alloc(n)
	c.load(p, cur)
	y := c.operand(s.Value)
	c.arith(cur, op, cur, y, t, c.typ(s.Value))
	c.store(p, cur)
}

func (c *fn) incDec(s *ast.IncDecStmt) {
	t := c.typ(s.X)
	p, ok := c.placeOf(s.X)
	if !ok {
		return
	}
	one := c.alloc(1)
	if t.Kind == types.KFloat {
		c.b.ABx(bytecode.LOADK, one, c.b.Const(bytecode.FloatK(1)))
	} else {
		c.b.AsBx(bytecode.LOADI, one, 1)
	}
	op := token.Add
	if !s.Inc {
		op = token.Sub
	}
	if p.kind == pReg {
		c.arith(p.reg, op, p.reg, one, t, t)
		return
	}
	cur := c.alloc(1)
	c.load(p, cur)
	c.arith(cur, op, cur, one, t, t)
	c.store(p, cur)
}

func (c *fn) branch(s *ast.BranchStmt) {
	for i := len(c.targets) - 1; i >= 0; i-- {
		t := c.targets[i]
		if s.Tok == token.Break {
			c.b.Jump(bytecode.JMP, 0, t.brk)
			return
		}
		if t.loop {
			c.b.Jump(bytecode.JMP, 0, t.cont)
			return
		}
	}
}

func (c *fn) cond(e ast.Expr, onFalse asm.Label) {
	mark := c.top
	r := c.operand(e)
	c.b.Jump(bytecode.JMPNOT, r, onFalse)
	c.top = mark
}

func (c *fn) ifStmt(s *ast.IfStmt) {
	elseL, end := c.b.NewLabel(), c.b.NewLabel()
	c.cond(s.Cond, elseL)
	if s.Then != nil {
		c.block(s.Then.Stmts)
	}
	if s.Else != nil {
		c.b.Jump(bytecode.JMP, 0, end)
	}
	c.b.Mark(elseL)
	if s.Else != nil {
		c.stmt(s.Else)
		c.b.Mark(end)
	}
}

func (c *fn) forStmt(s *ast.ForStmt) {
	mark := c.top
	if s.Init != nil {
		c.stmt(s.Init)
	}
	head, cont, exit := c.b.NewLabel(), c.b.NewLabel(), c.b.NewLabel()
	c.b.Mark(head)
	if s.Cond != nil {
		c.cond(s.Cond, exit)
	}
	c.targets = append(c.targets, target{brk: exit, cont: cont, loop: true})
	if s.Body != nil {
		c.block(s.Body.Stmts)
	}
	c.targets = c.targets[:len(c.targets)-1]
	c.b.Mark(cont)
	if s.Post != nil {
		c.stmt(s.Post)
	}
	c.b.Jump(bytecode.JMP, 0, head)
	c.b.Mark(exit)
	c.top = mark
}

func (c *fn) switchStmt(s *ast.SwitchStmt) {
	t := c.typ(s.Tag)
	tag := c.alloc(t.Slots())
	c.exprTo(s.Tag, tag)
	exit := c.b.NewLabel()
	bodies := make([]asm.Label, len(s.Cases))
	def := -1
	for i, cl := range s.Cases {
		bodies[i] = c.b.NewLabel()
		if cl.Default {
			def = i
		}
		for _, v := range cl.Values {
			mark := c.top
			vr := c.operand(v)
			eq := c.alloc(1)
			c.equal(eq, tag, vr, t)
			c.b.Jump(bytecode.JMPIF, eq, bodies[i])
			c.top = mark
		}
	}
	if def >= 0 {
		c.b.Jump(bytecode.JMP, 0, bodies[def])
	} else {
		c.b.Jump(bytecode.JMP, 0, exit)
	}
	c.targets = append(c.targets, target{brk: exit})
	for i, cl := range s.Cases {
		c.b.Mark(bodies[i])
		c.block(cl.Body)
		c.b.Jump(bytecode.JMP, 0, exit)
	}
	c.targets = c.targets[:len(c.targets)-1]
	c.b.Mark(exit)
}
