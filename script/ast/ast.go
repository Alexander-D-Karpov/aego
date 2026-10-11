// Package ast defines the Aego Script syntax tree.
//
// Some invariants the tree cannot express in its types but that the parser
// guarantees and the checker may rely on:
//
//   - The literals true, false and nil are *BasicLit with Kind token.True,
//     token.False or token.Nil — never *Ident.
//   - AttrArg.Value is only a *BasicLit or a *UnaryExpr{Op: token.Sub, X:
//     *BasicLit}; the parser rejects anything else (so @export(min=-1) parses
//     and @export(min=f()) never reaches the checker), which folds the minus.
//   - CompositeLit.Elems are either all keyed or all unkeyed; mixing the two is
//     a parser error.
//   - ForStmt.Init and ForStmt.Post are only *ShortVarStmt, *AssignStmt,
//     *IncDecStmt or *ExprStmt.
package ast

import (
	"aego/script/source"
	"aego/script/token"
)

type Loc struct {
	Sp source.Span
}

func (l *Loc) Span() source.Span {
	return l.Sp
}

type Node interface {
	Span() source.Span
}

type Expr interface {
	Node
	exprNode()
}

type Stmt interface {
	Node
	stmtNode()
}

type Decl interface {
	Node
	declNode()
}

type TypeExpr interface {
	Node
	typeNode()
}

type File struct {
	Loc
	ID      source.FileID
	Package *Ident
	Imports []*Import
	Decls   []Decl
}

type Import struct {
	Loc
	Path string
}

type Attr struct {
	Loc
	Name *Ident
	Args []*AttrArg
}

func (a *Attr) Arg(key string) (*AttrArg, bool) {
	for _, arg := range a.Args {
		if arg.Key != nil && arg.Key.Name == key {
			return arg, true
		}
	}
	return nil, false
}

type AttrArg struct {
	Loc
	Key   *Ident
	Value Expr
}

type Ident struct {
	Loc
	Name string
}

type BasicLit struct {
	Loc
	Kind token.Kind
	Raw  string
	Str  string
}

type Element struct {
	Loc
	Key   *Ident
	Value Expr
}

type CompositeLit struct {
	Loc
	Type  TypeExpr
	Elems []*Element
}

type ParenExpr struct {
	Loc
	X Expr
}

type SelectorExpr struct {
	Loc
	X   Expr
	Sel *Ident
}

type IndexExpr struct {
	Loc
	X     Expr
	Index Expr
}

type CallExpr struct {
	Loc
	Fun      Expr
	TypeArgs []TypeExpr
	Args     []Expr
}

type UnaryExpr struct {
	Loc
	Op token.Kind
	X  Expr
}

type BinaryExpr struct {
	Loc
	Op token.Kind
	X  Expr
	Y  Expr
}

type BadExpr struct {
	Loc
}

func (*Ident) exprNode()        {}
func (*BasicLit) exprNode()     {}
func (*CompositeLit) exprNode() {}
func (*ParenExpr) exprNode()    {}
func (*SelectorExpr) exprNode() {}
func (*IndexExpr) exprNode()    {}
func (*CallExpr) exprNode()     {}
func (*UnaryExpr) exprNode()    {}
func (*BinaryExpr) exprNode()   {}
func (*BadExpr) exprNode()      {}

type NamedType struct {
	Loc
	Pkg  *Ident
	Name *Ident
	Args []TypeExpr
}

type SliceType struct {
	Loc
	Elem TypeExpr
}

type BadType struct {
	Loc
}

func (*NamedType) typeNode() {}
func (*SliceType) typeNode() {}
func (*BadType) typeNode()   {}

type VarStmt struct {
	Loc
	Name  *Ident
	Type  TypeExpr
	Value Expr
}

type ShortVarStmt struct {
	Loc
	Name  *Ident
	Value Expr
}

type AssignStmt struct {
	Loc
	Target Expr
	Op     token.Kind
	Value  Expr
}

type IncDecStmt struct {
	Loc
	X   Expr
	Inc bool
}

type ExprStmt struct {
	Loc
	X Expr
}

type ReturnStmt struct {
	Loc
	Value Expr
}

type BranchStmt struct {
	Loc
	Tok token.Kind
}

type BlockStmt struct {
	Loc
	Stmts []Stmt
}

type IfStmt struct {
	Loc
	Cond Expr
	Then *BlockStmt
	Else Stmt
}

type ForStmt struct {
	Loc
	Init Stmt
	Cond Expr
	Post Stmt
	Body *BlockStmt
}

type CaseClause struct {
	Loc
	Values  []Expr
	Default bool
	Body    []Stmt
}

type SwitchStmt struct {
	Loc
	Tag   Expr
	Cases []*CaseClause
}

type BadStmt struct {
	Loc
}

func (*VarStmt) stmtNode()      {}
func (*ShortVarStmt) stmtNode() {}
func (*AssignStmt) stmtNode()   {}
func (*IncDecStmt) stmtNode()   {}
func (*ExprStmt) stmtNode()     {}
func (*ReturnStmt) stmtNode()   {}
func (*BranchStmt) stmtNode()   {}
func (*BlockStmt) stmtNode()    {}
func (*IfStmt) stmtNode()       {}
func (*ForStmt) stmtNode()      {}
func (*SwitchStmt) stmtNode()   {}
func (*BadStmt) stmtNode()      {}

type ConstDecl struct {
	Loc
	Doc   string
	Name  *Ident
	Type  TypeExpr
	Value Expr
}

type Field struct {
	Loc
	Doc  string
	Name *Ident
	Type TypeExpr
}

type StructDecl struct {
	Loc
	Doc    string
	Name   *Ident
	Fields []*Field
}

type EnumDecl struct {
	Loc
	Doc     string
	Name    *Ident
	Members []*Ident
}

type Param struct {
	Loc
	Name *Ident
	Type TypeExpr
}

type FuncDecl struct {
	Loc
	Doc       string
	Attrs     []*Attr
	Name      *Ident
	Params    []*Param
	Result    TypeExpr
	Coroutine bool
	Body      *BlockStmt
}

type ScriptVar struct {
	Loc
	Doc   string
	Attrs []*Attr
	Name  *Ident
	Type  TypeExpr
	Value Expr
}

type ScriptDecl struct {
	Loc
	Doc     string
	Attrs   []*Attr
	Name    *Ident
	Signals []*SignalDecl
	Vars    []*ScriptVar
	Funcs   []*FuncDecl
}

type BadDecl struct {
	Loc
}

func (*ConstDecl) declNode()  {}
func (*StructDecl) declNode() {}
func (*EnumDecl) declNode()   {}
func (*FuncDecl) declNode()   {}
func (*ScriptDecl) declNode() {}
func (*BadDecl) declNode()    {}
