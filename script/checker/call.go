package checker

import (
	"math"
	"strconv"

	"aego/script/abi"
	"aego/script/ast"
	"aego/script/diag"
	"aego/script/resolver"
	"aego/script/source"
	"aego/script/types"
)

func (c *checker) args(list []ast.Expr) {
	for _, a := range list {
		c.expr(a)
	}
}

func (c *checker) noTypeArgs(e *ast.CallExpr) {
	if len(e.TypeArgs) > 0 {
		c.errorf(diag.BadTypeArg, e.TypeArgs[0].Span(), "type arguments are only allowed on Get and Has")
	}
}

func (c *checker) argList(sp source.Span, args []ast.Expr, name string, params []*types.Type) {
	if len(args) != len(params) {
		c.errorf(diag.ArgCount, sp, "%s expects %d arguments, got %d", name, len(params), len(args))
		c.args(args)
		return
	}
	for i, a := range args {
		c.assign(a, c.value(a), params[i], "argument to "+name)
	}
}

func (c *checker) arguments(e *ast.CallExpr, name string, params []*types.Type) {
	c.argList(e.Sp, e.Args, name, params)
}

func (c *checker) abiTypes(list []abi.TypeRef) []*types.Type {
	out := make([]*types.Type, len(list))
	for i, r := range list {
		out[i] = c.u.FromABI(r, c.surface)
	}
	return out
}

func (c *checker) call(e *ast.CallExpr) operand {
	switch fun := e.Fun.(type) {
	case *ast.Ident:
		obj := c.lookup(fun.Name)
		if obj == nil {
			c.errorf(diag.UndefinedName, fun.Sp, "undefined: %s", fun.Name)
			c.args(e.Args)
			return invalid
		}
		c.use(fun, obj)
		return c.callObject(e, fun.Sp, obj)
	case *ast.SelectorExpr:
		if p := c.pkgOf(fun.X); p != nil {
			return c.callPackage(e, fun, p)
		}
		x := c.expr(fun.X)
		switch {
		case x.mode == mSignal:
			return c.callSignal(e, fun, x)
		case x.mode != mValue:
		case x.t.Kind == types.KNodeRef, x.t.Kind == types.KAssetRef:
			return c.callMethod(e, fun, x)
		case x.t.Kind == types.KScriptRef:
			return c.callScriptMethod(e, fun, x)
		case x.t.Kind == types.KCoroutine, x.t.Kind == types.KConnection:
			return c.callHandle(e, fun, x)
		}
		if x.mode != mInvalid {
			c.errorf(diag.NotCallable, fun.Sel.Sp, "%s is not a function", fun.Sel.Name)
		}
		c.args(e.Args)
		return invalid
	}
	if x := c.expr(e.Fun); x.mode != mInvalid {
		c.errorf(diag.NotCallable, e.Fun.Span(), "expression is not callable")
	}
	c.args(e.Args)
	return invalid
}

func (c *checker) callObject(e *ast.CallExpr, sp source.Span, obj *resolver.Object) operand {
	switch obj.Kind {
	case resolver.ObjBuiltin:
		return c.builtin(e, obj.Name)
	case resolver.ObjType:
		return c.conversion(e, obj.Type)
	case resolver.ObjFunc:
		f, ok := obj.Data.(*Func)
		if !ok {
			c.args(e.Args)
			return invalid
		}
		kind := CallFunc
		if f.Script != nil {
			kind = CallMethod
		}
		return c.callFunc(e, f, kind)
	}
	if x := c.objectOperand(sp, obj); x.mode != mInvalid {
		c.errorf(diag.NotCallable, sp, "%s is not a function", obj.Name)
	}
	c.args(e.Args)
	return invalid
}

func (c *checker) callFunc(e *ast.CallExpr, f *Func, kind CallKind) operand {
	c.noTypeArgs(e)
	if f.Decl.Coroutine {
		c.errorf(diag.CoroutineCall, e.Sp, "coroutine %s is called without start, write start %s(...)", f.Name, f.Name)
		c.args(e.Args)
		return invalid
	}
	c.arguments(e, f.Name, funcParams(f))
	c.a.Calls[e] = &Call{Kind: kind, Func: f}
	return result(f.Result)
}

func (c *checker) callPackage(e *ast.CallExpr, sel *ast.SelectorExpr, p *resolver.Object) operand {
	name := sel.Sel.Name
	if p.Target == "" {
		c.args(e.Args)
		return invalid
	}
	if !p.Host {
		obj := c.member(sel.Sel, p)
		if obj == nil {
			c.args(e.Args)
			return invalid
		}
		c.a.Selections[sel] = &Selection{Kind: SelPackage, Object: obj}
		return c.callObject(e, sel.Sel.Sp, obj)
	}
	if sig, ok := c.surface.Func(p.Target, name); ok {
		c.noTypeArgs(e)
		c.arguments(e, p.Name+"."+name, c.abiTypes(sig.Params))
		c.a.Calls[e] = &Call{Kind: CallHost, Module: p.Target, Name: name, Sig: sig}
		return result(c.u.FromABI(sig.Result, c.surface))
	}
	if en, ok := c.surface.Enum(p.Target, name); ok {
		return c.conversion(e, c.u.HostEnum(en))
	}
	c.errorf(diag.UnknownHost, sel.Sel.Sp, "%s has no function %s", p.Target, name)
	c.args(e.Args)
	return invalid
}

func (c *checker) callMethod(e *ast.CallExpr, sel *ast.SelectorExpr, recv operand) operand {
	name := sel.Sel.Name
	if recv.t.Kind == types.KNodeRef {
		switch name {
		case "Get", "Has":
			return c.intrinsic(e, sel, name)
		case "GetScript":
			return c.getScript(e, sel)
		}
	}
	c.noTypeArgs(e)
	sig, ok := c.surface.Method(types.ToABI(recv.t).Kind, name)
	if !ok {
		c.errorf(diag.UnknownHost, sel.Sel.Sp, "%s has no method %s", recv.t, name)
		c.args(e.Args)
		return invalid
	}
	c.arguments(e, name, c.abiTypes(sig.Params))
	c.a.Calls[e] = &Call{Kind: CallHostMethod, Name: name, Sig: sig, Recv: sel.X}
	return result(c.u.FromABI(sig.Result, c.surface))
}

func (c *checker) intrinsic(e *ast.CallExpr, sel *ast.SelectorExpr, name string) operand {
	if len(e.TypeArgs) != 1 {
		c.errorf(diag.BadTypeArg, e.Sp, "%s needs one component type argument, like %s<SpriteRenderer>()", name, name)
		c.args(e.Args)
		return invalid
	}
	if len(e.Args) != 0 {
		c.errorf(diag.ArgCount, e.Sp, "%s takes no arguments", name)
		c.args(e.Args)
	}
	nt, ok := e.TypeArgs[0].(*ast.NamedType)
	if !ok {
		c.errorf(diag.BadTypeArg, e.TypeArgs[0].Span(), "%s argument must be a component name", name)
		return invalid
	}
	comp := c.component(nt)
	if comp == nil {
		return invalid
	}
	if name == "Has" {
		c.a.Calls[e] = &Call{Kind: CallHasComponent, Component: comp, Recv: sel.X}
		return val(types.Bool)
	}
	c.a.Calls[e] = &Call{Kind: CallGetComponent, Component: comp, Recv: sel.X}
	return val(c.u.ComponentRef(comp))
}

func (c *checker) builtin(e *ast.CallExpr, name string) operand {
	c.noTypeArgs(e)
	switch name {
	case "len":
		if len(e.Args) != 1 {
			c.errorf(diag.ArgCount, e.Sp, "len expects 1 argument, got %d", len(e.Args))
			c.args(e.Args)
			return invalid
		}
		x := c.value(e.Args[0])
		switch {
		case x.mode == mInvalid:
			return invalid
		case x.t.Kind == types.KSlice:
		case x.t == types.String:
			if x.c != nil {
				return operand{mode: mValue, t: types.Int, c: types.IntConst(int64(len(x.c.Str)))}
			}
		default:
			c.errorf(diag.InvalidOperation, e.Args[0].Span(), "len is not defined on %s", x.t)
			return invalid
		}
		c.a.Calls[e] = &Call{Kind: CallLen}
		return val(types.Int)
	case "append":
		if len(e.Args) < 2 {
			c.errorf(diag.ArgCount, e.Sp, "append needs a slice and at least one value")
			c.args(e.Args)
			return invalid
		}
		s := c.value(e.Args[0])
		if s.mode == mInvalid {
			c.args(e.Args[1:])
			return invalid
		}
		if s.t.Kind != types.KSlice {
			c.errorf(diag.InvalidOperation, e.Args[0].Span(), "first argument to append must be a slice, not %s", s.t)
			c.args(e.Args[1:])
			return invalid
		}
		for _, a := range e.Args[1:] {
			c.assign(a, c.value(a), s.t.Elem, "append")
		}
		c.a.Calls[e] = &Call{Kind: CallAppend, Type: s.t}
		return val(s.t)
	}
	return invalid
}

func (c *checker) conversion(e *ast.CallExpr, to *types.Type) operand {
	c.noTypeArgs(e)
	if len(e.Args) != 1 {
		c.errorf(diag.ArgCount, e.Sp, "conversion to %s needs exactly one argument", to)
		c.args(e.Args)
		return invalid
	}
	x := c.value(e.Args[0])
	if x.mode == mInvalid {
		return val(to)
	}
	from := x.t
	ok := false
	switch to.Kind {
	case types.KInt:
		ok = from.IsNumeric() || from.Kind == types.KEnum
	case types.KFloat:
		ok = from.IsNumeric()
	case types.KString:
		ok = from.IsNumeric() || from == types.Bool || from == types.String
	case types.KEnum:
		ok = from.IsInteger() || from == to
	default:
		ok = from == to
	}
	if !ok {
		c.errorf(diag.BadConversion, e.Sp, "cannot convert %s to %s", from, to)
		return invalid
	}
	if from.IsUntyped() {
		c.record(e.Args[0], operand{mode: mValue, t: types.Default(from), c: x.c})
	}
	c.a.Calls[e] = &Call{Kind: CallConvert, Type: to}
	if x.c != nil {
		if k := convertConst(x.c, to); k != nil {
			return operand{mode: mValue, t: to, c: k}
		}
	}
	return val(to)
}

func convertConst(k *types.Const, to *types.Type) *types.Const {
	switch to.Kind {
	case types.KInt, types.KEnum:
		switch k.Kind {
		case types.CInt:
			return k
		case types.CFloat:
			if k.Float >= -9.223372036854776e18 && k.Float < 9.223372036854776e18 {
				return types.IntConst(int64(k.Float))
			}
		}
	case types.KFloat:
		if k.Kind == types.CInt || k.Kind == types.CFloat {
			if v := k.Float64(); math.Abs(v) <= math.MaxFloat32 {
				return types.FloatConst(float64(float32(v)))
			}
		}
	case types.KString:
		switch k.Kind {
		case types.CInt:
			return types.StringConst(strconv.FormatInt(k.Int, 10))
		case types.CFloat:
			return types.StringConst(strconv.FormatFloat(k.Float, 'g', -1, 32))
		case types.CBool:
			return types.StringConst(strconv.FormatBool(k.Bool))
		case types.CString:
			return k
		}
	}
	return nil
}
