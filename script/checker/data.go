package checker

import (
	"strings"

	"aego/script/abi"
	"aego/script/ast"
	"aego/script/diag"
	"aego/script/types"
)

const (
	dataModule  = "aego/data"
	dataPrefix  = "data:"
	tablePrefix = "table:"
)

func (c *checker) dataArg(arg *ast.NamedType, generic string) (string, bool) {
	if arg.Pkg == nil && len(arg.Args) == 0 {
		if _, ok := c.surface.Struct(dataModule, arg.Name.Name); ok {
			return arg.Name.Name, true
		}
	}
	if generic == "AssetRef" {
		c.errorf(diag.BadTypeArg, arg.Sp, "AssetRef argument must be Texture, Scene, Prefab, Mesh or a data type")
	} else {
		c.errorf(diag.BadTypeArg, arg.Sp, "TableRef argument must be a data type")
	}
	return "", false
}

func (c *checker) recordOf(t *types.Type) (*types.Type, bool, bool) {
	name, ok := strings.CutPrefix(t.Asset, dataPrefix)
	table := false
	if !ok {
		name, ok = strings.CutPrefix(t.Asset, tablePrefix)
		table = ok
	}
	if !ok {
		return nil, false, false
	}
	sd, ok := c.surface.Struct(dataModule, name)
	if !ok {
		return nil, false, false
	}
	return c.u.HostStruct(sd, c.surface), table, true
}

func (c *checker) dataField(s *ast.SelectorExpr, t *types.Type) (operand, bool) {
	st, table, ok := c.recordOf(t)
	if !ok || table {
		return invalid, false
	}
	f := st.Field(s.Sel.Name)
	if f == nil {
		return invalid, false
	}
	c.a.Selections[s] = &Selection{Kind: SelDataField, Field: f, Data: st}
	return operand{mode: mValue, t: f.Type}, true
}

func (c *checker) dataMethod(e *ast.CallExpr, sel *ast.SelectorExpr, recv operand) (operand, bool) {
	st, table, ok := c.recordOf(recv.t)
	if !ok {
		return invalid, false
	}
	var params []*types.Type
	var res *types.Type
	switch name := sel.Sel.Name; {
	case !table && name == "Copy":
		res = st
	case table && name == "Row":
		params, res = []*types.Type{types.String}, st
	case table && name == "Has":
		params, res = []*types.Type{types.String}, types.Bool
	case table && name == "Len":
		res = types.Int
	default:
		return invalid, false
	}
	c.noTypeArgs(e)
	c.arguments(e, recv.t.Local+"."+sel.Sel.Name, params)
	sig := abi.Signature{Result: types.ToABI(res)}
	for _, p := range params {
		sig.Params = append(sig.Params, types.ToABI(p))
	}
	c.a.Calls[e] = &Call{Kind: CallHostMethod, Name: sel.Sel.Name + "<" + st.Qualified() + ">", Sig: sig, Recv: sel.X}
	return result(res), true
}
