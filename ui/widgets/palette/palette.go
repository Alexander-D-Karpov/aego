// Package palette is the searchable picker of Add Component and Create (D344).
package palette

import (
	"slices"
	"strconv"
	"unicode/utf8"

	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

type Item struct {
	Key   string
	Label string
	// Keywords are alternative names used only for search, never for display.
	Keywords []string
	Category string
	Detail   string
	Icon     string
	Disabled string
	Uses     int
	Starred  bool
}

type Result struct {
	Pick  int
	Keep  bool
	Star  int
	Close bool
}

type rowKind uint8

const (
	rowItem rowKind = iota
	rowSection
	rowCategory
)

type row struct {
	kind   rowKind
	item   int
	label  string
	pos    [2]int
	nested bool
}

type hit struct {
	item int
	tier Tier
	pos  [2]int
}

type Palette struct {
	Recent   []int
	Open     map[string]bool
	Title    string
	Hint     string
	Describe func(ui *core.Context, r mathx.Rect, item int)
	// NoFavorites hides the star action in pickers that do not support favorites.
	NoFavorites bool
	// CategoryIcon names the icon drawn before a category; "" draws none.
	CategoryIcon func(category string) string
	// CategoryTitle is the shown name of a category; nil shows the category as is.
	CategoryTitle func(category string) string
	// RecentTitle and FavoritesTitle name the two sections above the categories.
	RecentTitle, FavoritesTitle string

	hintSrc, hintOut string
	hintW            float32
	id               core.ID
	items            []Item
	reasons          []string
	reasonW          float32
	text             []Text
	keywords         [][]Text
	cats             []string
	query            string
	last             string
	q                []rune
	hits             []hit
	pos              []int
	rows             []row
	sel              int
	scroll           float32
	follow           bool
	focus            bool
	open             bool
	popup            bool
	count            string
	ui               *core.Context
	clicked          int
	toggled          int
	starred          int
	rowFn            func(int, mathx.Rect)
	listH            float32
	opened           uint64
	subCategory      string
	subItems         []int
	subSelected      int
	subScroll        float32
	subKeyboard      bool
	subFollow        bool
	subPending       bool
	subAnchor        mathx.Rect
	subList          mathx.Rect
	pointer          mathx.Vec2
	pointerMoved     bool
}

const rowPad = 2

func New(id core.ID) *Palette {
	p := &Palette{id: id, Open: map[string]bool{}, clicked: -1, toggled: -1, starred: -1}
	p.rowFn = p.drawRow
	return p
}

func (p *Palette) Active() bool      { return p.open }
func (p *Palette) Query() string     { return p.query }
func (p *Palette) Items() []Item     { return p.items }
func (p *Palette) Item(i int) *Item  { return &p.items[i] }
func (p *Palette) SearchID() core.ID { return core.IDSeq(p.id, 1) }

// Selected is the highlighted item, or -1.
func (p *Palette) Selected() int {
	if p.sel >= 0 && p.sel < len(p.rows) && p.rows[p.sel].kind == rowItem {
		return p.rows[p.sel].item
	}
	return -1
}

func (p *Palette) SetItems(items []Item) {
	p.items = items
	p.reasonW = -1
	p.text = p.text[:0]
	p.keywords = p.keywords[:0]
	p.cats = p.cats[:0]
	for i := range items {
		p.text = append(p.text, Prepare(items[i].Label))
		keywords := make([]Text, len(items[i].Keywords))
		for j, keyword := range items[i].Keywords {
			keywords[j] = Prepare(keyword)
		}
		p.keywords = append(p.keywords, keywords)
		if c := items[i].Category; !slices.Contains(p.cats, c) {
			p.cats = append(p.cats, c)
		}
	}
	p.rebuild()
}

func (p *Palette) SetQuery(q string) {
	p.query = q
	p.rebuild()
}

// Refresh rebuilds the rows after the caller changed Recent, Open or the stars.
func (p *Palette) Refresh() {
	keep := p.Selected()
	p.rebuild()
	if keep >= 0 {
		for i, r := range p.rows {
			if r.kind == rowItem && r.item == keep {
				p.sel = i
				break
			}
		}
	}
}

// Show opens the palette with an empty query; the search field takes the keyboard at once.
func (p *Palette) Show(ui *core.Context) {
	p.show(ui, false)
	ui.OpenModal(p.id)
}

func (p *Palette) show(ui *core.Context, popup bool) {
	p.open, p.focus, p.scroll, p.opened = true, true, 0, ui.Frame()
	p.popup = popup
	p.subCategory, p.subKeyboard, p.subPending = "", false, false
	p.pointer = ui.Mouse()
	p.query = ""
	p.rebuild()
}

func (p *Palette) Hide(ui *core.Context) {
	if !p.open {
		return
	}
	p.open = false
	if !p.popup {
		if ui.ModalOpen(p.id) {
			ui.CloseModal()
		}
		ui.CancelFocus()
	} else if ui.Focus() == core.IDSeq(p.id, 1) {
		ui.CancelFocus()
	}
}

func (p *Palette) rebuild() {
	p.last = p.query
	p.q = Lower(p.query, p.q)
	p.rows = p.rows[:0]
	if len(p.q) == 0 {
		p.emptyRows()
	} else {
		p.search()
	}
	p.sel = p.next(-1, 1)
	p.scroll, p.follow = 0, true
	n := 0
	for _, r := range p.rows {
		if r.kind == rowItem && p.items[r.item].Disabled == "" {
			n++
		}
	}
	p.count = strconv.Itoa(n) + " " + widgets.Tr("editor.widgets.available", "available")
}

func (p *Palette) emptyRows() {
	if len(p.Recent) > 0 {
		p.rows = append(p.rows, row{kind: rowSection, label: or(p.RecentTitle, "Recent")})
		for _, i := range p.Recent {
			if i >= 0 && i < len(p.items) {
				p.rows = append(p.rows, row{item: i})
			}
		}
	}
	fav := false
	for i := range p.items {
		if p.items[i].Starred {
			if !fav {
				p.rows = append(p.rows, row{kind: rowSection, label: or(p.FavoritesTitle, "Favorites")})
				fav = true
			}
			p.rows = append(p.rows, row{item: i})
		}
	}
	for _, c := range p.cats {
		p.rows = append(p.rows, row{kind: rowCategory, label: c})
		if p.popup || !p.Open[c] {
			continue
		}
		for i := range p.items {
			if p.items[i].Category == c {
				p.rows = append(p.rows, row{item: i, nested: true})
			}
		}
	}
}

func (p *Palette) search() {
	p.hits, p.pos = p.hits[:0], p.pos[:0]
	for i := range p.items {
		start := len(p.pos)
		tier, pos := Match(p.q, &p.text[i], p.pos[start:])
		if tier == TierNone {
			for j := range p.keywords[i] {
				if candidate, _ := Match(p.q, &p.keywords[i][j], nil); candidate < tier {
					tier = candidate
				}
			}
			// Alias offsets do not refer to the displayed label.
			pos = nil
		}
		if tier == TierNone {
			continue
		}
		p.pos = append(p.pos[:start], pos...)
		p.hits = append(p.hits, hit{item: i, tier: tier, pos: [2]int{start, len(p.pos)}})
	}
	slices.SortStableFunc(p.hits, func(a, b hit) int {
		if a.tier != b.tier {
			return int(a.tier) - int(b.tier)
		}
		ia, ib := &p.items[a.item], &p.items[b.item]
		if (ia.Disabled == "") != (ib.Disabled == "") {
			if ia.Disabled == "" {
				return -1
			}
			return 1
		}
		if ia.Uses != ib.Uses {
			return ib.Uses - ia.Uses
		}
		pa, pb := 0, 0
		if a.pos[0] < a.pos[1] {
			pa = p.pos[a.pos[0]]
		}
		if b.pos[0] < b.pos[1] {
			pb = p.pos[b.pos[0]]
		}
		if pa != pb {
			return pa - pb
		}
		return len(ia.Label) - len(ib.Label)
	})
	for _, h := range p.hits {
		p.rows = append(p.rows, row{item: h.item, pos: h.pos})
	}
}

func (p *Palette) selectable(i int) bool {
	r := &p.rows[i]
	switch r.kind {
	case rowItem, rowCategory:
		return true
	}
	return false
}

func (p *Palette) next(from, dir int) int {
	for i := from + dir; i >= 0 && i < len(p.rows); i += dir {
		if p.selectable(i) {
			return i
		}
	}
	if from >= 0 && from < len(p.rows) {
		return from
	}
	return -1
}

func (p *Palette) move(n int) {
	if p.sel < 0 {
		p.sel = p.next(-1, 1)
		return
	}
	dir := 1
	if n < 0 {
		dir, n = -1, -n
	}
	for range n {
		j := p.next(p.sel, dir)
		if j == p.sel {
			break
		}
		p.sel = j
	}
	p.follow = true
}

func (p *Palette) toggle(i int, rowH float32) {
	c := p.rows[i].label
	p.Open[c] = !p.Open[c]
	scroll := p.scroll
	p.rebuild()
	for j, r := range p.rows {
		if r.kind == rowCategory && r.label == c {
			p.sel = j
			if p.Open[c] {
				// Put the opened category at the top when possible. VirtualList
				// clamps this at the end of the list on the next draw.
				scroll = float32(j) * rowH
			}
			break
		}
	}
	p.scroll, p.follow = scroll, false
}

func (p *Palette) keys(ui *core.Context, page int) Result {
	res := Result{Pick: -1, Star: -1}
	enter := ui.KeyPressed(input.KeyEnter) || ui.KeyPressed(input.KeyKPEnter)
	switch {
	case ui.KeyPressed(input.KeyEscape):
		res.Close = true
	case enter && p.sel >= 0 && p.rows[p.sel].kind == rowCategory:
		p.toggle(p.sel, ui.Theme.RowHeight+rowPad)
	case enter:
		res.Pick = p.Selected()
		res.Keep = ui.Mods().Has(input.ModShift)
	case ui.Primary() && ui.KeyPressed(input.KeyD):
		res.Star = p.Selected()
	case ui.KeyRepeated(input.KeyDown):
		p.move(1)
	case ui.KeyRepeated(input.KeyUp):
		p.move(-1)
	case ui.KeyRepeated(input.KeyPageDown):
		p.move(page)
	case ui.KeyRepeated(input.KeyPageUp):
		p.move(-page)
	case ui.KeyPressed(input.KeyHome):
		p.sel, p.follow = p.next(-1, 1), true
	case ui.KeyPressed(input.KeyEnd):
		p.sel, p.follow = p.next(len(p.rows), -1), true
	}
	return res
}

// Draw shows the palette in r; Describe draws the right pane for the highlighted item.
func (p *Palette) Draw(ui *core.Context, r mathx.Rect) Result {
	res := Result{Pick: -1, Star: -1}
	if !p.open {
		return res
	}
	th := &ui.Theme
	if ui.MousePressed(input.MouseLeft) && !r.Contains(ui.Mouse()) && ui.Frame() != p.opened {
		res.Close = true
		return res
	}
	if !ui.BeginModal(p.id, r) {
		p.open = false
		return res
	}
	p.ui = ui
	p.pointerMoved = ui.Mouse() != p.pointer
	p.pointer = ui.Mouse()
	rowH := th.RowHeight + rowPad
	qid := p.SearchID()
	if p.focus || ui.Focus() == 0 {
		ui.SetFocus(qid)
		b := ui.EditBuffer(qid, p.query)
		if p.focus {
			b.SelectAll()
		}
		p.focus = false
	}
	if ui.Focus() == qid {
		res = p.keys(ui, max(1, int(p.listH/rowH)-1))
	}
	if p.Title != "" {
		tr := ui.Cursor().Next(0)
		ui.TextIn(tr, p.Title, th.TextDim, core.AlignLeft)
	}
	field := ui.Cursor().Next(0)
	widgets.SearchField(ui, qid, field, &p.query)
	if p.query != p.last {
		p.rebuild()
	}
	if ui.Focus() == 0 && !res.Close {
		ui.SetFocus(qid)
		ui.EditBuffer(qid, p.query)
	}
	cur := ui.Cursor()
	lw := cur.W
	if p.Describe != nil {
		lw = float32(int(cur.W * 0.56))
	}
	p.listH = max(0, r.Y+r.H-cur.Y-th.RowHeight-th.Padding)
	list := mathx.Rect{X: cur.X, Y: cur.Y, W: lw, H: p.listH}
	desc := mathx.Rect{X: cur.X + lw + th.Padding, Y: cur.Y, W: cur.W - lw - th.Padding, H: list.H}
	if p.follow && p.sel >= 0 {
		widgets.EnsureVisible(&p.scroll, p.sel, rowH, list.H)
		p.follow = false
	}
	p.clicked, p.toggled, p.starred = -1, -1, -1
	widgets.VirtualList(ui, core.IDSeq(p.id, 2), list, len(p.rows), rowH, &p.scroll, p.rowFn)
	if len(p.rows) == 0 {
		ui.TextIn(list, widgets.Tr("editor.widgets.nothing_matches", "Nothing matches"), th.TextDisabled, core.AlignCenter)
	}
	if p.Describe != nil {
		ui.Rect(mathx.Rect{X: desc.X - th.Padding/2, Y: desc.Y, W: 1, H: desc.H}, th.Border)
		ui.PushClip(desc)
		p.Describe(ui, desc, p.Selected())
		ui.PopClip()
	}
	foot := mathx.Rect{X: r.X + th.Padding, Y: r.Y + r.H - th.RowHeight - th.Padding/2, W: r.W - 2*th.Padding, H: th.RowHeight}
	ui.TextIn(foot, p.count, th.TextDim, core.AlignLeft)
	cw := ui.TextSize(p.count).X + 16
	hint := mathx.Rect{X: foot.X + cw, Y: foot.Y, W: max(0, foot.W-cw), H: foot.H}
	if p.hintSrc != p.Hint || p.hintW != hint.W {
		p.hintSrc, p.hintW, p.hintOut = p.Hint, hint.W, widgets.Elide(ui, p.Hint, hint.W)
	}
	ui.TextIn(hint, p.hintOut, th.TextDim, core.AlignRight)
	ui.EndModal()
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

func (p *Palette) drawRow(i int, rr mathx.Rect) {
	ui := p.ui
	th := &ui.Theme
	rw := &p.rows[i]
	id := core.IDSeq(p.id, 100+i)
	ty := ui.TextY(rr)
	switch rw.kind {
	case rowSection:
		ui.Text(rw.label, mathx.Vec2{X: rr.X + 6, Y: ty}, th.TextDim)
		return
	case rowCategory:
		if p.popup {
			p.drawPopupCategory(i, rr)
			return
		}
		if ui.Pressed(id, rr) {
			p.sel, p.toggled = i, i
		}
		switch {
		case i == p.sel:
			ui.Rect(rr, th.Selection)
		case ui.Hot() == id:
			ui.Rect(rr, th.WidgetHot)
		}
		widgets.Chevron(ui, mathx.Rect{X: rr.X + 2, Y: rr.Y, W: rr.H, H: rr.H}, p.Open[rw.label], th.TextDim)
		x := rr.X + rr.H + 4
		if p.CategoryIcon != nil {
			if icon := p.CategoryIcon(rw.label); icon != "" && widgets.HasIcon(ui, icon) {
				widgets.IconAt(ui, mathx.Rect{X: x, Y: rr.Y + (rr.H-16)/2, W: 16, H: 16}, icon, th.TextDim)
				x += 22
			}
		}
		ui.Text(p.catTitle(rw.label), mathx.Vec2{X: x, Y: ty}, th.Text)
		return
	}
	it := &p.items[rw.item]
	if p.popup && p.pointerMoved && ui.Hit(rr) && p.subCategory != "" {
		p.closeSub(ui)
	}
	off := it.Disabled != ""
	star := mathx.Rect{X: rr.X + rr.W - rr.H, Y: rr.Y, W: rr.H, H: rr.H}
	sid := core.IDSeq(p.id, 100000+i)
	if p.Describe != nil && p.pointerMoved && ui.Hit(rr) {
		p.sel, p.follow = i, false
	}
	switch {
	case !p.NoFavorites && ui.Pressed(sid, star):
		p.starred = rw.item
	case ui.Pressed(id, rr) && off:
		p.sel = i
	case ui.Pressed(id, rr):
		p.sel, p.clicked = i, rw.item
	}
	switch {
	case i == p.sel && off:
		ui.Rect(rr, th.WidgetHot)
	case i == p.sel:
		ui.Rect(rr, th.Selection)
	case ui.Hot() == id:
		ui.Rect(rr, th.WidgetHot)
	}
	col, tint := th.Text, th.TextDim
	if off {
		col, tint = th.TextDisabled, th.TextDisabled
	}
	indent := float32(4)
	if len(p.q) == 0 && i > 0 {
		indent = 14
	}
	if rw.nested {
		indent = 40
	}
	widgets.IconAt(ui, mathx.Rect{X: rr.X + indent, Y: rr.Y + (rr.H-16)/2, W: 16, H: 16}, it.Icon, tint)
	tx := rr.X + indent + 22
	right := it.Detail
	if right == "" && len(p.q) > 0 {
		right = p.catTitle(it.Category)
	}
	if off {
		right = p.reason(rw.item, rr.W*0.45)
	}
	right = widgets.Elide(ui, right, rr.W*0.45)
	rwid := ui.TextSize(right).X
	endSpace := rr.H
	if p.NoFavorites {
		endSpace = 4
	}
	ui.PushClip(mathx.Rect{X: rr.X, Y: rr.Y, W: max(0, rr.W-endSpace-rwid-12), H: rr.H})
	ui.Text(it.Label, mathx.Vec2{X: tx, Y: ty}, col)
	for _, b := range p.pos[rw.pos[0]:rw.pos[1]] {
		if b >= len(it.Label) {
			continue
		}
		_, n := utf8.DecodeRuneInString(it.Label[b:])
		px := tx + ui.TextSize(it.Label[:b]).X
		ui.Rect(mathx.Rect{X: px, Y: ty + ui.LineHeight() - 1, W: ui.TextSize(it.Label[b : b+n]).X, H: 2}, th.Accent)
	}
	ui.PopClip()
	if right != "" {
		rc := th.TextDisabled
		if i == p.sel && !off {
			rc = th.TextDim
		}
		ui.TextIn(mathx.Rect{X: rr.X, Y: rr.Y, W: rr.W - endSpace - 4, H: rr.H}, right, rc, core.AlignRight)
	}
	if !p.NoFavorites {
		switch {
		case it.Starred:
			widgets.IconAt(ui, mathx.Rect{X: star.X + (star.W-16)/2, Y: star.Y + (star.H-16)/2, W: 16, H: 16}, "ui.star", th.Accent)
		case ui.Hot() == id || ui.Hot() == sid:
			widgets.IconAt(ui, mathx.Rect{X: star.X + (star.W-16)/2, Y: star.Y + (star.H-16)/2, W: 16, H: 16}, "ui.star", th.TextDisabled)
		}
	}
}

// reason is the elided Disabled text of item i, built again only when the width changes.
func (p *Palette) reason(i int, w float32) string {
	if w != p.reasonW || len(p.reasons) != len(p.items) {
		p.reasonW = w
		p.reasons = p.reasons[:0]
		for k := range p.items {
			r := ""
			if p.items[k].Disabled != "" {
				r = widgets.Elide(p.ui, p.items[k].Disabled, w)
			}
			p.reasons = append(p.reasons, r)
		}
	}
	return p.reasons[i]
}

func (p *Palette) catTitle(c string) string {
	if p.CategoryTitle != nil {
		return p.CategoryTitle(c)
	}
	return c
}

func or(s, def string) string {
	if s != "" {
		return s
	}
	return def
}
