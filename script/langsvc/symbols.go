package langsvc

import (
	"sort"

	"aego/script/ast"
	"aego/script/checker"
	"aego/script/resolver"
	"aego/script/source"
	"aego/script/types"
	"aego/script/workspace"
)

func objToken(o *resolver.Object) TokenKind {
	switch o.Kind {
	case resolver.ObjPkg:
		return TokNamespace
	case resolver.ObjType:
		if o.Type != nil {
			switch o.Type.Kind {
			case types.KEnum:
				return TokEnum
			case types.KStruct:
				return TokStruct
			}
		}
		return TokType
	case resolver.ObjGeneric:
		return TokType
	case resolver.ObjFunc:
		if f, ok := o.Data.(*checker.Func); ok && f.Script != nil {
			if f.Decl != nil && f.Decl.Coroutine {
				return TokCoroutine
			}
			return TokMethod
		}
		return TokFunction
	case resolver.ObjScript:
		return TokScript
	case resolver.ObjParam:
		return TokParameter
	case resolver.ObjField:
		return TokField
	case resolver.ObjConst:
		return TokConstant
	case resolver.ObjBuiltin:
		return TokBuiltin
	case resolver.ObjSignal:
		return TokSignal
	}
	return TokVariable
}

func (s *Service) SemanticTokens(file source.FileID) []Token {
	return semanticTokens(s.Latest(), file)
}

func semanticTokens(snap *workspace.Snapshot, file source.FileID) []Token {
	if snap == nil {
		return nil
	}
	f := snap.Files[file]
	if f == nil {
		return nil
	}
	a := snap.Analysis
	marked := map[*ast.Ident]TokenKind{}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.File:
			if x.Package != nil {
				marked[x.Package] = TokNamespace
			}
		case *ast.Attr:
			marked[x.Name] = TokAttribute
		case *ast.AttrArg:
			if x.Key != nil {
				marked[x.Key] = TokParameter
			}
		case *ast.EnumDecl:
			for _, m := range x.Members {
				marked[m] = TokEnumMember
			}
		case *ast.Element:
			if x.Key != nil {
				marked[x.Key] = TokField
			}
		case *ast.SelectorExpr:
			if sel := a.Selections[x]; sel != nil {
				switch sel.Kind {
				case checker.SelField, checker.SelLane, checker.SelScriptField:
					marked[x.Sel] = TokField
				case checker.SelComponentField:
					marked[x.Sel] = TokProperty
				case checker.SelEnumMember:
					marked[x.Sel] = TokEnumMember
				case checker.SelHostConst:
					marked[x.Sel] = TokConstant
				case checker.SelSignal:
					marked[x.Sel] = TokSignal
				}
			}
		case *ast.CallExpr:
			k := a.Calls[x]
			if k == nil {
				break
			}
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
				switch k.Kind {
				case checker.CallHost:
					marked[sel.Sel] = TokFunction
				case checker.CallHostMethod, checker.CallScriptMethod:
					marked[sel.Sel] = TokMethod
				case checker.CallGetComponent, checker.CallHasComponent, checker.CallGetScript,
					checker.CallEmit, checker.CallConnect, checker.CallDisconnect, checker.CallConnected,
					checker.CallCoStop, checker.CallCoRunning:
					marked[sel.Sel] = TokBuiltin
				}
			}
			if k.Kind == checker.CallGetComponent || k.Kind == checker.CallHasComponent || k.Kind == checker.CallGetScript {
				for _, ta := range x.TypeArgs {
					if nt, ok := ta.(*ast.NamedType); ok {
						marked[nt.Name] = TokType
						if k.Kind == checker.CallGetScript {
							marked[nt.Name] = TokScript
						}
						if nt.Pkg != nil {
							marked[nt.Pkg] = TokNamespace
						}
					}
				}
			}
		}
		return true
	})
	var out []Token
	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || id.Sp.End == id.Sp.Start {
			return true
		}
		k, ok := marked[id]
		if !ok {
			if o := object(a, id); o != nil {
				k, ok = objToken(o), true
			}
		}
		if ok {
			out = append(out, Token{Span: id.Sp, Kind: k})
		}
		return true
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Span.Start < out[j].Span.Start })
	n := 0
	for i, t := range out {
		if i > 0 && t.Span.Start == out[n-1].Span.Start {
			continue
		}
		out[n] = t
		n++
	}
	return out[:n]
}
