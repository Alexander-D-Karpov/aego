package checker

import (
	"aego/script/ast"
	"aego/script/diag"
	"aego/script/types"
)

func exportable(t *types.Type) bool {
	switch t.Kind {
	case types.KBool, types.KInt, types.KFloat, types.KString, types.KVec2, types.KVec3,
		types.KColor, types.KQuat, types.KRect, types.KEnum, types.KNodeRef, types.KAssetRef:
		return true
	}
	return false
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
		case "former":
			f.Former = append(f.Former, c.stringArgs(a)...)
		case "hide":
			c.errorf(diag.AttrTarget, a.Sp, "@hide applies to scripts, not to fields")
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
		case "unit", "display", "tip", "category":
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
			default:
				h.Category = x.c.Str
			}
		default:
			c.errorf(diag.BadAttrArg, arg.Key.Sp, "unknown @export argument %s", key)
		}
	}
	if h.HasMin && h.HasMax && h.Min > h.Max {
		c.errorf(diag.BadAttrArg, a.Sp, "min %g is greater than max %g", h.Min, h.Max)
	}
}
