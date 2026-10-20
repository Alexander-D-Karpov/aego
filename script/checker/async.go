package checker

import (
	"strings"

	"aego/script/abi"
	"aego/script/ast"
	"aego/script/diag"
	"aego/script/resolver"
	"aego/script/source"
	"aego/script/types"
)

var yieldNames = [...]string{"", "wait", "frames", "fixed", "await", "waitUnscaled"}

func scriptFullName(pkg, name string) string {
	return strings.ReplaceAll(pkg, "/", ".") + "." + name
}

func (c *checker) visible(st *ScriptType, name string) bool {
	return st.Pkg == c.rp.Path || abi.Exported(name)
}

func funcParams(f *Func) []*types.Type {
	out := make([]*types.Type, len(f.Params))
	for i, p := range f.Params {
		out[i] = p.Type
	}
	return out
}

func typesText(ts []*types.Type) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = t.String()
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func sameTypes(f *Func, params []*types.Type) bool {
	if len(f.Params) != len(params) {
		return false
	}
	for i, p := range f.Params {
		if p.Type != params[i] {
			return false
		}
	}
	return true
}

func (c *checker) hostSignal(s *ast.SelectorExpr, i int, def abi.SignalDef, comp *abi.Component, handle string) operand {
	hs := &HostSignal{Name: def.Name, Index: i, Params: c.abiTypes(def.Params), Comp: comp, Handle: handle}
	return operand{mode: mSignal, t: types.Void, hsig: hs, src: s.X}
}

func (c *checker) start(e *ast.StartExpr) operand {
	obj := c.lookup(e.Name.Name)
	if obj == nil {
		c.errorf(diag.UndefinedName, e.Name.Sp, "undefined: %s", e.Name.Name)
		c.args(e.Args)
		return invalid
	}
	c.use(e.Name, obj)
	f, _ := obj.Data.(*Func)
	if obj.Kind != resolver.ObjFunc || f == nil || !f.Decl.Coroutine || f.Script == nil || c.fn == nil || f.Script != c.fn.Script {
		c.errorf(diag.StartTarget, e.Name.Sp, "start needs a coroutine method of this script, %s is not one", e.Name.Name)
		c.args(e.Args)
		return invalid
	}
	c.argList(e.Sp, e.Args, f.Name, funcParams(f))
	c.a.Starts[e] = f
	return val(types.Coroutine)
}

func (c *checker) callSignal(e *ast.CallExpr, sel *ast.SelectorExpr, x operand) operand {
	if x.hsig != nil {
		return c.callHostSignal(e, sel, x)
	}
	sg := x.sig
	c.noTypeArgs(e)
	switch sel.Sel.Name {
	case "Emit":
		if x.src != nil {
			c.errorf(diag.BadConnect, sel.Sel.Sp, "signal %s can be emitted only by its own script", sg.Name)
			c.args(e.Args)
			return invalid
		}
		c.argList(e.Sp, e.Args, sg.Name+".Emit", sg.Params)
		c.a.Calls[e] = &Call{Kind: CallEmit, Signal: sg}
		return operand{mode: mVoid, t: types.Void}
	case "Connect":
		if len(e.Args) != 1 {
			c.errorf(diag.ArgCount, e.Sp, "Connect takes exactly one handler")
			c.args(e.Args)
			return invalid
		}
		sp := sg.Decl.Name.Sp
		h, recv, ok := c.handler(e.Args[0], sg.Name, sg.Params, &sp)
		if !ok {
			return invalid
		}
		c.a.Calls[e] = &Call{Kind: CallConnect, Signal: sg, Recv: x.src, Handler: h, HandlerRecv: recv}
		return val(types.Connection)
	}
	c.errorf(diag.UnknownField, sel.Sel.Sp, "signal %s has no method %s, use Emit or Connect", sg.Name, sel.Sel.Name)
	c.args(e.Args)
	return invalid
}

func (c *checker) callHostSignal(e *ast.CallExpr, sel *ast.SelectorExpr, x operand) operand {
	hs := x.hsig
	c.noTypeArgs(e)
	switch sel.Sel.Name {
	case "Emit":
		c.errorf(diag.BadConnect, sel.Sel.Sp, "signal %s is emitted by the engine, scripts can only connect to it", hs.Name)
	case "Connect":
		if len(e.Args) != 1 {
			c.errorf(diag.ArgCount, e.Sp, "Connect takes exactly one handler")
			break
		}
		h, recv, ok := c.handler(e.Args[0], hs.Name, hs.Params, nil)
		if !ok {
			return invalid
		}
		c.a.Calls[e] = &Call{Kind: CallHostConnect, Host: hs, Recv: x.src, Handler: h, HandlerRecv: recv}
		return val(types.Connection)
	default:
		c.errorf(diag.UnknownField, sel.Sel.Sp, "signal %s has no method %s, use Connect", hs.Name, sel.Sel.Name)
	}
	c.args(e.Args)
	return invalid
}

func (c *checker) handler(arg ast.Expr, name string, params []*types.Type, decl *source.Span) (*Func, ast.Expr, bool) {
	var f *Func
	var recv ast.Expr
	switch a := unparen(arg).(type) {
	case *ast.Ident:
		if o := c.lookup(a.Name); o != nil && o.Kind == resolver.ObjFunc {
			if fn, ok := o.Data.(*Func); ok && fn.Script != nil && c.fn != nil && fn.Script == c.fn.Script {
				c.use(a, o)
				f = fn
			}
		}
	case *ast.SelectorExpr:
		x := c.expr(a.X)
		if x.mode == mValue && x.t.Kind == types.KScriptRef {
			if st := c.a.scripts[x.t.Script]; st != nil && c.visible(st, a.Sel.Name) {
				if fn := st.Methods[a.Sel.Name]; fn != nil {
					if o := st.scope.LookupLocal(fn.Name); o != nil {
						c.use(a.Sel, o)
					}
					f, recv = fn, a.X
				}
			}
		}
	}
	if f == nil {
		c.errorf(diag.BadConnect, arg.Span(), "Connect needs a method name of this script or ref.Method")
		return nil, nil, false
	}
	if f.Decl.Coroutine || f.Script.IsLifecycle(f) {
		c.errorf(diag.BadConnect, arg.Span(), "%s cannot handle a signal, use a plain method", f.Name)
		return nil, nil, false
	}
	if !sameTypes(f, params) || f.Result != types.Void {
		d := diag.Errorf(diag.HandlerMismatch, arg.Span(), "handler %s%s does not match signal %s%s",
			f.Name, typesText(funcParams(f)), name, typesText(params))
		if decl != nil {
			d = d.WithNote(*decl, "signal %s%s", name, typesText(params))
		}
		c.a.Diagnostics = append(c.a.Diagnostics, d)
		return nil, nil, false
	}
	return f, recv, true
}

func (c *checker) callScriptMethod(e *ast.CallExpr, sel *ast.SelectorExpr, recv operand) operand {
	name := sel.Sel.Name
	st := c.a.scripts[recv.t.Script]
	var f *Func
	if st != nil {
		f = st.Methods[name]
	}
	switch {
	case f == nil:
		c.errorf(diag.UnknownField, sel.Sel.Sp, "%s has no method %s", recv.t, name)
	case !c.visible(st, name):
		c.errorf(diag.NotExported, sel.Sel.Sp, "%s.%s is not exported", st.Name, name)
	case st.IsLifecycle(f) || f.Decl.Coroutine:
		c.errorf(diag.ScriptRefCall, sel.Sel.Sp, "%s.%s cannot be called through a ScriptRef, lifecycle methods and coroutines run only on their own instance", st.Name, name)
	default:
		if o := st.scope.LookupLocal(name); o != nil {
			c.use(sel.Sel, o)
		}
		c.noTypeArgs(e)
		c.arguments(e, name, funcParams(f))
		c.a.Calls[e] = &Call{Kind: CallScriptMethod, Func: f, Recv: sel.X}
		return result(f.Result)
	}
	c.args(e.Args)
	return invalid
}

func (c *checker) callHandle(e *ast.CallExpr, sel *ast.SelectorExpr, recv operand) operand {
	c.noTypeArgs(e)
	name := sel.Sel.Name
	kind, res := CallCoStop, types.Void
	switch {
	case recv.t.Kind == types.KCoroutine && name == "Stop":
	case recv.t.Kind == types.KCoroutine && name == "Running":
		kind, res = CallCoRunning, types.Bool
	case recv.t.Kind == types.KConnection && name == "Disconnect":
		kind = CallDisconnect
	case recv.t.Kind == types.KConnection && name == "Connected":
		kind, res = CallConnected, types.Bool
	default:
		c.errorf(diag.UnknownField, sel.Sel.Sp, "%s has no method %s", recv.t, name)
		c.args(e.Args)
		return invalid
	}
	if len(e.Args) != 0 {
		c.errorf(diag.ArgCount, e.Sp, "%s takes no arguments", name)
		c.args(e.Args)
	}
	c.a.Calls[e] = &Call{Kind: kind, Recv: sel.X}
	return result(res)
}

func (c *checker) getScript(e *ast.CallExpr, sel *ast.SelectorExpr) operand {
	if len(e.TypeArgs) != 1 {
		c.errorf(diag.BadTypeArg, e.Sp, "GetScript needs one script type argument, like GetScript<Enemy>()")
		c.args(e.Args)
		return invalid
	}
	if len(e.Args) != 0 {
		c.errorf(diag.ArgCount, e.Sp, "GetScript takes no arguments")
		c.args(e.Args)
	}
	nt, ok := e.TypeArgs[0].(*ast.NamedType)
	if !ok {
		c.errorf(diag.ScriptRefTarget, e.TypeArgs[0].Span(), "GetScript argument must be a script type")
		return invalid
	}
	full, ok := c.scriptTarget(nt)
	if !ok {
		return invalid
	}
	c.a.Calls[e] = &Call{Kind: CallGetScript, Target: full, Recv: sel.X}
	return val(c.u.ScriptRef(full))
}

func (c *checker) scriptMember(s *ast.SelectorExpr, st *ScriptType) operand {
	name := s.Sel.Name
	o := st.scope.LookupLocal(name)
	if o == nil || o.Kind == resolver.ObjNode {
		c.errorf(diag.UnknownField, s.Sel.Sp, "%s has no field, signal or method %s", st.Name, name)
		return invalid
	}
	if !c.visible(st, name) {
		c.errorf(diag.NotExported, s.Sel.Sp, "%s.%s is not exported", st.Name, name)
		return invalid
	}
	c.use(s.Sel, o)
	switch o.Kind {
	case resolver.ObjField:
		f := o.Data.(*ScriptField)
		c.a.Selections[s] = &Selection{Kind: SelScriptField, Script: st, ScriptField: f}
		if f.Type == types.Invalid {
			return invalid
		}
		return operand{mode: mValue, t: f.Type, addr: true}
	case resolver.ObjSignal:
		sg := o.Data.(*ScriptSignal)
		c.a.Selections[s] = &Selection{Kind: SelSignal, Script: st, Signal: sg}
		return operand{mode: mSignal, t: types.Void, sig: sg, src: s.X}
	}
	c.errorf(diag.NotValue, s.Sp, "method %s must be called", name)
	return invalid
}

func (c *checker) scriptTarget(arg *ast.NamedType) (string, bool) {
	if len(arg.Args) > 0 {
		c.errorf(diag.ScriptRefTarget, arg.Args[0].Span(), "%s is not generic", arg.Name.Name)
		return "", false
	}
	var obj *resolver.Object
	if arg.Pkg != nil {
		p := c.lookup(arg.Pkg.Name)
		if p == nil || p.Kind != resolver.ObjPkg || p.Host || p.Target == "" {
			c.errorf(diag.ScriptRefTarget, arg.Sp, "%s.%s is not a script type", arg.Pkg.Name, arg.Name.Name)
			return "", false
		}
		c.use(arg.Pkg, p)
		if obj = c.member(arg.Name, p); obj == nil {
			return "", false
		}
	} else {
		if obj = c.lookup(arg.Name.Name); obj == nil {
			if obj = c.uniqueScript(arg); obj == nil {
				return "", false
			}
		}
		c.use(arg.Name, obj)
	}
	if obj.Kind != resolver.ObjScript {
		c.errorf(diag.ScriptRefTarget, arg.Sp, "%s is not a script type", arg.Name.Name)
		return "", false
	}
	return scriptFullName(obj.Pkg, obj.Name), true
}

func (c *checker) uniqueScript(arg *ast.NamedType) *resolver.Object {
	name := arg.Name.Name
	var found []*resolver.Object
	for _, p := range append([]string{c.rp.Path}, c.rp.Deps...) {
		rp := c.tab.Packages[p]
		if rp == nil {
			continue
		}
		o := rp.Scope.LookupLocal(name)
		if o == nil || o.Kind != resolver.ObjScript || (p != c.rp.Path && !o.Exported()) {
			continue
		}
		found = append(found, o)
	}
	switch len(found) {
	case 1:
		return found[0]
	case 0:
		c.errorf(diag.ScriptRefTarget, arg.Sp, "unknown script type %s, import the package that declares it", name)
	default:
		names := make([]string, len(found))
		for i, o := range found {
			names[i] = path(o.Pkg) + "." + o.Name
		}
		c.errorf(diag.ScriptRefTarget, arg.Sp, "script name %s is ambiguous, write one of: %s", name, strings.Join(names, ", "))
	}
	return nil
}

func path(pkg string) string {
	return pkg[strings.LastIndexByte(pkg, '/')+1:]
}

func (c *checker) yieldStmt(s *ast.YieldStmt) {
	if !c.fn.Decl.Coroutine {
		c.errorf(diag.YieldOutside, s.Sp, "yield is only allowed in a coroutine, declare it as func %s(...) coroutine", c.fn.Name)
	}
	var want *types.Type
	switch s.Kind {
	case ast.YieldWait, ast.YieldWaitUnscaled:
		want = types.Float
	case ast.YieldFrames, ast.YieldFixed:
		want = types.Int
	case ast.YieldAwait:
		want = types.Coroutine
	default:
		return
	}
	x := c.value(s.Value)
	if x.mode == mInvalid {
		return
	}
	if s.Kind == ast.YieldAwait && x.t.Kind == types.KHandle {
		if _, ok := c.surface.HandleMethod(x.t.Handle, "Done"); !x.t.Await || !ok {
			c.errorf(diag.YieldArg, s.Value.Span(), "%s cannot be awaited", x.t)
		}
		return
	}
	y := x
	if !c.coerce(s.Value, &y, want) {
		c.errorf(diag.YieldArg, s.Value.Span(), "yield %s needs %s, a Coroutine or an awaitable handle, not %s", yieldNames[s.Kind], want, x.t)
	}
}
