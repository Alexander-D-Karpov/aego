package aetext

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

type tokenKind uint8

const (
	tokEOF tokenKind = iota
	tokIdent
	tokString
	tokInt
	tokFloat
	tokColor
	tokPunct
)

type token struct {
	kind  tokenKind
	text  string
	i     int64
	f     float64
	color uint32
	pos   Pos
}

type lexer struct {
	src  []byte
	name string
	off  int
	line int
	col  int
}

func newLexer(src []byte, name string) *lexer {
	return &lexer{src: src, name: name, line: 1, col: 1}
}

func (l *lexer) pos() Pos {
	return Pos{Line: l.line, Col: l.col}
}

func (l *lexer) errorf(p Pos, format string, args ...any) *Diag {
	return &Diag{Name: l.name, Pos: p, Msg: sprintf(format, args...)}
}

func (l *lexer) advance(n int) {
	for i := 0; i < n && l.off < len(l.src); i++ {
		if l.src[l.off] == '\n' {
			l.line++
			l.col = 1
		} else {
			l.col++
		}
		l.off++
	}
}

func (l *lexer) skipSpace() {
	for l.off < len(l.src) {
		c := l.src[l.off]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			l.advance(1)
			continue
		}
		return
	}
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || c == '.' || (c >= '0' && c <= '9')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isHex(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func (l *lexer) next() (token, *Diag) {
	l.skipSpace()
	p := l.pos()
	if l.off >= len(l.src) {
		return token{kind: tokEOF, pos: p}, nil
	}
	c := l.src[l.off]

	switch c {
	case '=', '{', '}', '[', ']', ',', '(', ')':
		l.advance(1)
		return token{kind: tokPunct, text: string(c), pos: p}, nil
	case '"':
		return l.lexString(p)
	case '#':
		return l.lexColor(p)
	}
	if isIdentStart(c) {
		start := l.off
		for l.off < len(l.src) && isIdentPart(l.src[l.off]) {
			l.advance(1)
		}
		return token{kind: tokIdent, text: string(l.src[start:l.off]), pos: p}, nil
	}
	if isDigit(c) || c == '-' || c == '+' {
		return l.lexNumber(p)
	}
	return token{}, l.errorf(p, "unexpected character %q", string(c))
}

func (l *lexer) lexNumber(p Pos) (token, *Diag) {
	start := l.off
	if l.off < len(l.src) && (l.src[l.off] == '-' || l.src[l.off] == '+') {
		l.advance(1)
	}
	digits := 0
	for l.off < len(l.src) && isDigit(l.src[l.off]) {
		l.advance(1)
		digits++
	}
	isFloat := false
	if l.off < len(l.src) && l.src[l.off] == '.' {
		isFloat = true
		l.advance(1)
		for l.off < len(l.src) && isDigit(l.src[l.off]) {
			l.advance(1)
			digits++
		}
	}
	if l.off < len(l.src) && (l.src[l.off] == 'e' || l.src[l.off] == 'E') {
		isFloat = true
		l.advance(1)
		if l.off < len(l.src) && (l.src[l.off] == '-' || l.src[l.off] == '+') {
			l.advance(1)
		}
		for l.off < len(l.src) && isDigit(l.src[l.off]) {
			l.advance(1)
		}
	}
	if digits == 0 {
		return token{}, l.errorf(p, "malformed number")
	}
	text := string(l.src[start:l.off])
	if isFloat {
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return token{}, l.errorf(p, "malformed float %q", text)
		}
		return token{kind: tokFloat, text: text, f: f, pos: p}, nil
	}
	i, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return token{}, l.errorf(p, "integer out of range %q", text)
	}
	return token{kind: tokInt, text: text, i: i, pos: p}, nil
}

func (l *lexer) lexColor(p Pos) (token, *Diag) {
	l.advance(1)
	start := l.off
	for l.off < len(l.src) && isHex(l.src[l.off]) {
		l.advance(1)
	}
	text := string(l.src[start:l.off])
	if len(text) != 8 {
		return token{}, l.errorf(p, "color must be 8 hex digits, got %d", len(text))
	}
	v, err := strconv.ParseUint(text, 16, 32)
	if err != nil {
		return token{}, l.errorf(p, "malformed color")
	}
	return token{kind: tokColor, text: text, color: uint32(v), pos: p}, nil
}

func (l *lexer) lexString(p Pos) (token, *Diag) {
	l.advance(1)
	var sb strings.Builder
	for {
		if l.off >= len(l.src) {
			return token{}, l.errorf(p, "unterminated string")
		}
		c := l.src[l.off]
		if c == '\n' {
			return token{}, l.errorf(l.pos(), "newline inside string")
		}
		if c == '"' {
			l.advance(1)
			return token{kind: tokString, text: sb.String(), pos: p}, nil
		}
		if c != '\\' {
			sb.WriteByte(c)
			l.advance(1)
			continue
		}
		esc := l.pos()
		l.advance(1)
		if l.off >= len(l.src) {
			return token{}, l.errorf(esc, "unterminated escape")
		}
		switch l.src[l.off] {
		case '"':
			sb.WriteByte('"')
			l.advance(1)
		case '\\':
			sb.WriteByte('\\')
			l.advance(1)
		case 'n':
			sb.WriteByte('\n')
			l.advance(1)
		case 't':
			sb.WriteByte('\t')
			l.advance(1)
		case 'u':
			l.advance(1)
			if l.off+4 > len(l.src) {
				return token{}, l.errorf(esc, "short \\u escape")
			}
			hexDigits := string(l.src[l.off : l.off+4])
			v, err := strconv.ParseUint(hexDigits, 16, 32)
			if err != nil {
				return token{}, l.errorf(esc, "malformed \\u escape")
			}
			l.advance(4)
			var buf [4]byte
			n := utf8.EncodeRune(buf[:], rune(v))
			sb.Write(buf[:n])
		default:
			return token{}, l.errorf(esc, "unknown escape \\%s", string(l.src[l.off]))
		}
	}
}
