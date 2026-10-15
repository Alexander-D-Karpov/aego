package langsvc

import (
	"strings"

	"aego/script/abi"
	"aego/script/ast"
	"aego/script/checker"
	"aego/script/lexer"
	"aego/script/resolver"
	"aego/script/source"
	"aego/script/token"
	"aego/script/types"
	"aego/script/workspace"
)

type sigBuilder struct {
	b      strings.Builder
	params [][2]int
}

func (s *sigBuilder) param(text string) {
	if len(s.params) > 0 {
		s.b.WriteString(", ")
	}
	start := s.b.Len()
	s.b.WriteString(text)
	s.params = append(s.params, [2]int{start, s.b.Len()})
}

func funcSignature(snap *workspace.Snapshot, f *checker.Func) Signature {
	var sb sigBuilder
	sb.b.WriteString("func ")
	if f.Script != nil {
		sb.b.WriteString("(" + f.Script.Name + ") ")
	}
	sb.b.WriteString(f.Name + "(")
	for _, p := range f.Params {
		sb.param(p.Name + " " + typeString(p.Type))
	}
	sb.b.WriteByte(')')
	if f.Result != nil && f.Result != types.Void {
		sb.b.WriteString(" " + f.Result.String())
	}
	if f.Decl != nil && f.Decl.Coroutine {
		sb.b.WriteString(" coroutine")
	}
	doc := ""
	if f.Decl != nil {
		doc = f.Decl.Doc
	}
	if doc == "" && f.Script != nil {
		doc = builtinText(snap, f.Name)
	}
	return Signature{Label: sb.b.String(), Params: sb.params, Doc: doc}
}

func hostSignature(prefix string, s abi.Signature, names []string, doc, example string) Signature {
	var sb sigBuilder
	sb.b.WriteString(prefix + "(")
	for i, p := range s.Params {
		t := abiType(p)
		if i < len(names) {
			t = names[i] + " " + t
		}
		sb.param(t)
	}
	sb.b.WriteByte(')')
	if s.Result.Kind != abi.Void {
		sb.b.WriteString(" " + abiType(s.Result))
	}
	return Signature{Label: sb.b.String(), Params: sb.params, Doc: hostDoc(doc, example)}
}

func builtinSignature(snap *workspace.Snapshot, name string) (Signature, bool) {
	var sb sigBuilder
	switch name {
	case "len":
		sb.b.WriteString("func len(")
		sb.param("v string | []T")
		sb.b.WriteString(") int")
	case "append":
		sb.b.WriteString("func append(")
		sb.param("s []T")
		sb.param("v T")
		sb.b.WriteString(") []T")
	default:
		return Signature{}, false
	}
	return Signature{Label: sb.b.String(), Params: sb.params, Doc: builtinText(snap, name)}, true
}

func (s *Service) SignatureAt(snap *workspace.Snapshot, file source.FileID, src []byte, off int) (Signature, bool) {
	if snap == nil || off < 0 || off > len(src) {
		return Signature{}, false
	}
	toks, _, _ := lexer.TokenizeAll(file, src[:off], source.NewInterner())
	open, active, depth := -1, 0, 0
scan:
	for i := len(toks) - 1; i >= 0; i-- {
		t := toks[i]
		switch t.Kind {
		case token.RParen, token.RBrack, token.RBrace:
			depth++
		case token.LParen:
			if depth == 0 {
				open = i
				break scan
			}
			depth--
		case token.LBrack, token.LBrace:
			if depth == 0 {
				return Signature{}, false
			}
			depth--
		case token.Comma:
			if depth == 0 {
				active++
			}
		case token.Semicolon:
			if depth == 0 && !t.IsAuto() {
				return Signature{}, false
			}
		}
	}
	if open < 1 || toks[open-1].Kind != token.Ident {
		return Signature{}, false
	}
	name, qual := toks[open-1].Text, ""
	if open >= 3 && toks[open-2].Kind == token.Period && toks[open-3].Kind == token.Ident {
		qual = toks[open-3].Text
	}
	sig, ok := resolveCallee(snap, file, qual, name)
	if !ok {
		return Signature{}, false
	}
	sig.Active = min(active, len(sig.Params)-1)
	return sig, true
}

func scopeOf(snap *workspace.Snapshot, file source.FileID) (*resolver.Scope, string) {
	a := snap.Analysis
	pkg := snap.Packages[file]
	rp := a.Symbols.Packages[pkg]
	if rp == nil {
		return a.Symbols.Universe, pkg
	}
	if f := snap.Files[file]; f != nil {
		if fs := rp.FileScopes[f]; fs != nil {
			return fs, pkg
		}
	}
	return rp.Scope, pkg
}

func resolveCallee(snap *workspace.Snapshot, file source.FileID, qual, name string) (Signature, bool) {
	a := snap.Analysis
	scope, pkg := scopeOf(snap, file)
	if qual == "" {
		if o := scope.Lookup(name); o != nil {
			switch o.Kind {
			case resolver.ObjFunc:
				if f, ok := o.Data.(*checker.Func); ok {
					return funcSignature(snap, f), true
				}
			case resolver.ObjBuiltin:
				return builtinSignature(snap, name)
			}
		}
		for _, st := range a.Scripts {
			if st.Pkg != pkg {
				continue
			}
			if f := st.Methods[name]; f != nil {
				return funcSignature(snap, f), true
			}
		}
		return Signature{}, false
	}
	if o := scope.Lookup(qual); o != nil && o.Kind == resolver.ObjPkg && o.Target != "" {
		if o.Host {
			sig, ok := snap.Bindings.Func(o.Target, name)
			if !ok {
				return Signature{}, false
			}
			f := funcDoc(snap.Docs, o.Target, name)
			return hostSignature("func "+o.Target+"."+name, sig, f.Params, f.Doc, f.Example), true
		}
		if tp := a.Symbols.Packages[o.Target]; tp != nil {
			if x := tp.Scope.Lookup(name); x != nil && x.Kind == resolver.ObjFunc {
				if f, ok := x.Data.(*checker.Func); ok {
					return funcSignature(snap, f), true
				}
			}
		}
		return Signature{}, false
	}
	if name == "Emit" || name == "Connect" {
		if sg := signalNamed(a, pkg, qual); sg != nil {
			return signalSignature(snap, sg, name), true
		}
	}
	if f := scriptMethodNamed(a, pkg, name); f != nil {
		return funcSignature(snap, f), true
	}
	for _, recv := range [...]abi.Kind{abi.NodeRef, abi.AssetRef} {
		if sig, ok := snap.Bindings.Method(recv, name); ok {
			me := methodDoc(snap.Docs, recv, name)
			return hostSignature("func ("+recv.String()+") "+name, sig, me.Params, me.Doc, me.Example), true
		}
	}
	if comp, sig, ok := uniqueCompMethod(snap, name); ok {
		me := compMethodDoc(snap.Docs, comp.Name, name)
		return hostSignature("func ("+comp.Short+") "+name, sig, me.Params, me.Doc, me.Example), true
	}
	return Signature{}, false
}

func (s *Service) HintsIn(snap *workspace.Snapshot, file source.FileID) []Hint {
	if snap == nil {
		return nil
	}
	f := snap.Files[file]
	if f == nil {
		return nil
	}
	a := snap.Analysis
	var out []Hint
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		k := a.Calls[call]
		if k == nil {
			return true
		}
		var names []string
		switch k.Kind {
		case checker.CallFunc, checker.CallMethod:
			if k.Func != nil {
				for _, p := range k.Func.Params {
					names = append(names, p.Name)
				}
			}
		case checker.CallHost:
			names = funcDoc(snap.Docs, k.Module, k.Name).Params
		case checker.CallHostMethod:
			names = methodDoc(snap.Docs, types.ToABI(a.TypeOf(k.Recv)).Kind, k.Name).Params
		case checker.CallComponentMethod:
			names = compMethodDoc(snap.Docs, k.Component.Name, k.Name).Params
		}
		for i, arg := range call.Args {
			if i >= len(names) || names[i] == "" {
				continue
			}
			switch x := arg.(type) {
			case *ast.Ident:
				if strings.EqualFold(x.Name, names[i]) {
					continue
				}
			case *ast.SelectorExpr:
				if strings.EqualFold(x.Sel.Name, names[i]) {
					continue
				}
			}
			out = append(out, Hint{Off: arg.Span().Start, Label: names[i] + ":"})
		}
		return true
	})
	return out
}
