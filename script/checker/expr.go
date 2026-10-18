package checker

import (
	"math"

	"aego/script/abi"
	"aego/script/ast"
	"aego/script/diag"
	"aego/script/lexer"
	"aego/script/resolver"
	"aego/script/source"
	"aego/script/token"
	"aego/script/types"
)

func (c *checker) record(e ast.Expr, x operand) {
	if x.mode != mInvalid && x.mode != mSignal {
		c.a.Types[e] = TypeAndValue{Type: x.t, Const: x.c, Addr: x.addr}
	}
}

func (c *checker) expr(e ast.Expr) operand {
	var x operand
	switch e := e.(type) {
	case *ast.Ident:
		x = c.ident(e)
	case *ast.BasicLit:
		x = c.literal(e)
	case *ast.ParenExpr:
		x = c.expr(e.X)
	case *ast.SelectorExpr:
		x = c.selector(e)
	case *ast.IndexExpr:
		x = c.index(e)
	case *ast.CallExpr:
		x = c.call(e)
	case *ast.StartExpr:
		x = c.start(e)
	case *ast.UnaryExpr:
		x = c.unary(e)
	case *ast.BinaryExpr:
		x = c.binary(e)
	case *ast.CompositeLit:
		x = c.composite(e)
	default:
		x = invalid
	}
	c.record(e, x)
	return x
}

func (c *checker) value(e ast.Expr) operand {
	x := c.expr(e)
	switch x.mode {
	case mType:
		c.errorf(diag.NotValue, e.Span(), "%s is a type, not a value", x.t)
	case mVoid:
		c.errorf(diag.NotValue, e.Span(), "call does not return a value")
	case mFunc:
		c.errorf(diag.NotValue, e.Span(), "function must be called")
	case mSignal:
		n := x.signalName()
		c.errorf(diag.NotValue, e.Span(), "signal %s is not a value, use %s.Emit or %s.Connect", n, n, n)
	default:
		return x
	}
	return invalid
}

func (c *checker) objectOperand(sp source.Span, obj *resolver.Object) operand {
	switch obj.Kind {
	case resolver.ObjConst:
		c.constObj(obj)
		if obj.Type == nil || obj.Type == types.Invalid {
			return invalid
		}
		return operand{mode: mValue, t: obj.Type, c: obj.Const}
	case resolver.ObjVar, resolver.ObjParam, resolver.ObjField:
		if obj.Type == nil || obj.Type == types.Invalid {
			return invalid
		}
		return operand{mode: mValue, t: obj.Type, addr: true}
	case resolver.ObjNode:
		return val(types.NodeRef)
	case resolver.ObjType:
		return operand{mode: mType, t: obj.Type}
	case resolver.ObjFunc, resolver.ObjBuiltin:
		return operand{mode: mFunc, t: types.Invalid}
	case resolver.ObjSignal:
		if sg, ok := obj.Data.(*ScriptSignal); ok {
			return operand{mode: mSignal, t: types.Void, sig: sg}
		}
	case resolver.ObjGeneric:
		c.errorf(diag.BadTypeArg, sp, "%s needs a type argument", obj.Name)
	case resolver.ObjPkg:
		c.errorf(diag.NotValue, sp, "use of package %s without a selector", obj.Name)
	case resolver.ObjScript:
		c.errorf(diag.NotValue, sp, "script %s is not a value", obj.Name)
	}
	return invalid
}

func (c *checker) ident(id *ast.Ident) operand {
	if id.Name == "_" {
		c.errorf(diag.NotValue, id.Sp, "cannot use _ as a value")
		return invalid
	}
	obj := c.lookup(id.Name)
	if obj == nil {
		c.errorf(diag.UndefinedName, id.Sp, "undefined: %s", id.Name)
		return invalid
	}
	c.use(id, obj)
	return c.objectOperand(id.Sp, obj)
}

func (c *checker) literal(e *ast.BasicLit) operand {
	switch e.Kind {
	case token.Int:
		v, ok := lexer.ParseInt(e.Raw)
		if !ok || v > math.MaxInt64 {
			c.errorf(diag.ConstOverflow, e.Sp, "integer constant %s overflows int", e.Raw)
			return invalid
		}
		return operand{mode: mValue, t: types.UntypedInt, c: types.IntConst(int64(v))}
	case token.Float:
		v, ok := lexer.ParseFloat(e.Raw)
		if !ok || math.IsInf(v, 0) {
			c.errorf(diag.ConstOverflow, e.Sp, "float constant %s overflows", e.Raw)
			return invalid
		}
		return operand{mode: mValue, t: types.UntypedFloat, c: types.FloatConst(v)}
	case token.String:
		return operand{mode: mValue, t: types.String, c: types.StringConst(e.Str)}
	case token.True, token.False:
		return operand{mode: mValue, t: types.Bool, c: types.BoolConst(e.Kind == token.True)}
	case token.Nil:
		return val(types.Nil)
	}
	return invalid
}

func (c *checker) pkgOf(e ast.Expr) *resolver.Object {
	id, ok := e.(*ast.Ident)
	if !ok {
		return nil
	}
	o := c.lookup(id.Name)
	if o == nil || o.Kind != resolver.ObjPkg {
		return nil
	}
	c.use(id, o)
	return o
}

func (c *checker) member(sel *ast.Ident, p *resolver.Object) *resolver.Object {
	obj := c.tab.Packages[p.Target].Scope.LookupLocal(sel.Name)
	if obj == nil {
		c.errorf(diag.UndefinedName, sel.Sp, "undefined: %s.%s", p.Name, sel.Name)
		return nil
	}
	if !obj.Exported() {
		c.errorf(diag.NotExported, sel.Sp, "%s.%s is not exported", p.Name, sel.Name)
		return nil
	}
	c.use(sel, obj)
	return obj
}

func hostConst(k abi.Const) *types.Const {
	switch k.Type.Kind {
	case abi.Bool:
		return types.BoolConst(k.Bool)
	case abi.Float:
		return types.FloatConst(k.Float)
	case abi.String:
		return types.StringConst(k.Str)
	}
	return types.IntConst(k.Int)
}

func (c *checker) selector(s *ast.SelectorExpr) operand {
	if p := c.pkgOf(s.X); p != nil {
		return c.pkgMember(s, p)
	}
	x := c.expr(s.X)
	switch x.mode {
	case mInvalid:
		return invalid
	case mType:
		return c.typeMember(s, x.t)
	case mValue:
		return c.field(s, x)
	}
	c.errorf(diag.NotValue, s.X.Span(), "expression has no fields")
	return invalid
}

func (c *checker) pkgMember(s *ast.SelectorExpr, p *resolver.Object) operand {
	name := s.Sel.Name
	if p.Target == "" {
		return invalid
	}
	if !p.Host {
		obj := c.member(s.Sel, p)
		if obj == nil {
			return invalid
		}
		c.a.Selections[s] = &Selection{Kind: SelPackage, Object: obj}
		return c.objectOperand(s.Sel.Sp, obj)
	}
	if k, ok := c.surface.Const(p.Target, name); ok {
		t := c.u.FromABI(k.Type, c.surface)
		if t == types.Invalid {
			return invalid
		}
		c.a.Selections[s] = &Selection{Kind: SelHostConst, Object: p}
		return operand{mode: mValue, t: t, c: hostConst(k)}
	}
	if e, ok := c.surface.Enum(p.Target, name); ok {
		return operand{mode: mType, t: c.u.HostEnum(e)}
	}
	if sd, ok := c.surface.Struct(p.Target, name); ok {
		return operand{mode: mType, t: c.u.HostStruct(sd, c.surface)}
	}
	if _, ok := c.surface.Func(p.Target, name); ok {
		c.errorf(diag.NotValue, s.Sp, "function %s.%s must be called", p.Name, name)
		return invalid
	}
	c.errorf(diag.UnknownHost, s.Sel.Sp, "%s has no member %s", p.Target, name)
	return invalid
}

func (c *checker) typeMember(s *ast.SelectorExpr, t *types.Type) operand {
	if t.Kind != types.KEnum {
		c.errorf(diag.UnknownField, s.Sel.Sp, "type %s has no members", t)
		return invalid
	}
	v, ok := t.Member(s.Sel.Name)
	if !ok {
		c.errorf(diag.UnknownField, s.Sel.Sp, "%s has no member %s", t, s.Sel.Name)
		return invalid
	}
	c.a.Selections[s] = &Selection{Kind: SelEnumMember, Value: v}
	return operand{mode: mValue, t: t, c: types.IntConst(v)}
}

func (c *checker) field(s *ast.SelectorExpr, x operand) operand {
	name := s.Sel.Name
	t := x.t
	switch t.Kind {
	case types.KComponentRef:
		if cf, ok := t.Component.Field(name); ok {
			c.a.Selections[s] = &Selection{Kind: SelComponentField, Comp: cf}
			return operand{mode: mValue, t: c.u.FromABI(cf.Type, c.surface), addr: !cf.ReadOnly}
		}
		if i, ok := t.Component.Signal(name); ok {
			return c.hostSignal(s, i, t.Component.Signals[i], t.Component, "")
		}
	case types.KHandle:
		if i, sd, ok := c.surface.HandleSignal(t.Handle, name); ok {
			return c.hostSignal(s, i, sd, nil, t.Handle)
		}
		if _, ok := c.surface.HandleMethod(t.Handle, name); ok {
			c.errorf(diag.NotValue, s.Sp, "method %s must be called", name)
			return invalid
		}
	case types.KScriptRef:
		if st := c.a.scripts[t.Script]; st != nil {
			return c.scriptMember(s, st)
		}
	case types.KCoroutine, types.KConnection, types.KCurve:
	case types.KNodeRef, types.KAssetRef:
		if _, ok := c.surface.Method(types.ToABI(t).Kind, name); ok {
			c.errorf(diag.NotValue, s.Sp, "method %s must be called", name)
			return invalid
		}
	default:
		if f := t.Field(name); f != nil {
			if t.Kind == types.KStruct && t.Pkg != c.rp.Path && !abi.Exported(name) {
				c.errorf(diag.NotExported, s.Sel.Sp, "field %s of %s is not exported", name, t)
				return invalid
			}
			kind := SelField
			if f.Lane >= 0 {
				kind = SelLane
			}
			c.a.Selections[s] = &Selection{Kind: kind, Field: f}
			return operand{mode: mValue, t: f.Type, addr: x.addr}
		}
	}
	c.errorf(diag.UnknownField, s.Sel.Sp, "%s has no field %s", t, name)
	return invalid
}

func (c *checker) index(e *ast.IndexExpr) operand {
	x := c.value(e.X)
	i := c.value(e.Index)
	if x.mode == mInvalid {
		return invalid
	}
	if x.t.Kind != types.KSlice {
		c.errorf(diag.InvalidOperation, e.X.Span(), "cannot index a value of type %s", x.t)
		return invalid
	}
	if c.assign(e.Index, i, types.Int, "index") {
		if k := c.a.Types[e.Index].Const; k != nil && k.Int < 0 {
			c.errorf(diag.InvalidOperation, e.Index.Span(), "index must not be negative")
		}
	}
	return operand{mode: mValue, t: x.t.Elem, addr: true}
}

func (c *checker) unary(e *ast.UnaryExpr) operand {
	if e.Op == token.Sub {
		if lit, ok := e.X.(*ast.BasicLit); ok && lit.Kind == token.Int {
			if v, ok := lexer.ParseInt(lit.Raw); ok && v == 1<<63 {
				k := operand{mode: mValue, t: types.UntypedInt, c: types.IntConst(math.MinInt64)}
				c.record(lit, k)
				return k
			}
		}
	}
	x := c.value(e.X)
	if x.mode == mInvalid {
		return invalid
	}
	switch {
	case e.Op == token.Sub && x.t.IsVector():
		return val(x.t)
	case e.Op == token.Sub && x.t.IsNumeric(), e.Op == token.Not && x.t == types.Bool:
		if x.c == nil {
			return val(x.t)
		}
		k, why := foldUnary(e.Op, x.c)
		if why != "" {
			c.errorf(diag.ConstOverflow, e.Sp, "constant %s", why)
			return invalid
		}
		return operand{mode: mValue, t: x.t, c: k}
	}
	c.errorf(diag.InvalidOperation, e.Sp, "operator %s is not defined on %s", e.Op, x.t)
	return invalid
}

func (c *checker) binary(e *ast.BinaryExpr) operand {
	x := c.value(e.X)
	y := c.value(e.Y)
	if x.mode == mInvalid || y.mode == mInvalid {
		return invalid
	}
	return c.binop(e.Sp, e.Op, x, y, e.X, e.Y)
}

func (c *checker) badOp(sp source.Span, op token.Kind, x, y operand) operand {
	c.errorf(diag.InvalidOperation, sp, "operator %s is not defined on %s and %s", op, x.t, y.t)
	return invalid
}

func (c *checker) mismatch(sp source.Span, x, y operand) operand {
	c.errorf(diag.TypeMismatch, sp, "mismatched types %s and %s", x.t, y.t)
	return invalid
}

func (c *checker) binop(sp source.Span, op token.Kind, x, y operand, ex, ey ast.Expr) operand {
	switch op {
	case token.AndAnd, token.OrOr:
		if x.t != types.Bool || y.t != types.Bool {
			return c.badOp(sp, op, x, y)
		}
		return c.fold(sp, op, x, y, types.Bool)
	case token.Eq, token.Ne:
		if !c.match(&x, &y, ex, ey) {
			return c.mismatch(sp, x, y)
		}
		if x.t == types.Nil || !x.t.Comparable() {
			c.errorf(diag.InvalidOperation, sp, "values of type %s cannot be compared", x.t)
			return invalid
		}
		return c.fold(sp, op, x, y, types.Bool)
	case token.Lt, token.Le, token.Gt, token.Ge:
		if !c.match(&x, &y, ex, ey) {
			return c.mismatch(sp, x, y)
		}
		if !x.t.Ordered() {
			return c.badOp(sp, op, x, y)
		}
		return c.fold(sp, op, x, y, types.Bool)
	}
	if x.t.IsVector() || y.t.IsVector() {
		return c.vectorOp(sp, op, x, y, ex, ey)
	}
	if !c.match(&x, &y, ex, ey) {
		return c.mismatch(sp, x, y)
	}
	t := x.t
	switch {
	case op == token.Add && t == types.String:
	case !t.IsNumeric(), op == token.Rem && !t.IsInteger():
		return c.badOp(sp, op, x, y)
	}
	if (op == token.Quo || op == token.Rem) && t.IsInteger() && x.c == nil && y.c != nil && y.c.Kind == types.CInt && y.c.Int == 0 {
		c.errorf(diag.DivisionByZero, ey.Span(), "integer division by zero")
		return invalid
	}
	return c.fold(sp, op, x, y, t)
}

func (c *checker) fold(sp source.Span, op token.Kind, x, y operand, t *types.Type) operand {
	if x.c == nil || y.c == nil {
		return val(t)
	}
	k, why := foldBinary(op, x.c, y.c, x.t == types.Float)
	if why != "" {
		code := diag.ConstOverflow
		if why == whyDivZero {
			code = diag.DivisionByZero
		}
		c.errorf(code, sp, "constant %s", why)
		return invalid
	}
	return operand{mode: mValue, t: t, c: k}
}

func (c *checker) vectorOp(sp source.Span, op token.Kind, x, y operand, ex, ey ast.Expr) operand {
	switch op {
	case token.Add, token.Sub:
		if x.t == y.t {
			return val(x.t)
		}
	case token.Mul:
		if x.t.IsVector() && c.coerce(ey, &y, types.Float) {
			return val(x.t)
		}
		if y.t.IsVector() && c.coerce(ex, &x, types.Float) {
			return val(y.t)
		}
	case token.Quo:
		if x.t.IsVector() && c.coerce(ey, &y, types.Float) {
			return val(x.t)
		}
	}
	return c.badOp(sp, op, x, y)
}

func (c *checker) match(x, y *operand, ex, ey ast.Expr) bool {
	xu := x.t.IsUntyped() || x.t == types.Nil
	yu := y.t.IsUntyped() || y.t == types.Nil
	switch {
	case xu && !yu:
		return c.coerce(ex, x, y.t)
	case yu && !xu:
		return c.coerce(ey, y, x.t)
	case xu && yu:
		if x.t.IsNumeric() && y.t.IsNumeric() {
			if x.t == types.UntypedFloat || y.t == types.UntypedFloat {
				return c.coerce(ex, x, types.UntypedFloat) && c.coerce(ey, y, types.UntypedFloat)
			}
			return true
		}
	}
	return x.t == y.t
}

func (c *checker) coerce(e ast.Expr, x *operand, to *types.Type) bool {
	if x.t == to {
		return true
	}
	if x.t == types.Nil {
		if !to.IsRef() {
			return false
		}
		x.t = to
		c.record(e, *x)
		return true
	}
	if x.c == nil || !x.t.IsUntyped() {
		return false
	}
	k, why := types.Represent(x.c, to)
	if why != "" {
		return false
	}
	x.t, x.c = to, k
	c.record(e, *x)
	return true
}

func (c *checker) assign(e ast.Expr, x operand, to *types.Type, context string) bool {
	if x.mode == mInvalid || to == types.Invalid || x.t == types.Invalid {
		return false
	}
	y := x
	if c.coerce(e, &y, to) {
		return true
	}
	what := "value of type " + x.t.String()
	switch {
	case x.t == types.Nil:
		what = "nil"
	case x.c != nil && x.t.IsUntyped():
		if _, why := types.Represent(x.c, to); why != "" {
			what = "constant " + x.c.String() + " (" + why + ")"
		}
	}
	c.errorf(diag.TypeMismatch, e.Span(), "cannot use %s as %s in %s", what, to, context)
	return false
}

func (c *checker) composite(e *ast.CompositeLit) operand {
	t := c.typeExpr(e.Type)
	if t == types.Invalid {
		for _, el := range e.Elems {
			c.expr(el.Value)
		}
		return invalid
	}
	c.a.Composites[e] = t
	switch {
	case t.Kind == types.KSlice:
		for _, el := range e.Elems {
			if el.Key != nil {
				c.errorf(diag.InvalidComposite, el.Key.Sp, "slice literal elements cannot have keys")
			}
			c.assign(el.Value, c.value(el.Value), t.Elem, "slice literal")
		}
	case len(t.Fields) > 0:
		c.compositeFields(e, t)
	default:
		c.errorf(diag.InvalidComposite, e.Type.Span(), "cannot construct %s with a composite literal", t)
		return invalid
	}
	return val(t)
}

func (c *checker) compositeFields(e *ast.CompositeLit, t *types.Type) {
	if len(e.Elems) == 0 {
		return
	}
	keyed := e.Elems[0].Key != nil
	if !keyed && len(e.Elems) != len(t.Fields) {
		c.errorf(diag.InvalidComposite, e.Sp, "%s literal needs %d values, got %d", t, len(t.Fields), len(e.Elems))
	}
	seen := map[string]bool{}
	for i, el := range e.Elems {
		x := c.value(el.Value)
		if (el.Key != nil) != keyed {
			c.errorf(diag.InvalidComposite, el.Sp, "mixture of keyed and positional values")
			continue
		}
		var f *types.Field
		switch {
		case keyed:
			if f = t.Field(el.Key.Name); f == nil {
				c.errorf(diag.UnknownField, el.Key.Sp, "%s has no field %s", t, el.Key.Name)
				continue
			}
			if seen[f.Name] {
				c.errorf(diag.InvalidComposite, el.Key.Sp, "duplicate field %s", f.Name)
				continue
			}
			seen[f.Name] = true
			if t.Kind == types.KStruct && t.Pkg != c.rp.Path && !abi.Exported(f.Name) {
				c.errorf(diag.NotExported, el.Key.Sp, "field %s of %s is not exported", f.Name, t)
				continue
			}
		case i < len(t.Fields):
			f = t.Fields[i]
		default:
			continue
		}
		c.assign(el.Value, x, f.Type, "field "+f.Name)
	}
}
