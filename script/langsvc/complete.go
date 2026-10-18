package langsvc

import (
	"sort"
	"strings"

	"aego/script/ast"
	"aego/script/checker"
	"aego/script/lexer"
	"aego/script/resolver"
	"aego/script/source"
	"aego/script/token"
	"aego/script/types"
	"aego/script/workspace"
)

var keywords = []string{
	"if", "else", "for", "switch", "case", "default", "return", "break", "continue",
	"var", "const", "func", "type", "struct", "enum", "script", "import", "true", "false", "nil",
	"yield", "signal", "coroutine", "start",
}

const (
	rankLocal = iota
	rankScript
	rankPackage
	rankUniverse
	rankKeyword
)

type collector struct {
	prefix string
	rank   int
	seen   map[string]bool
	items  []Item
}

func matches(label, prefix string) bool {
	return len(prefix) <= len(label) && strings.EqualFold(label[:len(prefix)], prefix)
}

func (c *collector) add(it Item) {
	if c.seen[it.Label] || !matches(it.Label, c.prefix) {
		return
	}
	c.seen[it.Label] = true
	if it.Insert == "" {
		it.Insert = it.Label
	}
	it.rank = c.rank
	c.items = append(c.items, it)
}

func (c *collector) sorted() []Item {
	sort.SliceStable(c.items, func(i, j int) bool {
		a, b := c.items[i], c.items[j]
		ea, eb := strings.HasPrefix(a.Label, c.prefix), strings.HasPrefix(b.Label, c.prefix)
		if ea != eb {
			return ea
		}
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		return a.Label < b.Label
	})
	return c.items
}

func objectItem(snap *workspace.Snapshot, o *resolver.Object) Item {
	it := Item{Label: o.Name, Detail: describe(o), Doc: objectDoc(snap, o)}
	switch o.Kind {
	case resolver.ObjFunc, resolver.ObjBuiltin:
		it.Kind, it.Insert = ItemFunction, o.Name+"("
		if f, ok := o.Data.(*checker.Func); ok && f.Script != nil {
			it.Kind = ItemMethod
		}
	case resolver.ObjVar, resolver.ObjNode:
		it.Kind = ItemVariable
	case resolver.ObjParam:
		it.Kind = ItemParam
	case resolver.ObjField:
		it.Kind = ItemField
	case resolver.ObjConst:
		it.Kind = ItemConstant
	case resolver.ObjType, resolver.ObjGeneric:
		it.Kind = ItemType
		if o.Type != nil && o.Type.Kind == types.KEnum {
			it.Kind = ItemEnum
		}
	case resolver.ObjScript:
		it.Kind = ItemScript
	case resolver.ObjPkg:
		it.Kind = ItemModule
	}
	return it
}

func inComment(cs []lexer.Comment, off uint32) bool {
	for _, c := range cs {
		if c.Start >= off {
			return false
		}
		if off < c.End || (c.Line && off == c.End) {
			return true
		}
	}
	return false
}

func inString(toks []token.Token, src []byte, off uint32) bool {
	for _, t := range toks {
		if t.Start >= off {
			return false
		}
		if t.Kind != token.String {
			continue
		}
		if off < t.End {
			return true
		}
		if off == t.End && (t.End-t.Start < 2 || src[t.End-1] != '"') {
			return true
		}
	}
	return false
}

func exportArgs(toks []token.Token) bool {
	n := len(toks)
	if n == 0 || (toks[n-1].Kind != token.LParen && toks[n-1].Kind != token.Comma) {
		return false
	}
	depth := 0
	for i := n - 1; i >= 0; i-- {
		switch toks[i].Kind {
		case token.RParen:
			depth++
		case token.LParen:
			if depth == 0 {
				return i >= 2 && toks[i-1].Kind == token.Ident && toks[i-1].Text == "export" && toks[i-2].Kind == token.At
			}
			depth--
		}
	}
	return false
}

func (s *Service) complete(snap *workspace.Snapshot, file source.FileID, off uint32) Completion {
	out := Completion{Prefix: source.Span{File: file, Start: off, End: off}}
	src := snap.Sources.Source(file)
	f := snap.Files[file]
	if f == nil || int(off) > len(src) {
		return out
	}
	toks, comments, _ := lexer.TokenizeAll(file, src, source.NewInterner())
	if inComment(comments, off) || inString(toks, src, off) {
		return out
	}
	var before []token.Token
	for _, t := range toks {
		if t.Kind == token.EOF || t.Start >= off {
			break
		}
		if !t.IsAuto() {
			before = append(before, t)
		}
	}
	prefix := ""
	if n := len(before); n > 0 {
		last := before[n-1]
		if last.End == off && (last.Kind == token.Ident || last.Kind.IsKeyword()) {
			prefix = last.Text
			out.Prefix.Start = last.Start
			before = before[:n-1]
		}
	}
	c := &collector{prefix: prefix, seen: map[string]bool{}}
	n := len(before)
	pkg := snap.Packages[file]
	switch {
	case n > 0 && before[n-1].Kind == token.Ident && before[n-1].Text == "start":
		startItems(c, snap, f, off)
	case n > 0 && before[n-1].Kind == token.Yield:
		yieldItems(c, snap)
	case n > 0 && before[n-1].Kind == token.Period:
		s.members(c, snap, f, src, before[n-1].Start, pkg)
	case n > 2 && before[n-1].Kind == token.Lt && before[n-2].Kind == token.Ident && before[n-3].Kind == token.Period:
		typeArgs(c, snap, before[n-2].Text)
	case n > 3 && before[n-1].Kind == token.LParen && before[n-2].Kind == token.Ident && before[n-2].Text == "Connect" && before[n-3].Kind == token.Period:
		handlerItems(c, snap, f, src, before[n-3].Start, off)
	case n > 0 && before[n-1].Kind == token.At:
		for _, a := range []string{"export", "former", "hide", "pause"} {
			c.add(Item{Label: a, Kind: ItemAttribute, Doc: builtinText(snap, "@"+a)})
		}
	case exportArgs(before):
		for _, k := range []string{"min", "max", "step", "unit", "display", "tip", "category"} {
			c.add(Item{Label: k, Insert: k + "=", Kind: ItemProperty})
		}
	default:
		scope(c, snap, f, off)
	}
	out.Items = c.sorted()
	return out
}

func exprEndingAt(f *ast.File, end uint32) ast.Expr {
	var best ast.Expr
	ast.Inspect(f, func(n ast.Node) bool {
		sp := n.Span()
		if sp.Start > end || sp.End < end {
			return false
		}
		if e, ok := n.(ast.Expr); ok && sp.End == end && sp.Start < end {
			if _, bad := e.(*ast.BadExpr); !bad && (best == nil || sp.Start < best.Span().Start) {
				best = e
			}
		}
		return true
	})
	return best
}

func enumItems(c *collector, t *types.Type) {
	for _, m := range t.Members {
		c.add(Item{Label: m, Detail: t.String(), Kind: ItemEnumMember})
	}
}

func (s *Service) members(c *collector, snap *workspace.Snapshot, f *ast.File, src []byte, dot uint32, pkg string) {
	end := dot
	for end > 0 && (src[end-1] == ' ' || src[end-1] == '\t') {
		end--
	}
	x := exprEndingAt(f, end)
	if x == nil {
		return
	}
	a := snap.Analysis
	if sg := signalOf(a, x); sg != nil {
		signalMembers(c, snap, sg)
		return
	}
	if id, ok := x.(*ast.Ident); ok {
		if o := object(a, id); o != nil {
			switch {
			case o.Kind == resolver.ObjPkg:
				packageMembers(c, snap, o)
				return
			case o.Kind == resolver.ObjType && o.Type != nil && o.Type.Kind == types.KEnum:
				enumItems(c, o.Type)
				return
			}
		}
	}
	t := a.TypeOf(x)
	if sel, ok := x.(*ast.SelectorExpr); ok && t.Kind == types.KEnum && a.Selections[sel] == nil {
		enumItems(c, t)
		return
	}
	typeMembers(c, snap, t, pkg)
}

func typeMembers(c *collector, snap *workspace.Snapshot, t *types.Type, pkg string) {
	switch t.Kind {
	case types.KNodeRef, types.KAssetRef:
		kind := types.ToABI(t).Kind
		for _, m := range snap.Bindings.MethodList() {
			if m.Recv != kind {
				continue
			}
			me := methodDoc(snap.Docs, kind, m.Name)
			c.add(Item{Label: m.Name, Detail: hostSig("", m.Sig, me.Params), Doc: hostDoc(me.Doc, me.Example), Kind: ItemMethod, Insert: m.Name + "("})
		}
		if t.Kind == types.KNodeRef {
			c.add(Item{Label: "Get", Detail: "Get<Component>() ComponentRef", Kind: ItemMethod, Insert: "Get<"})
			c.add(Item{Label: "Has", Detail: "Has<Component>() bool", Kind: ItemMethod, Insert: "Has<"})
			c.add(Item{Label: "GetScript", Detail: "GetScript<Script>() ScriptRef", Doc: builtinText(snap, "GetScript"), Kind: ItemMethod, Insert: "GetScript<"})
		}
	case types.KHandle:
		for _, hm := range snap.Bindings.HandleMethodList() {
			if hm.Handle != t.Handle {
				continue
			}
			doc, _ := snap.Docs.HandleMethodDoc(hm.Handle, hm.Name)
			c.add(Item{Label: hm.Name, Detail: hostSig("", hm.Sig, doc.Params), Doc: hostDoc(doc.Doc, doc.Example), Kind: ItemMethod, Insert: hm.Name + "("})
		}
	case types.KComponentRef:
		if t.Component == nil {
			return
		}
		for _, fl := range t.Component.Fields {
			c.add(Item{Label: fl.Name, Detail: abiType(fl.Type), Doc: fieldDoc(snap.Docs, t.Component.Name, fl.Name), Kind: ItemProperty})
		}
	case types.KScriptRef:
		scriptRefMembers(c, snap, t, pkg)
	case types.KCoroutine:
		c.add(Item{Label: "Stop", Detail: "Stop()", Doc: builtinText(snap, "Stop"), Kind: ItemMethod, Insert: "Stop()"})
		c.add(Item{Label: "Running", Detail: "Running() bool", Doc: builtinText(snap, "Running"), Kind: ItemMethod, Insert: "Running()"})
	case types.KConnection:
		c.add(Item{Label: "Disconnect", Detail: "Disconnect()", Doc: builtinText(snap, "Disconnect"), Kind: ItemMethod, Insert: "Disconnect()"})
		c.add(Item{Label: "Connected", Detail: "Connected() bool", Doc: builtinText(snap, "Connected"), Kind: ItemMethod, Insert: "Connected()"})
	case types.KCurve:
	default:
		for _, fl := range t.Fields {
			c.add(Item{Label: fl.Name, Detail: typeString(fl.Type), Kind: ItemField})
		}
	}
}

func packageMembers(c *collector, snap *workspace.Snapshot, o *resolver.Object) {
	if o.Target == "" {
		return
	}
	if !o.Host {
		if rp := snap.Analysis.Symbols.Packages[o.Target]; rp != nil {
			for _, obj := range rp.Scope.Objects() {
				if obj.Exported() {
					c.add(objectItem(snap, obj))
				}
			}
		}
		return
	}
	tab := snap.Bindings
	for _, fn := range tab.FuncList() {
		if fn.Module == o.Target {
			f := funcDoc(snap.Docs, fn.Module, fn.Name)
			c.add(Item{Label: fn.Name, Detail: hostSig("", fn.Sig, f.Params), Doc: hostDoc(f.Doc, f.Example), Kind: ItemFunction, Insert: fn.Name + "("})
		}
	}
	for _, k := range tab.ConstList() {
		if k.Module == o.Target {
			doc := ""
			if mc, ok := snap.Docs.ConstDoc(k.Module, k.Name); ok {
				doc = mc.Doc
			}
			c.add(Item{Label: k.Name, Detail: abiType(k.Val.Type) + " = " + constValue(k.Val), Doc: doc, Kind: ItemConstant})
		}
	}
	for _, e := range tab.EnumList() {
		if e.Module == o.Target {
			c.add(Item{Label: e.Name, Detail: "enum " + e.Qualified(), Kind: ItemEnum})
		}
	}
}

func typeArgs(c *collector, snap *workspace.Snapshot, fn string) {
	switch fn {
	case "Get", "Has":
		comps := snap.Bindings.AllComponents()
		count := map[string]int{}
		for _, comp := range comps {
			count[comp.Short]++
		}
		for _, comp := range comps {
			label := comp.Short
			if count[comp.Short] > 1 {
				label = comp.Name
			}
			c.add(Item{Label: label, Detail: comp.Name, Doc: componentDoc(snap.Docs, comp.Name), Kind: ItemComponent})
		}
	case "GetScript":
		scripts := snap.Analysis.Scripts
		count := map[string]int{}
		for _, st := range scripts {
			count[st.Name]++
		}
		for _, st := range scripts {
			label := st.Name
			if count[st.Name] > 1 {
				label = lastElem(st.Pkg) + "." + st.Name
			}
			c.add(Item{Label: label, Detail: st.FullName, Doc: st.Decl.Doc, Kind: ItemScript, Insert: label + ">()"})
		}
	}
}

func enclosing(f *ast.File, off uint32) (*ast.FuncDecl, *ast.ScriptDecl) {
	for _, d := range f.Decls {
		if !contains(d.Span(), off) {
			continue
		}
		switch x := d.(type) {
		case *ast.FuncDecl:
			return x, nil
		case *ast.ScriptDecl:
			for _, fn := range x.Funcs {
				if contains(fn.Sp, off) {
					return fn, x
				}
			}
			return nil, x
		}
	}
	return nil, nil
}

func locals(list []ast.Stmt, off uint32, out *[]*ast.Ident) {
	for _, st := range list {
		sp := st.Span()
		if sp.Start >= off {
			return
		}
		switch x := st.(type) {
		case *ast.VarStmt:
			if sp.End <= off {
				*out = append(*out, x.Name)
			}
		case *ast.ShortVarStmt:
			if sp.End <= off {
				*out = append(*out, x.Name)
			}
		case *ast.BlockStmt:
			if contains(sp, off) {
				locals(x.Stmts, off, out)
			}
		case *ast.IfStmt:
			if x.Then != nil && contains(x.Then.Sp, off) {
				locals(x.Then.Stmts, off, out)
			} else if x.Else != nil && contains(x.Else.Span(), off) {
				locals([]ast.Stmt{x.Else}, off, out)
			}
		case *ast.ForStmt:
			if contains(sp, off) {
				if x.Init != nil {
					locals([]ast.Stmt{x.Init}, off, out)
				}
				if x.Body != nil && contains(x.Body.Sp, off) {
					locals(x.Body.Stmts, off, out)
				}
			}
		case *ast.SwitchStmt:
			for _, cl := range x.Cases {
				if contains(cl.Sp, off) {
					locals(cl.Body, off, out)
				}
			}
		}
	}
}

func scope(c *collector, snap *workspace.Snapshot, f *ast.File, off uint32) {
	a := snap.Analysis
	fd, sd := enclosing(f, off)
	c.rank = rankLocal
	if fd != nil {
		var ids []*ast.Ident
		if fd.Body != nil {
			locals(fd.Body.Stmts, off, &ids)
		}
		for i := len(ids) - 1; i >= 0; i-- {
			if o := a.Defs[ids[i]]; o != nil {
				c.add(objectItem(snap, o))
			}
		}
		for _, p := range fd.Params {
			if o := a.Defs[p.Name]; o != nil {
				c.add(objectItem(snap, o))
			}
		}
	}
	c.rank = rankScript
	if sd != nil {
		for _, st := range a.Scripts {
			if st.Decl != sd {
				continue
			}
			c.add(Item{Label: "node", Detail: "NodeRef", Doc: builtinText(snap, "node"), Kind: ItemVariable})
			for _, fl := range st.Fields {
				c.add(Item{Label: fl.Name, Detail: scriptField(fl), Doc: fl.Decl.Doc, Kind: ItemField})
			}
			for _, fn := range st.Funcs {
				doc := fn.Decl.Doc
				if doc == "" {
					doc = builtinText(snap, fn.Name)
				}
				c.add(Item{Label: fn.Name, Detail: funcSig(fn), Doc: doc, Kind: ItemMethod, Insert: fn.Name + "("})
			}
		}
	}
	c.rank = rankPackage
	if pkg, ok := snap.Packages[f.ID]; ok {
		if rp := a.Symbols.Packages[pkg]; rp != nil {
			for _, o := range rp.Scope.Objects() {
				c.add(objectItem(snap, o))
			}
			if fs := rp.FileScopes[f]; fs != nil {
				for _, o := range fs.Objects() {
					c.add(objectItem(snap, o))
				}
			}
		}
	}
	c.rank = rankUniverse
	for _, o := range a.Symbols.Universe.Objects() {
		c.add(objectItem(snap, o))
	}
	c.rank = rankKeyword
	switch {
	case sd != nil && fd == nil:
		for _, sn := range scriptSnippets {
			c.add(snippet(snap, sn))
		}
	case sd == nil && fd == nil:
		for _, sn := range topSnippets {
			c.add(snippet(snap, sn))
		}
	}
	for _, k := range keywords {
		c.add(Item{Label: k, Kind: ItemKeyword, Doc: builtinText(snap, k)})
	}
}
