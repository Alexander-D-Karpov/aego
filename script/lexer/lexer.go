package lexer

import (
	"bytes"
	"unicode/utf8"

	"aego/script/diag"
	"aego/script/source"
	"aego/script/token"
)

var (
	blockEnd = []byte("*/")
	bom      = []byte{0xEF, 0xBB, 0xBF}
)

type alt struct {
	c byte
	k token.Kind
}

type Lexer struct {
	file     source.FileID
	src      []byte
	off      int
	semi     bool
	diags    []diag.Diagnostic
	buf      []byte
	in       *source.Interner
	comments []Comment
}

func New(file source.FileID, src []byte, in *source.Interner) *Lexer {
	if in == nil {
		in = source.NewInterner()
	}
	l := &Lexer{file: file, src: src, in: in}
	if bytes.HasPrefix(src, bom) {
		l.off = len(bom)
	}
	return l
}

func Tokenize(file source.FileID, src []byte) ([]token.Token, []diag.Diagnostic) {
	return TokenizeIn(file, src, source.NewInterner())
}

func TokenizeIn(file source.FileID, src []byte, in *source.Interner) ([]token.Token, []diag.Diagnostic) {
	l := New(file, src, in)
	out := make([]token.Token, 0, 16+len(src)/4)
	for {
		t := l.Next()
		out = append(out, t)
		if t.Kind == token.EOF {
			return out, l.diags
		}
	}
}

func (l *Lexer) Diagnostics() []diag.Diagnostic {
	return l.diags
}

func (l *Lexer) at(i int) byte {
	if j := l.off + i; j < len(l.src) {
		return l.src[j]
	}
	return 0
}

func (l *Lexer) run(pred func(byte) bool) int {
	start := l.off
	for l.off < len(l.src) && pred(l.src[l.off]) {
		l.off++
	}
	return l.off - start
}

func (l *Lexer) intern(b []byte) string {
	return l.in.Intern(b)
}

func (l *Lexer) errorf(code string, start, end int, format string, args ...any) {
	sp := source.Span{File: l.file, Start: uint32(start), End: uint32(end)}
	l.diags = append(l.diags, diag.Errorf(code, sp, format, args...))
}

func (l *Lexer) auto(at int) token.Token {
	l.semi = false
	return token.Token{Kind: token.Semicolon, Start: uint32(at), End: uint32(at), Text: "\n"}
}

func endsStatement(k token.Kind) bool {
	switch k {
	case token.Ident, token.Int, token.Float, token.String,
		token.True, token.False, token.Nil,
		token.RParen, token.RBrack, token.RBrace,
		token.Return, token.Break, token.Continue, token.Yield,
		token.Inc, token.Dec:
		return true
	}
	return false
}

func (l *Lexer) emit(k token.Kind, start int, text, value string) token.Token {
	l.semi = endsStatement(k)
	return token.Token{Kind: k, Start: uint32(start), End: uint32(l.off), Text: text, Value: value}
}

func (l *Lexer) Next() token.Token {
	for l.off < len(l.src) {
		c := l.src[l.off]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			l.off++
		case c == '\n':
			if l.semi {
				t := l.auto(l.off)
				l.off++
				return t
			}
			l.off++
		case c == '/' && l.at(1) == '/':
			start := l.off
			for l.off < len(l.src) && l.src[l.off] != '\n' {
				l.off++
			}
			l.comments = append(l.comments, Comment{Start: uint32(start), End: uint32(l.off), Line: true})
		case c == '/' && l.at(1) == '*':
			start := l.off
			end := bytes.Index(l.src[l.off+2:], blockEnd)
			if end < 0 {
				l.errorf(diag.UnterminatedComment, start, start+2, "block comment is not terminated")
				l.off = len(l.src)
				l.comments = append(l.comments, Comment{Start: uint32(start), End: uint32(l.off)})
				continue
			}
			body := l.src[l.off+2 : l.off+2+end]
			l.off += end + 4
			l.comments = append(l.comments, Comment{Start: uint32(start), End: uint32(l.off)})
			if l.semi && bytes.IndexByte(body, '\n') >= 0 {
				return l.auto(start)
			}
		default:
			return l.scan()
		}
	}
	if l.semi {
		return l.auto(len(l.src))
	}
	return token.Token{Kind: token.EOF, Start: uint32(len(l.src)), End: uint32(len(l.src))}
}

func (l *Lexer) choose(def token.Kind, alts ...alt) token.Kind {
	if l.off < len(l.src) {
		for _, a := range alts {
			if l.src[l.off] == a.c {
				l.off++
				return a.k
			}
		}
	}
	return def
}

func (l *Lexer) scan() token.Token {
	start := l.off
	c := l.src[start]
	switch {
	case isLetter(c):
		l.run(isIdentPart)
		text := l.intern(l.src[start:l.off])
		k := token.Lookup(text)
		if k.IsReserved() {
			l.errorf(diag.ReservedWord, start, l.off, "%q is reserved for a future version of Aego Script", text)
			k = token.Ident
		}
		return l.emit(k, start, text, "")
	case isDigit(c) || (c == '.' && isDigit(l.at(1))):
		return l.number(start)
	case c == '"':
		return l.str(start)
	case c == '\'':
		l.off++
		for l.off < len(l.src) && l.src[l.off] != '\'' && l.src[l.off] != '\n' {
			l.off++
		}
		if l.off < len(l.src) && l.src[l.off] == '\'' {
			l.off++
		}
		l.errorf(diag.UnexpectedChar, start, l.off, "character literals are not supported, use a string")
		return l.emit(token.Illegal, start, string(l.src[start:l.off]), "")
	case c >= utf8.RuneSelf:
		r, size := utf8.DecodeRune(l.src[start:])
		l.off += size
		if r == utf8.RuneError && size == 1 {
			l.errorf(diag.InvalidUTF8, start, l.off, "invalid UTF-8 encoding")
		} else {
			l.errorf(diag.UnexpectedChar, start, l.off, "identifiers may use ASCII letters only; %q is not allowed", r)
		}
		return l.emit(token.Illegal, start, string(l.src[start:l.off]), "")
	}
	l.off++
	k := token.Illegal
	switch c {
	case '+':
		k = l.choose(token.Add, alt{'+', token.Inc}, alt{'=', token.AddAssign})
	case '-':
		k = l.choose(token.Sub, alt{'-', token.Dec}, alt{'=', token.SubAssign})
	case '*':
		k = l.choose(token.Mul, alt{'=', token.MulAssign})
	case '/':
		k = l.choose(token.Quo, alt{'=', token.QuoAssign})
	case '%':
		k = l.choose(token.Rem, alt{'=', token.RemAssign})
	case '=':
		k = l.choose(token.Assign, alt{'=', token.Eq})
	case '!':
		k = l.choose(token.Not, alt{'=', token.Ne})
	case '<':
		k = l.choose(token.Lt, alt{'=', token.Le})
	case '>':
		k = l.choose(token.Gt, alt{'=', token.Ge})
	case ':':
		k = l.choose(token.Colon, alt{'=', token.Define})
	case '&':
		k = l.choose(token.Illegal, alt{'&', token.AndAnd})
	case '|':
		k = l.choose(token.Illegal, alt{'|', token.OrOr})
	case '.':
		k = token.Period
	case ',':
		k = token.Comma
	case ';':
		k = token.Semicolon
	case '(':
		k = token.LParen
	case ')':
		k = token.RParen
	case '[':
		k = token.LBrack
	case ']':
		k = token.RBrack
	case '{':
		k = token.LBrace
	case '}':
		k = token.RBrace
	case '@':
		k = token.At
	}
	if k == token.Illegal {
		text := string(l.src[start:l.off])
		if c == '&' || c == '|' {
			l.errorf(diag.UnexpectedChar, start, l.off, "operator %q is not supported, Aego Script has no bitwise operators", text)
		} else {
			l.errorf(diag.UnexpectedChar, start, l.off, "unexpected character %q", text)
		}
		return l.emit(token.Illegal, start, text, "")
	}
	return l.emit(k, start, k.String(), "")
}
