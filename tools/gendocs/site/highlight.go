package site

import (
	"fmt"
	"html"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/code/highlight"
	"aego/ui/theme"
)

var tokenClass = [highlight.KindCount]string{
	highlight.Plain:       "",
	highlight.Keyword:     "kw",
	highlight.Number:      "num",
	highlight.String:      "str",
	highlight.Comment:     "com",
	highlight.Operator:    "op",
	highlight.Punctuation: "pun",
	highlight.Attribute:   "attr",
	highlight.Namespace:   "ns",
	highlight.Type:        "ty",
	highlight.BuiltinType: "bty",
	highlight.Function:    "fn",
	highlight.Method:      "me",
	highlight.Param:       "par",
	highlight.Variable:    "var",
	highlight.Field:       "fld",
	highlight.Property:    "prop",
	highlight.Constant:    "con",
	highlight.EnumMember:  "en",
	highlight.Builtin:     "bi",
	highlight.Script:      "scr",
	highlight.Error:       "err",
	highlight.Signal:      "sig",
	highlight.Coroutine:   "co",
	highlight.Bracket:     "br",
}

func highlightAES(src string) string {
	return highlightLinked(src, nil)
}

var linkable = map[highlight.Kind]bool{
	highlight.Type: true, highlight.Function: true, highlight.Method: true, highlight.Field: true,
	highlight.Namespace: true, highlight.Script: true, highlight.Signal: true, highlight.Plain: true,
}

var linkAlone = map[highlight.Kind]bool{highlight.Type: true, highlight.Namespace: true, highlight.Script: true}

func highlightLinked(src string, link func(name string) string) string {
	var b strings.Builder
	var spans []highlight.Span
	state := highlight.Normal
	lines := strings.SplitAfter(src, "\n")
	for _, line := range lines {
		text := strings.TrimSuffix(line, "\n")
		spans, state = highlight.Line([]byte(text), state, spans)
		at := 0
		prevWord, prevDot, prevEnd := "", false, -1
		for _, sp := range spans {
			if sp.Start > at {
				b.WriteString(html.EscapeString(text[at:sp.Start]))
			}
			word := text[sp.Start:sp.End]
			chunk := html.EscapeString(word)
			if cls := tokenClass[sp.Kind]; cls != "" {
				chunk = `<span class="t-` + cls + `">` + chunk + "</span>"
			}
			if link != nil && linkable[sp.Kind] && isWord(word) {
				href := ""
				if prevDot && prevWord != "" {
					href = link(prevWord + "." + word)
				}
				alone := linkAlone[sp.Kind] || sp.Kind == highlight.Plain && word[0] >= 'A' && word[0] <= 'Z'
				if href == "" && !prevDot && len(word) > 1 && alone && prevWord != "package" {
					href = link(word)
				}
				if href != "" {
					chunk = `<a class="code-ref" href="` + html.EscapeString(href) + `">` + chunk + "</a>"
				}
			}
			switch {
			case word == "." && sp.Start == prevEnd:
				prevDot = true
			case isWord(word):
				prevWord, prevDot = word, false
			default:
				prevWord, prevDot = "", false
			}
			prevEnd = sp.End
			b.WriteString(chunk)
			at = sp.End
		}
		if at < len(text) {
			b.WriteString(html.EscapeString(text[at:]))
		}
		if len(text) < len(line) {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func hex(c mathx.Color) string {
	to := func(v float32) int {
		return int(mathx.Clamp(mathx.LinearToSRGB(v), 0, 1)*255 + 0.5)
	}
	return fmt.Sprintf("#%02x%02x%02x", to(c.R), to(c.G), to(c.B))
}

func codeCSS() string {
	t := theme.Code()
	pal := highlight.Palette(&t)
	var b strings.Builder
	b.WriteString("/* generated from the editor's code theme (ui/theme.Code) */\n")
	b.WriteString(":root {\n    --code-bg: " + hex(t.Background) + ";\n    --code-gutter: " + hex(t.GutterBg) + ";\n    --code-text: " + hex(t.Plain) + ";\n    --code-line: " + hex(t.LineNo) + ";\n    --code-error: " + hex(t.Error) + ";\n}\n")
	for k, cls := range tokenClass {
		if cls == "" {
			continue
		}
		b.WriteString(".t-" + cls + " { color: " + hex(pal[k]) + "; }\n")
	}
	b.WriteString(".t-com { font-style: italic; }\n")
	return b.String()
}

func isWord(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return !(s[0] >= '0' && s[0] <= '9')
}
