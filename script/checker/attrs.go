package checker

import (
	"aego/script/ast"
	"aego/script/bytecode"
	"aego/script/diag"
	"aego/script/types"
)

func exportable(t *types.Type) bool {
	switch t.Kind {
	case types.KBool, types.KInt, types.KFloat, types.KString, types.KVec2, types.KVec3,
		types.KColor, types.KQuat, types.KRect, types.KEnum, types.KNodeRef, types.KAssetRef, types.KComponentRef:
		return true
	}
	return false
}

func persistable(t *types.Type) bool {
	switch t.Kind {
	case types.KBool, types.KInt, types.KFloat, types.KString, types.KVec2, types.KVec3,
		types.KColor, types.KQuat, types.KRect, types.KEnum, types.KNodeRef, types.KAssetRef:
		return true
	}
	return false
}

func (c *checker) pauseArg(st *ScriptType, a *ast.Attr) {
	vals := c.stringArgs(a)
	if len(vals) != 1 {
		c.errorf(diag.BadAttrArg, a.Sp, "@pause takes one mode: \"normal\", \"always\" or \"whenPaused\"")
		return
	}
	switch vals[0] {
	case "normal":
		st.Pause = bytecode.PauseNormal
	case "always":
		st.Pause = bytecode.PauseAlways
	case "whenPaused":
		st.Pause = bytecode.PauseWhenPaused
	default:
		c.errorf(diag.BadAttrArg, a.Args[0].Value.Span(), "unknown pause mode %q, use \"normal\", \"always\" or \"whenPaused\"", vals[0])
	}
}

func (c *checker) scriptAttrs(st *ScriptType, attrs []*ast.Attr) {
	seen := map[string]bool{}
	for _, a := range attrs {
		name := a.Name.Name
		if seen[name] {
			c.errorf(diag.DuplicateAttr, a.Sp, "@%s is given twice", name)
			continue
		}
		seen[name] = true
		switch name {
		case "hide":
			if len(a.Args) > 0 {
				c.errorf(diag.BadAttrArg, a.Args[0].Sp, "@hide takes no arguments")
			}
			st.Hidden = true
		case "former":
			st.Former = append(st.Former, c.stringArgs(a)...)
		case "pause":
			c.pauseArg(st, a)
		case "export":
			c.errorf(diag.AttrTarget, a.Sp, "@export applies to script fields, not to scripts")
		default:
			c.errorf(diag.UnknownAttr, a.Name.Sp, "unknown attribute @%s", name)
		}
	}
}

func (c *checker) fieldAttrs(f *ScriptField, attrs []*ast.Attr) {
	seen := map[string]bool{}
	for _, a := range attrs {
		name := a.Name.Name
		if seen[name] {
			c.errorf(diag.DuplicateAttr, a.Sp, "@%s is given twice", name)
			continue
		}
		seen[name] = true
		switch name {
		case "export":
			f.Export = true
			c.exportArgs(f, a)
			if f.Type != types.Invalid && !exportable(f.Type) {
				c.errorf(diag.NotExportable, f.Decl.Type.Span(), "fields of type %s cannot be exported", f.Type)
			}
		case "persist":
			if len(a.Args) > 0 {
				c.errorf(diag.BadAttrArg, a.Args[0].Sp, "@persist takes no arguments")
			}
			f.Persist = true
			if f.Type != types.Invalid && !persistable(f.Type) {
				c.errorf(diag.NotPersistable, f.Decl.Type.Span(), "fields of type %s cannot be saved; save what they point at in OnSave instead", f.Type)
			}
		case "former":
			f.Former = append(f.Former, c.stringArgs(a)...)
		case "hide", "pause":
			c.errorf(diag.AttrTarget, a.Sp, "@%s applies to scripts, not to fields", name)
		default:
			c.errorf(diag.UnknownAttr, a.Name.Sp, "unknown attribute @%s", name)
		}
	}
}

func (c *checker) stringArgs(a *ast.Attr) []string {
	if len(a.Args) == 0 {
		c.errorf(diag.BadAttrArg, a.Sp, "@%s needs at least one name", a.Name.Name)
		return nil
	}
	var out []string
	for _, arg := range a.Args {
		x := c.expr(arg.Value)
		switch {
		case arg.Key != nil:
			c.errorf(diag.BadAttrArg, arg.Key.Sp, "@%s takes plain strings", a.Name.Name)
		case x.mode == mInvalid:
		case x.c == nil || x.c.Kind != types.CString:
			c.errorf(diag.BadAttrArg, arg.Value.Span(), "@%s takes string constants", a.Name.Name)
		default:
			out = append(out, x.c.Str)
		}
	}
	return out
}

func (c *checker) exportArgs(f *ScriptField, a *ast.Attr) {
	h := &f.Hints
	seen := map[string]bool{}
	for _, arg := range a.Args {
		if arg.Key == nil {
			c.errorf(diag.BadAttrArg, arg.Sp, "@export arguments must be named, like min=0")
			continue
		}
		key := arg.Key.Name
		if seen[key] {
			c.errorf(diag.DuplicateAttr, arg.Key.Sp, "%s is given twice", key)
			continue
		}
		seen[key] = true
		x := c.expr(arg.Value)
		if x.mode == mInvalid {
			continue
		}
		switch key {
		case "min", "max", "step":
			if x.c == nil || (x.c.Kind != types.CInt && x.c.Kind != types.CFloat) {
				c.errorf(diag.BadAttrArg, arg.Value.Span(), "%s must be a number", key)
				continue
			}
			if !f.Type.IsNumeric() {
				c.errorf(diag.BadAttrArg, arg.Key.Sp, "%s applies only to int and float fields", key)
				continue
			}
			v := x.c.Float64()
			switch key {
			case "min":
				h.Min, h.HasMin = v, true
			case "max":
				h.Max, h.HasMax = v, true
			default:
				h.Step = v
			}
		case "unit", "display", "tip", "category", "gizmo", "space":
			if x.c == nil || x.c.Kind != types.CString {
				c.errorf(diag.BadAttrArg, arg.Value.Span(), "%s must be a string", key)
				continue
			}
			switch key {
			case "unit":
				h.Unit = x.c.Str
			case "display":
				h.Display = x.c.Str
			case "tip":
				h.Tip = x.c.Str
			case "category":
				h.Category = x.c.Str
			case "gizmo":
				if c.gizmoFits(f, x.c.Str, arg) {
					h.Gizmo = x.c.Str
				}
			default:
				if x.c.Str != "local" && x.c.Str != "world" {
					c.errorf(diag.BadAttrArg, arg.Value.Span(), "space must be \"local\" or \"world\"")
					continue
				}
				h.Space = x.c.Str
			}
		default:
			c.errorf(diag.BadAttrArg, arg.Key.Sp, "unknown @export argument %s", key)
		}
	}
	if h.HasMin && h.HasMax && h.Min > h.Max {
		c.errorf(diag.BadAttrArg, a.Sp, "min %g is greater than max %g", h.Min, h.Max)
	}
	if h.Space != "" && h.Gizmo == "" {
		c.errorf(diag.BadAttrArg, a.Sp, "space applies only together with gizmo")
	}
}

func (c *checker) gizmoFits(f *ScriptField, g string, arg *ast.AttrArg) bool {
	if f.Type == types.Invalid {
		return false
	}
	switch g {
	case "vector", "point":
		if f.Type.Kind != types.KVec2 {
			c.errorf(diag.BadAttrArg, arg.Value.Span(), "gizmo=%q needs a Vec2 field, %s has type %s", g, f.Name, f.Type)
			return false
		}
	case "radius":
		if f.Type.Kind != types.KFloat {
			c.errorf(diag.BadAttrArg, arg.Value.Span(), "gizmo=\"radius\" needs a float field, %s has type %s", f.Name, f.Type)
			return false
		}
	default:
		c.errorf(diag.BadAttrArg, arg.Value.Span(), "unknown gizmo %q, use vector, point or radius", g)
		return false
	}
	return true
}
