package ast

type YieldKind uint8

const (
	YieldNext YieldKind = iota
	YieldWait
	YieldFrames
	YieldFixed
	YieldAwait
	YieldWaitUnscaled
)

var yieldKindNames = [...]string{"next", "wait", "frames", "fixed", "await", "waitUnscaled"}

func (k YieldKind) String() string {
	if int(k) < len(yieldKindNames) {
		return yieldKindNames[k]
	}
	return "?"
}

type StartExpr struct {
	Loc
	Name *Ident
	Args []Expr
}

type YieldStmt struct {
	Loc
	Kind  YieldKind
	Value Expr
}

type SignalDecl struct {
	Loc
	Doc    string
	Name   *Ident
	Params []*Param
}

func (*StartExpr) exprNode() {}
func (*YieldStmt) stmtNode() {}
