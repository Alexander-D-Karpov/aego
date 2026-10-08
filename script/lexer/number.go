package lexer

import (
	"fmt"
	"strconv"
	"strings"

	"aego/script/diag"
	"aego/script/token"
)

func (l *Lexer) number(start int) token.Token {
	kind := token.Int
	hex := false
	bad := ""
	if l.at(0) == '0' && (l.at(1) == 'x' || l.at(1) == 'X') {
		hex = true
		l.off += 2
		if l.run(isHexOrUnderscore) == 0 {
			bad = "hexadecimal literal has no digits"
		}
	} else {
		l.run(isDigitOrUnderscore)
		if l.at(0) == '.' && isDigit(l.at(1)) {
			kind = token.Float
			l.off++
			l.run(isDigitOrUnderscore)
		} else if l.at(0) == '.' && !isLetter(l.at(1)) {
			kind = token.Float
			l.off++
			bad = "float literal needs digits after the point, write 1.0"
		}
		if c := l.at(0); c == 'e' || c == 'E' {
			kind = token.Float
			l.off++
			if c := l.at(0); c == '+' || c == '-' {
				l.off++
			}
			if l.run(isDigitOrUnderscore) == 0 {
				bad = "exponent has no digits"
			}
		}
	}
	if isIdentPart(l.at(0)) {
		sfx := l.off
		l.run(isIdentPart)
		if bad == "" {
			bad = fmt.Sprintf("invalid suffix %q on number", l.src[sfx:l.off])
		}
	}
	text := string(l.src[start:l.off])
	if bad == "" && !validUnderscores(text, hex) {
		bad = "'_' must separate successive digits"
	}
	if bad != "" {
		l.errorf(diag.MalformedNumber, start, l.off, "%s", bad)
	}
	return l.emit(kind, start, text, "")
}

func ParseInt(text string) (uint64, bool) {
	s := strings.ReplaceAll(text, "_", "")
	base := 10
	if len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		s, base = s[2:], 16
	}
	v, err := strconv.ParseUint(s, base, 64)
	return v, err == nil
}

func ParseFloat(text string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.ReplaceAll(text, "_", ""), 64)
	return v, err == nil
}

func validUnderscores(s string, hex bool) bool {
	start := 0
	if hex {
		start = 2
	}
	digit := isDigit
	if hex {
		digit = isHex
	}
	for i := start; i < len(s); i++ {
		if s[i] != '_' {
			continue
		}
		if i == start || i == len(s)-1 || !digit(s[i-1]) || !digit(s[i+1]) {
			return false
		}
	}
	return true
}
