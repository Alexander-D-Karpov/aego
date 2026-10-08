package lexer

import (
	"unicode/utf8"

	"aego/script/diag"
	"aego/script/token"
)

func (l *Lexer) str(start int) token.Token {
	l.off++
	l.buf = l.buf[:0]
	for {
		if l.off >= len(l.src) {
			l.errorf(diag.UnterminatedString, start, l.off, "string literal is not terminated")
			break
		}
		c := l.src[l.off]
		if c == '"' {
			l.off++
			break
		}
		if c == '\n' {
			l.errorf(diag.NewlineInString, start, l.off, "newline in string literal")
			break
		}
		if c == '\\' {
			l.escape()
			continue
		}
		if c < utf8.RuneSelf {
			l.buf = append(l.buf, c)
			l.off++
			continue
		}
		r, size := utf8.DecodeRune(l.src[l.off:])
		if r == utf8.RuneError && size == 1 {
			l.errorf(diag.InvalidUTF8, l.off, l.off+1, "invalid UTF-8 encoding in string literal")
		}
		l.buf = append(l.buf, l.src[l.off:l.off+size]...)
		l.off += size
	}
	return l.emit(token.String, start, string(l.src[start:l.off]), string(l.buf))
}

func simpleEscape(c byte) (byte, bool) {
	switch c {
	case 'n':
		return '\n', true
	case 't':
		return '\t', true
	case '"':
		return '"', true
	case '\\':
		return '\\', true
	}
	return 0, false
}

func (l *Lexer) escape() {
	start := l.off
	l.off++
	if l.off >= len(l.src) || l.src[l.off] == '\n' {
		l.errorf(diag.InvalidEscape, start, l.off, "escape sequence is not terminated")
		return
	}
	c := l.src[l.off]
	if e, ok := simpleEscape(c); ok {
		l.buf = append(l.buf, e)
		l.off++
		return
	}
	if c != 'u' {
		r, size := utf8.DecodeRune(l.src[l.off:])
		l.off += size
		l.errorf(diag.InvalidEscape, start, l.off, "unknown escape sequence \\%c", r)
		return
	}
	l.off++
	var r rune
	for i := 0; i < 4; i++ {
		d := hexVal(l.at(0))
		if d < 0 || l.off >= len(l.src) {
			l.errorf(diag.InvalidEscape, start, l.off, "\\u needs four hexadecimal digits")
			return
		}
		r = r<<4 | rune(d)
		l.off++
	}
	if r >= 0xD800 && r <= 0xDFFF {
		l.errorf(diag.InvalidEscape, start, l.off, "escape \\u%04X is a surrogate half", r)
		return
	}
	l.buf = utf8.AppendRune(l.buf, r)
}
