package site

import (
	"crypto/sha256"
	"embed"
	hexenc "encoding/hex"
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

	"aego/app/brand"
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
	posterAsset      = "assets/demo-poster.png"
	shotsDir         = "assets/shots/"
	scriptSection    = "## From a script"
	homeLead         = "A 2D-first game engine written in Go, with a built-in editor, its own scripting language and native Go extensions."
	installCmd       = "curl -fsSL https://aego.akarpov.ru/install.sh | sh"
	installPage      = "install"
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
	Playground  bool
	Repo        string
	Edit        string
	Version     string
	Commit      string
	Hero        string
	Poster      string
	Shots       string
	Demo        bool
	PlayVersion string
	TryVersion  string
	Release     *Release
	Icons       []string
	Time        time.Time
}

type Report struct {
	Pages   int
	Entries int
	Broken  []string
	Outline string
}

type layoutData struct {
	AssetV      string
	Background  template.CSS
	LightBack   template.CSS
	CSS         template.CSS
	Root        string
	Title       string
	PageTitle   string
	Description string
	Site        string
	Playground  bool
	PlayVersion string
	TryVersion  string
	PlaySize    int64
	TrySize     int64
	Repo        string
	Demo        bool
	Home        bool
	Nav         template.HTML
	Content     template.HTML
	Toc         template.HTML
	Pager       template.HTML
	Footer      template.HTML
}

type searchItem struct {
	Title   string `json:"t"`
	URL     string `json:"u"`
	Summary string `json:"s,omitempty"`
	Kind    string `json:"k"`
	ID      string `json:"i"`
	Group   string `json:"g,omitempty"`
	Section string `json:"c,omitempty"`
	Icon    string `json:"ic,omitempty"`
}

type navGroup struct {
	Group
	roots []string
}

type builder struct {
	// lightShots names the screenshots that also come in the light theme, as name-light.png (D345).
	lightShots map[string]bool
	exp        *Export
	out        string
	assetV     string
	missing    map[string]bool
	opt        Options
	byID       map[string]*Entry
	kids       map[string][]string
	groups     []navGroup
	groupOf    map[string]int
	own        map[string]bool
	urls       map[string]string
	order      []string
	pos        map[string]int
	hero       bool
	poster     bool
	icons      iconSet
	conv       *converter
	layout     *template.Template
	css        string
	report     Report
}

func Build(exp *Export, out string, opt Options) (Report, error) {
	exp.Entries = siteEntries(exp.Entries)
	keep := make(map[string]bool, len(exp.Entries))
	for _, e := range exp.Entries {
		keep[e.ID] = true
	}
	exp.Nav = pruneNav(exp.Nav, keep)
	for i := range exp.Groups {
		exp.Groups[i].Nav = pruneNav(exp.Groups[i].Nav, keep)
	}
	if opt.Edit == "" && opt.Repo != "" {
		opt.Edit = strings.TrimRight(opt.Repo, "/") + "/src/branch/main"
	}
	b := &builder{
		exp:     exp,
		out:     out,
		opt:     opt,
		byID:    map[string]*Entry{},
		missing: map[string]bool{},
		kids:    map[string][]string{},
		groupOf: map[string]int{},
		own:     map[string]bool{},
		urls:    map[string]string{},
		pos:     map[string]int{},
		conv:    newConverter(),
		icons:   loadIcons(opt.Icons),
	}
	b.lightShots = lightShots(opt.Shots)
	layout, err := template.New("layout").Funcs(template.FuncMap{
		"icon": func(root, name string) template.HTML { return template.HTML(b.icon(root, name, "")) },
	}).Parse(layoutSrc)
	if err != nil {
		return Report{}, err
	}
	b.layout = layout
	style, err := static.ReadFile("static/style.css")
	if err != nil {
		return Report{}, err
	}
	b.css = codeCSS() + string(style)
	b.assetV = b.assetVersion()
	b.hero = isFile(opt.Hero)
	b.poster = isFile(opt.Poster)
	b.index()
	b.route()
	b.report.Outline = b.outline()
	if err := b.write(); err != nil {
		return Report{}, err
	}
	if err := b.writeBrand(); err != nil {
		return Report{}, err
	}
	return b.report, nil
}

func (b *builder) writeBrand() error {
	for _, f := range []struct {
		size int
		name string
	}{{32, "favicon.png"}, {180, "apple-touch-icon.png"}} {
		data, ok := brand.PNG(f.size)
		if !ok {
			continue
		}
		dst := filepath.Join(b.out, "assets", f.name)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}
	return nil
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

var linkAttr = regexp.MustCompile(`(href|data-fallback)="([^"]*)"`)

// cleanURL drops .html from a site-relative link; the server maps /page to page.html.
func cleanURL(u string) string {
	if u == "" || strings.Contains(u, "://") || strings.HasPrefix(u, "#") || strings.HasPrefix(u, "mailto:") || strings.HasPrefix(u, "data:") {
		return u
	}
	path, rest := u, ""
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		path, rest = u[:i], u[i:]
	}
	switch {
	case path == "index.html":
		path = "./"
	case strings.HasSuffix(path, "/index.html"):
		path = strings.TrimSuffix(path, "index.html")
	case strings.HasSuffix(path, ".html"):
		path = strings.TrimSuffix(path, ".html")
	default:
		return u
	}
	return path + rest
}

func cleanLinks(page string) string {
	return linkAttr.ReplaceAllStringFunc(page, func(m string) string {
		sub := linkAttr.FindStringSubmatch(m)
		return sub[1] + `="` + cleanURL(sub[2]) + `"`
	})
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
	headRe   = regexp.MustCompile(`<h([23]) id="([^"]+)"( class="m")?[^>]*>(.*?)</h[23]>`)
	kindRe   = regexp.MustCompile(`<span class="kind[^"]*">[^<]*</span>`)
	tagRe    = regexp.MustCompile(`<[^>]+>`)
	mdImage  = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	mdLink   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
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

// metaDescription turns a summary into a plain sentence of at most about 160 characters.
func metaDescription(s string) string {
	s = mdLink.ReplaceAllString(mdImage.ReplaceAllString(s, ""), "$1")
	s = strings.Join(strings.Fields(plainMD(s)), " ")
	if strings.HasSuffix(s, ":") {
		s = strings.TrimSuffix(s, ":") + "."
	}
	if len(s) <= 160 {
		return s
	}
	cut := strings.LastIndexByte(s[:157], ' ')
	if cut < 80 {
		cut = 157
	}
	return strings.TrimRight(s[:cut], ",;:") + "…"
}

func plainMD(s string) string {
	s = mdCode.ReplaceAllString(s, "$1")
	s = mdStrong.ReplaceAllString(s, "$1$2")
	return mdEm.ReplaceAllString(s, "$1$3$2$4")
}

func (b *builder) resolve(id string) (string, bool) {
	if u, ok := b.urls[id]; ok {
		return u, true
	}
	id = strings.TrimSuffix(strings.TrimSpace(id), "()")
	if u, ok := b.urls[id]; ok {
		return u, true
	}
	if real, ok := b.exp.Aliases[id]; ok {
		u, ok := b.urls[real]
		return u, ok
	}
	return "", false
}

func (b *builder) code(root, src string) string {
	return highlightLinked(src, func(name string) string {
		if u, ok := b.resolve(name); ok {
			return root + u
		}
		return ""
	})
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
	return b.themedShots(out)
}

var shotImg = regexp.MustCompile(`<img src="([^"]*` + regexp.QuoteMeta(shotsDir) + `)([^"/]+)\.png"([^>]*)>`)

// lightShots lists the screenshots that have a light theme variant next to them.
func lightShots(dir string) map[string]bool {
	out := map[string]bool{}
	if dir == "" {
		return out
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*-light.png"))
	for _, f := range files {
		out[strings.TrimSuffix(filepath.Base(f), "-light.png")] = true
	}
	return out
}

// themedShots shows the light variant of a screenshot in the light theme; site.js keeps the source in step with the theme switch.
func (b *builder) themedShots(html string) string {
	return shotImg.ReplaceAllStringFunc(html, func(m string) string {
		x := shotImg.FindStringSubmatch(m)
		if !b.lightShots[x[2]] {
			return m
		}
		return `<picture class="themed"><source media="(prefers-color-scheme: light)" srcset="` + x[1] + x[2] + `-light.png">` + m + "</picture>"
	})
}

func kindName(e *Entry) string {
	if e.Kind == KindType && e.Component != nil {
		return "component"
	}
	return e.Kind
}

func kindLabel(e *Entry) string {
	if e.Kind == KindPage {
		return ""
	}
	return ` <span class="kind kind-` + esc(e.Kind) + `">` + esc(kindName(e)) + `</span>`
}

func (b *builder) details(c *strings.Builder, e *Entry, root string) {
	if e.Signature != "" {
		c.WriteString(`<pre class="sig"><code>` + b.code(root, e.Signature) + "</code></pre>\n")
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
		c.WriteString(`<pre class="code" data-lang="aes"><code class="lang-aes">` + b.code(root, e.Example) + "</code></pre>\n")
	} else if u := b.moduleExample(e); u != "" {
		c.WriteString(`<p class="see-example">See the <a href="` + esc(root+u) + `">module example</a> for this function in use.</p>` + "\n")
	}
	if strings.HasPrefix(e.ID, ScriptsPrefix) && e.Location.Path != "" {
		where := e.Location.Path
		if e.Location.Line > 0 {
			where += ":" + strconv.Itoa(e.Location.Line)
		}
		c.WriteString(`<p class="where">` + esc(where) + "</p>\n")
	}
}

func (b *builder) ref(root, id, text string) string {
	if u, ok := b.urls[id]; ok {
		ic := ""
		if e, ok := b.byID[id]; ok {
			ic = b.icon(root, b.entryIcon(e), "")
		}
		return `<a class="xref" href="` + esc(root+u) + `">` + ic + esc(text) + "</a>"
	}
	return "<code>" + esc(text) + "</code>"
}

func (b *builder) titleOf(id string) string {
	if e, ok := b.byID[id]; ok {
		return e.Title
	}
	return id
}

func (b *builder) componentMeta(e *Entry, root string) string {
	ci := e.Component
	var parts []string
	if ci.Create != "" {
		parts = append(parts, `Add it with <code>`+esc(ci.Create)+"</code>.")
	}
	if len(ci.Requires) > 0 {
		links := make([]string, len(ci.Requires))
		for i, id := range ci.Requires {
			links[i] = b.ref(root, id, b.titleOf(id))
		}
		parts = append(parts, "Needs "+strings.Join(links, ", ")+" on the same node.")
	}
	if ci.Unique {
		parts = append(parts, "One per scene.")
	}
	if len(parts) == 0 {
		return ""
	}
	return `<h2 id="` + esc(e.ID) + `-usage">Usage</h2>` + "\n<p>" + strings.Join(parts, " ") + "</p>\n"
}

func (b *builder) fieldTable(c *strings.Builder, e *Entry, root string, heading bool) {
	var set, live []*Entry
	for _, k := range b.kids[e.ID] {
		ke := b.byID[k]
		switch {
		case ke.Field == nil:
		case ke.Field.Runtime:
			live = append(live, ke)
		default:
			set = append(set, ke)
		}
	}
	byIndex := func(rows []*Entry) {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Field.Index < rows[j].Field.Index })
	}
	byIndex(set)
	byIndex(live)
	b.rows(c, e, root, set, heading, "fields", "Fields", "")
	b.rows(c, e, root, live, heading, "runtime", "Runtime state", "Values the component has while the game runs. They are not saved with the scene.")
}

func (b *builder) rows(c *strings.Builder, e *Entry, root string, rows []*Entry, heading bool, anchor, title, lead string) {
	if len(rows) == 0 {
		return
	}
	if heading {
		c.WriteString(`<h2 id="` + esc(e.ID) + `-` + anchor + `">` + title + `</h2>` + "\n")
	} else {
		c.WriteString(`<p class="label">` + title + `</p>` + "\n")
	}
	if lead != "" {
		c.WriteString("<p>" + lead + "</p>\n")
	}
	c.WriteString(`<table class="fields"><thead><tr><th>Name</th><th>Type</th><th>Default</th><th>Description</th></tr></thead><tbody>` + "\n")
	for _, f := range rows {
		fi := f.Field
		def := "—"
		if fi.Default != "" {
			def = "<code>" + esc(fi.Default) + "</code>"
		}
		var notes []string
		if fi.Range != "" {
			notes = append(notes, `<span class="range">`+esc(fi.Range)+"</span>")
		}
		for _, n := range fi.Notes {
			notes = append(notes, `<span class="note">`+esc(n)+"</span>")
		}
		c.WriteString(`<tr id="` + esc(f.ID) + `"><td class="fname"><a class="anchor" href="#` + esc(f.ID) + `">` + b.icon(root, b.entryIcon(f), "") + `<code>` + esc(f.Title) + "</code></a></td>")
		c.WriteString(`<td class="ftype"><code>` + esc(fi.Type) + `</code></td><td class="fdef">` + def + "</td>")
		desc := ""
		if strings.TrimSpace(f.Body) != "" {
			desc = b.markdown(root, f.ID, f.Body)
		}
		if len(notes) > 0 {
			desc += `<div class="fnotes">` + strings.Join(notes, " ") + "</div>"
		}
		c.WriteString(`<td class="fdesc">` + desc + "</td></tr>\n")
	}
	c.WriteString("</tbody></table>\n")
}

func (b *builder) entryList(c *strings.Builder, ids []string, root string) {
	c.WriteString(`<ul class="contents">` + "\n")
	for _, id := range ids {
		ke, ok := b.byID[id]
		if !ok {
			continue
		}
		c.WriteString("<li>" + b.ref(root, id, ke.Title))
		if ke.Summary != "" {
			c.WriteString(` <span class="summary">` + inlineMD(ke.Summary) + "</span>")
		}
		c.WriteString("</li>\n")
	}
	c.WriteString("</ul>\n")
}

// eventList shows each script event as its name in code, a dash and its summary.
func (b *builder) eventList(c *strings.Builder, ids []string, root string) {
	c.WriteString(`<ul class="events">` + "\n")
	for _, id := range ids {
		ke, ok := b.byID[id]
		if !ok {
			continue
		}
		name := `<code>` + esc(ke.Title) + `</code>`
		if u, ok := b.urls[id]; ok {
			name = `<a class="xref" href="` + esc(root+u) + `">` + name + "</a>"
		}
		c.WriteString("<li>" + name)
		if ke.Summary != "" {
			c.WriteString(" — " + inlineMD(ke.Summary))
		}
		c.WriteString("</li>\n")
	}
	c.WriteString("</ul>\n")
}

func (b *builder) section(c *strings.Builder, id, root string, depth int) {
	e := b.byID[id]
	h := "h" + strconv.Itoa(min(depth, 4))
	c.WriteString(`<section class="member" data-name="` + esc(strings.ToLower(e.Title)) + `">` + "\n")
	c.WriteString("<" + h + ` id="` + esc(e.ID) + `" class="m"><a class="anchor" href="#` + esc(e.ID) + `">` + b.icon(root, b.entryIcon(e), "") + esc(e.Title) + "</a>" + kindLabel(e) + "</" + h + ">\n")
	b.details(c, e, root)
	b.fieldTable(c, e, root, false)
	b.children(c, e, root, depth+1)
	c.WriteString("</section>\n")
}

func (b *builder) children(c *strings.Builder, e *Entry, root string, depth int) {
	var pages []string
	var members [2][]string
	for _, k := range b.kids[e.ID] {
		ke := b.byID[k]
		switch {
		case ke.Field != nil:
		case b.own[k]:
			pages = append(pages, k)
		case e.Component != nil && ke.Kind == "method":
			members[0] = append(members[0], k)
		case e.Component != nil && ke.Kind == "signal":
			members[1] = append(members[1], k)
		default:
			b.section(c, k, root, depth)
		}
	}
	for i, title := range []string{"Methods", "Signals"} {
		if len(members[i]) == 0 {
			continue
		}
		c.WriteString(`<h2 id="` + esc(e.ID) + `-` + strings.ToLower(title) + `">` + title + "</h2>\n")
		for _, k := range members[i] {
			b.section(c, k, root, depth+1)
		}
	}
	if len(pages) == 0 {
		return
	}
	c.WriteString(`<ul class="contents">` + "\n")
	for _, k := range pages {
		ke := b.byID[k]
		c.WriteString(`<li><a href="` + esc(root+b.urls[k]) + `">` + b.icon(root, b.entryIcon(ke), "") + esc(ke.Title) + "</a>" + kindLabel(ke))
		if ke.Summary != "" {
			c.WriteString(` <span class="summary">` + inlineMD(ke.Summary) + "</span>")
		}
		c.WriteString("</li>\n")
	}
	c.WriteString("</ul>\n")
}

func (b *builder) crumbs(e *Entry, root string) string {
	var trail, titles []string
	for p := e.Parent; p != ""; {
		pe, ok := b.byID[p]
		if !ok {
			break
		}
		if b.own[p] {
			trail = append([]string{`<a href="` + esc(root+b.urls[p]) + `">` + esc(pe.Title) + "</a>"}, trail...)
			titles = append([]string{pe.Title}, titles...)
		}
		p = pe.Parent
	}
	if gi, ok := b.groupOf[e.ID]; ok {
		g := b.groups[gi]
		head := []string{"<span>" + esc(sectionTitle(g.Section)) + "</span>"}
		next := e.Title
		if len(titles) > 0 {
			next = titles[0]
		}
		if g.Title != sectionTitle(g.Section) && g.Title != next {
			head = append(head, "<span>"+esc(g.Title)+"</span>")
		}
		trail = append(head, trail...)
	}
	if e.Kind != KindPage {
		trail = append([]string{`<b class="page-kind">` + esc(kindName(e)) + "</b>"}, trail...)
	}
	if len(trail) == 0 {
		return ""
	}
	return `<nav class="crumbs">` + strings.Join(trail, `<span class="sep">/</span>`) + "</nav>\n"
}

// soundButton toggles the sound of a try scene that plays any; try.js shows it once the scene runs.
func (b *builder) soundButton(e *Entry, root string) string {
	if !strings.Contains(e.Try, `component "audio.`) && !strings.Contains(e.TryScript, `"aego/audio"`) {
		return ""
	}
	return `<button type="button" class="try-sound" aria-pressed="false" hidden>` + b.icon(root, "cat.audio", "") + `<span>Sound</span></button>`
}

func (b *builder) tryBlock(c *strings.Builder, e *Entry, root string) {
	name := e.ID
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		name = name[i+1:]
	}
	prefabs := ""
	if len(e.TryPrefabs) > 0 {
		data, _ := json.Marshal(e.TryPrefabs)
		prefabs = string(data)
	}
	input := ""
	if e.Input {
		input = ` data-input="` + esc(strings.Join(e.TryKeys, ", ")) + `"`
	}
	c.WriteString(`<section class="try" data-select="` + esc(e.TrySelect) + `" data-name="` + esc(name) + `" data-scene="` + esc(e.Try) + `" data-code="` + esc(e.TryCode) + `" data-prefabs="` + esc(prefabs) + `"` + input + `>` + "\n")
	c.WriteString(`<h2 id="` + esc(e.ID) + `-try">Try it</h2>` + "\n")
	c.WriteString(`<p class="try-caption">A live scene with ` + esc(e.Title) + `: edit its fields in the Inspector on the right and the scene restarts with them.</p>` + "\n")
	c.WriteString(`<div class="try-stage"><button type="button" class="try-start">` + b.icon(root, "play.start", "") + `Try it</button><span class="try-hint">Runs this scene in the browser, with the Inspector next to it</span></div>` + "\n")
	c.WriteString(`<div class="try-bar"><button type="button" class="try-open">` + b.icon(root, "file.save", "") + `Open in editor</button><button type="button" class="try-copy">` + b.icon(root, "edit.copy", "") + `Copy as .aescene</button><button type="button" class="try-reset" disabled>` + b.icon(root, "ui.reset", "") + `Reset</button>` + b.soundButton(e, root) + `<span class="try-status" role="status"></span></div>` + "\n")
	c.WriteString(b.sceneTree(e.Try, root))
	if e.TryScript != "" {
		c.WriteString(`<details class="scene-tree try-script"><summary>` + b.icon(root, "file.script", "") + `<span>Script</span><span class="count">main.aes</span>` + b.icon(root, "ui.chevron_right", "chev") + `</summary>`)
		c.WriteString(b.markdown(root, e.ID, "```aes\n"+strings.TrimRight(e.TryScript, "\n")+"\n```\n"))
		c.WriteString("</details>\n")
	}
	c.WriteString("</section>\n")
}

func (b *builder) content(e *Entry, root string) string {
	var c strings.Builder
	c.WriteString(b.crumbs(e, root))
	c.WriteString(`<h1 id="` + esc(e.ID) + `">` + b.icon(root, b.entryIcon(e), "ic-title") + esc(e.Title) + "</h1>\n")
	if e.ID == installPage {
		c.WriteString(b.downloadBlock(root))
	}
	scripted := ""
	if e.Component != nil {
		if i := strings.Index(e.Body, scriptSection); i >= 0 {
			head := *e
			head.Body, scripted = e.Body[:i], e.Body[i:]
			e = &head
		}
	}
	if e.Component != nil {
		head := *e
		if strings.TrimSpace(head.Body) != "" {
			c.WriteString(`<div class="lead">` + b.markdown(root, e.ID, head.Body) + "</div>\n")
		}
		head.Body = ""
		b.details(&c, &head, root)
		c.WriteString(b.componentMeta(e, root))
	} else {
		b.details(&c, e, root)
	}
	if e.Try != "" && b.opt.Playground {
		b.tryBlock(&c, e, root)
	}
	b.fieldTable(&c, e, root, true)
	if scripted != "" {
		c.WriteString(b.markdown(root, e.ID, scripted))
	}
	if e.Kind == KindModule && e.Group == groupScriptAPI {
		b.apiIndex(&c, e, root)
	}
	if !b.diagnostics(&c, e, root) {
		b.children(&c, e, root, 2)
	}
	if e.Component != nil && len(e.Component.Events) > 0 {
		c.WriteString(`<h2 id="` + esc(e.ID) + `-events">Script events</h2>` + "\n")
		c.WriteString("<p>Scripts on a node with this component receive these events from the " + esc(e.Component.Module) + " module.</p>\n")
		b.eventList(&c, e.Component.Events, root)
	}
	if len(e.Related) > 0 {
		c.WriteString(`<h2 id="` + esc(e.ID) + `-related">Related guides</h2>` + "\n")
		b.entryList(&c, e.Related, root)
	}
	return c.String()
}

// tocMembers is how many member headings a toc lists before it leaves them to the page's index.
const tocMembers = 8

func toc(content string) string {
	var heads [][]string
	members := 0
	for _, x := range headRe.FindAllStringSubmatch(content, -1) {
		if x[1] != "2" {
			continue
		}
		if x[3] != "" {
			members++
		}
		heads = append(heads, x)
	}
	var c strings.Builder
	n := 0
	for _, x := range heads {
		if x[3] != "" && members > tocMembers {
			continue
		}
		text := strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(kindRe.ReplaceAllString(x[4], ""), "")))
		c.WriteString(`<li><a href="#` + x[2] + `">` + esc(text) + "</a></li>")
		n++
	}
	if n < 2 {
		return ""
	}
	return `<p class="toc-title">On this page</p><ul>` + c.String() + "</ul>"
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
	c.WriteString(`<ul class="tree">`)
	chev := b.icon(root, "ui.chevron_right", "chev")
	for _, id := range ids {
		if !b.own[id] {
			continue
		}
		sub := b.ownKids(id)
		link := b.navLink(id, root, current, b.icon(root, b.entryIcon(b.byID[id]), ""))
		if len(sub) == 0 {
			c.WriteString("<li>" + link + "</li>")
			continue
		}
		attr := ""
		if open[id] {
			attr = " open"
		}
		c.WriteString(`<li><details` + attr + `><summary>` + link + chev + "</summary>")
		b.navTree(c, sub, root, current, open)
		c.WriteString("</details></li>")
	}
	c.WriteString("</ul>")
}

func (b *builder) ownKids(id string) []string {
	var sub []string
	for _, k := range b.kids[id] {
		if b.own[k] {
			sub = append(sub, k)
		}
	}
	return sub
}

func (b *builder) navLink(id, root, current, ic string) string {
	cls := "item"
	aria := ""
	if id == current {
		cls += " current"
		aria = ` aria-current="page"`
	}
	return `<a class="` + cls + `"` + aria + ` href="` + esc(root+b.urls[id]) + `">` + ic + `<span>` + esc(b.byID[id].Title) + "</span></a>"
}

// head returns the page that stands for a group in the nav and the entries listed under it.
func (b *builder) head(g navGroup) (string, []string) {
	if len(g.roots) == 0 {
		return "", nil
	}
	first := g.roots[0]
	if len(g.roots) > 1 && b.byID[first].Title != g.Title {
		return "", g.roots
	}
	return first, append(b.ownKids(first), g.roots[1:]...)
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
	chev := b.icon(root, "ui.chevron_right", "chev")
	var c strings.Builder
	last := ""
	for gi, g := range b.groups {
		if g.Section != last {
			if last != "" {
				c.WriteString("</div>")
			}
			c.WriteString(`<div class="nav-section"><p class="section">` + esc(sectionTitle(g.Section)) + "</p>")
			last = g.Section
		}
		ic := b.icon(root, g.Icon, "")
		head, list := b.head(g)
		if head != "" && len(list) == 0 {
			c.WriteString(strings.Replace(b.navLink(head, root, current, ic), `class="item`, `class="item head`, 1))
			continue
		}
		attr := ""
		if gi == cur {
			attr = ` open data-current="true"`
		}
		summary := `<span class="item head">` + ic + "<span>" + esc(g.Title) + "</span></span>"
		if head != "" {
			summary = strings.Replace(b.navLink(head, root, current, ic), `class="item`, `class="item head`, 1)
		}
		c.WriteString(`<details class="group" data-group="` + esc(g.ID) + `"` + attr + "><summary>" + summary + chev + "</summary>")
		b.navTree(&c, list, root, current, open)
		c.WriteString("</details>")
	}
	if last != "" {
		c.WriteString("</div>")
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
	c.WriteString(`<p class="lead">` + esc(homeLead) + "</p>")
	dl := b.downloadBlock("")
	c.WriteString(dl)
	if u, ok := b.urls[installPage]; ok {
		c.WriteString(`<pre class="code install" data-lang="sh"><code>` + esc(installCmd) + "</code></pre>")
		c.WriteString(`<p class="install-note">Linux x86-64. <a href="` + esc(u) + `">Windows, macOS and source builds</a></p>`)
	}
	c.WriteString(`<div class="actions">`)
	if u := b.firstOf(SectionStart); u != "" {
		cls := "button primary"
		if dl != "" {
			cls = "button"
		}
		c.WriteString(`<a class="` + cls + `" href="` + esc(u) + `">Get started</a>`)
	}
	if b.opt.Playground {
		c.WriteString(`<a class="button" href="play/">Open the playground</a>`)
	}
	c.WriteString("</div></div>")
	switch {
	case b.opt.Demo:
		style := ""
		if b.poster {
			style = ` style="background-image:url(` + posterAsset + `)"`
		}
		c.WriteString(`<div class="hero-demo"><div class="demo" data-src="demo/"` + style + `><button type="button" class="demo-play" aria-label="Play the demo"><span class="disc">` + b.icon("", "play.start", "") + `</span><small>Play the demo · runs in the browser</small></button></div>`)
		c.WriteString(`<p class="demo-caption">A small platformer made with Aego and exported for the web. A/D or arrows to move, Space to jump. <a href="demo/">Open full screen</a></p></div>`)
	case b.hero:
		c.WriteString(`<img class="hero-shot" src="` + heroAsset + `" alt="The Aego editor">`)
	}
	c.WriteString("</section>\n")
	for _, s := range sectionOrder {
		var items []string
		for _, g := range b.groups {
			if g.Section != s || len(g.roots) == 0 {
				continue
			}
			first := b.byID[g.roots[0]]
			title := g.Title
			if head, list := b.head(g); head != "" && len(list) == 0 {
				title = first.Title
			}
			item := `<li><a href="` + esc(b.urls[g.roots[0]]) + `">` + b.icon("", g.Icon, "") + esc(title) + "</a>"
			if sum := strings.TrimSpace(mdImage.ReplaceAllString(first.Summary, "")); sum != "" {
				item += "<p>" + inlineMD(sum) + "</p>"
			}
			items = append(items, item+"</li>")
		}
		if len(items) == 0 {
			continue
		}
		c.WriteString(`<h2 id="` + esc(s) + `">` + esc(sectionTitle(s)) + `</h2><ul class="index">` + strings.Join(items, "") + "</ul>\n")
	}
	return c.String()
}

func (b *builder) emit(rel, title, current, content string, e *Entry) error {
	root := rootOf(rel)
	data := layoutData{
		AssetV:      b.assetV,
		Background:  template.CSS(pageBackground()),
		LightBack:   template.CSS(pageBackgroundLight()),
		CSS:         template.CSS(strings.ReplaceAll(b.css, "url(fonts/", "url("+root+"assets/fonts/")),
		Root:        root,
		Title:       title,
		PageTitle:   title + " · " + b.exp.Title,
		Description: homeLead,
		Site:        b.exp.Title,
		Playground:  b.opt.Playground,
		PlayVersion: b.opt.PlayVersion,
		TryVersion:  b.opt.TryVersion,
		PlaySize:    fileSize(filepath.Join(b.out, "play", "aego.wasm")),
		TrySize:     fileSize(filepath.Join(b.out, "try", "aego-docs.wasm")),
		Repo:        b.opt.Repo,
		Demo:        b.opt.Demo,
		Home:        current == "",
		Nav:         template.HTML(b.nav(root, current)),
		Content:     template.HTML(content),
		Footer:      template.HTML(b.footer(e, root)),
	}
	if current == "" {
		data.PageTitle = b.exp.Title + " — 2D game engine in Go"
	} else if e != nil {
		if d := metaDescription(e.Summary); d != "" {
			data.Description = d
		}
	}
	if current != "" {
		if t := toc(content); t != "" {
			data.Toc = template.HTML(t)
			inline := `<details class="toc-inline"><summary>On this page</summary>` + strings.TrimPrefix(t, `<p class="toc-title">On this page</p>`) + "</details>\n"
			if i := strings.Index(content, "<h2"); i >= 0 {
				data.Content = template.HTML(content[:i] + inline + content[i:])
			}
		}
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
	return os.WriteFile(dst, []byte(cleanLinks(buf.String())), 0o644)
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
		head, list := b.head(g)
		if head != "" {
			c.WriteString("  " + b.byID[head].Title + " -> " + b.urls[head] + "\n")
		} else {
			c.WriteString("  " + g.Title + "\n")
		}
		b.walkOwn(list, 2, func(id string, depth int) {
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
			line := "- [" + e.Title + "](" + cleanURL(b.urls[id]) + ")"
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
		item := searchItem{Title: e.Title, URL: cleanURL(u), Summary: plainMD(e.Summary), Kind: e.Kind, ID: e.ID, Icon: b.entryIcon(e)}
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
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(b.out, "assets", "icons.svg"), []byte(b.icons.sprite), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(b.out, "assets", "code.css"), []byte(codeCSS()), 0o644); err != nil {
		return err
	}
	if b.hero {
		if err := copyFile(b.opt.Hero, filepath.Join(b.out, filepath.FromSlash(heroAsset))); err != nil {
			return err
		}
	}
	if b.poster {
		if err := copyFile(b.opt.Poster, filepath.Join(b.out, filepath.FromSlash(posterAsset))); err != nil {
			return err
		}
	}
	if b.opt.Shots == "" {
		return nil
	}
	shots, _ := filepath.Glob(filepath.Join(b.opt.Shots, "*.png"))
	if len(shots) > 0 {
		if err := os.MkdirAll(filepath.Join(b.out, filepath.FromSlash(shotsDir)), 0o755); err != nil {
			return err
		}
	}
	for _, f := range shots {
		if err := copyFile(f, filepath.Join(b.out, filepath.FromSlash(shotsDir), filepath.Base(f))); err != nil {
			return err
		}
	}
	return nil
}

func isFile(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// moduleExample links a Script API function without its own example to its module's runnable example.
func (b *builder) moduleExample(e *Entry) string {
	if e.Kind != KindFunction && e.Kind != KindMethod {
		return ""
	}
	m, ok := b.byID[e.Parent]
	if !ok || m.Kind != KindModule {
		return ""
	}
	for _, s := range m.Snippets {
		if (s.Run || s.Check) && s.Lang == "aes" {
			if u, ok := b.urls[m.ID]; ok {
				return u + "#example"
			}
		}
	}
	return ""
}

const groupScriptAPI = "ref-script"

// apiIndex lists a Script API module's functions and types with their signatures, above the full sections.
func (b *builder) apiIndex(c *strings.Builder, e *Entry, root string) {
	var rows []*Entry
	consts := 0
	for _, k := range b.kids[e.ID] {
		ke := b.byID[k]
		switch ke.Kind {
		case KindFunction, KindMethod, KindType:
			rows = append(rows, ke)
		default:
			consts++
		}
	}
	if len(rows) == 0 {
		return
	}
	c.WriteString(`<h2 id="` + esc(e.ID) + `-index">Functions</h2>` + "\n")
	c.WriteString(`<div class="api-index"><label class="member-filter">` + b.icon(root, "ui.search", "") + `<input type="search" placeholder="Filter ` + strconv.Itoa(len(rows)+consts) + ` members of ` + esc(e.Title) + `" aria-label="Filter members"></label>` + "\n")
	c.WriteString(`<table class="api">` + "\n")
	for _, r := range rows {
		sig := r.Signature
		if i := strings.IndexByte(sig, '('); i >= 0 && r.Kind != KindType {
			sig = sig[i:]
		} else {
			sig = ""
		}
		href := "#" + r.ID
		if b.own[r.ID] {
			href = root + b.urls[r.ID]
		}
		c.WriteString(`<tr data-name="` + esc(strings.ToLower(r.Title)) + `"><td class="fname"><a href="` + esc(href) + `">` + b.icon(root, b.entryIcon(r), "") + esc(r.Title) + `</a></td><td><code class="lang-aes">` + b.code(root, sig) + `</code></td><td>` + inlineMD(r.Summary) + "</td></tr>\n")
	}
	c.WriteString("</table>")
	if consts > 0 {
		c.WriteString(`<p class="api-more">` + strconv.Itoa(consts) + ` constants and enum values are listed below.</p>`)
	}
	c.WriteString("</div>\n")
}

// siteEntries drops editor-only pages and everything under them.
func siteEntries(all []Entry) []Entry {
	parent := make(map[string]string, len(all))
	only := map[string]bool{}
	for _, e := range all {
		parent[e.ID] = e.Parent
		if e.EditorOnly {
			only[e.ID] = true
		}
	}
	hidden := func(id string) bool {
		for n := 0; id != "" && n < 64; n++ {
			if only[id] {
				return true
			}
			id = parent[id]
		}
		return false
	}
	out := all[:0:0]
	for _, e := range all {
		if !hidden(e.ID) {
			out = append(out, e)
		}
	}
	return out
}

func pruneNav(nodes []Node, keep map[string]bool) []Node {
	var out []Node
	for _, n := range nodes {
		if keep[n.ID] {
			n.Children = pruneNav(n.Children, keep)
			out = append(out, n)
		}
	}
	return out
}

// assetVersion names the current styles, scripts and icons, so a new build gets new asset URLs.
func (b *builder) assetVersion() string {
	h := sha256.New()
	_ = fs.WalkDir(static, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := static.ReadFile(p)
		if err == nil {
			h.Write([]byte(p))
			h.Write(data)
		}
		return err
	})
	h.Write([]byte(b.icons.sprite))
	h.Write([]byte(codeCSS()))
	return hexenc.EncodeToString(h.Sum(nil))[:12]
}
