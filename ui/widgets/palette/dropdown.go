package palette

import (
	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

// ShowPopup prepares the same searchable items for a non-modal popup. The
// caller owns the popup and must hide the palette when the popup closes.
func (p *Palette) ShowPopup(ui *core.Context) {
	p.show(ui, true)
}

// DrawPopup draws inside an already begun popup, using a bounded, scrollable
// list instead of the modal palette's two-column layout.
func (p *Palette) DrawPopup(ui *core.Context, maxListHeight float32) Result {
	res := Result{Pick: -1, Star: -1}
	if !p.open || !p.popup {
		return res
	}
	th := &ui.Theme
	p.ui = ui
	p.pointerMoved = ui.Mouse() != p.pointer
	p.pointer = ui.Mouse()
	rowH := th.RowHeight + rowPad
	qid := core.IDSeq(p.id, 1)
	if p.focus || ui.Focus() == 0 {
		ui.SetFocus(qid)
		b := ui.EditBuffer(qid, p.query)
		if p.focus {
			b.SelectAll()
		}
		p.focus = false
	}
	if ui.Focus() == qid {
		page := max(1, int(p.listH/rowH)-1)
		if p.query == "" {
			res = p.popupKeys(ui, page)
		} else {
			res = p.keys(ui, page)
		}
	}
	field := ui.Cursor().Next(0)
	widgets.SearchField(ui, qid, field, &p.query)
	composition, _ := ui.Composition()
	if p.query == "" && composition == "" && ui.Focus() == qid {
		ui.PushClip(field)
		ui.Text(widgets.Tr("editor.widgets.search", "Search"), mathx.Vec2{X: field.X + 6, Y: ui.TextY(field)}, th.TextDisabled)
		ui.PopClip()
	}
	if p.query != p.last {
		p.closeSub(ui)
		p.rebuild()
	}
	if ui.Focus() == 0 && !res.Close {
		ui.SetFocus(qid)
		ui.EditBuffer(qid, p.query)
	}
	p.listH = min(float32(max(1, len(p.rows)))*rowH, max(1, maxListHeight))
	list := ui.Cursor().Next(p.listH)
	if p.follow && p.sel >= 0 {
		widgets.EnsureVisible(&p.scroll, p.sel, rowH, list.H)
		p.follow = false
	}
	p.clicked, p.toggled, p.starred = -1, -1, -1
	widgets.VirtualList(ui, core.IDSeq(p.id, 2), list, len(p.rows), rowH, &p.scroll, p.rowFn)
	p.drawSub(ui, rowH)
	if len(p.rows) == 0 {
		ui.TextIn(list, widgets.Tr("editor.widgets.nothing_matches", "Nothing matches"), th.TextDisabled, core.AlignCenter)
	}
	if p.Hint != "" {
		hint := ui.Cursor().Next(0)
		if p.hintSrc != p.Hint || p.hintW != hint.W {
			p.hintSrc, p.hintW, p.hintOut = p.Hint, hint.W, widgets.Elide(ui, p.Hint, hint.W)
		}
		ui.TextIn(hint, p.hintOut, th.TextDim, core.AlignLeft)
		id := core.IDSeq(p.id, 3)
		ui.MouseOver(id, hint)
		widgets.Tooltip(ui, id, p.Hint)
	}
	switch {
	case p.toggled >= 0:
		p.toggle(p.toggled, rowH)
	case p.starred >= 0:
		res.Star = p.starred
	case p.clicked >= 0:
		res.Pick, res.Keep = p.clicked, ui.Mods().Has(input.ModShift)
	}
	return res
}

func (p *Palette) closeSub(ui *core.Context) {
	ui.CloseChildren()
	p.subCategory, p.subKeyboard, p.subPending = "", false, false
}

func (p *Palette) selectCategory(category string, keyboard bool) {
	if p.subCategory != category {
		p.subCategory, p.subScroll, p.subSelected = category, 0, -1
		p.subItems = p.subItems[:0]
		for i := range p.items {
			if p.items[i].Category == category {
				p.subItems = append(p.subItems, i)
				if p.subSelected < 0 && p.items[i].Disabled == "" {
					p.subSelected = len(p.subItems) - 1
				}
			}
		}
		p.subPending = true
	}
	p.subKeyboard = keyboard
	p.subFollow = keyboard
}

func (p *Palette) popupKeys(ui *core.Context, page int) Result {
	res := Result{Pick: -1, Star: -1}
	enter := ui.KeyPressed(input.KeyEnter) || ui.KeyPressed(input.KeyKPEnter)
	if p.subKeyboard && p.subCategory != "" {
		switch {
		case ui.KeyPressed(input.KeyEscape), ui.KeyPressed(input.KeyLeft):
			p.closeSub(ui)
		case enter && p.subSelected >= 0:
			res.Pick, res.Keep = p.subItems[p.subSelected], ui.Mods().Has(input.ModShift)
		case ui.Primary() && ui.KeyPressed(input.KeyD) && p.subSelected >= 0:
			res.Star = p.subItems[p.subSelected]
		case ui.KeyRepeated(input.KeyDown):
			p.moveSub(1)
		case ui.KeyRepeated(input.KeyUp):
			p.moveSub(-1)
		case ui.KeyRepeated(input.KeyPageDown):
			p.moveSub(page)
		case ui.KeyRepeated(input.KeyPageUp):
			p.moveSub(-page)
		case ui.KeyPressed(input.KeyHome):
			p.subSelected = -1
			p.moveSub(1)
		case ui.KeyPressed(input.KeyEnd):
			p.subSelected = len(p.subItems)
			p.moveSub(-1)
		}
		return res
	}
	if p.sel >= 0 && p.sel < len(p.rows) && p.rows[p.sel].kind == rowCategory && (enter || ui.KeyPressed(input.KeyRight)) {
		p.selectCategory(p.rows[p.sel].label, true)
		return res
	}
	if ui.KeyPressed(input.KeyEscape) && p.subCategory != "" {
		p.closeSub(ui)
		return res
	}
	if ui.KeyRepeated(input.KeyDown) || ui.KeyRepeated(input.KeyUp) || ui.KeyPressed(input.KeyHome) || ui.KeyPressed(input.KeyEnd) {
		p.closeSub(ui)
	}
	return p.keys(ui, page)
}

func (p *Palette) moveSub(n int) {
	dir := 1
	if n < 0 {
		dir, n = -1, -n
	}
	for range n {
		for i := p.subSelected + dir; i >= 0 && i < len(p.subItems); i += dir {
			if p.items[p.subItems[i]].Disabled == "" {
				p.subSelected = i
				break
			}
		}
	}
	p.subFollow = true
}

func (p *Palette) drawPopupCategory(i int, r mathx.Rect) {
	ui, th := p.ui, &p.ui.Theme
	rw := &p.rows[i]
	id := core.IDSeq(p.id, 100+i)
	hot := ui.MouseOver(id, r)
	pressed := ui.Pressed(id, r)
	if hot && (p.pointerMoved || pressed) {
		p.sel = i
		p.selectCategory(rw.label, false)
	}
	if p.subCategory == rw.label {
		p.subAnchor = r
	}
	if hot || p.subCategory == rw.label || (i == p.sel && p.subCategory == "") {
		ui.RoundRect(r, th.Radius.Menu, th.Selection)
	}
	if p.CategoryIcon != nil {
		if icon := p.CategoryIcon(rw.label); widgets.HasIcon(ui, icon) {
			widgets.IconAt(ui, mathx.Rect{X: r.X + 4, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, icon, th.TextDim)
		}
	}
	ui.Text(p.catTitle(rw.label), mathx.Vec2{X: r.X + 26, Y: ui.TextY(r)}, th.Text)
	widgets.Chevron(ui, mathx.Rect{X: r.X + r.W - r.H, Y: r.Y, W: r.H, H: r.H}, false, th.TextDim)
}

func (p *Palette) drawSub(ui *core.Context, rowH float32) {
	if p.subCategory == "" {
		return
	}
	id := core.IDSeq(p.id, 4)
	width := min(float32(280), ui.Size().X)
	if p.subPending {
		ui.CloseChildren()
		x := p.subAnchor.X + p.subAnchor.W + ui.Theme.Padding
		if x+width > ui.Size().X {
			x = max(0, p.subAnchor.X-ui.Theme.Padding-width)
		}
		ui.OpenPopup(id, mathx.Vec2{X: x, Y: p.subAnchor.Y - ui.Theme.Padding})
		p.subPending = false
	}
	if !ui.BeginPopup(id, width) {
		p.subCategory, p.subKeyboard = "", false
		return
	}
	height := min(float32(len(p.subItems))*rowH, max(rowH, ui.Size().Y-2*ui.Theme.Padding), 16*rowH)
	list := ui.Cursor().Next(height)
	p.subList = list
	if p.subFollow && p.subSelected >= 0 {
		widgets.EnsureVisible(&p.subScroll, p.subSelected, rowH, height)
		p.subFollow = false
	}
	widgets.VirtualList(ui, core.IDSeq(p.id, 5), list, len(p.subItems), rowH, &p.subScroll, p.drawSubRow)
	ui.EndPopup()
}

func (p *Palette) drawSubRow(i int, r mathx.Rect) {
	ui, th := p.ui, &p.ui.Theme
	item := p.subItems[i]
	it := &p.items[item]
	id := core.IDSeq(p.id, 200000+item)
	star := mathx.Rect{X: r.X + r.W - r.H, Y: r.Y, W: r.H, H: r.H}
	sid := core.IDSeq(p.id, 300000+item)
	hot := ui.MouseOver(id, r)
	if hot && (p.pointerMoved || ui.MousePressed(input.MouseLeft)) {
		p.subSelected, p.subKeyboard = i, true
		p.subFollow = false
	}
	switch {
	case ui.Pressed(sid, star):
		p.starred = item
	case ui.Pressed(id, r) && it.Disabled == "":
		p.clicked = item
	}
	if hot || (p.subKeyboard && i == p.subSelected) {
		ui.RoundRect(r, th.Radius.Menu, th.Selection)
	}
	col, tint := th.Text, th.TextDim
	if it.Disabled != "" {
		col, tint = th.TextDisabled, th.TextDisabled
	}
	if widgets.HasIcon(ui, it.Icon) {
		widgets.IconAt(ui, mathx.Rect{X: r.X + 4, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, it.Icon, tint)
	}
	ui.PushClip(mathx.Rect{X: r.X, Y: r.Y, W: max(0, r.W-r.H), H: r.H})
	ui.Text(it.Label, mathx.Vec2{X: r.X + 26, Y: ui.TextY(r)}, col)
	ui.PopClip()
	if it.Starred || ui.Hot() == id || ui.Hot() == sid {
		color := th.TextDisabled
		if it.Starred {
			color = th.Accent
		}
		widgets.IconAt(ui, mathx.Rect{X: star.X + (star.W-16)/2, Y: star.Y + (star.H-16)/2, W: 16, H: 16}, "ui.star", color)
	}
}
