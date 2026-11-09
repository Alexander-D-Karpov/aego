package site

import (
	"bytes"
	"html"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type pageCtx struct {
	root        string
	id          string
	runs        int
	examples    int
	play        bool
	resolve     func(id string) (string, bool)
	autoResolve func(id string) (string, bool)
	broken      func(target string)
}

type converter struct {
	md  goldmark.Markdown
	cur *pageCtx
}

func newConverter() *converter {
	c := &converter{}
	c.md = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(util.Prioritized(refTransformer{c}, 100)),
		),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(util.Prioritized(codeRenderer{c}, 100)),
		),
	)
	return c
}

func (c *converter) render(p *pageCtx, md string) (string, error) {
	c.cur = p
	var buf bytes.Buffer
	if err := c.md.Convert([]byte(md), &buf); err != nil {
		return "", err
	}
	return tableCode.ReplaceAllStringFunc(buf.String(), func(t string) string {
		return inlineCode.ReplaceAllStringFunc(t, func(c string) string {
			src := html.UnescapeString(inlineCode.FindStringSubmatch(c)[1])
			return `<code class="hl">` + highlightAES(src) + "</code>"
		})
	}), nil
}

var (
	tableCode           = regexp.MustCompile(`(?s)<table>.*?</table>`)
	inlineCode          = regexp.MustCompile(`<code>([^<]*)</code>`)
	diagnosticReference = regexp.MustCompile(`^[A-Z]{2,}[0-9]{3,}$`)
)

type refTransformer struct {
	c *converter
}

func codeText(n ast.Node, src []byte) string {
	var b strings.Builder
	for k := n.FirstChild(); k != nil; k = k.NextSibling() {
		if t, ok := k.(*ast.Text); ok {
			b.Write(t.Segment.Value(src))
		}
	}
	return b.String()
}

func (t refTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	p := t.c.cur
	src := reader.Source()
	var spans []*ast.CodeSpan
	var titles []*ast.Heading
	// The page template owns h1. Registry entries can append a complete
	// article after a schema comment, so removing only a leading title is
	// insufficient. Keep its existing anchor without repeating the page title.
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch x := n.(type) {
		case *ast.Heading:
			if x.Level == 1 {
				titles = append(titles, x)
				return ast.WalkSkipChildren, nil
			}
		case *ast.Link:
			if id, ok := strings.CutPrefix(string(x.Destination), "ref:"); ok {
				if u, ok := p.resolve(id); ok {
					x.Destination = []byte(p.root + u)
				} else {
					p.broken(id)
					x.Destination = []byte("#")
				}
			}
			return ast.WalkSkipChildren, nil
		case *ast.Image:
			if rest, ok := strings.CutPrefix(string(x.Destination), "shots/"); ok {
				x.Destination = []byte(p.root + shotsDir + rest)
			}
			return ast.WalkSkipChildren, nil
		case *ast.CodeSpan:
			spans = append(spans, x)
		}
		return ast.WalkContinue, nil
	})
	for i, title := range titles {
		id := p.id + "-article-" + strconv.Itoa(i+1)
		if value, ok := title.AttributeString("id"); ok {
			if raw, ok := value.([]byte); ok {
				id = string(raw)
			}
		}
		parent := title.Parent()
		parent.ReplaceChild(parent, title, &pageAnchor{id: id})
	}
	for _, s := range spans {
		name := strings.TrimSuffix(codeText(s, src), "()")
		// A bare name may be a node, local variable, enum or field. Without
		// symbol resolution, only explicit technical IDs are safe to link.
		if !qualifiedReference(name) || p.autoResolve == nil {
			continue
		}
		u, ok := p.autoResolve(name)
		if !ok {
			continue
		}
		parent := s.Parent()
		link := ast.NewLink()
		link.Destination = []byte(p.root + u)
		parent.ReplaceChild(parent, s, link)
		link.AppendChild(link, s)
	}
}

var kindPageAnchor = ast.NewNodeKind("PageAnchor")

type pageAnchor struct {
	ast.BaseBlock
	id string
}

func (*pageAnchor) Kind() ast.NodeKind              { return kindPageAnchor }
func (n *pageAnchor) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

func qualifiedReference(name string) bool {
	if name == "" || strings.ContainsAny(name, " \t\n\"'<>[]{}") {
		return false
	}
	return diagnosticReference.MatchString(name) || strings.Contains(name, "/") || strings.Contains(name, ".") && name[0] >= 'a' && name[0] <= 'z'
}

type codeRenderer struct {
	c *converter
}

func (r codeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindPageAnchor, r.anchor)
	reg.Register(ast.KindFencedCodeBlock, r.fenced)
	reg.Register(ast.KindCodeBlock, r.indented)
}

func (r codeRenderer) anchor(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		w.WriteString(`<span class="legacy-anchor" id="` + html.EscapeString(n.(*pageAnchor).id) + `"></span>` + "\n")
	}
	return ast.WalkSkipChildren, nil
}

func (r codeRenderer) indented(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	var code strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		code.Write(seg.Value(src))
	}
	w.WriteString(snippet(r.c.cur, "", code.String(), false, ""))
	return ast.WalkSkipChildren, nil
}

func (r codeRenderer) fenced(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	b := n.(*ast.FencedCodeBlock)
	var fields []string
	if b.Info != nil {
		fields = strings.Fields(string(b.Info.Segment.Value(src)))
	}
	lang := ""
	if len(fields) > 0 {
		lang = fields[0]
	}
	var code strings.Builder
	lines := b.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		code.Write(seg.Value(src))
	}
	flags := fields[min(1, len(fields)):]
	run := lang == "aes" && slices.Contains(flags, "run")
	anchor := ""
	if run || lang == "aes" && slices.Contains(flags, "check") {
		r.c.cur.examples++
		anchor = exampleAnchor(r.c.cur.id, r.c.cur.examples)
	}
	w.WriteString(snippet(r.c.cur, lang, code.String(), run, anchor))
	return ast.WalkSkipChildren, nil
}

func exampleAnchor(id string, number int) string {
	return id + "-example-" + strconv.Itoa(number)
}

func snippet(p *pageCtx, lang, code string, run bool, anchor string) string {
	body := html.EscapeString(code)
	kind := lang
	if kind == "" {
		kind = guessLang(code)
	}
	switch kind {
	case "go":
		body = highlightAES(code)
	case "aetext", "aescene", "aeprefab", "aego", "aedata":
		body = highlightAEText(code)
	case "sh", "bash", "shell", "powershell", "ps1":
		body = highlightShell(code)
	}
	if kind == "aes" {
		// The lexer does not resolve imports, locals or shadowing. Highlight
		// executable code without guessing links; authors can link API names
		// explicitly in the explanation next to the unchanged source.
		body = highlightAES(code)
	}
	attr := ""
	if anchor != "" {
		attr = ` id="` + html.EscapeString(anchor) + `"`
	}
	if lang != "" {
		attr += ` data-lang="` + html.EscapeString(lang) + `"`
	}
	pre := `<pre class="code"` + attr + `><code class="lang-` + html.EscapeString(lang) + `">` + body + `</code></pre>`
	if !run {
		return pre + "\n"
	}
	p.runs++
	if !p.play {
		return pre + "\n"
	}
	ex := p.id + "/" + strconv.Itoa(p.runs)
	open := p.root + "play/?example=" + url.QueryEscape(ex)
	return `<div class="snippet" data-example="` + html.EscapeString(ex) + `">` + pre +
		`<div class="snippet-bar"><button type="button" class="run">Run</button>` +
		`<a class="open" href="` + html.EscapeString(open) + `">Open in playground</a></div>` +
		`<ol class="output" hidden></ol></div>` + "\n"
}
