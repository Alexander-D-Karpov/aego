package site

import (
	"html"
	"strings"
)

var aesKeywords = map[string]bool{
	"package": true, "import": true, "script": true, "type": true, "struct": true, "enum": true,
	"var": true, "const": true, "func": true, "return": true, "if": true, "else": true, "for": true,
	"switch": true, "case": true, "default": true, "break": true, "continue": true, "true": true,
	"false": true, "nil": true, "coroutine": true, "start": true, "yield": true, "signal": true,
}

func span(b *strings.Builder, kind, text string) {
	if kind == "" {
		b.WriteString(html.EscapeString(text))
		return
	}
	b.WriteString(`<span class="t-` + kind + `">` + html.EscapeString(text) + `</span>`)
}

func isIdent(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func highlightAES(src string) string {
	var b strings.Builder
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				j = len(src) - i
			}
			span(&b, "comment", src[i:i+j])
			i += j
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			j := strings.Index(src[i+2:], "*/")
			end := len(src)
			if j >= 0 {
				end = i + 2 + j + 2
			}
			span(&b, "comment", src[i:end])
			i = end
		case c == '"' || c == '`':
			j := i + 1
			for j < len(src) && src[j] != c && src[j] != '\n' {
				if c == '"' && src[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(src))
			span(&b, "string", src[i:j])
			i = j
		case c >= '0' && c <= '9':
			j := i
			for j < len(src) && (isIdent(src[j]) || src[j] == '.') {
				j++
			}
			span(&b, "number", src[i:j])
			i = j
		case c == '@' || isIdent(c):
			j := i + 1
			for j < len(src) && isIdent(src[j]) {
				j++
			}
			w := src[i:j]
			switch {
			case c == '@':
				span(&b, "attr", w)
			case aesKeywords[w]:
				span(&b, "keyword", w)
			case w[0] >= 'A' && w[0] <= 'Z':
				span(&b, "type", w)
			default:
				span(&b, "", w)
			}
			i = j
		default:
			span(&b, "", src[i:i+1])
			i++
		}
	}
	return b.String()
}
