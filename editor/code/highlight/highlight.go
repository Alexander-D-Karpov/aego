package highlight

import (
	"sort"
	"strings"

	"aego/script/token"
)

type Kind uint8

const (
	Plain Kind = iota
	Keyword
	Number
	String
	Comment
	Operator
	Punctuation
	Attribute
	Namespace
	Type
	BuiltinType
	Function
	Method
	Param
	Variable
	Field
	Property
	Constant
	EnumMember
	Builtin
	Script
	Error
	Signal
	Coroutine
	KindCount
)

type State uint8

const (
	Normal State = iota
	InComment
)

type Span struct {
	Start int
	End   int
	Kind  Kind
}

type Sem struct {
	Start int
	End   int
	Kind  Kind
}

var builtinTypes = map[string]bool{
	"bool": true, "int": true, "float": true, "string": true,
	"Vec2": true, "Vec3": true, "Color": true, "Quat": true, "Rect": true,
	"Transform2D": true, "Transform3D": true,
	"NodeRef": true, "AssetRef": true, "ComponentRef": true,
}

var builtinFuncs = map[string]bool{"len": true, "append": true}

const punctuation = "(){}[],;:"

func letter(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func digit(c byte) bool {
	return c >= '0' && c <= '9'
}

func ident(c byte) bool {
	return letter(c) || digit(c)
}

func closeComment(text []byte, from int) int {
	for i := from; i+1 < len(text); i++ {
		if text[i] == '*' && text[i+1] == '/' {
			return i + 2
		}
	}
	return -1
}

func stringEnd(text []byte, i int) int {
	j := i + 1
	for j < len(text) && text[j] != '"' {
		if text[j] == '\\' {
			j++
		}
		j++
	}
	return min(j+1, len(text))
}

func skipSpace(text []byte, i int) int {
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	return i
}

func prevNonSpace(text []byte, i int) byte {
	for i--; i >= 0; i-- {
		if c := text[i]; c != ' ' && c != '\t' {
			return c
		}
	}
	return 0
}

func Scan(text []byte, in State) State {
	n := len(text)
	i := 0
	if in == InComment {
		end := closeComment(text, 0)
		if end < 0 {
			return InComment
		}
		i = end
	}
	for i < n {
		c := text[i]
		switch {
		case c == '"':
			i = stringEnd(text, i)
		case c == '/' && i+1 < n && text[i+1] == '/':
			return Normal
		case c == '/' && i+1 < n && text[i+1] == '*':
			end := closeComment(text, i+2)
			if end < 0 {
				return InComment
			}
			i = end
		default:
			i++
		}
	}
	return Normal
}

var yieldForms = map[string]bool{"wait": true, "frames": true, "fixed": true}

func classify(text []byte, i, j int, word, prev string) Kind {
	if t := token.Lookup(word); t.IsKeyword() || t.IsReserved() {
		return Keyword
	}
	switch prev {
	case "script":
		return Script
	case "type", "enum":
		return Type
	case "func":
		return Function
	case "package":
		return Namespace
	case "signal":
		return Signal
	case "start":
		return Coroutine
	case "yield":
		if yieldForms[word] {
			return Keyword
		}
	}
	if builtinTypes[word] || word == "Coroutine" || word == "Connection" || word == "ScriptRef" {
		return BuiltinType
	}
	next := skipSpace(text, j)
	call := next < len(text) && text[next] == '('
	if prevNonSpace(text, i) == '.' {
		if call || (next < len(text) && text[next] == '<') {
			return Method
		}
		return Field
	}
	if word == "node" {
		return Builtin
	}
	if word == "start" && next < len(text) && letter(text[next]) {
		return Keyword
	}
	if call {
		if builtinFuncs[word] {
			return Builtin
		}
		return Function
	}
	return Plain
}

func Line(text []byte, in State, dst []Span) ([]Span, State) {
	dst = dst[:0]
	n := len(text)
	i := 0
	if in == InComment {
		end := closeComment(text, 0)
		if end < 0 {
			return append(dst, Span{0, n, Comment}), InComment
		}
		dst = append(dst, Span{0, end, Comment})
		i = end
	}
	prev := ""
	for i < n {
		c := text[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '/' && i+1 < n && text[i+1] == '/':
			return append(dst, Span{i, n, Comment}), Normal
		case c == '/' && i+1 < n && text[i+1] == '*':
			end := closeComment(text, i+2)
			if end < 0 {
				return append(dst, Span{i, n, Comment}), InComment
			}
			dst = append(dst, Span{i, end, Comment})
			i, prev = end, ""
		case c == '"':
			j := stringEnd(text, i)
			dst = append(dst, Span{i, j, String})
			i, prev = j, ""
		case digit(c) || (c == '.' && i+1 < n && digit(text[i+1])):
			j := i + 1
			for j < n && (ident(text[j]) || text[j] == '.') {
				j++
			}
			dst = append(dst, Span{i, j, Number})
			i, prev = j, ""
		case c == '@':
			j := i + 1
			for j < n && ident(text[j]) {
				j++
			}
			dst = append(dst, Span{i, j, Attribute})
			i, prev = j, ""
		case letter(c):
			j := i + 1
			for j < n && ident(text[j]) {
				j++
			}
			word := string(text[i:j])
			dst = append(dst, Span{i, j, classify(text, i, j, word, prev)})
			i, prev = j, word
		case c >= 0x80:
			j := i + 1
			for j < n && text[j] >= 0x80 {
				j++
			}
			dst = append(dst, Span{i, j, Error})
			i, prev = j, ""
		default:
			k := Operator
			if strings.IndexByte(punctuation, c) >= 0 {
				k = Punctuation
			}
			dst = append(dst, Span{i, i + 1, k})
			i++
			prev = ""
		}
	}
	return dst, Normal
}

func overridable(k Kind) bool {
	switch k {
	case Plain, Function, Method, Field:
		return true
	}
	return false
}

func Overlay(spans []Span, lineStart int, sem []Sem) {
	if len(sem) == 0 {
		return
	}
	for i := range spans {
		if !overridable(spans[i].Kind) {
			continue
		}
		abs := lineStart + spans[i].Start
		j := sort.Search(len(sem), func(k int) bool { return sem[k].Start >= abs })
		if j < len(sem) && sem[j].Start == abs && sem[j].End == lineStart+spans[i].End {
			spans[i].Kind = sem[j].Kind
		}
	}
}
