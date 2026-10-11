package help

import (
	"strings"

	"aego/core/mathx"
	"aego/editor/context"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	ReferenceID  = "reference"
	tocWidth     = 210
	tocMinWidth  = 560
	codeIndent   = 14
	bulletIndent = 16
)

type kind uint8

const (
	kText kind = iota
	kH1
	kH2
	kH3
	kCode
	kTable
	kBullet
	kRule
	kBlank
)

type block struct {
	kind kind
	text string
}

type line struct {
	kind   kind
	text   string
	indent float32
}

type entry struct {
	label string
	line  int
	depth int
}

type Reference struct {
	load      func() ([]byte, error)
	loaded    bool
	err       string
	blocks    []block
	width     float32
	lines     []line
	toc       []entry
	scroll    float32
	tocScroll float32
	query     string
	last      string
	hits      []int
	hit       int
	follow    bool
}

func NewReference(load func() ([]byte, error)) *Reference {
	return &Reference{load: load, last: "\x00"}
}

func (r *Reference) ID() string {
	return ReferenceID
}

func (r *Reference) Title() string {
	return "Script Reference"
}

func clean(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "**", ""), "`", "")
}

func parse(src string) []block {
	var out []block
	var para []string
	inCode := false
	flush := func() {
		if len(para) > 0 {
			out = append(out, block{kText, clean(strings.Join(para, " "))})
			para = para[:0]
		}
	}
	for _, raw := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(raw)
		if strings.HasPrefix(t, "```") {
			flush()
			inCode = !inCode
			if !inCode {
				out = append(out, block{kind: kBlank})
			}
			continue
		}
		if inCode {
			out = append(out, block{kCode, strings.ReplaceAll(raw, "\t", "    ")})
			continue
		}
		switch {
		case t == "":
			flush()
			out = append(out, block{kind: kBlank})
		case strings.HasPrefix(t, "### "):
			flush()
			out = append(out, block{kH3, clean(t[4:])})
		case strings.HasPrefix(t, "## "):
			flush()
			out = append(out, block{kH2, clean(t[3:])})
		case strings.HasPrefix(t, "# "):
			flush()
			out = append(out, block{kH1, clean(t[2:])})
		case t == "---":
			flush()
			out = append(out, block{kind: kRule})
		case strings.HasPrefix(t, "|"):
			flush()
			if strings.Trim(t, "|-: ") == "" {
				continue
			}
			cells := strings.Split(strings.Trim(t, "|"), "|")
			for i := range cells {
				cells[i] = clean(strings.TrimSpace(cells[i]))
			}
			out = append(out, block{kTable, strings.Join(cells, "   |   ")})
		case strings.HasPrefix(t, "- "), strings.HasPrefix(t, "* "):
			flush()
			out = append(out, block{kBullet, clean(t[2:])})
		default:
			para = append(para, t)
		}
	}
	flush()
	return out
}

func wrap(ui *core.Context, s string, w float32) []string {
	if w <= 40 || ui.TextSize(s).X <= w {
		return []string{s}
	}
	var out []string
	cur := ""
	for _, word := range strings.Fields(s) {
		next := word
		if cur != "" {
			next = cur + " " + word
		}
		if cur != "" && ui.TextSize(next).X > w {
			out = append(out, cur)
			cur = word
			continue
		}
		cur = next
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func (r *Reference) blank() {
	if n := len(r.lines); n > 0 && r.lines[n-1].kind != kBlank {
		r.lines = append(r.lines, line{kind: kBlank})
	}
}

func (r *Reference) layout(ui *core.Context, w float32) {
	r.width = w
	r.lines, r.toc = r.lines[:0], r.toc[:0]
	bullet := widgets.Glyph(ui, '•', "-") + " "
	for _, b := range r.blocks {
		switch b.kind {
		case kH1, kH2, kH3:
			r.blank()
			r.toc = append(r.toc, entry{label: b.text, line: len(r.lines), depth: int(b.kind - kH1)})
			r.lines = append(r.lines, line{kind: b.kind, text: b.text})
		case kText:
			for _, s := range wrap(ui, b.text, w) {
				r.lines = append(r.lines, line{kind: kText, text: s})
			}
		case kBullet:
			for i, s := range wrap(ui, b.text, w-bulletIndent) {
				if i == 0 {
					r.lines = append(r.lines, line{kind: kBullet, text: bullet + s})
					continue
				}
				r.lines = append(r.lines, line{kind: kBullet, text: s, indent: bulletIndent})
			}
		case kCode:
			r.lines = append(r.lines, line{kind: kCode, text: b.text, indent: codeIndent})
		case kTable:
			r.lines = append(r.lines, line{kind: kTable, text: b.text})
		case kRule:
			r.lines = append(r.lines, line{kind: kRule})
		case kBlank:
			r.blank()
		}
	}
	r.last = "\x00"
}

func (r *Reference) search() {
	r.last = r.query
	r.hits, r.hit = r.hits[:0], 0
	q := strings.ToLower(strings.TrimSpace(r.query))
	if q == "" {
		return
	}
	for i, l := range r.lines {
		if strings.Contains(strings.ToLower(l.text), q) {
			r.hits = append(r.hits, i)
		}
	}
	r.follow = true
}

func (r *Reference) next() {
	if len(r.hits) == 0 {
		return
	}
	r.hit = (r.hit + 1) % len(r.hits)
	r.follow = true
}

func (r *Reference) Draw(ctx *context.Editor, rc mathx.Rect) {
	ui := ctx.UI
	th := &ui.Theme
	if !r.loaded {
		r.loaded = true
		if data, err := r.load(); err != nil {
			r.err = err.Error()
		} else {
			r.blocks = parse(string(data))
		}
	}
	if r.err != "" {
		widgets.LabelWrapped(ui, "The reference is not available: "+r.err, true)
		return
	}
	bar := ui.Cursor().Next(0)
	qid := ui.ID("search")
	enter := ui.Focus() == qid && (ui.KeyPressed(input.KeyEnter) || ui.KeyPressed(input.KeyKPEnter))
	widgets.SearchField(ui, qid, bar, &r.query)
	if enter {
		r.next()
		ui.SetFocus(qid)
	}
	top := bar.Y + bar.H + th.Spacing
	body := mathx.Rect{X: rc.X, Y: top, W: rc.W, H: max(0, rc.Y+rc.H-top)}
	content := body
	if body.W > tocMinWidth {
		r.drawTOC(ui, mathx.Rect{X: body.X, Y: body.Y, W: tocWidth, H: body.H})
		content = mathx.Rect{X: body.X + tocWidth + 6, Y: body.Y, W: body.W - tocWidth - 6, H: body.H}
	}
	if w := content.W - 2*th.Padding - th.ScrollbarW; w != r.width {
		r.layout(ui, w)
	}
	if r.query != r.last {
		r.search()
	}
	rowH := th.RowHeight
	if r.follow && len(r.hits) > 0 {
		widgets.EnsureVisible(&r.scroll, r.hits[r.hit], rowH, content.H)
	}
	r.follow = false
	cur := -1
	if len(r.hits) > 0 {
		cur = r.hits[r.hit]
	}
	widgets.VirtualList(ui, ui.ID("text"), content, len(r.lines), rowH, &r.scroll, func(i int, row mathx.Rect) {
		l := r.lines[i]
		x := row.X + th.Padding + l.indent
		ty := ui.TextY(row)
		if l.kind == kCode {
			ui.Rect(row, th.Code.Background)
		}
		if i == cur {
			ui.Rect(row, th.SelectionDim)
		}
		switch l.kind {
		case kH1, kH2:
			ui.Text(l.text, mathx.Vec2{X: x, Y: ty}, th.Accent)
			ui.Rect(mathx.Rect{X: x, Y: row.Y + row.H - 2, W: row.W - 2*th.Padding, H: 1}, th.Separator)
		case kH3:
			ui.Text(l.text, mathx.Vec2{X: x, Y: ty}, th.Warn)
		case kCode:
			ui.Text(l.text, mathx.Vec2{X: x, Y: ty}, th.Code.Plain)
		case kTable:
			ui.Text(l.text, mathx.Vec2{X: x, Y: ty}, th.TextDim)
		case kRule:
			ui.Rect(mathx.Rect{X: x, Y: row.Y + row.H/2, W: row.W - 2*th.Padding, H: 1}, th.Border)
		default:
			ui.Text(l.text, mathx.Vec2{X: x, Y: ty}, th.Text)
		}
	})
}

func (r *Reference) drawTOC(ui *core.Context, rc mathx.Rect) {
	th := &ui.Theme
	ui.Rect(rc, th.PanelAlt)
	widgets.VirtualList(ui, ui.ID("toc"), rc, len(r.toc), th.RowHeight, &r.tocScroll, func(i int, row mathx.Rect) {
		e := r.toc[i]
		id := core.IDSeq(ui.ID("tocrow"), i)
		if ui.Pressed(id, row) {
			r.scroll = float32(e.line) * th.RowHeight
		}
		if ui.Hot() == id {
			ui.Rect(row, th.WidgetHot)
		}
		col := th.Text
		if e.depth == 0 {
			col = th.Accent
		} else if e.depth > 1 {
			col = th.TextDim
		}
		ui.Text(e.label, mathx.Vec2{X: row.X + 6 + float32(e.depth)*th.Indent, Y: ui.TextY(row)}, col)
	})
}
