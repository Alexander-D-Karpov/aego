package scripteditor

import (
	"aego/editor/core/fuzzy"
	"aego/engine/i18n"
	"path"
	"sort"
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/code"
	"aego/editor/core/code/highlight"
	"aego/editor/core/context"
	"aego/platform/input"
	"aego/script/diag"
	"aego/script/langsvc"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/widgets"
)

const (
	outlineRows = 16
	peekLines   = 12
	flashTime   = 0.6
	lensBase    = 500
	crumbBase   = 600
	outlineBase = 300
	peekBase    = 400
	scriptHead  = "script "
)

type outlineItem struct {
	label  string
	detail string
	depth  int
	off    int
}

type outlineState struct {
	open  bool
	focus bool
	query string
	sel   int
	items []outlineItem
	view  []int
}

type peekState struct {
	open bool
	doc  *code.Doc
	path string
	off  int
	line int
	top  int
	acc  float32
}

type crumb struct {
	label string
	off   int
}

type lens struct {
	line int
	full string
	rect mathx.Rect
}

func (p *Panel) command(ui *core.Context, primary, shift, alt bool) bool {
	c := p.doc.Code
	ro := p.doc.ReadOnly
	switch {
	case primary && ui.KeyPressed(input.KeyBackslash):
		p.host.split(p, shift)
	case alt && !primary && ui.KeyRepeated(input.KeyUp):
		if !ro {
			c.ShiftLines(-1)
		}
	case alt && !primary && ui.KeyRepeated(input.KeyDown):
		if !ro {
			c.ShiftLines(1)
		}
	case primary && alt && ui.KeyPressed(input.KeyLeft):
		p.host.Back()
	case primary && alt && ui.KeyPressed(input.KeyRight):
		p.host.Forward()
	case primary && alt && ui.KeyPressed(input.KeyB):
		p.implementation(ui)
	case !primary && ui.KeyPressed(input.KeyF2):
		p.nextError(ui, shift)
	case alt && !primary && ui.KeyPressed(input.KeyF7):
		p.usages(ui)
	case !primary && shift && ui.KeyPressed(input.KeyF12):
		p.usages(ui)
	case !primary && !shift && ui.KeyPressed(input.KeyF12):
		p.definition(ui)
	case primary && !shift && ui.KeyPressed(input.KeyF12):
		p.openOutline(ui)
	case primary && !shift && ui.KeyPressed(input.KeyB):
		p.definition(ui)
	case !primary && !shift && !alt && ui.KeyPressed(input.KeyF1):
		p.docsAt()
	case primary && shift && ui.KeyPressed(input.KeyI):
		p.peekDefinition(ui)
	case primary && shift && ui.KeyPressed(input.KeyK):
		if !ro {
			c.DeleteLines()
		}
	case primary && shift && ui.KeyPressed(input.KeyW):
		p.shrinkSel()
	case primary && !shift && ui.KeyPressed(input.KeyW):
		p.expandSel(ui)
	case primary && !shift && ui.KeyRepeated(input.KeyD):
		if !ro {
			c.DuplicateLines()
		}
	default:
		return false
	}
	return true
}

func (p *Panel) wordAt() string {
	c := p.doc.Code
	off := c.Cursor()
	l := c.Line(off)
	b := c.LineBytes(l, nil)
	i := off - c.LineStart(l)
	j := i
	for i > 0 && identByte(b[i-1]) {
		i--
	}
	for j < len(b) && identByte(b[j]) {
		j++
	}
	return string(b[i:j])
}

func (p *Panel) selectionQuery() string {
	if s := p.doc.Code.Selected(); s != "" && !strings.Contains(s, "\n") {
		return s
	}
	return p.wordAt()
}

func (p *Panel) jumpTo(off int) {
	p.host.active = p
	p.host.mark()
	c := p.doc.Code
	c.SetCursor(off, false)
	p.reveal = true
	p.flashLine, p.flashAt = c.Line(off), p.host.d.Now()
}

func (p *Panel) nextError(ui *core.Context, back bool) {
	c := p.doc.Code
	onlyErrors := false
	for _, d := range p.diags {
		if d.Severity == diag.Error {
			onlyErrors = true
			break
		}
	}
	var list []diag.Diagnostic
	for _, d := range p.diags {
		if !onlyErrors || d.Severity == diag.Error {
			list = append(list, d)
		}
	}
	if len(list) == 0 {
		if first, _, ok := p.host.d.Problems.FirstError(""); ok && first.Path != p.doc.Path {
			p.host.d.Nav.Open(first.Location())
			return
		}
		p.flash(ui.Time, i18n.Tr("editor.scripteditor.no_problems_in_file", "No problems in this file"))
		return
	}
	cur := c.Cursor()
	pick := list[0]
	if back {
		pick = list[len(list)-1]
		for i := len(list) - 1; i >= 0; i-- {
			if int(list[i].Primary.Start) < cur {
				pick = list[i]
				break
			}
		}
	} else {
		for _, d := range list {
			if int(d.Primary.Start) > cur {
				pick = d
				break
			}
		}
	}
	c.SetCursor(int(pick.Primary.Start), false)
	p.reveal = true
	msg := pick.Code + "  " + pick.Message
	if h := diag.HelpText(pick.Code); h != "" {
		msg += "\n" + h
	}
	p.flash(ui.Time, msg)
}

func (p *Panel) expandSel(ui *core.Context) {
	c := p.doc.Code
	if c.Sel != p.selTop {
		p.selStack = p.selStack[:0]
	}
	lo, hi := c.Sel.Range()
	if lo == hi {
		prev := c.Sel
		c.SelectWord(lo)
		if a, b := c.Sel.Range(); a != b {
			p.selStack = append(p.selStack, prev)
			p.selTop = c.Sel
			return
		}
		c.Sel = prev
	}
	file, ok := p.file()
	a, ok1 := c.Unmap(p.snapRev, lo)
	b, ok2 := c.Unmap(p.snapRev, hi)
	if !ok || !ok1 || !ok2 || p.snap == nil {
		p.flash(ui.Time, analyzing())
		return
	}
	for _, sp := range p.host.lang.RangesIn(p.snap, file, uint32(a), uint32(b)) {
		s, e, ok := c.MapSpan(p.snapRev, int(sp.Start), int(sp.End))
		if !ok || s > lo || e < hi || (s == lo && e == hi) {
			continue
		}
		p.selStack = append(p.selStack, c.Sel)
		c.Select(s, e)
		p.selTop = c.Sel
		return
	}
}

func (p *Panel) shrinkSel() {
	c := p.doc.Code
	n := len(p.selStack)
	if n == 0 || c.Sel != p.selTop {
		p.selStack = p.selStack[:0]
		return
	}
	prev := p.selStack[n-1]
	p.selStack = p.selStack[:n-1]
	c.Select(prev.Anchor, prev.Head)
	p.selTop = c.Sel
}

func (p *Panel) usages(ui *core.Context) {
	file, ok := p.file()
	off, ok2 := p.snapOffset()
	if !ok || !ok2 {
		p.flash(ui.Time, analyzing())
		return
	}
	locs := p.host.lang.ReferencesIn(p.snap, file, uint32(off))
	if len(locs) == 0 {
		p.flash(ui.Time, i18n.Tr("editor.scripteditor.no_usages", "No usages found"))
		return
	}
	p.host.showUsages(p.wordAt(), locs, p.snap)
}

func (p *Panel) openOutline(ui *core.Context) {
	file, ok := p.file()
	if !ok || p.snap == nil {
		p.flash(ui.Time, analyzing())
		return
	}
	o := &p.outline
	o.items = o.items[:0]
	c := p.doc.Code
	var walk func([]langsvc.Symbol, int)
	walk = func(list []langsvc.Symbol, depth int) {
		for _, s := range list {
			if off, _, ok := c.MapSpan(p.snapRev, int(s.Selection.Start), int(s.Selection.End)); ok {
				o.items = append(o.items, outlineItem{label: s.Name, detail: s.Kind.String(), depth: depth, off: off})
			}
			walk(s.Children, depth+1)
		}
	}
	walk(p.host.lang.SymbolsIn(p.snap, file), 0)
	o.open, o.focus, o.query, o.sel = true, true, "", 0
	p.comp.open = false
	p.filterOutline()
}

func (p *Panel) filterOutline() {
	o := &p.outline
	o.view = o.view[:0]
	o.sel = 0
	q := strings.TrimSpace(o.query)
	if q == "" {
		for i := range o.items {
			o.view = append(o.view, i)
		}
		return
	}
	type hit struct{ i, score int }
	var hits []hit
	for i, it := range o.items {
		if s, _, ok := fuzzy.Match(q, it.label, nil); ok {
			hits = append(hits, hit{i, s})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	for _, h := range hits {
		o.view = append(o.view, h.i)
	}
}

func (p *Panel) closeOutline(ui *core.Context) {
	p.outline.open = false
	ui.SetFocus(p.fid)
}

func (p *Panel) pickOutline(ui *core.Context, k int) {
	o := &p.outline
	if k < 0 || k >= len(o.view) {
		return
	}
	off := o.items[o.view[k]].off
	p.closeOutline(ui)
	p.jumpTo(off)
}

func (p *Panel) drawOutline(ui *core.Context) {
	o := &p.outline
	if !o.open {
		return
	}
	th := &ui.Theme
	qid := core.IDSeq(p.fid, outlineBase)
	if o.focus {
		ui.SetFocus(qid)
		ui.EditBuffer(qid, o.query).SelectAll()
		o.focus = false
	}
	if ui.Focus() != qid {
		o.open = false
		return
	}
	switch {
	case ui.KeyPressed(input.KeyEscape):
		p.closeOutline(ui)
		return
	case ui.KeyPressed(input.KeyEnter), ui.KeyPressed(input.KeyKPEnter):
		p.pickOutline(ui, o.sel)
		return
	case ui.KeyRepeated(input.KeyDown):
		o.sel = min(o.sel+1, len(o.view)-1)
	case ui.KeyRepeated(input.KeyUp):
		o.sel = max(o.sel-1, 0)
	}
	rh := ui.LineHeight() + 6
	rows := min(len(o.view), outlineRows)
	w := min(p.area.W-40, 460)
	r := mathx.Rect{X: p.area.X + (p.area.W-w)/2, Y: p.area.Y + 8, W: w, H: th.RowHeight + 12 + float32(max(rows, 1))*rh}
	picked := -1
	ui.BeginOverlay(r, draw.LayerTop)
	ui.Rect(r, th.PanelAlt)
	ui.Outline(r, th.Border)
	fr := mathx.Rect{X: r.X + 4, Y: r.Y + 4, W: r.W - 8, H: th.RowHeight}
	if changed, _ := widgets.TextFieldRect(ui, qid, fr, &o.query, true); changed {
		p.filterOutline()
	}
	start := max(0, min(o.sel-rows+1, len(o.view)-rows))
	for i := 0; i < rows; i++ {
		k := start + i
		it := o.items[o.view[k]]
		rr := mathx.Rect{X: r.X + 2, Y: fr.Y + fr.H + 4 + float32(i)*rh, W: r.W - 4, H: rh}
		id := core.IDSeq(p.fid, outlineBase+1+i)
		if ui.Pressed(id, rr) {
			picked = k
		}
		switch {
		case k == o.sel:
			ui.Rect(rr, th.Selection)
		case ui.Hot() == id:
			ui.Rect(rr, th.WidgetHot)
		}
		x := rr.X + 6 + float32(it.depth)*th.Indent
		ui.Text(it.label, mathx.Vec2{X: x, Y: ui.TextY(rr)}, th.Text)
		ui.Text(it.detail, mathx.Vec2{X: x + ui.TextSize(it.label).X + 12, Y: ui.TextY(rr)}, th.TextDim)
	}
	if len(o.view) == 0 {
		er := mathx.Rect{X: r.X, Y: fr.Y + fr.H + 4, W: r.W, H: rh}
		ui.TextIn(er, i18n.Tr("editor.scripteditor.no_symbols_match", "No symbols match"), th.TextDisabled, core.AlignCenter)
	}
	ui.EndOverlay()
	if picked >= 0 {
		p.pickOutline(ui, picked)
	}
}

func (p *Panel) peekDefinition(ui *core.Context) {
	file, ok := p.file()
	off, ok2 := p.snapOffset()
	if !ok || !ok2 {
		p.flash(ui.Time, analyzing())
		return
	}
	loc, ok := p.host.lang.DefinitionIn(p.snap, file, uint32(off))
	switch {
	case !ok:
		p.flash(ui.Time, i18n.Tr("editor.scripteditor.no_definition", "No definition found"))
		return
	case loc.Native:
		p.flash(ui.Time, i18n.Tr("editor.scripteditor.defined_in_native", "Defined in native code, use F12 to see its documentation"))
		return
	}
	pk := peekState{open: true, path: loc.Path}
	if loc.Path == p.doc.Path {
		s, _, ok := p.doc.Code.MapSpan(p.snapRev, int(loc.Span.Start), int(loc.Span.End))
		if !ok {
			p.flash(ui.Time, analyzing())
			return
		}
		pk.doc, pk.off = p.doc.Code, s
	} else {
		pk.doc, pk.off = code.New(p.snap.Sources.Source(loc.Span.File)), int(loc.Span.Start)
	}
	pk.line = pk.doc.Line(pk.off)
	pk.top = max(0, pk.line-3)
	p.peek = pk
	p.comp.open = false
}

func (p *Panel) openPeek() {
	pk := p.peek
	p.peek.open = false
	if pk.doc == nil {
		return
	}
	if pk.doc == p.doc.Code {
		p.jumpTo(pk.off)
		return
	}
	line, col := pk.doc.LineCol(pk.off)
	p.host.Goto(context.At(pk.path, line+1, col+1))
}

func (p *Panel) peekLine(ui *core.Context, d *code.Doc, l int, x0, ty float32, pal *[highlight.KindCount]mathx.Color) {
	font := ui.Font()
	p.line = d.LineBytes(l, p.line[:0])
	disp, idx := expand(p.line, nil, nil)
	spans, _ := highlight.Line(p.line, d.LineState(l), nil)
	for _, s := range spans {
		da, db := idx[s.Start], idx[s.End]
		if da < db {
			ui.Text(string(disp[da:db]), mathx.Vec2{X: x0 + font.XRunes(disp, da), Y: ty}, pal[s.Kind])
		}
	}
}

func (p *Panel) drawPeek(ui *core.Context) {
	pk := &p.peek
	if !pk.open || pk.doc == nil {
		return
	}
	th := &ui.Theme
	ui.PushFont(ui.MonoFont())
	defer ui.PopFont()
	lh := p.lineH
	w := min(p.area.W-24, 760)
	h := lh*peekLines + th.RowHeight + 6
	r := mathx.Rect{X: p.area.X + 12, Y: p.caret.Y + 2, W: w, H: h}
	if r.Y+r.H > p.area.Y+p.area.H {
		r.Y = max(p.area.Y, p.caret.Y-lh-h-2)
	}
	if ui.MousePressed(input.MouseLeft) && !r.Contains(ui.Mouse()) {
		pk.open = false
		return
	}
	pal := palette(th)
	ui.BeginOverlay(r, draw.LayerTop)
	ui.Rect(r, th.Code.Background)
	ui.Outline(r, th.Accent)
	hdr := mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: th.RowHeight}
	ui.Rect(hdr, th.Header)
	ui.Text(pk.path+":"+strconv.Itoa(pk.line+1), mathx.Vec2{X: hdr.X + th.Padding, Y: ui.TextY(hdr)}, th.Text)
	ui.TextIn(hdr, i18n.Tr("editor.scripteditor.enter_open_esc_close", "Enter open · Esc close"), th.TextDim, core.AlignRight)
	body := mathx.Rect{X: r.X, Y: hdr.Y + hdr.H + 3, W: r.W, H: r.H - hdr.H - 3}
	id := core.IDSeq(p.fid, peekBase)
	open := false
	if ui.MouseOver(id, body) {
		if d := ui.ScrollY(lh * 3); d != 0 {
			pk.acc -= d / lh
			n := int(pk.acc)
			pk.acc -= float32(n)
			pk.top += n
		}
	}
	if ui.Pressed(id, body) && ui.DoubleClicked(id) {
		open = true
	}
	pk.top = mathx.Clamp(pk.top, 0, max(0, pk.doc.Lines()-peekLines))
	ui.PushClip(body)
	x0 := body.X + 8
	for i := 0; i < peekLines; i++ {
		l := pk.top + i
		if l >= pk.doc.Lines() {
			break
		}
		y := body.Y + float32(i)*lh
		if l == pk.line {
			ui.Rect(mathx.Rect{X: body.X, Y: y, W: body.W, H: lh}, th.Code.Occurrence)
		}
		p.peekLine(ui, pk.doc, l, x0, y+(lh-ui.LineHeight())/2, &pal)
	}
	ui.PopClip()
	ui.EndOverlay()
	if open {
		p.openPeek()
	}
}

func (p *Panel) updateCrumbs() {
	c := p.doc.Code
	k := mark{rev: c.Rev(), off: c.Cursor(), snap: p.snap}
	if k == p.crumbKey {
		return
	}
	file, ok := p.file()
	off, ok2 := p.snapOffset()
	if !ok || !ok2 {
		return
	}
	p.crumbKey = k
	p.crumbs = p.crumbs[:0]
	list := p.host.lang.SymbolsIn(p.snap, file)
	for len(list) > 0 {
		var next []langsvc.Symbol
		for _, s := range list {
			if int(s.Span.Start) > off || off > int(s.Span.End) {
				continue
			}
			if at, _, ok := c.MapSpan(p.snapRev, int(s.Selection.Start), int(s.Selection.End)); ok {
				p.crumbs = append(p.crumbs, crumb{label: s.Name, off: at})
			}
			next = s.Children
			break
		}
		list = next
	}
}

func (p *Panel) drawCrumbs(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	p.updateCrumbs()
	ui.Rect(r, th.Code.GutterBg)
	ui.PushClip(r)
	ty := ui.TextY(r)
	x := r.X + th.Padding
	name := path.Base(p.doc.Path)
	ui.Text(name, mathx.Vec2{X: x, Y: ty}, th.TextDim)
	x += ui.TextSize(name).X
	sep := " " + widgets.Glyph(ui, '›', ">") + " "
	sw := ui.TextSize(sep).X
	jump := -1
	for i, cb := range p.crumbs {
		ui.Text(sep, mathx.Vec2{X: x, Y: ty}, th.TextDisabled)
		x += sw
		w := ui.TextSize(cb.label).X
		cr := mathx.Rect{X: x - 2, Y: r.Y, W: w + 4, H: r.H}
		id := core.IDSeq(p.fid, crumbBase+i)
		if ui.Clicked(id, cr) {
			jump = cb.off
		}
		col := th.Text
		if ui.Hot() == id {
			col = th.Accent
		}
		ui.Text(cb.label, mathx.Vec2{X: x, Y: ty}, col)
		x += w
	}
	ui.PopClip()
	if jump >= 0 {
		p.jumpTo(jump)
		ui.SetFocus(p.fid)
	}
}

func (p *Panel) updateLens() {
	c := p.doc.Code
	k := mark{rev: c.Rev(), snap: p.snap}
	if k == p.lensKey {
		return
	}
	p.lensKey = k
	p.lens = p.lens[:0]
	file, ok := p.file()
	if !ok || p.snap == nil {
		return
	}
	for _, s := range p.host.lang.SymbolsIn(p.snap, file) {
		if s.Kind != langsvc.SymScript || !strings.HasPrefix(s.Detail, scriptHead) {
			continue
		}
		at, _, ok := c.MapSpan(p.snapRev, int(s.Selection.Start), int(s.Selection.End))
		if !ok {
			continue
		}
		p.lens = append(p.lens, lens{line: c.Line(at), full: strings.TrimPrefix(s.Detail, scriptHead)})
	}
}

func lensLabel(n int) string {
	switch n {
	case 0:
		return "not attached to any node"
	case 1:
		return "attached to 1 node"
	}
	return i18n.Tr("editor.scripteditor.attached_to_n_nodes", "attached to {n} nodes", i18n.Str("n", strconv.Itoa(n)))
}

func (p *Panel) docsAt() {
	ref := ""
	if file, ok := p.file(); ok && p.snap != nil {
		if off, ok := p.snapOffset(); ok {
			if h, ok := p.host.lang.HoverIn(p.snap, file, uint32(off)); ok {
				ref = h.Ref
			}
		}
	}
	p.host.d.Nav.Open(context.DocsAt(ref, p.wordAt()))
}
