package site

import (
	"html"
	"strconv"
	"strings"
	"unicode/utf8"

	"aego/editor/core/code/highlight"
)

const defaultDiagFile = "demo/main.aes"

type diagGroup struct {
	id, title string
	codes     []string
}

// diagnostics renders the code entries under e grouped by phase; false when e has none with examples.
func (b *builder) diagnostics(c *strings.Builder, e *Entry, root string) bool {
	var groups []*diagGroup
	byPhase := map[string]*diagGroup{}
	var rest []string
	for _, k := range b.kids[e.ID] {
		ke := b.byID[k]
		if ke.Kind != KindCode || ke.Diag == nil {
			rest = append(rest, k)
			continue
		}
		g := byPhase[ke.Diag.Phase]
		if g == nil {
			g = &diagGroup{id: "phase-" + slug(ke.Diag.Phase), title: phaseTitle(ke.Diag.Phase)}
			byPhase[ke.Diag.Phase] = g
			groups = append(groups, g)
		}
		g.codes = append(g.codes, k)
	}
	if len(groups) == 0 {
		return false
	}
	c.WriteString(`<ul class="contents diag-index">` + "\n")
	for _, g := range groups {
		c.WriteString(`<li><a href="#` + esc(g.id) + `">` + esc(g.title) + `</a><span class="summary">`)
		for i, k := range g.codes {
			if i > 0 {
				c.WriteString(" · ")
			}
			c.WriteString(`<a href="#` + esc(k) + `">` + esc(k) + "</a>")
		}
		c.WriteString("</span></li>\n")
	}
	c.WriteString("</ul>\n")
	for _, g := range groups {
		c.WriteString(`<h2 id="` + esc(g.id) + `">` + esc(g.title) + "</h2>\n")
		for _, k := range g.codes {
			b.diagCode(c, b.byID[k], root)
		}
	}
	for _, k := range rest {
		b.section(c, k, root, 2)
	}
	return true
}

// phaseTitle names the groups of module codes; script phases keep their own names.
func phaseTitle(p string) string {
	switch p {
	case "scene":
		return "Components"
	case "asset":
		return "Assets"
	}
	return p
}

func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

func (b *builder) diagCode(c *strings.Builder, e *Entry, root string) {
	d := e.Diag
	sev := d.Severity
	if sev != "warning" && sev != "info" {
		sev = "error"
	}
	c.WriteString(`<section class="member diag">` + "\n")
	c.WriteString(`<h3 class="diag-head" id="` + esc(e.ID) + `"><a class="anchor" href="#` + esc(e.ID) + `">` + esc(e.Title) + `</a><span class="sev sev-` + sev + `">` + sev + "</span></h3>\n")
	if e.Summary != "" {
		c.WriteString("<p>" + inlineMD(e.Summary) + "</p>\n")
	}
	if d.Why != "" {
		c.WriteString(`<p class="label">No example</p>` + "\n<p>" + inlineMD(d.Why) + "</p>\n")
	}
	if d.Bad != "" {
		b.diagFiles(c, d, root, false)
		if d.Message != "" {
			cls := "diag-msg"
			if sev == "warning" {
				cls += " warning"
			}
			c.WriteString(`<div class="` + cls + `">` + esc(diagLine(d, e.ID)) + "</div>\n")
		}
	}
	if d.Phase == "scene" || d.Phase == "asset" {
		c.WriteString(b.markdown(root, e.ID, e.Body))
		if d.Example != "" {
			c.WriteString(`<p class="label">Example: ` + esc(d.ExampleFile) + "</p>\n")
			c.WriteString(b.sceneTree(d.Example, root))
		}
	}
	if d.Fix != "" {
		c.WriteString("<p><strong>Fix:</strong> " + inlineMD(d.Fix) + "</p>\n")
	}
	if d.Good != "" {
		b.diagFiles(c, d, root, true)
	}
	c.WriteString("</section>\n")
}

func diagLine(d *DiagInfo, code string) string {
	msg := code + " " + d.Message
	if d.At == "" || d.Line == 0 {
		return msg
	}
	at := d.At + ":" + strconv.Itoa(d.Line)
	if d.Col > 0 {
		at += ":" + strconv.Itoa(d.Col)
	}
	return at + ": " + msg
}

func (b *builder) diagFiles(c *strings.Builder, d *DiagInfo, root string, good bool) {
	label := "Broken"
	if good {
		label = "Fixed"
	}
	type file struct{ path, src string }
	var files []file
	for _, f := range d.Files {
		src := f.Bad
		if good {
			src = f.Good
		}
		files = append(files, file{f.Path, src})
	}
	main := d.Bad
	if good {
		main = d.Good
	}
	files = append(files, file{d.File, main})
	for _, f := range files {
		title := label
		if len(files) > 1 || f.path != defaultDiagFile {
			title += " · " + f.path
		}
		c.WriteString(`<p class="label">` + esc(title) + "</p>\n")
		body := ""
		if !good && f.path == d.At && d.Line > 0 {
			body = highlightMarked(f.src, d.Line, d.Col, d.EndLine, d.EndCol)
		} else {
			body = b.code(root, f.src)
		}
		c.WriteString(`<pre class="code" data-lang="aes"><code class="lang-aes">` + body + "</code></pre>\n")
	}
}

// highlightMarked highlights src and marks line, underlining columns col to endCol (1-based, in runes) when known.
func highlightMarked(src string, line, col, endLine, endCol int) string {
	var b strings.Builder
	var spans []highlight.Span
	state := highlight.Normal
	lines := strings.SplitAfter(src, "\n")
	for i, raw := range lines {
		text := strings.TrimSuffix(raw, "\n")
		spans, state = highlight.Line([]byte(text), state, spans)
		if i+1 != line {
			writeTokens(&b, text, spans, -1, -1)
		} else {
			from, to := -1, -1
			if col > 0 {
				from = byteCol(text, col)
				to = len(text)
				if endLine == line && endCol > col {
					to = byteCol(text, endCol)
				}
				if to <= from && from < len(text) {
					_, n := utf8.DecodeRuneInString(text[from:])
					to = from + n
				}
			}
			b.WriteString(`<span class="hl-line">`)
			writeTokens(&b, text, spans, from, to)
			b.WriteString("</span>")
		}
		if len(text) < len(raw) {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func byteCol(text string, col int) int {
	off := 0
	for n := 1; n < col && off < len(text); n++ {
		_, size := utf8.DecodeRuneInString(text[off:])
		off += size
	}
	return off
}

func writeTokens(b *strings.Builder, text string, spans []highlight.Span, from, to int) {
	cls := make([]string, len(text))
	for _, sp := range spans {
		for i := sp.Start; i < sp.End && i < len(text); i++ {
			cls[i] = tokenClass[sp.Kind]
		}
	}
	under := false
	for i := 0; i < len(text); {
		in := i >= from && i < to
		if in != under {
			if in {
				b.WriteString(`<span class="hl-span">`)
			} else {
				b.WriteString("</span>")
			}
			under = in
		}
		j := i + 1
		for j < len(text) && cls[j] == cls[i] && (j >= from && j < to) == in {
			j++
		}
		chunk := html.EscapeString(text[i:j])
		if cls[i] != "" {
			chunk = `<span class="t-` + cls[i] + `">` + chunk + "</span>"
		}
		b.WriteString(chunk)
		i = j
	}
	if under {
		b.WriteString("</span>")
	}
}
