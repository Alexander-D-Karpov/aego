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

// lightTokens is the code palette for the docs' light theme; the dark one is the editor's.
var lightTokens = map[string]string{
	"kw": "#a626a4", "num": "#986801", "str": "#3f7f3a", "com": "#80838c", "op": "#4b4f58", "pun": "#6a6f7a",
	"attr": "#946200", "ns": "#b4510f", "ty": "#0b7285", "bty": "#0b7285", "fn": "#2459c7", "me": "#2459c7",
	"par": "#a3530a", "var": "#383a42", "fld": "#c0392b", "prop": "#c0392b", "con": "#986801", "en": "#986801",
	"bi": "#b0257a", "scr": "#0b7285", "err": "#d1242f", "sig": "#b4510f", "co": "#0f766e", "br": "#6a6f7a",
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

func pageBackground() string {
	return hex(theme.Editor().Background)
}

func cssColor(c mathx.Color) string {
	if c.A >= 1 {
		return hex(c)
	}
	h := hex(c)
	return fmt.Sprintf("rgb(%s %s %s / %.2f)", fmt.Sprint(hexByte(h[1:3])), fmt.Sprint(hexByte(h[3:5])), fmt.Sprint(hexByte(h[5:7])), c.A)
}

func hexByte(s string) int {
	var v int
	fmt.Sscanf(s, "%02x", &v)
	return v
}

func codeCSS() string {
	t := theme.Code()
	pal := highlight.Palette(&t)
	var b strings.Builder
	b.WriteString("/* generated from the editor's themes (ui/theme.Code and ui/theme.Editor) */\n")
	ld := func(light, dark string) string { return "light-dark(" + light + ", " + dark + ")" }
	b.WriteString(":root {\n    --code-bg: " + ld("#f5f6f8", hex(t.Background)) + ";\n    --code-gutter: " + ld("#eceef1", hex(t.GutterBg)) + ";\n    --code-text: " + ld("#383a42", hex(t.Plain)) + ";\n    --code-line: " + ld("#9aa0a9", hex(t.LineNo)) + ";\n    --code-error: " + ld("#d1242f", hex(t.Error)) + ";\n")
	e := theme.Editor()
	for _, v := range []struct {
		name string
		c    mathx.Color
	}{
		{"bg", e.Background}, {"header", e.Header}, {"panel", e.Panel}, {"panel-alt", e.PanelAlt}, {"hover", e.TabHot}, {"widget", e.Widget},
		{"border", e.Border}, {"separator", e.Separator}, {"text", e.Text}, {"text-dim", e.TextDim}, {"unit", e.Unit},
		{"accent", e.Accent}, {"error", e.Error}, {"warn", e.Warn}, {"ok", e.Ok},
		{"axis-x", e.AxisX}, {"axis-y", e.AxisY}, {"axis-z", e.AxisZ},
	} {
		b.WriteString("    --ed-" + v.name + ": " + cssColor(v.c) + ";\n")
	}
	b.WriteString("}\n")
	for k, cls := range tokenClass {
		if cls == "" {
			continue
		}
		b.WriteString(".t-" + cls + " { color: " + ld(lightTokens[cls], hex(pal[k])) + "; }\n")
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
