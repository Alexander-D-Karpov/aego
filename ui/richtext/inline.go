package richtext

import "strings"

func push(out []Span, text string, st Style, target string) []Span {
	if text == "" {
		return out
	}
	if n := len(out); n > 0 && out[n-1].Style == st && out[n-1].Target == target {
		out[n-1].Text += text
		return out
	}
	return append(out, Span{Text: text, Style: st, Target: target})
}

func wordChar(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func edge(s string, i int, closing bool) bool {
	if closing {
		return i > 0 && s[i-1] != ' '
	}
	if i+1 >= len(s) || s[i+1] == ' ' {
		return false
	}
	return s[i] == '*' || i == 0 || !wordChar(s[i-1])
}

func Inline(s string) []Span {
	return inline(s, "")
}

func inline(s, target string) []Span {
	var out []Span
	var cur strings.Builder
	strong, em := false, false
	style := func() Style {
		switch {
		case strong:
			return Strong
		case em:
			return Emphasis
		}
		return Plain
	}
	emit := func() {
		out = push(out, cur.String(), style(), target)
		cur.Reset()
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && strings.IndexByte("\\`*_[]()#|<>-!", s[i+1]) >= 0:
			cur.WriteByte(s[i+1])
			i++
		case c == '`':
			j := strings.IndexByte(s[i+1:], '`')
			if j < 0 {
				cur.WriteByte(c)
				continue
			}
			emit()
			out = push(out, s[i+1:i+1+j], Code, target)
			i += j + 1
		case c == '[':
			j := strings.Index(s[i:], "](")
			if j < 0 {
				cur.WriteByte(c)
				continue
			}
			k := strings.IndexByte(s[i+j+2:], ')')
			if k < 0 {
				cur.WriteByte(c)
				continue
			}
			emit()
			label, dest := s[i+1:i+j], s[i+j+2:i+j+2+k]
			for _, sp := range inline(label, dest) {
				out = push(out, sp.Text, sp.Style, sp.Target)
			}
			i += j + 2 + k
		case c == '*' && i+1 < len(s) && s[i+1] == '*':
			emit()
			strong = !strong
			i++
		case (c == '*' || c == '_') && edge(s, i, em):
			emit()
			em = !em
		default:
			cur.WriteByte(c)
		}
	}
	emit()
	return out
}

func PlainText(spans []Span) string {
	var b strings.Builder
	for _, sp := range spans {
		b.WriteString(sp.Text)
	}
	return b.String()
}
