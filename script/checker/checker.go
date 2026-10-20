package checker

import (
	"aego/script/abi"
	"aego/script/ast"
	"aego/script/diag"
	"aego/script/resolver"
	"aego/script/source"
	"aego/script/types"
)

type Options struct {
	Module string
}

type mode uint8

const (
	mInvalid mode = iota
	mValue
	mVoid
	mType
	mFunc
	mSignal
)

type operand struct {
	mode mode
	t    *types.Type
	c    *types.Const
	addr bool
	sig  *ScriptSignal
	hsig *HostSignal
	src  ast.Expr
}

func (x operand) signalName() string {
	switch {
	case x.hsig != nil:
		return x.hsig.Name
	case x.sig != nil:
		return x.sig.Name
	}
	return "signal"
}

var invalid = operand{mode: mInvalid, t: types.Invalid}

func val(t *types.Type) operand {
	return operand{mode: mValue, t: t}
}

func result(t *types.Type) operand {
	if t == types.Void {
		return operand{mode: mVoid, t: types.Void}
	}
	if t == types.Invalid {
		return invalid
	}
	return val(t)
}

type state struct {
	pkg    *Package
	rp     *resolver.Package
	scope  *resolver.Scope
	fn     *Func
	loops  int
	breaks int
}

type checker struct {
	a       *Analysis
	surface abi.Surface
	tab     *resolver.Table
	u       *types.Interner
	consts  map[*resolver.Object]uint8
	state
}

func Check(sm *source.Manager, files map[string][]*ast.File, surface abi.Surface, opt Options) *Analysis {
	if surface == nil {
		surface = abi.NewTable()
	}
	tab, ds := resolver.Resolve(sm, files, surface)
	a := newAnalysis(tab, surface)
	a.Diagnostics = append(a.Diagnostics, ds...)
	c := &checker{a: a, surface: surface, tab: tab, u: a.Universe, consts: map[*resolver.Object]uint8{}}
	for _, p := range tab.Order {
		c.collect(p)
	}
	for _, p := range tab.Order {
		c.bodies(p)
	}
	c.unusedImports()
	diag.Sort(a.Diagnostics)
	return a
}

func (c *checker) errorf(code string, sp source.Span, format string, args ...any) {
	c.a.Diagnostics = append(c.a.Diagnostics, diag.Errorf(code, sp, format, args...))
}

func (c *checker) lookup(name string) *resolver.Object {
	return c.scope.Lookup(name)
}

func (c *checker) use(id *ast.Ident, obj *resolver.Object) {
	obj.Used = true
	c.a.Uses[id] = obj
	c.a.References = append(c.a.References, Reference{Ident: id, Object: obj})
}

func (c *checker) def(id *ast.Ident, obj *resolver.Object) {
	c.a.Defs[id] = obj
	c.a.References = append(c.a.References, Reference{Ident: id, Object: obj, Def: true})
}

func (c *checker) enter(p string) {
	c.rp = c.tab.Packages[p]
	c.pkg = c.a.Packages[p]
	if c.pkg == nil {
		c.pkg = &Package{Path: p, Res: c.rp, Imports: c.rp.Deps}
		c.a.Packages[p] = c.pkg
	}
}

func (c *checker) decls(fn func(d ast.Decl)) {
	for _, f := range c.rp.Files {
		for _, d := range f.Decls {
			c.scope = c.rp.FileScopes[f]
			fn(d)
		}
	}
}

func (c *checker) declObj(name *ast.Ident, d ast.Node) *resolver.Object {
	obj := c.rp.Scope.LookupLocal(name.Name)
	if obj == nil || obj.Decl != d {
		return nil
	}
	return obj
}

func (c *checker) collect(p string) {
	c.enter(p)
	for _, o := range c.rp.Scope.Objects() {
		if o.Ident != nil {
			c.def(o.Ident, o)
		}
	}
	c.decls(func(d ast.Decl) {
		switch d := d.(type) {
		case *ast.StructDecl:
			c.structFields(d)
		case *ast.EnumDecl:
			if obj := c.declObj(d.Name, d); obj != nil {
				c.pkg.Enums = append(c.pkg.Enums, obj.Type)
			}
		}
	})
	c.decls(func(d ast.Decl) {
		if sd, ok := d.(*ast.StructDecl); ok {
			if obj := c.declObj(sd.Name, sd); obj != nil && !obj.Type.Layout() {
				c.errorf(diag.RecursiveStruct, sd.Name.Sp, "struct %s contains itself by value", obj.Name)
			}
		}
	})
	c.decls(func(d ast.Decl) {
		switch d := d.(type) {
		case *ast.ConstDecl:
			if obj := c.declObj(d.Name, d); obj != nil {
				c.constObj(obj)
			}
		case *ast.FuncDecl:
			if obj := c.declObj(d.Name, d); obj != nil {
				f := c.signature(d, nil)
				obj.Data = f
				c.pkg.Funcs = append(c.pkg.Funcs, f)
			}
		}
	})
	c.decls(func(d ast.Decl) {
		if sd, ok := d.(*ast.ScriptDecl); ok {
			if obj := c.declObj(sd.Name, sd); obj != nil {
				c.scriptHeader(obj, sd)
			}
		}
	})
}

func (c *checker) structFields(d *ast.StructDecl) {
	obj := c.declObj(d.Name, d)
	if obj == nil {
		return
	}
	t := obj.Type
	seen := map[string]bool{}
	for _, f := range d.Fields {
		ft := c.typeExpr(f.Type)
		if seen[f.Name.Name] {
			c.errorf(diag.DuplicateDecl, f.Name.Sp, "field %s is declared twice", f.Name.Name)
			continue
		}
		seen[f.Name.Name] = true
		t.Fields = append(t.Fields, &types.Field{Name: f.Name.Name, Type: ft, Lane: -1})
		c.def(f.Name, &resolver.Object{Kind: resolver.ObjField, Name: f.Name.Name, Pkg: c.rp.Path, Ident: f.Name, Decl: f, Type: ft})
	}
	c.pkg.Structs = append(c.pkg.Structs, t)
}

func constType(t *types.Type) bool {
	switch t.Kind {
	case types.KBool, types.KInt, types.KFloat, types.KString, types.KEnum, types.KUntypedInt, types.KUntypedFloat:
		return true
	}
	return false
}

func (c *checker) constObj(obj *resolver.Object) {
	switch c.consts[obj] {
	case 2:
		return
	case 1:
		c.errorf(diag.ConstCycle, obj.Span(), "initialization cycle: constant %s refers to itself", obj.Name)
		obj.Type = types.Invalid
		c.consts[obj] = 2
		return
	}
	c.consts[obj] = 1
	saved := c.state
	c.enter(obj.Pkg)
	c.scope = c.rp.FileScopes[obj.File]
	c.fn = nil
	d := obj.Decl.(*ast.ConstDecl)
	var want *types.Type
	if d.Type != nil {
		want = c.typeExpr(d.Type)
	}
	x := c.value(d.Value)
	t, v := types.Invalid, (*types.Const)(nil)
	switch {
	case x.mode == mInvalid || want == types.Invalid:
	case x.c == nil:
		c.errorf(diag.NotConstant, d.Value.Span(), "value of constant %s is not a constant expression", obj.Name)
	case want != nil:
		if c.assign(d.Value, x, want, "constant declaration") {
			t, v = want, c.a.Types[d.Value].Const
		}
	default:
		t, v = x.t, x.c
	}
	if t != types.Invalid && !constType(t) {
		c.errorf(diag.BadConstType, d.Value.Span(), "constants of type %s are not allowed", t)
		t, v = types.Invalid, nil
	}
	if c.consts[obj] == 1 {
		obj.Type, obj.Const = t, v
		c.consts[obj] = 2
		c.a.Packages[obj.Pkg].Consts = append(c.a.Packages[obj.Pkg].Consts, obj)
	}
	c.state = saved
}

func (c *checker) signature(d *ast.FuncDecl, st *ScriptType) *Func {
	if len(d.Attrs) > 0 {
		c.errorf(diag.AttrTarget, d.Attrs[0].Sp, "attributes on functions are not supported yet")
	}
	if d.Coroutine {
		c.a.Coroutines[d] = true
		if st == nil || d.Result != nil {
			c.errorf(diag.CoroutineDecl, d.Name.Sp, "coroutine %s must be a script method without a result", d.Name.Name)
		}
	}
	f := &Func{Name: d.Name.Name, Pkg: c.rp.Path, Decl: d, Script: st, Result: types.Void}
	seen := map[string]bool{}
	for _, p := range d.Params {
		t := c.typeExpr(p.Type)
		obj := &resolver.Object{Kind: resolver.ObjParam, Name: p.Name.Name, Pkg: c.rp.Path, Ident: p.Name, Decl: p, Type: t, Index: len(f.Locals)}
		if p.Name.Name != "_" && seen[p.Name.Name] {
			c.errorf(diag.Redeclared, p.Name.Sp, "parameter %s is declared twice", p.Name.Name)
		}
		seen[p.Name.Name] = true
		f.Params = append(f.Params, obj)
		f.Locals = append(f.Locals, obj)
		c.def(p.Name, obj)
	}
	if d.Result != nil {
		f.Result = c.typeExpr(d.Result)
	}
	return f
}

func (c *checker) insert(s *resolver.Scope, o *resolver.Object) bool {
	if prev := s.Insert(o); prev != nil {
		c.a.Diagnostics = append(c.a.Diagnostics, diag.Errorf(diag.DuplicateDecl, o.Span(), "%s redeclared in this script", o.Name).
			WithNote(prev.Span(), "previous declaration"))
		return false
	}
	return true
}

func (c *checker) scriptHeader(obj *resolver.Object, d *ast.ScriptDecl) {
	st := &ScriptType{
		Name:     d.Name.Name,
		Pkg:      c.rp.Path,
		FullName: scriptFullName(c.rp.Path, d.Name.Name),
		Methods:  map[string]*Func{},
		Decl:     d,
	}
	obj.Data = st
	c.pkg.Scripts = append(c.pkg.Scripts, st)
	c.a.Scripts = append(c.a.Scripts, st)
	c.a.scripts[st.FullName] = st
	c.scriptAttrs(st, d.Attrs)
	st.scope = resolver.NewScope(c.scope)
	st.scope.Insert(&resolver.Object{Kind: resolver.ObjNode, Name: "node", Pkg: c.rp.Path, Type: types.NodeRef})
	for _, sd := range d.Signals {
		sg := &ScriptSignal{Name: sd.Name.Name, Script: st, Decl: sd}
		for _, p := range sd.Params {
			sg.Params = append(sg.Params, c.typeExpr(p.Type))
			sg.Names = append(sg.Names, p.Name.Name)
		}
		so := &resolver.Object{Kind: resolver.ObjSignal, Name: sg.Name, Pkg: c.rp.Path, Ident: sd.Name, Decl: sd, Data: sg}
		if c.insert(st.scope, so) {
			sg.Index = len(st.Signals)
			st.Signals = append(st.Signals, sg)
		}
		c.def(sd.Name, so)
	}
	for i, v := range d.Vars {
		t := c.typeExpr(v.Type)
		f := &ScriptField{Name: v.Name.Name, Type: t, Index: i, Slot: st.Slots, Decl: v}
		st.Slots += t.Slots()
		c.fieldAttrs(f, v.Attrs)
		if v.Value != nil {
			c.fieldDefault(f, v)
		}
		st.Fields = append(st.Fields, f)
		fo := &resolver.Object{Kind: resolver.ObjField, Name: f.Name, Pkg: c.rp.Path, Ident: v.Name, Decl: v, Type: t, Index: i, Data: f}
		c.insert(st.scope, fo)
		c.def(v.Name, fo)
	}
	for _, fd := range d.Funcs {
		f := c.signature(fd, st)
		fo := &resolver.Object{Kind: resolver.ObjFunc, Name: f.Name, Pkg: c.rp.Path, Ident: fd.Name, Decl: fd, Data: f}
		if c.insert(st.scope, fo) {
			st.Methods[f.Name] = f
		}
		st.Funcs = append(st.Funcs, f)
		c.def(fd.Name, fo)
		c.lifecycle(st, f)
	}
}

func (c *checker) fieldDefault(f *ScriptField, v *ast.ScriptVar) {
	x := c.value(v.Value)
	switch {
	case x.mode == mInvalid:
	case x.t == types.Nil:
		c.assign(v.Value, x, f.Type, "field initializer")
	case x.c == nil && x.t == f.Type && f.Type.IsVector():
		if k, ok := c.laneConst(v.Value, f.Type); ok {
			f.Default = k
			return
		}
		c.errorf(diag.FieldInit, v.Value.Span(), "initial value of %s must be a constant, every component of the %s literal must be a constant", f.Name, f.Type)
	case x.c == nil:
		c.errorf(diag.FieldInit, v.Value.Span(), "initial value of %s must be a constant", f.Name)
	case c.assign(v.Value, x, f.Type, "field initializer"):
		f.Default = c.a.Types[v.Value].Const
	}
}

func (c *checker) laneConst(e ast.Expr, t *types.Type) (*types.Const, bool) {
	lit, ok := unparen(e).(*ast.CompositeLit)
	if !ok || c.a.Composites[lit] != t || len(t.Fields) > 4 {
		return nil, false
	}
	var lanes [4]float64
	for i, el := range lit.Elems {
		idx := i
		if el.Key != nil {
			idx = -1
			for j, f := range t.Fields {
				if f.Name == el.Key.Name {
					idx = j
				}
			}
		}
		k := c.a.Types[el.Value].Const
		if idx < 0 || idx >= len(t.Fields) || k == nil || (k.Kind != types.CInt && k.Kind != types.CFloat) {
			return nil, false
		}
		lanes[idx] = float64(float32(k.Float64()))
	}
	return types.LanesConst(lanes, len(t.Fields)), true
}

func (c *checker) lifecycle(st *ScriptType, f *Func) {
	want := 0
	switch f.Name {
	case "Ready":
		st.Ready = f
	case "Reloaded":
		st.Reloaded = f
	case "FixedUpdate":
		want = 1
		st.FixedUpdate = f
	case "Update":
		want = 1
		st.Update = f
	default:
		c.event(st, f)
		return
	}
	if f.Decl.Coroutine {
		c.errorf(diag.CoroutineLifecycle, f.Decl.Name.Sp, "lifecycle method %s cannot be a coroutine, start one from it instead", f.Name)
	}
	ok := len(f.Params) == want && f.Result == types.Void && (want == 0 || f.Params[0].Type == types.Float)
	if !ok {
		sig := "()"
		if want == 1 {
			sig = "(dt float)"
		}
		c.errorf(diag.Lifecycle, f.Decl.Name.Sp, "%s must have the signature func %s%s", f.Name, f.Name, sig)
	}
}

func (c *checker) event(st *ScriptType, f *Func) {
	sig, ok := c.surface.Event(f.Name)
	if !ok {
		return
	}
	st.Events = append(st.Events, f)
	if f.Decl.Coroutine {
		c.errorf(diag.CoroutineLifecycle, f.Decl.Name.Sp, "event handler %s cannot be a coroutine, start one from it instead", f.Name)
	}
	want := c.abiTypes(sig.Params)
	ok = len(f.Params) == len(want) && f.Result == types.Void
	for i := 0; ok && i < len(want); i++ {
		ok = f.Params[i].Type == want[i]
	}
	if !ok {
		c.errorf(diag.EventSignature, f.Decl.Name.Sp, "%s handles the engine event %s and must have the signature func %s%s", f.Name, f.Name, f.Name, sig)
	}
}

func (c *checker) hostType(target, name string) *types.Type {
	if e, ok := c.surface.Enum(target, name); ok {
		return c.u.HostEnum(e)
	}
	if sd, ok := c.surface.Struct(target, name); ok {
		return c.u.HostStruct(sd, c.surface)
	}
	if h, ok := c.surface.Handle(target + "." + name); ok {
		return c.u.Handle(h)
	}
	return nil
}

func (c *checker) bodies(p string) {
	if rp := c.tab.Packages[p]; rp != nil && rp.Cyclic {
		return
	}
	c.enter(p)
	for _, f := range c.rp.Files {
		fs := c.rp.FileScopes[f]
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if obj := c.declObj(d.Name, d); obj != nil {
					if fn, ok := obj.Data.(*Func); ok {
						c.body(fn, fs)
					}
				}
			case *ast.ScriptDecl:
				if obj := c.declObj(d.Name, d); obj != nil {
					if st, ok := obj.Data.(*ScriptType); ok {
						for _, fn := range st.Funcs {
							c.body(fn, st.scope)
						}
					}
				}
			}
		}
	}
}

func (c *checker) body(f *Func, parent *resolver.Scope) {
	c.fn = f
	c.scope = resolver.NewScope(parent)
	c.loops, c.breaks = 0, 0
	for _, p := range f.Params {
		if p.Name != "_" {
			c.scope.Insert(p)
		}
	}
	if f.Decl.Body != nil {
		c.stmts(f.Decl.Body.Stmts)
		if f.Result != types.Void && f.Result != types.Invalid && !terminating(f.Decl.Body.Stmts) {
			sp := f.Decl.Body.Sp
			if sp.End > sp.Start {
				sp.Start = sp.End - 1
			}
			c.errorf(diag.MissingReturn, sp, "missing return at the end of %s", f.Name)
		}
	}
	for _, o := range f.Locals {
		if o.Kind == resolver.ObjVar && !o.Used {
			c.a.Diagnostics = append(c.a.Diagnostics, diag.Warnf(diag.UnusedVar, o.Span(), "%s declared and not used", o.Name))
		}
	}
	c.fn = nil
}

func (c *checker) unusedImports() {
	for _, p := range c.tab.Order {
		rp := c.tab.Packages[p]
		for _, f := range rp.Files {
			for _, o := range rp.FileScopes[f].Objects() {
				if o.Kind == resolver.ObjPkg && !o.Used && o.Target != "" {
					c.a.Diagnostics = append(c.a.Diagnostics, diag.Warnf(diag.UnusedImport, o.Span(), "%q imported and not used", o.Target))
				}
			}
		}
	}
}
