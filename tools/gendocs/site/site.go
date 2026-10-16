package site

import (
	"embed"
	"encoding/json"
	"html"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

//go:embed templates/layout.html
var layoutSrc string

//go:embed static
var static embed.FS

type Options struct {
	Playground bool
	Repo       string
}

type Report struct {
	Pages   int
	Entries int
	Broken  []string
}

type layoutData struct {
	Root       string
	Title      string
	Site       string
	Playground bool
	Repo       string
	Nav        template.HTML
	Content    template.HTML
}

type searchItem struct {
	Title   string `json:"t"`
	URL     string `json:"u"`
	Summary string `json:"s,omitempty"`
	Kind    string `json:"k"`
	ID      string `json:"i"`
}

type builder struct {
	exp    *Export
	out    string
	opt    Options
	byID   map[string]*Entry
	kids   map[string][]string
	roots  []string
	own    map[string]bool
	urls   map[string]string
	conv   *converter
	layout *template.Template
	report Report
}

func Build(exp *Export, out string, opt Options) (Report, error) {
	layout, err := template.New("layout").Parse(layoutSrc)
	if err != nil {
		return Report{}, err
	}
	b := &builder{
		exp:    exp,
		out:    out,
		opt:    opt,
		byID:   map[string]*Entry{},
		kids:   map[string][]string{},
		own:    map[string]bool{},
		urls:   map[string]string{},
		conv:   newConverter(),
		layout: layout,
	}
	b.index()
	b.route()
	if err := b.write(); err != nil {
		return Report{}, err
	}
	return b.report, nil
}

func (b *builder) index() {
	for i := range b.exp.Entries {
		e := &b.exp.Entries[i]
		b.byID[e.ID] = e
	}
	var walk func(parent string, nodes []Node)
	walk = func(parent string, nodes []Node) {
		for _, n := range nodes {
			if _, ok := b.byID[n.ID]; !ok {
				continue
			}
			if parent == "" {
				b.roots = append(b.roots, n.ID)
			} else {
				b.kids[parent] = append(b.kids[parent], n.ID)
			}
			walk(n.ID, n.Children)
		}
	}
	walk("", b.exp.Nav)
}

func (b *builder) ownPage(e *Entry) bool {
	switch e.Kind {
	case KindModule, KindType:
		return true
	case KindPage:
		return e.Parent == "" || titled(e.Body) || len(b.kids[e.ID]) > 0
	}
	return e.Parent == ""
}

func pagePath(e *Entry) string {
	if e.Kind == KindPage {
		return cleanPath(e.ID) + ".html"
	}
	return "ref/" + cleanPath(e.ID) + ".html"
}

func (b *builder) route() {
	var walk func(owner string, ids []string)
	walk = func(owner string, ids []string) {
		for _, id := range ids {
			e := b.byID[id]
			next := owner
			if owner == "" || b.ownPage(e) {
				b.own[id] = true
				b.urls[id] = pagePath(e)
				next = b.urls[id]
			} else {
				b.urls[id] = owner + "#" + id
			}
			walk(next, b.kids[id])
		}
	}
	walk("", b.roots)
}

func rootOf(p string) string {
	return strings.Repeat("../", strings.Count(p, "/"))
}

func esc(s string) string {
	return html.EscapeString(s)
}

var (
	mdCode   = regexp.MustCompile("`([^`]+)`")
	mdStrong = regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__`)
	mdEm     = regexp.MustCompile(`(^|[^\w*])\*([^*\s][^*]*)\*|(^|[^\w])_([^_\s][^_]*)_`)
)

func inlineMD(s string) string {
	parts := mdCode.Split(s, -1)
	codes := mdCode.FindAllStringSubmatch(s, -1)
	var b strings.Builder
	for i, part := range parts {
		t := esc(part)
		t = mdStrong.ReplaceAllString(t, "<strong>$1$2</strong>")
		t = mdEm.ReplaceAllString(t, "$1$3<em>$2$4</em>")
		b.WriteString(t)
		if i < len(codes) {
			b.WriteString("<code>" + esc(codes[i][1]) + "</code>")
		}
	}
	return b.String()
}

func plainMD(s string) string {
	s = mdCode.ReplaceAllString(s, "$1")
	s = mdStrong.ReplaceAllString(s, "$1$2")
	return mdEm.ReplaceAllString(s, "$1$3$2$4")
}

func (b *builder) resolve(id string) (string, bool) {
	u, ok := b.urls[id]
	return u, ok
}

func (b *builder) markdown(root, id, md string) string {
	p := &pageCtx{
		root:    root,
		id:      id,
		play:    b.opt.Playground,
		resolve: b.resolve,
		broken:  func(target string) { b.report.Broken = append(b.report.Broken, id+" -> ref:"+target) },
	}
	out, err := b.conv.render(p, md)
	if err != nil {
		return "<p>" + esc(err.Error()) + "</p>"
	}
	return out
}

func kindLabel(e *Entry) string {
	if e.Kind == KindPage {
		return ""
	}
	return `<span class="kind kind-` + esc(e.Kind) + `">` + esc(e.Kind) + `</span>`
}

func (b *builder) details(c *strings.Builder, e *Entry, root string) {
	if e.Signature != "" {
		c.WriteString(`<pre class="sig"><code>` + highlightAES(e.Signature) + "</code></pre>\n")
	}
	body := e.Body
	if b.own[e.ID] {
		body = stripTitle(body)
	}
	if strings.TrimSpace(body) != "" {
		c.WriteString(b.markdown(root, e.ID, body))
	}
	if e.Example != "" {
		c.WriteString(`<p class="label">Example</p>` + "\n")
		c.WriteString(`<pre class="code"><code class="lang-aes">` + highlightAES(e.Example) + "</code></pre>\n")
	}
	if strings.HasPrefix(e.ID, ScriptsPrefix) && e.Location.Path != "" {
		where := e.Location.Path
		if e.Location.Line > 0 {
			where += ":" + strconv.Itoa(e.Location.Line)
		}
		c.WriteString(`<p class="where">` + esc(where) + "</p>\n")
	}
}

func (b *builder) section(c *strings.Builder, id, root string, depth int) {
	e := b.byID[id]
	h := "h" + strconv.Itoa(min(depth, 4))
	c.WriteString(`<section class="member">` + "\n")
	c.WriteString("<" + h + ` id="` + esc(e.ID) + `"><a class="anchor" href="#` + esc(e.ID) + `">` + esc(e.Title) + "</a>" + kindLabel(e) + "</" + h + ">\n")
	b.details(c, e, root)
	b.children(c, e, root, depth+1)
	c.WriteString("</section>\n")
}

func (b *builder) children(c *strings.Builder, e *Entry, root string, depth int) {
	var pages []string
	for _, k := range b.kids[e.ID] {
		if b.own[k] {
			pages = append(pages, k)
			continue
		}
		b.section(c, k, root, depth)
	}
	if len(pages) == 0 {
		return
	}
	c.WriteString(`<ul class="contents">` + "\n")
	for _, k := range pages {
		ke := b.byID[k]
		c.WriteString(`<li><a href="` + esc(root+b.urls[k]) + `">` + esc(ke.Title) + "</a>" + kindLabel(ke))
		if ke.Summary != "" {
			c.WriteString(`<span class="summary">` + inlineMD(ke.Summary) + "</span>")
		}
		c.WriteString("</li>\n")
	}
	c.WriteString("</ul>\n")
}

func (b *builder) crumbs(e *Entry, root string) string {
	var trail []string
	for p := e.Parent; p != ""; {
		pe, ok := b.byID[p]
		if !ok {
			break
		}
		if b.own[p] {
			trail = append([]string{`<a href="` + esc(root+b.urls[p]) + `">` + esc(pe.Title) + "</a>"}, trail...)
		}
		p = pe.Parent
	}
	if len(trail) == 0 {
		return ""
	}
	return `<nav class="crumbs">` + strings.Join(trail, ` <span>/</span> `) + "</nav>\n"
}

func (b *builder) content(e *Entry, root string) string {
	var c strings.Builder
	c.WriteString(b.crumbs(e, root))
	c.WriteString(`<h1 id="` + esc(e.ID) + `">` + esc(e.Title) + kindLabel(e) + "</h1>\n")
	b.details(&c, e, root)
	b.children(&c, e, root, 2)
	return c.String()
}

func (b *builder) navTree(c *strings.Builder, ids []string, root, current string, open map[string]bool) {
	c.WriteString("<ul>")
	for _, id := range ids {
		if !b.own[id] {
			continue
		}
		e := b.byID[id]
		var sub []string
		for _, k := range b.kids[id] {
			if b.own[k] {
				sub = append(sub, k)
			}
		}
		cls := ""
		if id == current {
			cls = ` class="current"`
		}
		link := `<a` + cls + ` href="` + esc(root+b.urls[id]) + `">` + esc(e.Title) + "</a>"
		if len(sub) == 0 {
			c.WriteString("<li>" + link + "</li>")
			continue
		}
		attr := ""
		if open[id] {
			attr = " open"
		}
		c.WriteString("<li><details" + attr + "><summary>" + link + "</summary>")
		b.navTree(c, sub, root, current, open)
		c.WriteString("</details></li>")
	}
	c.WriteString("</ul>")
}

func (b *builder) nav(root, current string) string {
	open := map[string]bool{}
	for p := current; p != ""; {
		open[p] = true
		e, ok := b.byID[p]
		if !ok {
			break
		}
		p = e.Parent
	}
	var guides, reference []string
	for _, id := range b.roots {
		if b.byID[id].Kind == KindPage {
			guides = append(guides, id)
		} else {
			reference = append(reference, id)
		}
	}
	var c strings.Builder
	if len(guides) > 0 {
		c.WriteString(`<p class="group">Guides</p>`)
		b.navTree(&c, guides, root, current, open)
	}
	if len(reference) > 0 {
		c.WriteString(`<p class="group">Reference</p>`)
		b.navTree(&c, reference, root, current, open)
	}
	return c.String()
}

func (b *builder) home() string {
	var c strings.Builder
	c.WriteString(`<h1>` + esc(b.exp.Title) + "</h1>\n")
	c.WriteString(`<p class="lead">Guides, the language reference and every engine module, generated from the same source as the editor's hover and docs panel.</p>` + "\n")
	group := func(title string, page bool) {
		c.WriteString("<h2>" + esc(title) + "</h2>\n" + `<ul class="contents">` + "\n")
		for _, id := range b.roots {
			e := b.byID[id]
			if (e.Kind == KindPage) != page {
				continue
			}
			c.WriteString(`<li><a href="` + esc(b.urls[id]) + `">` + esc(e.Title) + "</a>" + kindLabel(e))
			if e.Summary != "" {
				c.WriteString(`<span class="summary">` + inlineMD(e.Summary) + "</span>")
			}
			c.WriteString("</li>\n")
		}
		c.WriteString("</ul>\n")
	}
	group("Guides", true)
	group("Reference", false)
	return c.String()
}

func (b *builder) emit(rel, title, current, content string) error {
	root := rootOf(rel)
	var buf strings.Builder
	err := b.layout.Execute(&buf, layoutData{
		Root:       root,
		Title:      title,
		Site:       b.exp.Title,
		Playground: b.opt.Playground,
		Repo:       b.opt.Repo,
		Nav:        template.HTML(b.nav(root, current)),
		Content:    template.HTML(content),
	})
	if err != nil {
		return err
	}
	dst := filepath.Join(b.out, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	b.report.Pages++
	return os.WriteFile(dst, []byte(buf.String()), 0o644)
}

func (b *builder) write() error {
	if err := os.MkdirAll(b.out, 0o755); err != nil {
		return err
	}
	var search []searchItem
	for i := range b.exp.Entries {
		e := &b.exp.Entries[i]
		u, ok := b.urls[e.ID]
		if !ok {
			continue
		}
		b.report.Entries++
		search = append(search, searchItem{Title: e.Title, URL: u, Summary: plainMD(e.Summary), Kind: e.Kind, ID: e.ID})
		if !b.own[e.ID] {
			continue
		}
		rel := b.urls[e.ID]
		if err := b.emit(rel, e.Title, e.ID, b.content(e, rootOf(rel))); err != nil {
			return err
		}
	}
	if err := b.emit("index.html", b.exp.Title, "", b.home()); err != nil {
		return err
	}
	data, err := json.Marshal(search)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(b.out, "search.json"), data, 0o644); err != nil {
		return err
	}
	return b.assets()
}

func (b *builder) assets() error {
	return fs.WalkDir(static, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := static.ReadFile(p)
		if err != nil {
			return err
		}
		dst := filepath.Join(b.out, "assets", filepath.FromSlash(strings.TrimPrefix(p, "static/")))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
}
