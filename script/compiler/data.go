package compiler

import (
	"aego/script/abi"
	"aego/script/ast"
	"aego/script/bytecode"
	"aego/script/checker"
	"aego/script/types"
)

func (c *fn) dataFieldTo(e *ast.SelectorExpr, sel *checker.Selection, dst int) {
	recv := types.ToABI(c.typ(e.X))
	res := types.ToABI(sel.Data)
	sig := abi.Signature{Params: []abi.TypeRef{recv}, Result: res}
	imp := c.u.hostImport(methodModule(recv), "Copy<"+sel.Data.Qualified()+">", sig)
	n := sel.Data.Slots()
	mark := c.top
	tmp := c.alloc(max(n, 1))
	c.invoke(bytecode.CALLH, imp, []ast.Expr{e.X}, []int{1}, n, tmp, true)
	c.move(dst, tmp+sel.Field.Slot, sel.Field.Type.Slots())
	c.top = mark
}
