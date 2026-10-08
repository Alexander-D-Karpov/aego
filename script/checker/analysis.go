package checker

import (
	"aego/script/abi"
	"aego/script/ast"
	"aego/script/diag"
	"aego/script/resolver"
	"aego/script/types"
)

type TypeAndValue struct {
	Type  *types.Type
	Const *types.Const
	Addr  bool
}

type CallKind uint8

const (
	CallFunc CallKind = iota
	CallMethod
	CallHost
	CallHostMethod
	CallLen
	CallAppend
	CallConvert
	CallGetComponent
	CallHasComponent
)

type Call struct {
	Kind      CallKind
	Func      *Func
	Module    string
	Name      string
	Sig       abi.Signature
	Component *abi.Component
	Type      *types.Type
	Recv      ast.Expr
}

type SelKind uint8

const (
	SelField SelKind = iota
	SelLane
	SelPackage
	SelEnumMember
	SelComponentField
	SelHostConst
)

type Selection struct {
	Kind   SelKind
	Field  *types.Field
	Object *resolver.Object
	Value  int64
	Comp   *abi.Field
}

type Hints struct {
	Min, Max, Step float64
	HasMin, HasMax bool
	Unit           string
	Display        string
	Tip            string
	Category       string
}

type ScriptField struct {
	Name    string
	Former  []string
	Type    *types.Type
	Export  bool
	Hints   Hints
	Default *types.Const
	Index   int
	Slot    int
	Decl    *ast.ScriptVar
}

type ScriptType struct {
	Name        string
	Pkg         string
	FullName    string
	Former      []string
	Hidden      bool
	Fields      []*ScriptField
	Funcs       []*Func
	Methods     map[string]*Func
	Ready       *Func
	FixedUpdate *Func
	Update      *Func
	Slots       int
	Decl        *ast.ScriptDecl
	scope       *resolver.Scope
}

type Func struct {
	Name   string
	Pkg    string
	Decl   *ast.FuncDecl
	Params []*resolver.Object
	Result *types.Type
	Script *ScriptType
	Locals []*resolver.Object
}

func (f *Func) FullName() string {
	if f.Script != nil {
		return f.Script.FullName + "." + f.Name
	}
	return f.Pkg + "." + f.Name
}

type Package struct {
	Path    string
	Res     *resolver.Package
	Funcs   []*Func
	Scripts []*ScriptType
	Consts  []*resolver.Object
	Structs []*types.Type
	Enums   []*types.Type
	Imports []string
}

type Reference struct {
	Ident  *ast.Ident
	Object *resolver.Object
	Def    bool
}

type Analysis struct {
	Packages    map[string]*Package
	Order       []string
	Symbols     *resolver.Table
	Types       map[ast.Expr]TypeAndValue
	Defs        map[*ast.Ident]*resolver.Object
	Uses        map[*ast.Ident]*resolver.Object
	Calls       map[*ast.CallExpr]*Call
	Selections  map[*ast.SelectorExpr]*Selection
	Composites  map[*ast.CompositeLit]*types.Type
	References  []Reference
	Diagnostics []diag.Diagnostic
	Scripts     []*ScriptType
	Universe    *types.Interner
	Surface     abi.Surface
}

func newAnalysis(tab *resolver.Table, s abi.Surface) *Analysis {
	return &Analysis{
		Packages:   map[string]*Package{},
		Order:      tab.Order,
		Symbols:    tab,
		Types:      map[ast.Expr]TypeAndValue{},
		Defs:       map[*ast.Ident]*resolver.Object{},
		Uses:       map[*ast.Ident]*resolver.Object{},
		Calls:      map[*ast.CallExpr]*Call{},
		Selections: map[*ast.SelectorExpr]*Selection{},
		Composites: map[*ast.CompositeLit]*types.Type{},
		Universe:   types.NewInterner(),
		Surface:    s,
	}
}

func (a *Analysis) HasErrors() bool {
	return diag.HasErrors(a.Diagnostics)
}

func (a *Analysis) TypeOf(e ast.Expr) *types.Type {
	if tv, ok := a.Types[e]; ok {
		return tv.Type
	}
	return types.Invalid
}
