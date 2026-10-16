package docs

import (
	"slices"
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/services"
	regdocs "aego/engine/docs"
	"aego/ui/core"
	"aego/ui/richtext"
	"aego/ui/widgets"
)

const (
	ID         = "reference"
	Language   = "language"
	historyMax = 64
	outputLang = "output"
)

type row struct {
	id    string
	depth int
	title string
	kids  bool
}

type Panel struct {
	Run     func(code string) []string
	reg     *regdocs.Registry
	nav     services.Navigation
	notes   services.Notices
	query   string
	shown   string
	rows    []row
	open    map[string]bool
	current string
	back    []string
	fwd     []string
	text    richtext.Text
	scroll  float32
	split   float32
	dirty   bool
	reveal  bool
}

func New(reg *regdocs.Registry, nav services.Navigation, notes services.Notices) *Panel {
	p := &Panel{reg: reg, nav: nav, notes: notes, open: map[string]bool{}, split: 0.3, dirty: true}
	p.text.Highlight = Highlight
	return p
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	return "Documentation"
}

func (p *Panel) Scope() string {
	return "docs"
}

func (p *Panel) Current() string {
	return p.current
}

func (p *Panel) Show(ref, query string) {
	if p.reg == nil {
		return
	}
	if ref != "" {
		if e, ok := p.reg.Resolve(ref); ok {
			p.query = ""
			p.navigate(e.ID, true)
			return
		}
	}
	if q := strings.TrimSpace(query); q != "" {
		p.query = q
		p.dirty = true
		if hits := p.reg.Search(q, 1); len(hits) > 0 {
			p.navigate(hits[0].ID, true)
		}
		return
	}
	if p.current == "" {
		p.navigate(Language, true)
	}
}

func (p *Panel) navigate(id string, record bool) {
	e, ok := p.reg.Get(id)
	if !ok {
		return
	}
	if record && p.current != "" && p.current != id {
		p.back = append(p.back, p.current)
		if len(p.back) > historyMax {
			p.back = p.back[1:]
		}
		p.fwd = p.fwd[:0]
	}
	p.current = id
	for a := e.Parent; a != ""; {
		p.open[a] = true
		pe, ok := p.reg.Get(a)
		if !ok {
			break
		}
		a = pe.Parent
	}
	p.text.Set(p.page(e))
	p.dirty, p.reveal = true, true
}

func (p *Panel) step(from, to *[]string) {
	n := len(*from)
	if n == 0 {
		return
	}
	id := (*from)[n-1]
	*from = (*from)[:n-1]
	*to = append(*to, p.current)
	p.navigate(id, false)
}

func stripTitle(md string) string {
	t := strings.TrimLeft(md, "\n")
	if !strings.HasPrefix(t, "# ") {
		return md
	}
	if i := strings.IndexByte(t, '\n'); i >= 0 {
		return t[i+1:]
	}
	return ""
}

func spans(s string, st richtext.Style) []richtext.Span {
	return []richtext.Span{{Text: s, Style: st}}
}

func (p *Panel) page(e regdocs.Entry) []richtext.Block {
	out := []richtext.Block{{Kind: richtext.Heading, Level: 1, Spans: spans(e.Title, richtext.Plain)}}
	if e.Kind != regdocs.Page {
		meta := e.Kind.String()
		if e.Owner != "" {
			meta += " · " + e.Owner
		}
		out = append(out, richtext.Block{Kind: richtext.Paragraph, Spans: spans(meta, richtext.Emphasis)})
	}
	if e.Signature != "" {
		out = append(out, richtext.Block{Kind: richtext.CodeBlock, Lang: "aes", Lines: strings.Split(e.Signature, "\n")})
	}
	out = append(out, richtext.Parse(stripTitle(e.Body))...)
	if e.Example != "" {
		out = append(out,
			richtext.Block{Kind: richtext.Heading, Level: 3, Spans: spans("Example", richtext.Plain)},
			richtext.Block{Kind: richtext.CodeBlock, Lang: "aes", Lines: strings.Split(e.Example, "\n")})
	}
	var links []richtext.Span
	if e.Location.Stub != "" {
		links = append(links, richtext.Span{Text: "Open stub", Target: e.Location.Stub + "#" + e.Location.Symbol})
	}
	if strings.HasPrefix(e.ID, regdocs.ScriptsPrefix) && e.Location.Path != "" {
		if len(links) > 0 {
			links = append(links, richtext.Span{Text: "  ·  "})
		}
		links = append(links, richtext.Span{Text: "Open source", Target: "open:" + e.Location.Path + ":" + strconv.Itoa(e.Location.Line)})
	}
	if len(links) > 0 {
		out = append(out, richtext.Block{Kind: richtext.Paragraph, Spans: links})
	}
	if kids := p.reg.Children(e.ID); len(kids) > 0 {
		out = append(out, richtext.Block{Kind: richtext.Heading, Level: 2, Spans: spans("Contents", richtext.Plain)})
		for _, k := range kids {
			item := []richtext.Span{{Text: k.Title, Target: "ref:" + k.ID}}
			if k.Summary != "" {
				item = append(item, richtext.Span{Text: " — " + k.Summary})
			}
			out = append(out, richtext.Block{Kind: richtext.ListItem, Spans: item})
		}
	}
	p.autolink(out)
	return out
}

func (p *Panel) link(list []richtext.Span) {
	for i := range list {
		sp := &list[i]
		if sp.Style == richtext.Code && sp.Target == "" {
			if _, ok := p.reg.Get(sp.Text); ok {
				sp.Target = "ref:" + sp.Text
			}
		}
	}
}

func (p *Panel) autolink(blocks []richtext.Block) {
	for i := range blocks {
		p.link(blocks[i].Spans)
		for _, r := range blocks[i].Rows {
			for _, c := range r {
				p.link(c)
			}
		}
	}
}

func (p *Panel) rebuild() {
	p.dirty = false
	p.shown = p.query
	p.rows = p.rows[:0]
	if q := strings.TrimSpace(p.query); q != "" {
		for _, e := range p.reg.Search(q, 200) {
			p.rows = append(p.rows, row{id: e.ID, title: e.Title + "  ·  " + e.Kind.String()})
		}
		return
	}
	var walk func(list []regdocs.Entry, depth int)
	walk = func(list []regdocs.Entry, depth int) {
		for _, e := range list {
			kids := p.reg.Children(e.ID)
			p.rows = append(p.rows, row{id: e.ID, depth: depth, title: e.Title, kids: len(kids) > 0})
			if len(kids) > 0 && p.open[e.ID] {
				walk(kids, depth+1)
			}
		}
	}
	walk(p.reg.Roots(), 0)
}

func (p *Panel) follow(target string) {
	switch {
	case strings.HasPrefix(target, context.StubPrefix):
		path, sym, _ := strings.Cut(target, "#")
		l := context.At(path, 1, 1)
		l.Symbol = sym
		p.nav.Open(l)
	case strings.HasPrefix(target, "open:"):
		rest := strings.TrimPrefix(target, "open:")
		path, line := rest, 1
		if i := strings.LastIndexByte(rest, ':'); i > 0 {
			if n, err := strconv.Atoi(rest[i+1:]); err == nil {
				path, line = rest[:i], n
			}
		}
		p.nav.Open(context.At(path, line, 1))
	case strings.HasPrefix(target, "http://"), strings.HasPrefix(target, "https://"):
		p.notes.Notify(context.LevelInfo, target)
	default:
		if e, ok := p.reg.Resolve(target); ok {
			p.navigate(e.ID, true)
			return
		}
		p.notes.Notify(context.LevelWarn, "No documentation entry "+strings.TrimPrefix(target, "ref:"))
	}
}

func (p *Panel) run(i int) {
	blocks := p.text.Blocks()
	if p.Run == nil || i < 0 || i >= len(blocks) {
		return
	}
	out := p.Run(strings.Join(blocks[i].Lines, "\n"))
	if next := i + 1; next < len(blocks) && blocks[next].Lang == outputLang {
		blocks[next].Lines = out
	} else {
		blocks = slices.Insert(blocks, next, richtext.Block{Kind: richtext.CodeBlock, Lang: outputLang, Lines: out})
	}
	p.text.Replace(blocks)
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	if p.reg == nil {
		widgets.LabelWrapped(ui, "Documentation is not loaded.", true)
		return
	}
	if p.current == "" {
		p.navigate(Language, false)
	}
	left, right := widgets.Splitter(ui, ui.ID("split"), r, false, &p.split)
	p.drawTree(ui, left)
	p.drawPage(ui, right)
}

func (p *Panel) drawTree(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	bar := mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: th.RowHeight + 4}
	if widgets.SearchField(ui, ui.ID("search"), bar, &p.query) || p.query != p.shown {
		p.dirty = true
	}
	if p.dirty {
		p.rebuild()
	}
	list := mathx.Rect{X: r.X, Y: bar.Y + bar.H + th.Spacing, W: r.W, H: max(0, r.H-bar.H-th.Spacing)}
	if p.reveal {
		p.reveal = false
		for i, rw := range p.rows {
			if rw.id == p.current {
				widgets.EnsureVisible(&p.scroll, i, th.RowHeight, list.H)
				break
			}
		}
	}
	base := ui.ID("row")
	widgets.VirtualList(ui, ui.ID("tree"), list, len(p.rows), th.RowHeight, &p.scroll, func(i int, rr mathx.Rect) {
		rw := p.rows[i]
		ev := widgets.TreeRowRect(ui, core.IDSeq(base, i), rr, widgets.TreeRow{
			Depth: rw.depth, Label: rw.title, HasChildren: rw.kids, Open: p.open[rw.id],
			Selected: rw.id == p.current, Focused: true,
		})
		switch {
		case ev.Toggled:
			p.open[rw.id] = !p.open[rw.id]
			p.dirty = true
		case ev.Pressed:
			if rw.kids && rw.id == p.current {
				p.open[rw.id] = !p.open[rw.id]
				p.dirty = true
			}
			p.navigate(rw.id, true)
		}
	})
}

func (p *Panel) crumbs(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	e, ok := p.reg.Get(p.current)
	if !ok {
		return
	}
	var trail []regdocs.Entry
	for a := e.Parent; a != ""; {
		pe, ok := p.reg.Get(a)
		if !ok {
			break
		}
		trail = append([]regdocs.Entry{pe}, trail...)
		a = pe.Parent
	}
	x := r.X
	jump := ""
	ui.PushClip(r)
	for i, pe := range trail {
		w := ui.TextSize(pe.Title).X
		cr := mathx.Rect{X: x, Y: r.Y, W: w, H: r.H}
		cid := core.IDSeq(ui.ID("crumb"), i)
		if ui.Clicked(cid, cr) {
			jump = pe.ID
		}
		col := th.TextDim
		if ui.Hot() == cid {
			col = th.Accent
		}
		ui.Text(pe.Title, mathx.Vec2{X: x, Y: ui.TextY(r)}, col)
		x += w
		ui.Text(" / ", mathx.Vec2{X: x, Y: ui.TextY(r)}, th.TextDisabled)
		x += ui.TextSize(" / ").X
	}
	ui.Text(e.Title, mathx.Vec2{X: x, Y: ui.TextY(r)}, th.Text)
	ui.PopClip()
	if jump != "" {
		p.navigate(jump, true)
	}
}

func (p *Panel) drawPage(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	bar := mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: th.RowHeight + 4}
	bw := bar.H
	if widgets.ButtonRect(ui, ui.ID("back"), mathx.Rect{X: bar.X, Y: bar.Y, W: bw, H: bar.H}, widgets.Glyph(ui, '←', "<")) {
		p.step(&p.back, &p.fwd)
	}
	if widgets.ButtonRect(ui, ui.ID("fwd"), mathx.Rect{X: bar.X + bw + 2, Y: bar.Y, W: bw, H: bar.H}, widgets.Glyph(ui, '→', ">")) {
		p.step(&p.fwd, &p.back)
	}
	cx := bar.X + 2*bw + 10
	p.crumbs(ui, mathx.Rect{X: cx, Y: bar.Y, W: max(0, bar.X+bar.W-cx), H: bar.H})
	body := mathx.Rect{X: r.X, Y: bar.Y + bar.H + th.Spacing, W: r.W, H: max(0, r.H-bar.H-th.Spacing)}
	p.text.RunButtons = p.Run != nil
	act := p.text.Draw(ui, ui.ID("page"), body)
	if act.Target != "" {
		p.follow(act.Target)
	}
	if act.Run >= 0 {
		p.run(act.Run)
	}
}
