package langsvc

import (
	"sort"
	"strconv"

	"aego/script/ast"
	"aego/script/checker"
	"aego/script/resolver"
	"aego/script/source"
	"aego/script/types"
	"aego/script/workspace"
)

func (s *Service) Hover(file source.FileID, off uint32) (Hover, bool) {
	return s.HoverIn(s.Latest(), file, off)
}

func (s *Service) HoverIn(snap *workspace.Snapshot, file source.FileID, off uint32) (Hover, bool) {
	if snap == nil {
		return Hover{}, false
	}
	f := snap.Files[file]
	if f == nil {
		return Hover{}, false
	}
	if id, stack := identAt(f, off); id != nil {
		if o := object(snap.Analysis, id); o != nil {
			return Hover{Signature: describe(o), Doc: objectDoc(o), Span: id.Sp}, true
		}
		if h, ok := hoverMember(snap, id, stack); ok {
			h.Span = id.Sp
			return h, true
		}
		if n := len(stack); n > 0 {
			if a, ok := stack[n-1].(*ast.Attr); ok && a.Name == id {
				if d := builtinText("@" + id.Name); d != "" {
					return Hover{Signature: "@" + id.Name, Doc: d, Span: id.Sp}, true
				}
			}
		}
	}
	return keywordHover(snap, file, off)
}

func hoverMember(snap *workspace.Snapshot, id *ast.Ident, stack []ast.Node) (Hover, bool) {
	a, docs := snap.Analysis, snap.Docs
	if k := componentArg(a, id, stack); k != nil {
		return Hover{Signature: "component " + k.Component.Name, Doc: componentDoc(docs, k.Component.Name)}, true
	}
	sel := selectorWith(stack, id)
	if sel == nil {
		return Hover{}, false
	}
	if call := callOf(stack, sel); call != nil {
		if k := a.Calls[call]; k != nil {
			if h, ok := asyncCallHover(k); ok {
				return h, true
			}
			switch k.Kind {
			case checker.CallHost:
				f := funcDoc(docs, k.Module, k.Name)
				return Hover{Signature: hostSig("func "+k.Module+"."+k.Name, k.Sig, f.Params), Doc: hostDoc(f.Doc, f.Example)}, true
			case checker.CallHostMethod:
				recv := a.TypeOf(k.Recv)
				me := methodDoc(docs, types.ToABI(recv).Kind, k.Name)
				return Hover{Signature: hostSig("func ("+recv.String()+") "+k.Name, k.Sig, me.Params), Doc: hostDoc(me.Doc, me.Example)}, true
			case checker.CallGetComponent:
				return Hover{
					Signature: "func (NodeRef) Get<" + k.Component.Name + ">() ComponentRef<" + k.Component.Short + ">",
					Doc:       componentDoc(docs, k.Component.Name),
				}, true
			case checker.CallHasComponent:
				return Hover{
					Signature: "func (NodeRef) Has<" + k.Component.Name + ">() bool",
					Doc:       componentDoc(docs, k.Component.Name),
				}, true
			}
		}
	}
	x := a.Selections[sel]
	if x == nil {
		return Hover{}, false
	}
	switch x.Kind {
	case checker.SelComponentField:
		comp := ""
		if t := a.TypeOf(sel.X); t.Component != nil {
			comp = t.Component.Name
		}
		return Hover{Signature: "field " + x.Comp.Name + " " + abiType(x.Comp.Type), Doc: fieldDoc(docs, comp, x.Comp.Name)}, true
	case checker.SelField, checker.SelLane:
		h := Hover{Signature: "field " + x.Field.Name + " " + typeString(x.Field.Type)}
		if d := fieldDecl(a, x.Field); d != nil {
			h.Doc = d.Doc
		}
		return h, true
	case checker.SelEnumMember:
		return Hover{Signature: "const " + sel.Sel.Name + " = " + strconv.FormatInt(x.Value, 10)}, true
	case checker.SelHostConst:
		if c, ok := a.Surface.Const(x.Object.Target, sel.Sel.Name); ok {
			doc := ""
			if mc, ok := docs.ConstDoc(x.Object.Target, sel.Sel.Name); ok {
				doc = mc.Doc
			}
			return Hover{Signature: "const " + x.Object.Target + "." + sel.Sel.Name + " " + abiType(c.Type) + " = " + constValue(c), Doc: doc}, true
		}
	case checker.SelSignal:
		return Hover{Signature: signalDecl(x.Signal), Doc: x.Signal.Decl.Doc}, true
	}
	return Hover{}, false
}

func (s *Service) Definition(file source.FileID, off uint32) (Location, bool) {
	return s.DefinitionIn(s.Latest(), file, off)
}

func (s *Service) DefinitionIn(snap *workspace.Snapshot, file source.FileID, off uint32) (Location, bool) {
	if snap == nil {
		return Location{}, false
	}
	f := snap.Files[file]
	if f == nil {
		return Location{}, false
	}
	id, stack := identAt(f, off)
	if id == nil {
		return Location{}, false
	}
	a, docs := snap.Analysis, snap.Docs
	builtin := StubPath(StubBuiltin)
	comps := StubPath(StubComponents)
	if o := object(a, id); o != nil {
		switch {
		case o.Ident != nil:
			return loc(snap, o.Ident.Sp), true
		case o.Kind == resolver.ObjPkg && o.Host:
			return Location{Native: true, Stub: StubPath(o.Target)}, true
		case o.Kind == resolver.ObjPkg && o.Target != "":
			if rp := a.Symbols.Packages[o.Target]; rp != nil && len(rp.Files) > 0 && rp.Files[0].Package != nil {
				return loc(snap, rp.Files[0].Package.Sp), true
			}
		case o.Kind == resolver.ObjType, o.Kind == resolver.ObjGeneric:
			return Location{Native: true, Stub: builtin, Symbol: "type:" + o.Name}, true
		case o.Kind == resolver.ObjNode:
			return Location{Native: true, Stub: builtin, Symbol: "type:NodeRef"}, true
		case o.Kind == resolver.ObjBuiltin:
			return Location{Native: true, Stub: builtin, Symbol: "func:" + o.Name}, true
		}
		return Location{}, false
	}
	if k := componentArg(a, id, stack); k != nil {
		return Location{Native: true, Stub: comps, Symbol: "component:" + k.Component.Name}, true
	}
	sel := selectorWith(stack, id)
	if sel == nil {
		return Location{}, false
	}
	if call := callOf(stack, sel); call != nil {
		if k := a.Calls[call]; k != nil {
			switch k.Kind {
			case checker.CallHost:
				fd := funcDoc(docs, k.Module, k.Name)
				return Location{Native: true, Path: fd.Loc, Stub: StubPath(k.Module), Symbol: "func:" + k.Name}, true
			case checker.CallHostMethod:
				kind := types.ToABI(a.TypeOf(k.Recv)).Kind
				me := methodDoc(docs, kind, k.Name)
				return Location{Native: true, Path: me.Loc, Stub: builtin, Symbol: "method:" + kind.String() + "." + k.Name}, true
			case checker.CallGetComponent, checker.CallHasComponent:
				return Location{Native: true, Stub: comps, Symbol: "component:" + k.Component.Name}, true
			}
		}
	}
	if x := a.Selections[sel]; x != nil {
		switch x.Kind {
		case checker.SelField:
			if d := fieldDecl(a, x.Field); d != nil {
				return loc(snap, d.Name.Sp), true
			}
		case checker.SelComponentField:
			comp := ""
			if t := a.TypeOf(sel.X); t.Component != nil {
				comp = t.Component.Name
			}
			return Location{Native: true, Stub: comps, Symbol: "field:" + comp + "." + x.Comp.Name}, true
		case checker.SelHostConst:
			return Location{Native: true, Stub: StubPath(x.Object.Target), Symbol: "const:" + sel.Sel.Name}, true
		}
	}
	return Location{}, false
}

func (s *Service) References(file source.FileID, off uint32) []Location {
	return s.ReferencesIn(s.Latest(), file, off)
}

func (s *Service) ReferencesIn(snap *workspace.Snapshot, file source.FileID, off uint32) []Location {
	if snap == nil {
		return nil
	}
	f := snap.Files[file]
	if f == nil {
		return nil
	}
	id, stack := identAt(f, off)
	if id == nil {
		return nil
	}
	a := snap.Analysis
	var out []Location
	if o := object(a, id); o != nil {
		if fld := structField(a, o); fld != nil {
			out = fieldRefs(snap, fld)
		} else {
			for _, r := range a.References {
				if r.Object == o {
					out = append(out, loc(snap, r.Ident.Sp))
				}
			}
		}
	} else if sel := selectorWith(stack, id); sel != nil {
		if x := a.Selections[sel]; x != nil && x.Kind == checker.SelField {
			out = fieldRefs(snap, x.Field)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Span.Start < out[j].Span.Start
	})
	return out
}

func fieldRefs(snap *workspace.Snapshot, f *types.Field) []Location {
	a := snap.Analysis
	var out []Location
	if d := fieldDecl(a, f); d != nil {
		out = append(out, loc(snap, d.Name.Sp))
	}
	for sel, x := range a.Selections {
		if x.Field == f && x.Kind == checker.SelField {
			out = append(out, loc(snap, sel.Sel.Sp))
		}
	}
	return out
}
