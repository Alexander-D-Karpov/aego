package checker

import (
	"strings"

	"aego/script/abi"
	"aego/script/ast"
	"aego/script/diag"
	"aego/script/resolver"
	"aego/script/types"
)

func (c *checker) typeExpr(e ast.TypeExpr) *types.Type {
	switch t := e.(type) {
	case *ast.SliceType:
		el := c.typeExpr(t.Elem)
		if el == types.Invalid {
			return types.Invalid
		}
		return c.u.SliceOf(el)
	case *ast.NamedType:
		return c.namedType(t)
	}
	return types.Invalid
}

func (c *checker) noArgs(t *ast.NamedType) {
	if len(t.Args) > 0 {
		c.errorf(diag.BadTypeArg, t.Args[0].Span(), "%s is not generic", t.Name.Name)
	}
}

func (c *checker) namedType(t *ast.NamedType) *types.Type {
	var obj *resolver.Object
	if t.Pkg != nil {
		p := c.lookup(t.Pkg.Name)
		if p == nil || p.Kind != resolver.ObjPkg {
			c.errorf(diag.UndefinedName, t.Pkg.Sp, "undefined package %s", t.Pkg.Name)
			return types.Invalid
		}
		c.use(t.Pkg, p)
		switch {
		case p.Target == "":
			return types.Invalid
		case p.Host:
			ht := c.hostType(p.Target, t.Name.Name)
			if ht == nil {
				c.errorf(diag.UnknownHost, t.Name.Sp, "%s has no type %s", p.Target, t.Name.Name)
				return types.Invalid
			}
			c.noArgs(t)
			return ht
		}
		if obj = c.member(t.Name, p); obj == nil {
			return types.Invalid
		}
	} else {
		if obj = c.lookup(t.Name.Name); obj == nil {
			c.errorf(diag.UndefinedName, t.Name.Sp, "undefined: %s", t.Name.Name)
			return types.Invalid
		}
		c.use(t.Name, obj)
	}
	switch obj.Kind {
	case resolver.ObjType:
		c.noArgs(t)
		return obj.Type
	case resolver.ObjGeneric:
		return c.generic(obj.Name, t)
	}
	c.errorf(diag.NotAType, t.Name.Sp, "%s is not a type", t.Name.Name)
	return types.Invalid
}

func (c *checker) generic(name string, t *ast.NamedType) *types.Type {
	if len(t.Args) != 1 {
		c.errorf(diag.BadTypeArg, t.Sp, "%s needs exactly one type argument", name)
		return types.Invalid
	}
	arg, ok := t.Args[0].(*ast.NamedType)
	if !ok {
		c.errorf(diag.BadTypeArg, t.Args[0].Span(), "%s argument must be a type name", name)
		return types.Invalid
	}
	switch name {
	case "AssetRef":
		if kind, ok := types.AssetKind(arg.Name.Name); ok && arg.Pkg == nil && len(arg.Args) == 0 {
			return c.u.AssetRef(kind)
		}
		if data, ok := c.dataArg(arg, name); ok {
			return c.u.AssetRef(dataPrefix + data)
		}
		return types.Invalid
	case "TableRef":
		if data, ok := c.dataArg(arg, name); ok {
			return c.u.AssetRef(tablePrefix + data)
		}
		return types.Invalid
	case "ScriptRef":
		full, ok := c.scriptTarget(arg)
		if !ok {
			return types.Invalid
		}
		return c.u.ScriptRef(full)
	}
	comp := c.component(arg)
	if comp == nil {
		return types.Invalid
	}
	return c.u.ComponentRef(comp)
}

func (c *checker) component(arg *ast.NamedType) *abi.Component {
	name := arg.Name.Name
	if arg.Pkg != nil {
		name = arg.Pkg.Name + "." + name
	}
	list := c.surface.Components(name)
	switch len(list) {
	case 1:
		return list[0]
	case 0:
		c.errorf(diag.BadTypeArg, arg.Sp, "unknown component %s", name)
	default:
		names := make([]string, len(list))
		for i, l := range list {
			names[i] = l.Name
		}
		c.errorf(diag.BadTypeArg, arg.Sp, "component name %s is ambiguous, write one of: %s", name, strings.Join(names, ", "))
	}
	return nil
}
