package ast

import (
	"io"
	"strconv"
	"strings"

	"aego/script/source"
)

func Label(n Node) string {
	switch x := n.(type) {
	case *File:
		if x.Package != nil {
			return "File " + x.Package.Name
		}
		return "File"
	case *Import:
		return "Import " + strconv.Quote(x.Path)
	case *Attr:
		return "Attr"
	case *AttrArg:
		return "AttrArg"
	case *Ident:
		return "Ident " + x.Name
	case *BasicLit:
		return "BasicLit " + x.Kind.String() + " " + x.Raw
	case *CompositeLit:
		return "CompositeLit"
	case *Element:
		return "Element"
	case *ParenExpr:
		return "Paren"
	case *SelectorExpr:
		return "Selector"
	case *IndexExpr:
		return "Index"
	case *CallExpr:
		return "Call targs=" + strconv.Itoa(len(x.TypeArgs)) + " args=" + strconv.Itoa(len(x.Args))
	case *UnaryExpr:
		return "Unary " + x.Op.String()
	case *BinaryExpr:
		return "Binary " + x.Op.String()
	case *BadExpr:
		return "BadExpr"
	case *NamedType:
		return "NamedType"
	case *SliceType:
		return "SliceType"
	case *BadType:
		return "BadType"
	case *VarStmt:
		return "Var"
	case *ShortVarStmt:
		return "ShortVar"
	case *AssignStmt:
		return "Assign " + x.Op.String()
	case *IncDecStmt:
		if x.Inc {
			return "IncDec ++"
		}
		return "IncDec --"
	case *ExprStmt:
		return "ExprStmt"
	case *ReturnStmt:
		return "Return"
	case *BranchStmt:
		return "Branch " + x.Tok.String()
	case *BlockStmt:
		return "Block"
	case *IfStmt:
		if x.Else != nil {
			return "If else"
		}
		return "If"
	case *ForStmt:
		return "For init=" + strconv.FormatBool(x.Init != nil) + " cond=" + strconv.FormatBool(x.Cond != nil) + " post=" + strconv.FormatBool(x.Post != nil)
	case *SwitchStmt:
		return "Switch"
	case *CaseClause:
		if x.Default {
			return "Default"
		}
		return "Case"
	case *BadStmt:
		return "BadStmt"
	case *ConstDecl:
		return "Const"
	case *StructDecl:
		return "Struct"
	case *Field:
		return "Field"
	case *EnumDecl:
		return "Enum"
	case *FuncDecl:
		return "Func"
	case *Param:
		return "Param"
	case *ScriptDecl:
		return "Script"
	case *ScriptVar:
		return "ScriptVar"
	case *BadDecl:
		return "BadDecl"
	}
	return "?"
}

func Fprint(w io.Writer, sm *source.Manager, n Node) error {
	var b strings.Builder
	write(&b, sm, n, 0)
	_, err := io.WriteString(w, b.String())
	return err
}

func Sprint(sm *source.Manager, n Node) string {
	var b strings.Builder
	write(&b, sm, n, 0)
	return b.String()
}

func write(b *strings.Builder, sm *source.Manager, n Node, depth int) {
	for i := 0; i < depth; i++ {
		b.WriteString("  ")
	}
	b.WriteString(Label(n))
	if sp := n.Span(); sm != nil && sp.Valid() {
		line, col := sm.LineCol(sp.File, sp.Start)
		b.WriteByte(' ')
		b.WriteString(strconv.Itoa(line))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(col))
	}
	b.WriteByte('\n')
	for _, c := range Children(n) {
		write(b, sm, c, depth+1)
	}
}
