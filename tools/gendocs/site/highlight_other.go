package site

import (
	"html"
	"regexp"
	"strings"
)

// guessLang names the language of an unlabelled block from its content.
func guessLang(code string) string {
	first := strings.TrimSpace(strings.SplitN(strings.TrimLeft(code, "\n"), "\n", 2)[0])
	switch {
	case strings.HasPrefix(first, "$ ") || strings.HasPrefix(first, "aego ") || strings.HasPrefix(first, "make ") ||
		strings.HasPrefix(first, "go ") || strings.HasPrefix(first, "curl ") || strings.HasPrefix(first, "git ") ||
		strings.HasPrefix(first, "claude "):
		return "sh"
	case strings.HasPrefix(first, "{\"") || strings.HasPrefix(first, "[{"):
		return "aetext"
	case aesLike.MatchString(code):
		return "aes"
	case strings.HasSuffix(first, "{") || strings.Contains(first, "="):
		return "aetext"
	}
	return ""
}

var aesLike = regexp.MustCompile(`(?m)^\s*(package|import|script|func|var|const|@[a-z]+|if|for|switch|return|yield)\b|:=|\bnode\.|\)\s*\{\s*$|^\s*[a-z]\w*\.[A-Z]\w*[\[(]`)

func span(cls, text string) string {
	return `<span class="t-` + cls + `">` + html.EscapeString(text) + "</span>"
}

func wordAt(s string, i int) int {
	j := i
	for j < len(s) && (isWordByte(s[j]) || s[j] == '.' || s[j] == '-' && j > i) {
		j++
	}
	return j
}

func isWordByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func quoted(s string, i int) int {
	q := s[i]
	j := i + 1
	for j < len(s) && s[j] != q {
		if s[j] == '\\' {
			j++
		}
		j++
	}
	return min(j+1, len(s))
}

// highlightAEText colours scene, project and settings files: block heads, keys, strings, numbers, colours and refs.
func highlightAEText(src string) string {
	var b strings.Builder
	for n, line := range strings.Split(src, "\n") {
		if n > 0 {
			b.WriteByte('\n')
		}
		head := true
		for i := 0; i < len(line); {
			c := line[i]
			switch {
			case c == '#' && i+1 < len(line) && isHex(line[i+1]):
				j := i + 1
				for j < len(line) && isHex(line[j]) {
					j++
				}
				b.WriteString(span("num", line[i:j]))
				i = j
			case c == '/' && i+1 < len(line) && line[i+1] == '/', c == '#':
				b.WriteString(span("com", line[i:]))
				i = len(line)
			case c == '"':
				j := quoted(line, i)
				b.WriteString(span("str", line[i:j]))
				i = j
			case c >= '0' && c <= '9' || c == '-' && i+1 < len(line) && line[i+1] >= '0' && line[i+1] <= '9':
				j := i + 1
				for j < len(line) && (line[j] >= '0' && line[j] <= '9' || line[j] == '.' || line[j] == 'e') {
					j++
				}
				b.WriteString(span("num", line[i:j]))
				i = j
			case isWordByte(c):
				j := wordAt(line, i)
				w := line[i:j]
				k := j
				for k < len(line) && line[k] == ' ' {
					k++
				}
				switch {
				case k < len(line) && line[k] == '=':
					b.WriteString(span("fld", w))
				case k < len(line) && line[k] == '(':
					b.WriteString(span("fn", w))
				case head:
					b.WriteString(span("kw", w))
				case w == "true" || w == "false":
					b.WriteString(span("kw", w))
				default:
					b.WriteString(span("en", w))
				}
				head = false
				i = j
			default:
				if c != ' ' && c != '\t' {
					head = c == '{' || c == '}'
				}
				b.WriteString(html.EscapeString(string(c)))
				i++
			}
		}
	}
	return b.String()
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// highlightShell colours commands, flags, strings and comments of shell examples.
func highlightShell(src string) string {
	var b strings.Builder
	for n, line := range strings.Split(src, "\n") {
		if n > 0 {
			b.WriteByte('\n')
		}
		cmd := true
		for i := 0; i < len(line); {
			c := line[i]
			switch {
			case c == '#' && (i == 0 || line[i-1] == ' '):
				b.WriteString(span("com", line[i:]))
				i = len(line)
			case c == '"' || c == '\'':
				j := quoted(line, i)
				b.WriteString(span("str", line[i:j]))
				i = j
			case c == '|' || c == '&' || c == ';':
				b.WriteString(span("op", string(c)))
				cmd = true
				i++
			case c == '-' && (i == 0 || line[i-1] == ' '):
				j := i
				for j < len(line) && line[j] != ' ' && line[j] != '=' {
					j++
				}
				b.WriteString(span("attr", line[i:j]))
				i = j
			case c != ' ' && c != '\\':
				j := i
				for j < len(line) && line[j] != ' ' && line[j] != '|' && line[j] != ';' {
					j++
				}
				if cmd {
					b.WriteString(span("fn", line[i:j]))
					cmd = false
				} else {
					b.WriteString(html.EscapeString(line[i:j]))
				}
				i = j
			default:
				b.WriteString(html.EscapeString(string(c)))
				i++
			}
		}
	}
	return b.String()
}
