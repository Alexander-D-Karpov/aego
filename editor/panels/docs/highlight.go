package docs

import "aego/ui/richtext"

var keywords = map[string]bool{
	"package": true, "import": true, "script": true, "type": true, "struct": true, "enum": true,
	"var": true, "const": true, "func": true, "return": true, "if": true, "else": true, "for": true,
	"switch": true, "case": true, "default": true, "break": true, "continue": true, "true": true,
	"false": true, "nil": true, "coroutine": true, "start": true, "yield": true, "signal": true,
}

func identByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func Highlight(lang, src string) []richtext.Token {
	if lang != "aes" && lang != "" {
		return nil
	}
	var out []richtext.Token
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			out = append(out, richtext.Token{Start: i, End: len(src), Kind: "comment"})
			return out
		case c == '"' || c == '`':
			j := i + 1
			for j < len(src) && src[j] != c {
				if c == '"' && src[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(src))
			out = append(out, richtext.Token{Start: i, End: j, Kind: "string"})
			i = j
		case c >= '0' && c <= '9':
			j := i
			for j < len(src) && (identByte(src[j]) || src[j] == '.') {
				j++
			}
			out = append(out, richtext.Token{Start: i, End: j, Kind: "number"})
			i = j
		case c == '@' || identByte(c):
			j := i + 1
			for j < len(src) && identByte(src[j]) {
				j++
			}
			w := src[i:j]
			switch {
			case c == '@':
				out = append(out, richtext.Token{Start: i, End: j, Kind: "attr"})
			case keywords[w]:
				out = append(out, richtext.Token{Start: i, End: j, Kind: "keyword"})
			case w[0] >= 'A' && w[0] <= 'Z':
				out = append(out, richtext.Token{Start: i, End: j, Kind: "type"})
			}
			i = j
		default:
			i++
		}
	}
	return out
}
