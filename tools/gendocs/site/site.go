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
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed templates/layout.html
var layoutSrc string

//go:embed static
var static embed.FS

const (
	SectionStart     = "start"
	SectionGuides    = "guides"
	SectionReference = "reference"
	otherGroup       = "other"
	heroAsset        = "assets/hero.png"
	homeLead         = "Guides, the language reference and every engine module, generated from the same source as the editor's hover and docs panel."
)

var sectionOrder = []string{SectionStart, SectionGuides, SectionReference}

var sectionTitles = map[string]string{
	SectionStart:     "Getting started",
	SectionGuides:    "Guides",
	SectionReference: "Reference",
}

func sectionRank(s string) int {
	for i, x := range sectionOrder {
		if x == s {
			return i
		}
	}
	return len(sectionOrder)
}

func sectionTitle(s string) string {
	if t, ok := sectionTitles[s]; ok {
		return t
	}
	return s
}

type Options struct {
	Playground bool
	Repo       string
	Edit       string
	Version    string
	Commit     string
	Hero       string
	Time       time.Time
}

type Report struct {
	Pages   int
	Entries int
	Broken  []string
	Outline string
}

type layoutData struct {
	Root       string
	Title      string
	Site       string
	Playground bool
	Repo       string
	Home       bool
	Nav        template.HTML
	Content    template.HTML
	Toc        template.HTML
	Pager      template.HTML
	Footer     template.HTML
}

type searchItem struct {
	Title   string `json:"t"`
	URL     string `json:"u"`
	Summary string `json:"s,omitempty"`
	Kind    string `json:"k"`
	ID      string `json:"i"`
	Group   string `json:"g,omitempty"`
	Section string `json:"c,omitempty"`
}

type navGroup struct {
	Group
	roots []string
}

type builder struct {
	exp     *Export
	out     string
	opt     Options
	byID    map[string]*Entry
	kids    map[string][]string
	groups  []navGroup
	groupOf map[string]int
	own     map[string]bool
	urls    map[string]string
	order   []string
	pos     map[string]int
	hero    bool
	conv    *converter
	layout  *template.Template
	report  Report
}

func Build(exp *Export, out string, opt Options) (Report, error) {
	layout, err := template.New("layout").Parse(layoutSrc)
	if err != nil {
		return Report{}, err
	}
	if opt.Edit == "" && opt.Repo != "" {
		opt.Edit = strings.TrimRight(opt.Repo, "/") + "/src/branch/main"
	}
	b := &builder{
		exp:     exp,
		out:     out,
		opt:     opt,
		byID:    map[string]*Entry{},
		kids:    map[string][]string{},
		groupOf: map[string]int{},
		own:     map[string]bool{},
		urls:    map[string]string{},
		pos:     map[string]int{},
		conv:    newConverter(),
		layout:  layout,
	}
	if opt.Hero != "" {
		if st, err := os.Stat(opt.Hero); err == nil && !st.IsDir() {
			b.hero = true
		}
	}
	b.index()
	b.route()
	b.report.Outline = b.outline()
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
	seen := map[string]bool{}
	var walk func(gi int, parent string, nodes []Node)
	walk = func(gi int, parent string, nodes []Node) {
		for _, n := range nodes {
			if _, ok := b.byID[n.ID]; !ok || seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			b.groupOf[n.ID] = gi
			if parent == "" {
				b.groups[gi].roots = append(b.groups[gi].roots, n.ID)
			} else {
				b.kids[parent] = append(b.kids[parent], n.ID)
			}
			walk(gi, n.ID, n.Children)
		}
	}
	gs := append([]GroupNav(nil), b.exp.Groups...)
	sort.SliceStable(gs, func(i, j int) bool {
		if ri, rj := sectionRank(gs[i].Section), sectionRank(gs[j].Section); ri != rj {
			return ri < rj
		}
		return gs[i].Order < gs[j].Order
	})
	for _, g := range gs {
		b.groups = append(b.groups, navGroup{Group: g.Group})
		walk(len(b.groups)-1, "", g.Nav)
	}
	var rest []Node
	for _, n := range b.exp.Nav {
		if !seen[n.ID] {
			rest = append(rest, n)
		}
	}
	if len(rest) > 0 {
		b.groups = append(b.groups, navGroup{Group: Group{ID: otherGroup, Title: "Other", Section: SectionReference, Order: 1 << 20}})
		walk(len(b.groups)-1, "", rest)
	}
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
				b.pos[id] = len(b.order)
				b.order = append(b.order, id)
				next = b.urls[id]
			} else {
				b.urls[id] = owner + "#" + id
			}
			walk(next, b.kids[id])
		}
	}
	for _, g := range b.groups {
		walk("", g.roots)
	}
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
	headRe   = regexp.MustCompile(`<h([23]) id="([^"]+)"[^>]*>(.*?)</h[23]>`)
	kindRe   = regexp.MustCompile(`<span class="kind[^"]*">[^<]*</span>`)
	tagRe    = regexp.MustCompile(`<[^>]+>`)
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
	if gi, ok := b.groupOf[e.ID]; ok {
		g := b.groups[gi]
		head := []string{"<span>" + esc(sectionTitle(g.Section)) + "</span>"}
		if g.Title != sectionTitle(g.Section) {
			head = append(head, "<span>"+esc(g.Title)+"</span>")
		}
		trail = append(head, trail...)
	}
	if len(trail) == 0 {
		return ""
	}
	return `<nav class="crumbs">` + strings.Join(trail, `<span class="sep">/</span>`) + "</nav>\n"
}

func (b *builder) content(e *Entry, root string) string {
	var c strings.Builder
	c.WriteString(b.crumbs(e, root))
	c.WriteString(`<h1 id="` + esc(e.ID) + `">` + esc(e.Title) + kindLabel(e) + "</h1>\n")
	b.details(&c, e, root)
	b.children(&c, e, root, 2)
	return c.String()
}

func toc(content string) string {
	m := headRe.FindAllStringSubmatch(content, -1)
	if len(m) < 2 {
		return ""
	}
	var c strings.Builder
	c.WriteString(`<p class="toc-title">On this page</p><ul>`)
	for _, x := range m {
		text := strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(kindRe.ReplaceAllString(x[3], ""), "")))
		c.WriteString(`<li class="l` + x[1] + `"><a href="#` + x[2] + `">` + esc(text) + "</a></li>")
	}
	c.WriteString("</ul>")
	return c.String()
}

func (b *builder) pager(id, root string) string {
	i, ok := b.pos[id]
	if !ok {
		return ""
	}
	var c strings.Builder
	c.WriteString(`<nav class="pager">`)
	if i > 0 {
		p := b.order[i-1]
		c.WriteString(`<a class="prev" href="` + esc(root+b.urls[p]) + `"><span>Previous</span>` + esc(b.byID[p].Title) + "</a>")
	} else {
		c.WriteString("<span></span>")
	}
	if i+1 < len(b.order) {
		n := b.order[i+1]
		c.WriteString(`<a class="next" href="` + esc(root+b.urls[n]) + `"><span>Next</span>` + esc(b.byID[n].Title) + "</a>")
	}
	c.WriteString("</nav>")
	return c.String()
}

func (b *builder) footer(e *Entry, root string) string {
	o := b.opt
	name := "Aego"
	if o.Version != "" {
		name += " " + o.Version
	}
	left := []string{esc(name)}
	if o.Commit != "" {
		c := esc(o.Commit)
		if o.Repo != "" {
			c = `<a href="` + esc(strings.TrimRight(o.Repo, "/")+"/commit/"+o.Commit) + `">` + c + "</a>"
		}
		left = append(left, c)
	}
	if !o.Time.IsZero() {
		left = append(left, "built "+o.Time.UTC().Format("2006-01-02"))
	}
	var right []string
	if e != nil && e.Location.Source != "" && o.Edit != "" {
		u := strings.TrimRight(o.Edit, "/") + "/" + e.Location.Source
		if e.Location.Line > 0 {
			u += "#L" + strconv.Itoa(e.Location.Line)
		}
		right = append(right, `<a href="`+esc(u)+`">Edit this page</a>`)
	}
	if o.Playground {
		right = append(right, `<a href="`+root+`play/">Playground</a>`)
	}
	if o.Repo != "" {
		right = append(right, `<a href="`+esc(o.Repo)+`">Source</a>`)
	}
	return `<footer class="foot"><span>` + strings.Join(left, " · ") + `</span><span class="links">` + strings.Join(right, "") + "</span></footer>"
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
			cls = ` class="current" aria-current="page"`
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
	cur := -1
	if gi, ok := b.groupOf[current]; ok {
		cur = gi
	}
	var c strings.Builder
	last := ""
	for gi, g := range b.groups {
		if g.Section != last {
			c.WriteString(`<p class="section">` + esc(sectionTitle(g.Section)) + "</p>")
			last = g.Section
		}
		attr := ""
		if gi == cur {
			attr = ` open data-current="true"`
		}
		c.WriteString(`<details class="group" data-group="` + esc(g.ID) + `"` + attr + "><summary>" + esc(g.Title) + "</summary>")
		b.navTree(&c, g.roots, root, current, open)
		c.WriteString("</details>")
	}
	return c.String()
}

func (b *builder) firstOf(section string) string {
	for _, g := range b.groups {
		if g.Section == section && len(g.roots) > 0 {
			return b.urls[g.roots[0]]
		}
	}
	return ""
}

func (b *builder) home() string {
	var c strings.Builder
	c.WriteString(`<section class="hero"><div class="hero-text"><h1>` + esc(b.exp.Title) + "</h1>")
	c.WriteString(`<p class="lead">` + esc(homeLead) + `</p><div class="actions">`)
	if u := b.firstOf(SectionStart); u != "" {
		c.WriteString(`<a class="button primary" href="` + esc(u) + `">Get started</a>`)
	}
	if b.opt.Playground {
		c.WriteString(`<a class="button" href="play/">Open the playground</a>`)
	}
	if u := b.firstOf(SectionReference); u != "" {
		c.WriteString(`<a class="button" href="` + esc(u) + `">Reference</a>`)
	}
	c.WriteString("</div></div>")
	if b.hero {
		c.WriteString(`<img class="hero-shot" src="` + heroAsset + `" alt="The Aego editor">`)
	}
	c.WriteString("</section>\n")
	for _, s := range sectionOrder {
		var cards []string
		for _, g := range b.groups {
			if g.Section != s || len(g.roots) == 0 {
				continue
			}
			first := b.byID[g.roots[0]]
			card := `<a class="card" href="` + esc(b.urls[g.roots[0]]) + `"><strong>` + esc(g.Title) + "</strong>"
			if first.Summary != "" {
				card += "<span>" + inlineMD(first.Summary) + "</span>"
			}
			cards = append(cards, card+"</a>")
		}
		if len(cards) == 0 {
			continue
		}
		c.WriteString("<h2>" + esc(sectionTitle(s)) + `</h2><div class="cards">` + strings.Join(cards, "") + "</div>\n")
	}
	return c.String()
}

func (b *builder) emit(rel, title, current, content string, e *Entry) error {
	root := rootOf(rel)
	data := layoutData{
		Root:       root,
		Title:      title,
		Site:       b.exp.Title,
		Playground: b.opt.Playground,
		Repo:       b.opt.Repo,
		Home:       current == "",
		Nav:        template.HTML(b.nav(root, current)),
		Content:    template.HTML(content),
		Footer:     template.HTML(b.footer(e, root)),
	}
	if current != "" {
		data.Toc = template.HTML(toc(content))
		data.Pager = template.HTML(b.pager(current, root))
	}
	var buf strings.Builder
	if err := b.layout.Execute(&buf, data); err != nil {
		return err
	}
	dst := filepath.Join(b.out, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	b.report.Pages++
	return os.WriteFile(dst, []byte(buf.String()), 0o644)
}

func (b *builder) walkOwn(ids []string, depth int, fn func(id string, depth int)) {
	for _, id := range ids {
		if !b.own[id] {
			continue
		}
		fn(id, depth)
		b.walkOwn(b.kids[id], depth+1, fn)
	}
}

func (b *builder) outline() string {
	var c strings.Builder
	last := ""
	for _, g := range b.groups {
		if g.Section != last {
			c.WriteString(sectionTitle(g.Section) + "\n")
			last = g.Section
		}
		c.WriteString("  " + g.Title + "\n")
		b.walkOwn(g.roots, 2, func(id string, depth int) {
			c.WriteString(strings.Repeat("  ", depth) + b.byID[id].Title + " -> " + b.urls[id] + "\n")
		})
	}
	return c.String()
}

func (b *builder) llms() string {
	var c strings.Builder
	c.WriteString("# " + b.exp.Title + "\n\n> " + homeLead + "\n")
	last := ""
	for _, g := range b.groups {
		if g.Section != last {
			c.WriteString("\n## " + sectionTitle(g.Section) + "\n")
			last = g.Section
		}
		c.WriteString("\n### " + g.Title + "\n\n")
		b.walkOwn(g.roots, 0, func(id string, _ int) {
			e := b.byID[id]
			line := "- [" + e.Title + "](" + b.urls[id] + ")"
			if s := plainMD(e.Summary); s != "" {
				line += ": " + s
			}
			c.WriteString(line + "\n")
		})
	}
	return c.String()
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
		item := searchItem{Title: e.Title, URL: u, Summary: plainMD(e.Summary), Kind: e.Kind, ID: e.ID}
		if gi, ok := b.groupOf[e.ID]; ok {
			item.Group, item.Section = b.groups[gi].Title, b.groups[gi].Section
		}
		search = append(search, item)
	}
	for _, id := range b.order {
		e := b.byID[id]
		rel := b.urls[id]
		if err := b.emit(rel, e.Title, id, b.content(e, rootOf(rel)), e); err != nil {
			return err
		}
	}
	if err := b.emit("index.html", b.exp.Title, "", b.home(), nil); err != nil {
		return err
	}
	data, err := json.Marshal(search)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(b.out, "search.json"), data, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(b.out, "llms.txt"), []byte(b.llms()), 0o644); err != nil {
		return err
	}
	return b.assets()
}

func (b *builder) assets() error {
	err := fs.WalkDir(static, "static", func(p string, d fs.DirEntry, err error) error {
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
	if err != nil || !b.hero {
		return err
	}
	data, err := os.ReadFile(b.opt.Hero)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(b.out, filepath.FromSlash(heroAsset)), data, 0o644)
}
