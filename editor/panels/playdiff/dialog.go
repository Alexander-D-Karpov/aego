package playdiff

import (
	"strconv"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	pd "aego/editor/core/playdiff"
	"aego/editor/core/services"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

type Deps struct {
	Docs    services.Documents
	Notices services.Notices
}

type Dialog struct {
	d      Deps
	id     core.ID
	open   bool
	doc    document.ID
	res    pd.Result
	scroll float32
}

func New(d Deps) *Dialog {
	return &Dialog{d: d, id: core.IDOf("playdiff")}
}

func (g *Dialog) Active() bool {
	return g.open
}

func (g *Dialog) Open(ui *core.Context, doc document.ID, res pd.Result) {
	g.open, g.doc, g.res, g.scroll = true, doc, res, 0
	ui.OpenModal(g.id)
}

func (g *Dialog) close(ui *core.Context) {
	g.open = false
	g.res = pd.Result{}
	ui.CloseModal()
}

func check(ui *core.Context, id core.ID, r mathx.Rect, state int) bool {
	th := &ui.Theme
	clicked := ui.Clicked(id, r)
	box := mathx.Rect{X: r.X + 3, Y: r.Y + 3, W: r.W - 6, H: r.H - 6}
	switch state {
	case 1:
		ui.Rect(box, th.Accent)
		widgets.CheckMark(ui, box, th.Background)
	case 2:
		ui.Rect(box, ui.WidgetColor(id))
		ui.Outline(box, th.Border)
		ui.Rect(mathx.Rect{X: box.X + 3, Y: box.Y + box.H/2 - 1, W: box.W - 6, H: 2}, th.Accent)
	default:
		ui.Rect(box, ui.WidgetColor(id))
		ui.Outline(box, th.Border)
	}
	return clicked
}

type row struct {
	header bool
	first  int
	last   int
}

func (g *Dialog) rows() []row {
	var out []row
	ch := g.res.Changes
	for i := 0; i < len(ch); {
		j := i
		for j < len(ch) && ch[j].Node == ch[i].Node {
			j++
		}
		out = append(out, row{header: true, first: i, last: j})
		for k := i; k < j; k++ {
			out = append(out, row{first: k, last: k + 1})
		}
		i = j
	}
	return out
}

func (g *Dialog) state(a, b int) int {
	on := 0
	for i := a; i < b; i++ {
		if g.res.Changes[i].Checked {
			on++
		}
	}
	switch on {
	case 0:
		return 0
	case b - a:
		return 1
	}
	return 2
}

func (g *Dialog) set(a, b int, v bool) {
	for i := a; i < b; i++ {
		g.res.Changes[i].Checked = v
	}
}

func (g *Dialog) apply() {
	d := g.d.Docs.Documents().Get(g.doc)
	if d == nil {
		g.d.Notices.Notify(context.LevelError, "The scene was closed, the changes are dropped")
		return
	}
	n, err := pd.Apply(d, g.res.Changes)
	switch {
	case err != nil:
		g.d.Notices.Notify(context.LevelError, err.Error())
	case n > 0:
		g.d.Notices.Notify(context.LevelInfo, "Applied "+strconv.Itoa(n)+" change(s) from Play, Undo reverts them")
	}
}

func (g *Dialog) Draw(ui *core.Context) {
	if !g.open {
		return
	}
	th := &ui.Theme
	sz := ui.Size()
	w := min(760, sz.X-40)
	h := min(520, sz.Y-60)
	r := mathx.Rect{X: (sz.X - w) / 2, Y: max(20, sz.Y*0.1), W: w, H: h}
	if !ui.BeginModal(g.id, r) {
		g.open = false
		return
	}
	if ui.KeyPressed(input.KeyEscape) {
		ui.EndModal()
		g.close(ui)
		return
	}
	title := ui.Cursor().Next(0)
	ui.Text("Changes made during Play", mathx.Vec2{X: title.X, Y: ui.TextY(title)}, th.Text)
	all := g.state(0, len(g.res.Changes))
	ar := mathx.Rect{X: title.X + title.W - title.H, Y: title.Y, W: title.H, H: title.H}
	if len(g.res.Changes) > 0 && check(ui, ui.ID("all"), ar, all) {
		g.set(0, len(g.res.Changes), all != 1)
	}
	widgets.LabelDim(ui, "Fields you changed in the Inspector during Play are checked. Everything else changed by the game itself.")
	rows := g.rows()
	foot := th.RowHeight*2 + th.Padding*2
	if len(g.res.Created)+len(g.res.Removed) > 0 {
		foot += th.RowHeight
	}
	cur := ui.Cursor()
	list := mathx.Rect{X: cur.X, Y: cur.Y, W: cur.W, H: max(0, r.Y+r.H-cur.Y-foot)}
	rowH := th.RowHeight + 2
	widgets.VirtualList(ui, ui.ID("list"), list, len(rows), rowH, &g.scroll, func(i int, rr mathx.Rect) {
		g.drawRow(ui, i, rows[i], rr)
	})
	if len(rows) == 0 {
		ui.TextIn(list, "No fields differ from the scene", th.TextDisabled, core.AlignCenter)
	}
	ui.Cursor().Y = list.Y + list.H + th.Spacing
	if n, m := len(g.res.Created), len(g.res.Removed); n+m > 0 {
		widgets.LabelDim(ui, strconv.Itoa(n)+" node(s) created and "+strconv.Itoa(m)+" removed during Play are not transferred")
	}
	btn := ui.Cursor().Next(0)
	bw := float32(140)
	if widgets.ButtonRect(ui, ui.ID("discard"), mathx.Rect{X: btn.X + btn.W - bw, Y: btn.Y, W: bw, H: btn.H}, "Discard") {
		ui.EndModal()
		g.close(ui)
		return
	}
	n := 0
	for i := range g.res.Changes {
		if g.res.Changes[i].Checked {
			n++
		}
	}
	label := "Apply " + strconv.Itoa(n) + " to scene"
	if widgets.ButtonRect(ui, ui.ID("apply"), mathx.Rect{X: btn.X + btn.W - 2*bw - 6, Y: btn.Y, W: bw, H: btn.H}, label) && n > 0 {
		g.apply()
		ui.EndModal()
		g.close(ui)
		return
	}
	ui.EndModal()
}

func (g *Dialog) drawRow(ui *core.Context, i int, rw row, rr mathx.Rect) {
	th := &ui.Theme
	id := core.IDSeq(ui.ID("row"), i)
	cb := mathx.Rect{X: rr.X, Y: rr.Y, W: rr.H, H: rr.H}
	if rw.header {
		ui.Rect(rr, th.CardHeader)
		st := g.state(rw.first, rw.last)
		if check(ui, id, cb, st) {
			g.set(rw.first, rw.last, st != 1)
		}
		ui.PushClip(rr)
		ui.Text(g.res.Changes[rw.first].Path, mathx.Vec2{X: cb.X + cb.W + 4, Y: ui.TextY(rr)}, th.Text)
		ui.PopClip()
		return
	}
	c := &g.res.Changes[rw.first]
	cb.X += th.Indent
	if check(ui, id, cb, map[bool]int{false: 0, true: 1}[c.Checked]) {
		c.Checked = !c.Checked
	}
	x := cb.X + cb.W + 4
	name := c.Owner
	if c.Kind == pd.KindField {
		name += " · " + c.Label
	}
	nameW := min(rr.W*0.3, ui.TextSize(name).X+8)
	ui.PushClip(mathx.Rect{X: x, Y: rr.Y, W: nameW, H: rr.H})
	ui.Text(name, mathx.Vec2{X: x, Y: ui.TextY(rr)}, th.TextDim)
	ui.PopClip()
	x += nameW + 8
	text := c.Text(false) + "  →  " + c.Text(true)
	ui.PushClip(mathx.Rect{X: x, Y: rr.Y, W: max(0, rr.X+rr.W-x), H: rr.H})
	ui.Text(text, mathx.Vec2{X: x, Y: ui.TextY(rr)}, th.Text)
	ui.PopClip()
	widgets.Tooltip(ui, id, c.Text(false)+"\n→ "+c.Text(true))
}
