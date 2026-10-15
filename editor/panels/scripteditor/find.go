package scripteditor

import (
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/code/search"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

func (p *Panel) options() search.Options {
	return search.Options{Case: p.find.matchCase}
}

func (p *Panel) openFind(replace bool) {
	p.find.open, p.find.replace, p.find.focus = true, replace, true
	if s := p.doc.Code.Selected(); s != "" && !strings.Contains(s, "\n") {
		p.find.query = s
	}
}

func (p *Panel) findNext(ui *core.Context, back bool) {
	c := p.doc.Code
	q := p.find.query
	if q == "" {
		return
	}
	lo, hi := c.Sel.Range()
	from := hi
	if back {
		from = lo
	}
	off, ok := search.Find(c.Bytes(), q, from, back, p.options())
	if !ok {
		p.flash(ui.Time, "No matches for "+q)
		return
	}
	c.Select(off, off+len(q))
	p.reveal = true
}

func (p *Panel) replaceOne(ui *core.Context) {
	c := p.doc.Code
	sel := c.Selected()
	same := sel == p.find.query || (!p.find.matchCase && strings.EqualFold(sel, p.find.query))
	if sel != "" && same {
		c.ReplaceSelection(p.find.repl)
	}
	p.findNext(ui, false)
}

func (p *Panel) drawFind(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	ui.Rect(r, th.PanelAlt)
	row := mathx.Rect{X: r.X + 4, Y: r.Y + 2, W: r.W - 8, H: th.RowHeight}
	bw := th.RowHeight + 8
	field := mathx.Rect{X: row.X, Y: row.Y, W: max(60, row.W-3*(bw+4)-48), H: row.H}
	qid := core.IDSeq(p.fid, 10)
	if p.find.focus {
		ui.SetFocus(qid)
		ui.EditBuffer(qid, p.find.query).SelectAll()
		p.find.focus = false
	}
	focused := ui.Focus() == qid
	enter := focused && (ui.KeyPressed(input.KeyEnter) || ui.KeyPressed(input.KeyKPEnter))
	escape := focused && ui.KeyPressed(input.KeyEscape)
	shift := ui.Mods().Has(input.ModShift)
	widgets.TextFieldRect(ui, qid, field, &p.find.query, true)
	if enter {
		p.findNext(ui, shift)
		ui.SetFocus(qid)
	}
	if escape {
		p.find.open = false
		ui.SetFocus(p.fid)
		return
	}
	x := field.X + field.W + 4
	if widgets.ButtonRect(ui, core.IDSeq(p.fid, 11), mathx.Rect{X: x, Y: row.Y, W: bw, H: row.H}, widgets.Glyph(ui, '↑', "^")) {
		p.findNext(ui, true)
	}
	x += bw + 4
	if widgets.ButtonRect(ui, core.IDSeq(p.fid, 12), mathx.Rect{X: x, Y: row.Y, W: bw, H: row.H}, widgets.Glyph(ui, '↓', "v")) {
		p.findNext(ui, false)
	}
	x += bw + 4
	widgets.ToggleButton(ui, core.IDSeq(p.fid, 13), mathx.Rect{X: x, Y: row.Y, W: 40, H: row.H}, "Aa", &p.find.matchCase)
	if !p.find.replace {
		return
	}
	row.Y += row.H + 4
	rf := mathx.Rect{X: row.X, Y: row.Y, W: field.W, H: row.H}
	widgets.TextFieldRect(ui, core.IDSeq(p.fid, 14), rf, &p.find.repl, true)
	x = rf.X + rf.W + 4
	if widgets.ButtonRect(ui, core.IDSeq(p.fid, 15), mathx.Rect{X: x, Y: row.Y, W: 70, H: row.H}, "Replace") {
		p.replaceOne(ui)
	}
	x += 74
	if widgets.ButtonRect(ui, core.IDSeq(p.fid, 16), mathx.Rect{X: x, Y: row.Y, W: 50, H: row.H}, "All") {
		n := p.doc.Code.ReplaceAll(p.find.query, p.find.repl, p.options())
		p.flash(ui.Time, "Replaced "+strconv.Itoa(n)+" occurrence(s)")
	}
}
