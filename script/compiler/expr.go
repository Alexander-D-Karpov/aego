package compiler

import (
	"math"

	"aego/script/abi"
	"aego/script/ast"
	"aego/script/bytecode"
	"aego/script/checker"
	"aego/script/resolver"
	"aego/script/token"
	"aego/script/types"
)

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

func (c *fn) typ(e ast.Expr) *types.Type {
	return c.u.a.TypeOf(e)
}

func (c *fn) move(dst, src, n int) {
	switch {
	case dst == src || n == 0:
	case n == 1:
		c.b.ABC(bytecode.MOVE, dst, src, 0)
	default:
		c.b.ABC(bytecode.MOVEN, dst, src, n)
	}
}

func (c *fn) zero(dst, n int) {
	for i := 0; i < n; i++ {
		c.b.ABC(bytecode.LOADNIL, dst+i, 0, 0)
	}
}

func (c *fn) loadConst(dst int, k *types.Const, t *types.Type) {
	switch {
	case t.Kind == types.KFloat || t.Kind == types.KUntypedFloat:
		v := float32(k.Float64())
		if math.Float32bits(v) == 0 {
			c.b.ABC(bytecode.LOADNIL, dst, 0, 0)
			return
		}
		c.b.ABx(bytecode.LOADK, dst, c.b.Const(bytecode.FloatK(v)))
	case k.Kind == types.CString:
		if k.Str == "" {
			c.b.ABC(bytecode.LOADNIL, dst, 0, 0)
			return
		}
		c.b.ABx(bytecode.LOADK, dst, c.b.Const(bytecode.StringK(k.Str)))
	case k.Kind == types.CBool:
		v := 0
		if k.Bool {
			v = 1
		}
		c.b.ABC(bytecode.LOADBOOL, dst, v, 0)
	case k.Int >= math.MinInt16 && k.Int <= math.MaxInt16:
		c.b.AsBx(bytecode.LOADI, dst, int(k.Int))
	default:
		c.b.ABx(bytecode.LOADK, dst, c.b.Const(bytecode.IntK(k.Int)))
	}
}

func (c *fn) operand(e ast.Expr) int {
	if id, ok := unparen(e).(*ast.Ident); ok {
		if obj := c.u.a.Uses[id]; obj != nil && (obj.Kind == resolver.ObjVar || obj.Kind == resolver.ObjParam) {
			if r, ok := c.regs[obj]; ok {
				return r
			}
		}
	}
	r := c.alloc(c.typ(e).Slots())
	c.exprTo(e, r)
	return r
}

func (c *fn) exprTo(e ast.Expr, dst int) {
	if tv := c.u.a.Types[e]; tv.Const != nil {
		c.loadConst(dst, tv.Const, tv.Type)
		return
	}
	mark := c.top
	switch e := e.(type) {
	case *ast.ParenExpr:
		c.exprTo(e.X, dst)
	case *ast.BasicLit:
		c.b.ABC(bytecode.LOADNIL, dst, 0, 0)
	case *ast.Ident, *ast.IndexExpr:
		if p, ok := c.placeOf(e); ok {
			c.load(p, dst)
		}
	case *ast.SelectorExpr:
		c.selectorTo(e, dst)
	case *ast.CallExpr:
		c.callTo(e, dst, true)
	case *ast.StartExpr:
		c.startTo(e, dst)
	case *ast.UnaryExpr:
		c.unaryTo(e, dst)
	case *ast.BinaryExpr:
		c.binaryTo(e, dst)
	case *ast.CompositeLit:
		c.compositeTo(e, dst)
	}
	c.top = mark
}

type pkind uint8

const (
	pReg pkind = iota
	pSelf
	pIndex
	pComp
	pLane
	pXField
)

type place struct {
	kind  pkind
	reg   int
	slot  int
	idx   int
	off   int
	width int
	field int
	lane  int
	n     int
	inner *place
}

func (c *fn) placeOf(e ast.Expr) (place, bool) {
	switch e := e.(type) {
	case *ast.ParenExpr:
		return c.placeOf(e.X)
	case *ast.Ident:
		obj := c.u.a.Uses[e]
		if obj == nil {
			return place{}, false
		}
		switch obj.Kind {
		case resolver.ObjVar, resolver.ObjParam:
			r, ok := c.regs[obj]
			return place{kind: pReg, reg: r, n: obj.Type.Slots()}, ok
		case resolver.ObjField:
			f := obj.Data.(*checker.ScriptField)
			return place{kind: pSelf, slot: f.Slot + 1, n: f.Type.Slots()}, true
		case resolver.ObjNode:
			return place{kind: pSelf, slot: NodeSlot, n: 1}, true
		}
	case *ast.SelectorExpr:
		sel := c.u.a.Selections[e]
		if sel == nil {
			return place{}, false
		}
		switch sel.Kind {
		case checker.SelField:
			p, ok := c.placeOf(e.X)
			if !ok {
				return place{}, false
			}
			switch p.kind {
			case pReg:
				p.reg += sel.Field.Slot
			case pSelf:
				p.slot += sel.Field.Slot
			case pIndex, pXField:
				p.off += sel.Field.Slot
			default:
				return place{}, false
			}
			p.n = sel.Field.Type.Slots()
			return p, true
		case checker.SelLane:
			p, ok := c.placeOf(e.X)
			if !ok {
				return place{}, false
			}
			return place{kind: pLane, lane: sel.Field.Lane, inner: &p, n: 1}, true
		case checker.SelComponentField:
			return place{kind: pComp, reg: c.operand(e.X), field: sel.Comp.Index, n: 1}, true
		case checker.SelScriptField:
			r := c.operand(e.X)
			w := sel.ScriptField.Type.Slots()
			return place{kind: pXField, reg: r, field: c.u.xfield(sel.Script, sel.ScriptField), width: w, n: w}, true
		}
	case *ast.IndexExpr:
		s := c.operand(e.X)
		i := c.operand(e.Index)
		w := c.typ(e.X).Elem.Slots()
		return place{kind: pIndex, reg: s, idx: i, width: w, n: w}, true
	}
	return place{}, false
}

func (c *fn) load(p place, dst int) {
	mark := c.top
	switch p.kind {
	case pReg:
		c.move(dst, p.reg, p.n)
	case pSelf:
		for i := 0; i < p.n; i++ {
			c.b.ABx(bytecode.GETSF, dst+i, p.slot+i)
		}
	case pComp:
		c.b.ABC(bytecode.GETCF, dst, p.reg, p.field)
	case pIndex:
		if p.off == 0 && p.n == p.width {
			c.b.ABC(bytecode.INDEX, dst, p.reg, p.idx)
			break
		}
		t := c.alloc(p.width)
		c.b.ABC(bytecode.INDEX, t, p.reg, p.idx)
		c.move(dst, t+p.off, p.n)
	case pXField:
		if p.off == 0 && p.n == p.width {
			c.b.ABC(bytecode.GETXF, dst, p.reg, p.field)
			break
		}
		t := c.alloc(p.width)
		c.b.ABC(bytecode.GETXF, t, p.reg, p.field)
		c.move(dst, t+p.off, p.n)
	case pLane:
		t := c.alloc(1)
		c.load(*p.inner, t)
		c.b.ABC(bytecode.GETL, dst, t, p.lane)
	}
	c.top = mark
}

func (c *fn) store(p place, src int) {
	mark := c.top
	switch p.kind {
	case pReg:
		c.move(p.reg, src, p.n)
	case pSelf:
		for i := 0; i < p.n; i++ {
			c.b.ABx(bytecode.SETSF, src+i, p.slot+i)
		}
	case pComp:
		c.b.ABC(bytecode.SETCF, p.reg, src, p.field)
	case pIndex:
		if p.off == 0 && p.n == p.width {
			c.b.ABC(bytecode.SETINDEX, p.reg, p.idx, src)
			break
		}
		t := c.alloc(p.width)
		c.b.ABC(bytecode.INDEX, t, p.reg, p.idx)
		c.move(t+p.off, src, p.n)
		c.b.ABC(bytecode.SETINDEX, p.reg, p.idx, t)
	case pXField:
		if p.off == 0 && p.n == p.width {
			c.b.ABC(bytecode.SETXF, p.reg, src, p.field)
			break
		}
		t := c.alloc(p.width)
		c.b.ABC(bytecode.GETXF, t, p.reg, p.field)
		c.move(t+p.off, src, p.n)
		c.b.ABC(bytecode.SETXF, p.reg, t, p.field)
	case pLane:
		t := c.alloc(1)
		c.load(*p.inner, t)
		c.b.ABC(bytecode.SETL, t, src, p.lane)
		c.store(*p.inner, t)
	}
	c.top = mark
}

func (c *fn) selectorTo(e *ast.SelectorExpr, dst int) {
	sel := c.u.a.Selections[e]
	if sel == nil {
		return
	}
	if p, ok := c.placeOf(e); ok {
		c.load(p, dst)
		return
	}
	x := c.operand(e.X)
	switch sel.Kind {
	case checker.SelField:
		c.move(dst, x+sel.Field.Slot, sel.Field.Type.Slots())
	case checker.SelLane:
		c.b.ABC(bytecode.GETL, dst, x, sel.Field.Lane)
	case checker.SelComponentField:
		c.b.ABC(bytecode.GETCF, dst, x, sel.Comp.Index)
	}
}

func (c *fn) invoke(op bytecode.Opcode, idx int, args []ast.Expr, params []int, results, dst int, want bool) {
	mark := c.top
	total := 0
	for _, n := range params {
		total += n
	}
	window := max(total, results, 1)
	w := c.alloc(window)
	off := 0
	for i, a := range args {
		c.exprTo(a, w+off)
		off += params[i]
		c.top = w + window
	}
	c.b.ABx(op, w, idx)
	if want && results > 0 {
		c.move(dst, w, results)
	}
	c.top = mark
}

func (c *fn) callTo(e *ast.CallExpr, dst int, want bool) {
	call := c.u.a.Calls[e]
	if call == nil {
		return
	}
	switch call.Kind {
	case checker.CallLen:
		r := c.operand(e.Args[0])
		if c.typ(e.Args[0]).Kind == types.KString {
			c.b.ABC(bytecode.LENS, dst, r, 0)
		} else {
			c.b.ABC(bytecode.LEN, dst, r, 0)
		}
	case checker.CallAppend:
		c.exprTo(e.Args[0], dst)
		c.b.ABx(bytecode.ENSURE, dst, c.u.typeIndex(call.Type, e.Span()))
		for _, a := range e.Args[1:] {
			mark := c.top
			v := c.operand(a)
			c.b.ABC(bytecode.APPEND, dst, dst, v)
			c.top = mark
		}
	case checker.CallConvert:
		c.convert(e, call.Type, dst)
	case checker.CallFunc, checker.CallMethod:
		f := call.Func
		op, idx := bytecode.CALL, 0
		if i, ok := c.u.funcs[f]; ok && f.Pkg == c.u.pkg.Path {
			idx = i
		} else {
			op, idx = bytecode.CALLX, c.u.extern(f)
		}
		c.invoke(op, idx, e.Args, funcSlots(f), f.Result.Slots(), dst, want)
	case checker.CallHost:
		imp := c.u.hostImport(call.Module, call.Name, call.Sig)
		c.invoke(bytecode.CALLH, imp, e.Args, c.u.sigSlots(call.Sig.Params), c.u.abiSlots(call.Sig.Result), dst, want)
	case checker.CallHostMethod:
		recv := types.ToABI(c.typ(call.Recv))
		sig := abi.Signature{Params: append([]abi.TypeRef{recv}, call.Sig.Params...), Result: call.Sig.Result}
		imp := c.u.hostImport("#"+recv.Kind.String(), call.Name, sig)
		args := append([]ast.Expr{call.Recv}, e.Args...)
		c.invoke(bytecode.CALLH, imp, args, c.u.sigSlots(sig.Params), c.u.abiSlots(sig.Result), dst, want)
	case checker.CallComponentMethod:
		recv := abi.ComponentOf(call.Component.Name)
		sig := abi.Signature{Params: append([]abi.TypeRef{recv}, call.Sig.Params...), Result: call.Sig.Result}
		imp := c.u.hostImport("#"+call.Component.Name, call.Name, sig)
		args := append([]ast.Expr{call.Recv}, e.Args...)
		c.invoke(bytecode.CALLH, imp, args, c.u.sigSlots(sig.Params), c.u.abiSlots(sig.Result), dst, want)
	case checker.CallGetComponent, checker.CallHasComponent:
		name, res := "Get<"+call.Component.Name+">", abi.ComponentOf(call.Component.Name)
		if call.Kind == checker.CallHasComponent {
			name, res = "Has<"+call.Component.Name+">", abi.T(abi.Bool)
		}
		sig := abi.Sig(res, abi.T(abi.NodeRef))
		imp := c.u.hostImport("#NodeRef", name, sig)
		c.invoke(bytecode.CALLH, imp, []ast.Expr{call.Recv}, []int{1}, 1, dst, want)
	default:
		c.asyncCall(e, call, dst, want)
	}
}

func (c *fn) convert(e *ast.CallExpr, to *types.Type, dst int) {
	from := c.typ(e.Args[0])
	r := c.operand(e.Args[0])
	switch {
	case to.Kind == types.KInt && from.Kind == types.KFloat:
		c.b.ABC(bytecode.FTOI, dst, r, 0)
	case to.Kind == types.KFloat && from.IsInteger():
		c.b.ABC(bytecode.ITOF, dst, r, 0)
	case to.Kind == types.KString && from.IsInteger():
		c.b.ABC(bytecode.ITOS, dst, r, 0)
	case to.Kind == types.KString && from.Kind == types.KFloat:
		c.b.ABC(bytecode.FTOS, dst, r, 0)
	case to.Kind == types.KString && from.Kind == types.KBool:
		c.b.ABC(bytecode.BTOS, dst, r, 0)
	default:
		c.move(dst, r, 1)
	}
}

func (c *fn) unaryTo(e *ast.UnaryExpr, dst int) {
	t := c.typ(e)
	r := c.operand(e.X)
	switch {
	case e.Op == token.Not:
		c.b.ABC(bytecode.NOT, dst, r, 0)
	case t.IsVector():
		c.b.ABC(bytecode.VNEG, dst, r, 0)
	case t.Kind == types.KFloat:
		c.b.ABC(bytecode.NEGF, dst, r, 0)
	default:
		c.b.ABC(bytecode.NEGI, dst, r, 0)
	}
}

func (c *fn) binaryTo(e *ast.BinaryExpr, dst int) {
	if e.Op == token.AndAnd || e.Op == token.OrOr {
		end := c.b.NewLabel()
		c.exprTo(e.X, dst)
		if e.Op == token.AndAnd {
			c.b.Jump(bytecode.JMPNOT, dst, end)
		} else {
			c.b.Jump(bytecode.JMPIF, dst, end)
		}
		c.exprTo(e.Y, dst)
		c.b.Mark(end)
		return
	}
	// ISNIL tests x == nil without loading nil.
	if e.Op == token.Eq || e.Op == token.Ne {
		if other, ok := nilCompareOther(e); ok {
			r := c.operand(other)
			c.b.ABC(bytecode.ISNIL, dst, r, 0)
			if e.Op == token.Ne {
				c.b.ABC(bytecode.NOT, dst, dst, 0)
			}
			return
		}
	}
	xt, yt := c.typ(e.X), c.typ(e.Y)
	x := c.operand(e.X)
	y := c.operand(e.Y)
	switch e.Op {
	case token.Eq, token.Ne:
		c.equal(dst, x, y, xt)
		if e.Op == token.Ne {
			c.b.ABC(bytecode.NOT, dst, dst, 0)
		}
	case token.Lt, token.Le, token.Gt, token.Ge:
		if e.Op == token.Gt || e.Op == token.Ge {
			x, y = y, x
		}
		strict := e.Op == token.Lt || e.Op == token.Gt
		var op bytecode.Opcode
		switch {
		case xt.Kind == types.KFloat:
			op = bytecode.LEF
			if strict {
				op = bytecode.LTF
			}
		case xt.Kind == types.KString:
			op = bytecode.LES
			if strict {
				op = bytecode.LTS
			}
		default:
			op = bytecode.LEI
			if strict {
				op = bytecode.LTI
			}
		}
		c.b.ABC(op, dst, x, y)
	default:
		c.arith(dst, e.Op, x, y, xt, yt)
	}
}

func nilCompareOther(e *ast.BinaryExpr) (ast.Expr, bool) {
	switch {
	case isNilLit(e.X):
		return e.Y, true
	case isNilLit(e.Y):
		return e.X, true
	}
	return nil, false
}

func isNilLit(e ast.Expr) bool {
	lit, ok := unparen(e).(*ast.BasicLit)
	return ok && lit.Kind == token.Nil
}

func (c *fn) arith(dst int, op token.Kind, x, y int, xt, yt *types.Type) {
	switch {
	case xt.IsVector() || yt.IsVector():
		switch op {
		case token.Add:
			c.b.ABC(bytecode.VADD, dst, x, y)
		case token.Sub:
			c.b.ABC(bytecode.VSUB, dst, x, y)
		case token.Mul:
			if xt.IsVector() {
				c.b.ABC(bytecode.VMULS, dst, x, y)
			} else {
				c.b.ABC(bytecode.VMULS, dst, y, x)
			}
		case token.Quo:
			c.b.ABC(bytecode.VDIVS, dst, x, y)
		}
	case xt.Kind == types.KString:
		c.b.ABC(bytecode.ADDS, dst, x, y)
	case xt.Kind == types.KFloat:
		c.b.ABC(floatArithOp(op), dst, x, y)
	default:
		c.b.ABC(intArithOp(op), dst, x, y)
	}
}

func floatArithOp(op token.Kind) bytecode.Opcode {
	switch op {
	case token.Add:
		return bytecode.ADDF
	case token.Sub:
		return bytecode.SUBF
	case token.Mul:
		return bytecode.MULF
	case token.Quo:
		return bytecode.DIVF
	}
	return bytecode.NOP
}

func intArithOp(op token.Kind) bytecode.Opcode {
	switch op {
	case token.Add:
		return bytecode.ADDI
	case token.Sub:
		return bytecode.SUBI
	case token.Mul:
		return bytecode.MULI
	case token.Quo:
		return bytecode.DIVI
	case token.Rem:
		return bytecode.MODI
	}
	return bytecode.NOP
}

type leaf struct {
	off int
	t   *types.Type
}

func leaves(t *types.Type, base int, out []leaf) []leaf {
	switch t.Kind {
	case types.KStruct, types.KTransform2D, types.KTransform3D:
		for _, f := range t.Fields {
			out = leaves(f.Type, base+f.Slot, out)
		}
		return out
	}
	return append(out, leaf{off: base, t: t})
}

func (c *fn) eqSlot(dst, x, y int, t *types.Type) {
	switch {
	case t.Kind == types.KFloat:
		c.b.ABC(bytecode.EQF, dst, x, y)
	case t.Kind == types.KString:
		c.b.ABC(bytecode.EQS, dst, x, y)
	case t.IsVector():
		c.b.ABC(bytecode.EQV, dst, x, y)
	default:
		c.b.ABC(bytecode.EQ, dst, x, y)
	}
}

func (c *fn) equal(dst, x, y int, t *types.Type) {
	if t.Slots() <= 1 && t.Kind != types.KStruct {
		c.eqSlot(dst, x, y, t)
		return
	}
	mark := c.top
	tmp := c.alloc(1)
	fail, end := c.b.NewLabel(), c.b.NewLabel()
	for _, l := range leaves(t, 0, nil) {
		c.eqSlot(tmp, x+l.off, y+l.off, l.t)
		c.b.Jump(bytecode.JMPNOT, tmp, fail)
	}
	c.b.ABC(bytecode.LOADBOOL, dst, 1, 0)
	c.b.Jump(bytecode.JMP, 0, end)
	c.b.Mark(fail)
	c.b.ABC(bytecode.LOADBOOL, dst, 0, 0)
	c.b.Mark(end)
	c.top = mark
}

func (c *fn) compositeTo(e *ast.CompositeLit, dst int) {
	t := c.u.a.Composites[e]
	if t == nil {
		return
	}
	switch {
	case t.Kind == types.KSlice:
		c.b.ABx(bytecode.NEWSLICE, dst, c.u.typeIndex(t, e.Span()))
		for _, el := range e.Elems {
			mark := c.top
			v := c.operand(el.Value)
			c.b.ABC(bytecode.APPEND, dst, dst, v)
			c.top = mark
		}
	case t.IsVector():
		n := len(t.Fields)
		tmp := c.alloc(n)
		c.zero(tmp, n)
		for i, el := range e.Elems {
			lane := i
			if el.Key != nil {
				lane = t.Field(el.Key.Name).Lane
			}
			c.exprTo(el.Value, tmp+lane)
		}
		op := bytecode.VEC4
		switch n {
		case 2:
			op = bytecode.VEC2
		case 3:
			op = bytecode.VEC3
		}
		c.b.ABC(op, dst, tmp, 0)
	default:
		c.zero(dst, t.Slots())
		for i, el := range e.Elems {
			var f *types.Field
			if el.Key != nil {
				f = t.Field(el.Key.Name)
			} else if i < len(t.Fields) {
				f = t.Fields[i]
			}
			if f != nil {
				c.exprTo(el.Value, dst+f.Slot)
			}
		}
	}
}
