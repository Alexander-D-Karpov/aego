package langsvc

import (
	"strings"

	"aego/script/abi"
	"aego/script/ast"
	"aego/script/checker"
	"aego/script/resolver"
	"aego/script/types"
	"aego/script/workspace"
)

func paramsText(sg *checker.ScriptSignal) string {
	parts := make([]string, len(sg.Params))
	for i, p := range sg.Params {
		parts[i] = sg.Names[i] + " " + typeString(p)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func signalDecl(sg *checker.ScriptSignal) string {
	return "signal " + sg.Name + paramsText(sg)
}

func lastElem(pkg string) string {
	return pkg[strings.LastIndexByte(pkg, '/')+1:]
}

func shortScript(full string) string {
	return full[strings.LastIndexByte(full, '.')+1:]
}

func signalOf(a *checker.Analysis, x ast.Expr) *checker.ScriptSignal {
	switch e := x.(type) {
	case *ast.Ident:
		if o := object(a, e); o != nil && o.Kind == resolver.ObjSignal {
			sg, _ := o.Data.(*checker.ScriptSignal)
			return sg
		}
	case *ast.SelectorExpr:
		if sel := a.Selections[e]; sel != nil && sel.Kind == checker.SelSignal {
			return sel.Signal
		}
	}
	return nil
}

func scriptAt(a *checker.Analysis, sd *ast.ScriptDecl) *checker.ScriptType {
	if sd == nil {
		return nil
	}
	for _, st := range a.Scripts {
		if st.Decl == sd {
			return st
		}
	}
	return nil
}

func sameParams(f *checker.Func, sg *checker.ScriptSignal) bool {
	if len(f.Params) != len(sg.Params) {
		return false
	}
	for i, p := range f.Params {
		if p.Type != sg.Params[i] {
			return false
		}
	}
	return true
}

func handlerCandidate(st *checker.ScriptType, f *checker.Func) bool {
	return !f.Decl.Coroutine && !st.IsLifecycle(f) && f.Result == types.Void
}

func startItems(c *collector, snap *workspace.Snapshot, f *ast.File, off uint32) {
	_, sd := enclosing(f, off)
	st := scriptAt(snap.Analysis, sd)
	if st == nil {
		return
	}
	for _, fn := range st.Funcs {
		if fn.Decl.Coroutine {
			c.add(Item{Label: fn.Name, Detail: funcSig(fn), Doc: fn.Decl.Doc, Kind: ItemMethod, Insert: fn.Name + "("})
		}
	}
}

func yieldItems(c *collector, snap *workspace.Snapshot) {
	for _, y := range []Item{
		{Label: "wait", Detail: "yield wait(seconds float)", Insert: "wait(${1:seconds})"},
		{Label: "waitUnscaled", Detail: "yield waitUnscaled(seconds float)", Insert: "waitUnscaled(${1:seconds})"},
		{Label: "frames", Detail: "yield frames(n int)", Insert: "frames(${1:n})"},
		{Label: "fixed", Detail: "yield fixed(n int)", Insert: "fixed(${1:n})"},
	} {
		y.Kind = ItemSnippet
		y.Doc = builtinText(snap, "yield "+y.Label)
		c.add(y)
	}
}

func signalMembers(c *collector, snap *workspace.Snapshot, sg *checker.ScriptSignal) {
	c.add(Item{Label: "Emit", Detail: "Emit" + paramsText(sg), Doc: builtinText(snap, "Emit"), Kind: ItemMethod, Insert: "Emit("})
	c.add(Item{Label: "Connect", Detail: "Connect(handler func" + paramsText(sg) + ") Connection", Doc: builtinText(snap, "Connect"), Kind: ItemMethod, Insert: "Connect("})
}

func scriptRefMembers(c *collector, snap *workspace.Snapshot, t *types.Type, pkg string) {
	st := snap.Analysis.ScriptByName(t.Script)
	if st == nil {
		return
	}
	visible := func(name string) bool { return st.Pkg == pkg || abi.Exported(name) }
	for _, f := range st.Fields {
		if visible(f.Name) {
			c.add(Item{Label: f.Name, Detail: scriptField(f), Doc: f.Decl.Doc, Kind: ItemField})
		}
	}
	for _, sg := range st.Signals {
		if visible(sg.Name) {
			c.add(Item{Label: sg.Name, Detail: signalDecl(sg), Doc: sg.Decl.Doc, Kind: ItemSignal})
		}
	}
	for _, fn := range st.Funcs {
		if visible(fn.Name) && !st.IsLifecycle(fn) && !fn.Decl.Coroutine {
			c.add(Item{Label: fn.Name, Detail: funcSig(fn), Doc: fn.Decl.Doc, Kind: ItemMethod, Insert: fn.Name + "("})
		}
	}
}

func handlerItems(c *collector, snap *workspace.Snapshot, f *ast.File, src []byte, dot, off uint32) {
	end := dot
	for end > 0 && (src[end-1] == ' ' || src[end-1] == '\t') {
		end--
	}
	a := snap.Analysis
	x := exprEndingAt(f, end)
	if x == nil {
		return
	}
	sg := signalOf(a, x)
	_, sd := enclosing(f, off)
	st := scriptAt(a, sd)
	if sg == nil || st == nil {
		return
	}
	for _, fn := range st.Funcs {
		if !handlerCandidate(st, fn) {
			continue
		}
		it := Item{Label: fn.Name, Detail: funcSig(fn), Doc: fn.Decl.Doc, Kind: ItemMethod}
		c.rank = rankLocal
		if !sameParams(fn, sg) {
			c.rank = rankScript
			it.Detail += "   · does not match " + signalDecl(sg)
		}
		c.add(it)
	}
}

func signalNamed(a *checker.Analysis, pkg, name string) *checker.ScriptSignal {
	var found *checker.ScriptSignal
	for _, st := range a.Scripts {
		sg := st.Signal(name)
		if sg == nil {
			continue
		}
		if st.Pkg == pkg {
			return sg
		}
		if found == nil {
			found = sg
		}
	}
	return found
}

func scriptMethodNamed(a *checker.Analysis, pkg, name string) *checker.Func {
	var found *checker.Func
	for _, st := range a.Scripts {
		f := st.Methods[name]
		if f == nil || st.IsLifecycle(f) || f.Decl.Coroutine {
			continue
		}
		if st.Pkg == pkg {
			return f
		}
		if found == nil {
			found = f
		}
	}
	return found
}

func signalSignature(snap *workspace.Snapshot, sg *checker.ScriptSignal, method string) Signature {
	var sb sigBuilder
	sb.b.WriteString("func (signal " + sg.Name + ") " + method + "(")
	if method == "Emit" {
		for i, p := range sg.Params {
			sb.param(sg.Names[i] + " " + typeString(p))
		}
		sb.b.WriteByte(')')
	} else {
		sb.param("handler func" + paramsText(sg))
		sb.b.WriteString(") Connection")
	}
	return Signature{Label: sb.b.String(), Params: sb.params, Doc: builtinText(snap, method)}
}

func asyncCallHover(snap *workspace.Snapshot, k *checker.Call) (Hover, bool) {
	switch k.Kind {
	case checker.CallEmit:
		return Hover{Signature: "func (signal " + k.Signal.Name + ") Emit" + paramsText(k.Signal), Doc: builtinText(snap, "Emit")}, true
	case checker.CallConnect:
		return Hover{Signature: "func (signal " + k.Signal.Name + ") Connect(handler func" + paramsText(k.Signal) + ") Connection", Doc: builtinText(snap, "Connect")}, true
	case checker.CallDisconnect:
		return Hover{Signature: "func (Connection) Disconnect()", Doc: builtinText(snap, "Disconnect")}, true
	case checker.CallConnected:
		return Hover{Signature: "func (Connection) Connected() bool", Doc: builtinText(snap, "Connected")}, true
	case checker.CallCoStop:
		return Hover{Signature: "func (Coroutine) Stop()", Doc: builtinText(snap, "Stop")}, true
	case checker.CallCoRunning:
		return Hover{Signature: "func (Coroutine) Running() bool", Doc: builtinText(snap, "Running")}, true
	case checker.CallGetScript:
		s := shortScript(k.Target)
		return Hover{Signature: "func (NodeRef) GetScript<" + s + ">() ScriptRef<" + s + ">", Doc: builtinText(snap, "GetScript")}, true
	}
	return Hover{}, false
}
