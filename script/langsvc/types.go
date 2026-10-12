package langsvc

import "aego/script/source"

type ItemKind uint8

const (
	ItemKeyword ItemKind = iota
	ItemVariable
	ItemParam
	ItemField
	ItemProperty
	ItemMethod
	ItemFunction
	ItemConstant
	ItemType
	ItemEnum
	ItemEnumMember
	ItemModule
	ItemComponent
	ItemScript
	ItemAttribute
	ItemSnippet
	ItemSignal
)

type TokenKind uint8

const (
	TokNamespace TokenKind = iota
	TokType
	TokStruct
	TokEnum
	TokEnumMember
	TokFunction
	TokMethod
	TokParameter
	TokVariable
	TokField
	TokProperty
	TokConstant
	TokBuiltin
	TokScript
	TokAttribute
	TokSignal
	TokCoroutine
)

type SymbolKind uint8

const (
	SymConst SymbolKind = iota
	SymStruct
	SymField
	SymEnum
	SymEnumMember
	SymFunc
	SymScript
	SymMethod
	SymSignal
)

func (k SymbolKind) String() string {
	switch k {
	case SymConst:
		return "const"
	case SymStruct:
		return "struct"
	case SymField:
		return "field"
	case SymEnum:
		return "enum"
	case SymEnumMember:
		return "member"
	case SymFunc:
		return "func"
	case SymScript:
		return "script"
	case SymSignal:
		return "signal"
	}
	return "method"
}

type Completion struct {
	Prefix source.Span
	Items  []Item
}

type Item struct {
	Label  string
	Detail string
	Doc    string
	Kind   ItemKind
	Insert string
	rank   int
}

type Hover struct {
	Signature string
	Doc       string
	Ref       string
	Span      source.Span
}

type Location struct {
	File   source.FileID
	Path   string
	Span   source.Span
	Native bool
	Stub   string
	Symbol string
}

type Token struct {
	Span source.Span
	Kind TokenKind
}

type Symbol struct {
	Name      string
	Kind      SymbolKind
	Detail    string
	Span      source.Span
	Selection source.Span
	Children  []Symbol
}

type SymbolRef struct {
	Name      string
	Container string
	Detail    string
	Kind      SymbolKind
	Path      string
	Line      int
	Col       int
}

type Hint struct {
	Off   uint32
	Label string
}

type Signature struct {
	Label  string
	Params [][2]int
	Active int
	Doc    string
}

type Stub struct {
	Path  string
	Text  string
	Lines map[string]int
	Locs  map[string]string
}
