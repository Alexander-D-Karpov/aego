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
	CallStart
	CallEmit
	CallConnect
	CallDisconnect
	CallConnected
	CallCoStop
	CallCoRunning
	CallGetScript
	CallScriptMethod
	CallComponentMethod
)

type Call struct {
	Kind        CallKind
	Func        *Func
	Module      string
	Name        string
	Sig         abi.Signature
	Component   *abi.Component
	Type        *types.Type
	Recv        ast.Expr
	Signal      *ScriptSignal
	Handler     *Func
	HandlerRecv ast.Expr
	Target      string
}

type SelKind uint8

const (
	SelField SelKind = iota
	SelLane
	SelPackage
	SelEnumMember
	SelComponentField
	SelHostConst
	SelSignal
	SelScriptField
)

type Selection struct {
	Kind        SelKind
	Field       *types.Field
	Object      *resolver.Object
	Value       int64
	Comp        *abi.Field
	Script      *ScriptType
	ScriptField *ScriptField
	Signal      *ScriptSignal
}

type Hints struct {
	Min, Max, Step float64
	HasMin, HasMax bool
	Unit           string
	Display        string
	Tip            string
	Category       string
	Gizmo          string
	Space          string
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

type ScriptSignal struct {
	Name   string
	Params []*types.Type
	Names  []string
	Index  int
	Script *ScriptType
	Decl   *ast.SignalDecl
}

type ScriptType struct {
	Name        string
	Pkg         string
	FullName    string
	Former      []string
	Hidden      bool
	Fields      []*ScriptField
	Signals     []*ScriptSignal
	Funcs       []*Func
	Methods     map[string]*Func
	Ready       *Func
	FixedUpdate *Func
	Update      *Func
	Reloaded    *Func
	Events      []*Func
	Slots       int
	Decl        *ast.ScriptDecl
	scope       *resolver.Scope
}

func (st *ScriptType) Field(name string) *ScriptField {
	for _, f := range st.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func (st *ScriptType) Signal(name string) *ScriptSignal {
	for _, s := range st.Signals {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (st *ScriptType) IsEvent(f *Func) bool {
	for _, e := range st.Events {
		if e == f {
			return true
		}
	}
	return false
}

func (st *ScriptType) IsLifecycle(f *Func) bool {
	return f != nil && (f == st.Ready || f == st.FixedUpdate || f == st.Update || f == st.Reloaded || st.IsEvent(f))
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
	Coroutines  map[*ast.FuncDecl]bool
	Starts      map[*ast.StartExpr]*Func
	References  []Reference
	Diagnostics []diag.Diagnostic
	Scripts     []*ScriptType
	Universe    *types.Interner
	Surface     abi.Surface
	scripts     map[string]*ScriptType
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
		Coroutines: map[*ast.FuncDecl]bool{},
		Starts:     map[*ast.StartExpr]*Func{},
		Universe:   types.NewInterner(),
		Surface:    s,
		scripts:    map[string]*ScriptType{},
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

func (a *Analysis) ScriptByName(full string) *ScriptType {
	return a.scripts[full]
}
