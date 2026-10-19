package highlight

import (
	"aego/core/mathx"
	"aego/ui/theme"
)

func Palette(t *theme.CodeTheme) [KindCount]mathx.Color {
	var p [KindCount]mathx.Color
	p[Plain] = t.Plain
	p[Keyword] = t.Keyword
	p[Number] = t.Number
	p[String] = t.String
	p[Comment] = t.Comment
	p[Operator] = t.Operator
	p[Punctuation] = t.Punctuation
	p[Bracket] = t.Punctuation
	p[Attribute] = t.Attribute
	p[Namespace] = t.Namespace
	p[Type] = t.Type
	p[BuiltinType] = t.BuiltinType
	p[Function] = t.Function
	p[Method] = t.Method
	p[Param] = t.Param
	p[Variable] = t.Local
	p[Field] = t.Field
	p[Property] = t.Property
	p[Constant] = t.Const
	p[EnumMember] = t.EnumMember
	p[Builtin] = t.Builtin
	p[Script] = t.Script
	p[Error] = t.Error
	p[Signal] = t.Signal
	p[Coroutine] = t.Coroutine
	return p
}
