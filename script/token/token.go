package token

import "strconv"

type Kind uint8

const (
	Illegal Kind = iota
	EOF
	Ident
	Int
	Float
	String

	Add
	Sub
	Mul
	Quo
	Rem
	Eq
	Ne
	Lt
	Le
	Gt
	Ge
	AndAnd
	OrOr
	Not
	Assign
	Define
	AddAssign
	SubAssign
	MulAssign
	QuoAssign
	RemAssign
	Inc
	Dec
	Period
	Comma
	Semicolon
	Colon
	LParen
	RParen
	LBrack
	RBrack
	LBrace
	RBrace
	At

	keywordBeg
	Package
	Import
	Script
	Type
	Struct
	Enum
	Var
	Const
	Func
	Return
	If
	Else
	For
	Switch
	Case
	Default
	Break
	Continue
	True
	False
	Nil
	Yield
	Signal
	Coroutine
	keywordEnd

	reservedBeg
	Await
	Select
	Go
	Chan
	Defer
	Interface
	Map
	reservedEnd
)

var names = [...]string{
	Illegal:   "ILLEGAL",
	EOF:       "EOF",
	Ident:     "IDENT",
	Int:       "INT",
	Float:     "FLOAT",
	String:    "STRING",
	Add:       "+",
	Sub:       "-",
	Mul:       "*",
	Quo:       "/",
	Rem:       "%",
	Eq:        "==",
	Ne:        "!=",
	Lt:        "<",
	Le:        "<=",
	Gt:        ">",
	Ge:        ">=",
	AndAnd:    "&&",
	OrOr:      "||",
	Not:       "!",
	Assign:    "=",
	Define:    ":=",
	AddAssign: "+=",
	SubAssign: "-=",
	MulAssign: "*=",
	QuoAssign: "/=",
	RemAssign: "%=",
	Inc:       "++",
	Dec:       "--",
	Period:    ".",
	Comma:     ",",
	Semicolon: ";",
	Colon:     ":",
	LParen:    "(",
	RParen:    ")",
	LBrack:    "[",
	RBrack:    "]",
	LBrace:    "{",
	RBrace:    "}",
	At:        "@",
	Package:   "package",
	Import:    "import",
	Script:    "script",
	Type:      "type",
	Struct:    "struct",
	Enum:      "enum",
	Var:       "var",
	Const:     "const",
	Func:      "func",
	Return:    "return",
	If:        "if",
	Else:      "else",
	For:       "for",
	Switch:    "switch",
	Case:      "case",
	Default:   "default",
	Break:     "break",
	Continue:  "continue",
	True:      "true",
	False:     "false",
	Nil:       "nil",
	Yield:     "yield",
	Signal:    "signal",
	Coroutine: "coroutine",
	Await:     "await",
	Select:    "select",
	Go:        "go",
	Chan:      "chan",
	Defer:     "defer",
	Interface: "interface",
	Map:       "map",
}

var keywords = func() map[string]Kind {
	m := make(map[string]Kind, int(reservedEnd-keywordBeg))
	for k := keywordBeg + 1; k < keywordEnd; k++ {
		m[names[k]] = k
	}
	for k := reservedBeg + 1; k < reservedEnd; k++ {
		m[names[k]] = k
	}
	return m
}()

func (k Kind) String() string {
	if int(k) < len(names) && names[k] != "" {
		return names[k]
	}
	return "token(" + strconv.Itoa(int(k)) + ")"
}

func Lookup(ident string) Kind {
	if k, ok := keywords[ident]; ok {
		return k
	}
	return Ident
}

func (k Kind) IsKeyword() bool {
	return k > keywordBeg && k < keywordEnd
}

func (k Kind) IsReserved() bool {
	return k > reservedBeg && k < reservedEnd
}

func (k Kind) IsLiteral() bool {
	return k == Int || k == Float || k == String
}

func (k Kind) IsOperator() bool {
	return k >= Add && k <= At
}

func (k Kind) IsAssign() bool {
	return k == Assign || (k >= AddAssign && k <= RemAssign)
}

func (k Kind) AssignOp() Kind {
	switch k {
	case AddAssign:
		return Add
	case SubAssign:
		return Sub
	case MulAssign:
		return Mul
	case QuoAssign:
		return Quo
	case RemAssign:
		return Rem
	}
	return Illegal
}

const (
	LowestPrec = 0
	UnaryPrec  = 6
)

func (k Kind) Precedence() int {
	switch k {
	case OrOr:
		return 1
	case AndAnd:
		return 2
	case Eq, Ne, Lt, Le, Gt, Ge:
		return 3
	case Add, Sub:
		return 4
	case Mul, Quo, Rem:
		return 5
	}
	return LowestPrec
}

type Token struct {
	Kind  Kind
	Start uint32
	End   uint32
	Text  string
	Value string
}

func (t Token) IsAuto() bool {
	return t.Kind == Semicolon && t.Text == "\n"
}

func (t Token) String() string {
	switch {
	case t.IsAuto():
		return "newline"
	case t.Kind == Ident || t.Kind.IsLiteral():
		return t.Kind.String() + "(" + t.Text + ")"
	}
	return t.Kind.String()
}
