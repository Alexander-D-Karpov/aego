package scripteditor

import (
	"strconv"
	"unicode/utf8"

	"aego/core/mathx"
	"aego/editor/core/code"
	"aego/editor/core/code/highlight"
	"aego/platform/input"
	"aego/render/text"
	"aego/script/diag"
	"aego/script/langsvc"
	"aego/ui/core"
)

const (
	tabWidth     = code.TabWidth
	revealMargin = 3
	viewKeep     = 512
	viewSlack    = 64
	hintPad      = 8
)

type lineHint struct {
	at    int
	label string
	w     float32
}

type lineView struct {
	valid   bool
	hash    uint64
	in      highlight.State
	font    *text.Font
	semVer  uint64
	hintVer uint64
	indent  int
	disp    []rune
	idx     []int
	base    []highlight.Span
	levels  []int
	spans   []highlight.Span
	texts   []string
	xs      []float32
	hints   []lineHint
}

func (v *lineView) x(f *text.Font, i int) float32 {
	x := f.XRunes(v.disp, i)
	for _, h := range v.hints {
		if h.at <= i {
			x += h.w
		}
	}
	return x
}

func (v *lineView) index(f *text.Font, x float32) int {
	for i, r := range v.disp {
		if x < v.x(f, i)+f.Advance(r)/2 {
			return i
		}
	}
	return len(v.disp)
}

func semKind(k langsvc.TokenKind) highlight.Kind {
	switch k {
	case langsvc.TokNamespace:
		return highlight.Namespace
	case langsvc.TokType, langsvc.TokStruct, langsvc.TokEnum:
		return highlight.Type
	case langsvc.TokScript:
		return highlight.Script
	case langsvc.TokEnumMember:
		return highlight.EnumMember
	case langsvc.TokFunction:
		return highlight.Function
	case langsvc.TokMethod:
		return highlight.Method
	case langsvc.TokParameter:
		return highlight.Param
	case langsvc.TokField:
		return highlight.Field
	case langsvc.TokProperty:
		return highlight.Property
	case langsvc.TokConstant:
		return highlight.Constant
	case langsvc.TokBuiltin:
		return highlight.Builtin
	case langsvc.TokAttribute:
		return highlight.Attribute
	case langsvc.TokSignal:
		return highlight.Signal
	case langsvc.TokCoroutine:
		return highlight.Coroutine
	}
	return highlight.Variable
}

func palette(th *core.Theme) [highlight.KindCount]mathx.Color {
	t := &th.Code
	var p [highlight.KindCount]mathx.Color
	p[highlight.Plain] = t.Plain
	p[highlight.Keyword] = t.Keyword
	p[highlight.Number] = t.Number
	p[highlight.String] = t.String
	p[highlight.Comment] = t.Comment
	p[highlight.Operator] = t.Operator
	p[highlight.Punctuation] = t.Punctuation
	p[highlight.Bracket] = t.Punctuation
	p[highlight.Attribute] = t.Attribute
	p[highlight.Namespace] = t.Namespace
	p[highlight.Type] = t.Type
	p[highlight.BuiltinType] = t.BuiltinType
	p[highlight.Function] = t.Function
	p[highlight.Method] = t.Method
	p[highlight.Param] = t.Param
	p[highlight.Variable] = t.Local
	p[highlight.Field] = t.Field
	p[highlight.Property] = t.Property
	p[highlight.Constant] = t.Const
	p[highlight.EnumMember] = t.EnumMember
	p[highlight.Builtin] = t.Builtin
	p[highlight.Script] = t.Script
	p[highlight.Error] = t.Error
	p[highlight.Signal] = t.Signal
	p[highlight.Coroutine] = t.Coroutine
	return p
}

func hashBytes(b []byte) uint64 {
	h := uint64(14695981039346656037)
	for _, c := range b {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}

func expand(raw []byte, disp []rune, idx []int) ([]rune, []int) {
	for i := 0; i < len(raw); {
		r, size := utf8.DecodeRune(raw[i:])
		for k := 0; k < size; k++ {
			idx = append(idx, len(disp))
		}
		if r == '\t' {
			for n := tabWidth - len(disp)%tabWidth; n > 0; n-- {
				disp = append(disp, ' ')
			}
		} else {
			disp = append(disp, r)
		}
		i += size
	}
	return disp, append(idx, len(disp))
}

func indentOf(disp []rune) int {
	n := 0
	for n < len(disp) && disp[n] == ' ' {
		n++
	}
	if n == len(disp) {
		return 0
	}
	return n
}

func byteAt(idx []int, d int) int {
	for b, v := range idx {
		if v >= d {
			return b
		}
	}
	return len(idx) - 1
}

func (p *Panel) view(l int, font *text.Font) *lineView {
	c := p.doc.Code
	p.line = c.LineBytes(l, p.line[:0])
	h := hashBytes(p.line)
	in := c.LineState(l)
	v := p.views[l]
	if v == nil {
		v = &lineView{}
		p.views[l] = v
	}
	if !v.valid || v.hash != h || v.in != in {
		v.disp, v.idx = expand(p.line, v.disp[:0], v.idx[:0])
		v.base, _ = highlight.Line(p.line, in, v.base[:0])
		v.levels = highlight.BracketLevels(v.base, p.line, in.Depth(), v.levels[:0])
		v.indent = indentOf(v.disp)
		v.hash, v.in, v.valid, v.font = h, in, true, nil
	}
	if v.semVer != p.semVer || v.font != font || v.hintVer != p.inlayVer {
		v.hints = v.hints[:0]
		for _, ih := range p.inlays[l] {
			if ih.off < 0 || ih.off >= len(v.idx) {
				continue
			}
			v.hints = append(v.hints, lineHint{at: v.idx[ih.off], label: ih.label, w: font.Measure(ih.label).X + hintPad})
		}
		v.spans = append(v.spans[:0], v.base...)
		highlight.Overlay(v.spans, c.LineStart(l), p.sem)
		v.texts, v.xs = v.texts[:0], v.xs[:0]
		for _, s := range v.spans {
			da, db := v.idx[s.Start], v.idx[s.End]
			v.texts = append(v.texts, string(v.disp[da:db]))
			v.xs = append(v.xs, v.x(font, da))
		}
		v.semVer, v.font, v.hintVer = p.semVer, font, p.inlayVer
	}
	return v
}

func (p *Panel) prune(first, last int) {
	if len(p.views) <= viewKeep {
		return
	}
	for l := range p.views {
		if l < first-viewSlack || l > last+viewSlack {
			delete(p.views, l)
		}
	}
}

func (p *Panel) caretX(font *text.Font) (int, float32) {
	c := p.doc.Code
	l := c.Line(c.Cursor())
	v := p.view(l, font)
	return l, v.x(font, v.idx[c.Cursor()-c.LineStart(l)])
}

func (p *Panel) revealCaret(font *text.Font, area, txt mathx.Rect) {
	l, x := p.caretX(font)
	lh := p.lineH
	y := float32(l) * lh
	margin := min(lh*revealMargin, max(0, (area.H-lh)/2))
	switch {
	case y < p.scrollY+margin:
		p.scrollY = y - margin
	case y+lh > p.scrollY+area.H-margin:
		p.scrollY = y + lh - area.H + margin
	}
	switch {
	case x < p.scrollX:
		p.scrollX = max(0, x-40)
	case x > p.scrollX+txt.W-40:
		p.scrollX = x - txt.W + 80
	}
}

func (p *Panel) hit(ui *core.Context, txt mathx.Rect) int {
	c := p.doc.Code
	m := ui.Mouse()
	l := min(max(int((m.Y-txt.Y+p.scrollY)/p.lineH), 0), c.Lines()-1)
	font := ui.Font()
	v := p.view(l, font)
	d := v.index(font, m.X-(txt.X+p.pad-p.scrollX))
	return c.LineStart(l) + byteAt(v.idx, d)
}

func (p *Panel) trackHover(ui *core.Context, txt mathx.Rect) {
	if p.dragging || p.comp.open {
		return
	}
	off := p.hit(ui, txt)
	if off != p.hover.off {
		p.hover = hoverState{off: off, since: ui.Time}
		return
	}
	if p.hover.done || ui.Time-p.hover.since < hoverDelay {
		return
	}
	p.hover.done = true
	if d, ok := p.diagAt(off); ok {
		p.hover.dg, p.hover.hasDiag = d, true
	}
	file, ok := p.file()
	if !ok || p.snap == nil {
		return
	}
	if old, ok := p.doc.Code.Unmap(p.snapRev, off); ok {
		p.hover.info, p.hover.ok = p.host.lang.HoverIn(p.snap, file, uint32(old))
		if p.hover.ok {
			p.hover.builtin = p.builtinAt(file, old)
		}
	}
}

func (p *Panel) mouse(ui *core.Context, area, txt mathx.Rect) {
	c := p.doc.Code
	for i := range p.lens {
		if p.lens[i].rect.W > 0 && ui.Clicked(core.IDSeq(p.fid, lensBase+i), p.lens[i].rect) {
			p.host.selectAttached(p.lens[i].full)
		}
	}
	over := ui.MouseOver(p.fid, area)
	if over {
		h := ui.ScrollX(40)
		v := ui.ScrollY(p.lineH * 3)
		if ui.Mods().Has(input.ModShift) {
			h, v = h-v, 0
		}
		if h != 0 || v != 0 {
			p.scrollX += h
			p.scrollY -= v
			p.hover = hoverState{off: -1}
		}
	}
	if ui.Pressed(p.fid, txt) {
		ui.SetFocus(p.fid)
		off := p.hit(ui, txt)
		shift := ui.Mods().Has(input.ModShift)
		p.closePopups()
		p.blinkAt = ui.Time
		switch {
		case ui.Primary() && !shift:
			c.SetCursor(off, false)
			p.definition(ui)
		case ui.DoubleClicked(p.fid):
			c.SelectWord(off)
		default:
			c.SetCursor(off, shift)
			p.dragging = true
		}
	}
	if p.dragging {
		if ui.MouseDown(input.MouseLeft) {
			c.SetCursor(p.hit(ui, txt), true)
		} else {
			p.dragging = false
		}
	}
	if !over || !txt.Contains(ui.Mouse()) {
		p.hover = hoverState{off: -1}
		return
	}
	for i := range p.lens {
		if p.lens[i].rect.Contains(ui.Mouse()) {
			ui.SetCursorShape(core.CursorPointer)
			return
		}
	}
	ui.SetCursorShape(core.CursorText)
	if ui.Primary() {
		ui.SetCursorShape(core.CursorPointer)
	}
	p.trackHover(ui, txt)
}

func (p *Panel) keys(ui *core.Context) {
	c := p.doc.Code
	ro := p.doc.ReadOnly
	mods := ui.Mods()
	shift := mods.Has(input.ModShift)
	alt := mods.Has(input.ModAlt)
	word := mods&ui.WordMod() != 0
	primary := ui.Primary()
	before := c.Rev()
	moved := false
	edit := func(fn func()) {
		if !ro {
			fn()
		}
	}
	if p.comp.open && p.compKeys(ui) {
		p.blinkAt = ui.Time
		return
	}
	if p.peek.open && (ui.KeyPressed(input.KeyEnter) || ui.KeyPressed(input.KeyKPEnter)) {
		p.openPeek()
		return
	}
	if ui.KeyPressed(input.KeyEscape) {
		switch {
		case p.peek.open:
			p.peek.open = false
		case p.sig.open:
			p.sig.open = false
		case p.comp.open || p.note != "":
			p.closePopups()
		default:
			p.find.open = false
		}
	}
	if p.command(ui, primary, shift, alt) {
		p.reveal = true
		p.blinkAt = ui.Time
		p.hover = hoverState{off: -1}
		return
	}
	if t := ui.TextInput(); len(t) > 0 && !primary && !ro {
		s := string(t)
		if !p.autoPair(c, s) {
			c.Type(s)
		}
		p.typed(ui, s)
	}
	move := func(k input.Scancode, d code.Dir, w bool) {
		if ui.KeyRepeated(k) {
			c.Move(d, w, shift)
			moved = true
		}
	}
	move(input.KeyLeft, code.Left, word)
	move(input.KeyRight, code.Right, word)
	move(input.KeyUp, code.Up, false)
	move(input.KeyDown, code.Down, false)
	if ui.KeyPressed(input.KeyHome) {
		if primary {
			c.Move(code.DocStart, false, shift)
		} else {
			c.Move(code.LineStart, false, shift)
		}
		moved = true
	}
	if ui.KeyPressed(input.KeyEnd) {
		if primary {
			c.Move(code.DocEnd, false, shift)
		} else {
			c.Move(code.LineEnd, false, shift)
		}
		moved = true
	}
	if ui.KeyRepeated(input.KeyPageUp) {
		c.MoveLines(-p.viewLines, shift)
		moved = true
	}
	if ui.KeyRepeated(input.KeyPageDown) {
		c.MoveLines(p.viewLines, shift)
		moved = true
	}
	if ui.KeyRepeated(input.KeyBackspace) {
		edit(func() {
			if word || !p.deletePair(c) {
				c.Delete(code.Left, word)
			}
			p.refilter()
		})
	}
	if ui.KeyRepeated(input.KeyDelete) {
		edit(func() { c.Delete(code.Right, word) })
	}
	if ui.KeyRepeated(input.KeyEnter) || ui.KeyRepeated(input.KeyKPEnter) {
		edit(c.Newline)
	}
	if ui.KeyRepeated(input.KeyTab) {
		edit(func() {
			lo, hi := c.Sel.Range()
			switch {
			case shift:
				c.Unindent()
			case c.Line(lo) != c.Line(hi):
				c.Indent()
			default:
				c.Type("\t")
			}
		})
	}
	if primary {
		switch {
		case ui.KeyPressed(input.KeyA):
			c.SelectAll()
		case ui.KeyPressed(input.KeyC):
			if s := c.Selected(); s != "" && ui.Clipboard != nil {
				ui.Clipboard.SetText(s)
			}
		case ui.KeyPressed(input.KeyX):
			if s := c.Selected(); s != "" && ui.Clipboard != nil {
				ui.Clipboard.SetText(s)
				edit(func() { c.Cut() })
			}
		case ui.KeyPressed(input.KeyV):
			if ui.Clipboard != nil {
				edit(func() { c.Paste(ui.Clipboard.Text()) })
			}
		case ui.KeyRepeated(input.KeyZ):
			edit(func() {
				if shift {
					c.Redo()
				} else {
					c.Undo()
				}
			})
		case ui.KeyRepeated(input.KeyY):
			edit(func() { c.Redo() })
		case ui.KeyPressed(input.KeyS):
			edit(func() { p.save() })
		case ui.KeyPressed(input.KeyF):
			p.openFind(false)
		case ui.KeyPressed(input.KeyH):
			p.openFind(!ro)
		case ui.KeyPressed(input.KeySlash):
			edit(c.ToggleComment)
		case ui.KeyPressed(input.KeyF4):
			p.host.requestClose(p)
		case ui.KeyPressed(input.KeySpace):
			edit(func() { p.requestCompletion(ui) })
		case !shift && ui.KeyPressed(input.KeyP):
			if !p.querySignature() {
				p.flash(ui.Time, "No call at the cursor")
			}
		case ui.KeyPressed(input.KeyQ):
			p.quickDoc(ui)
		}
	}
	if moved || c.Rev() != before {
		p.reveal = true
		p.blinkAt = ui.Time
		p.hover = hoverState{off: -1}
		if p.noteAt <= ui.Time+noteTime {
			p.note = ""
		} else {
			p.note, p.noteAt = "", 0
		}
		if moved && p.comp.open {
			p.refilter()
		}
	}
}

func wave(ui *core.Context, xa, xb, base float32, col mathx.Color) {
	if xb-xa < 6 {
		xb = xa + 6
	}
	for x := xa; x < xb; x += 4 {
		ui.Rect(mathx.Rect{X: x, Y: base, W: 2, H: 1.5}, col)
		ui.Rect(mathx.Rect{X: x + 2, Y: base + 1, W: 2, H: 1.5}, col)
	}
}

func (p *Panel) underlines(ui *core.Context, font *text.Font, v *lineView, x0, y float32, ls, le int) {
	th := &ui.Theme
	for _, d := range p.diags {
		s, e := int(d.Primary.Start), int(d.Primary.End)
		if e <= s {
			e = s + 1
		}
		if e < ls || s > le {
			continue
		}
		a, b := max(s, ls)-ls, min(e, le)-ls
		col := th.Error
		if d.Severity == diag.Warning {
			col = th.Warn
		}
		wave(ui, x0+v.x(font, v.idx[a]), x0+v.x(font, v.idx[b]), y+p.lineH-3, col)
	}
}

func (p *Panel) drawBrackets(ui *core.Context, font *text.Font, v *lineView, x0, y float32, ls, le int) {
	if p.brState == code.BracketNone {
		return
	}
	ct := &ui.Theme.Code
	col, at, n := ct.BracketMatch, [2]int{p.brA, p.brB}, 2
	if p.brState == code.BracketUnmatched {
		col, n = ct.BracketMismatch, 1
	}
	for _, a := range at[:n] {
		if a < ls || a >= le {
			continue
		}
		xa := x0 + v.x(font, v.idx[a-ls])
		xb := x0 + v.x(font, v.idx[a-ls+1])
		ui.List().RectOutline(mathx.Rect{X: xa, Y: y + 1, W: xb - xa, H: p.lineH - 2}, col, 1)
	}
}

func (p *Panel) drawSpans(ui *core.Context, font *text.Font, v *lineView, x0, y, ty float32, ls int, pal *[highlight.KindCount]mathx.Color) {
	ct := &ui.Theme.Code
	c := p.doc.Code
	for i, s := range v.spans {
		t := v.texts[i]
		if t == "" {
			continue
		}
		col := pal[s.Kind]
		if s.Kind == highlight.Bracket {
			switch {
			case c.Unmatched(ls + s.Start):
				col = ct.BracketMismatch
				wave(ui, x0+v.xs[i], x0+v.x(font, v.idx[s.End]), y+p.lineH-3, col)
			case p.host.Rainbow && v.levels[i] >= 0:
				col = ct.Rainbow[v.levels[i]%len(ct.Rainbow)]
			}
		}
		ui.Text(t, mathx.Vec2{X: x0 + v.xs[i], Y: ty}, col)
	}
}

func (p *Panel) drawText(ui *core.Context, area mathx.Rect) {
	th := &ui.Theme
	ct := &th.Code
	ui.PushFont(ui.MonoFont())
	defer ui.PopFont()
	font := ui.Font()
	if font == nil || area.H <= 0 {
		return
	}
	c := p.doc.Code
	pal := palette(th)
	p.lineH = ui.LineHeight() + 3
	lh := p.lineH
	count := c.Lines()
	charW := font.Advance('0')
	markerW := ct.Gutter.MarkerW * charW
	numEnd := area.X + markerW + float32(len(strconv.Itoa(count))+1)*charW
	gw := numEnd - area.X + ct.Gutter.NumberPad*charW
	p.pad = ct.Gutter.CodePad * charW
	mw := float32(0)
	if p.host.Minimap && area.W-gw > mapMinW {
		mw = mapWidth
	}
	txt := mathx.Rect{X: area.X + gw, Y: area.Y, W: max(0, area.W-gw-mw), H: area.H}
	p.viewLines = max(1, int(area.H/lh)-1)
	p.mouse(ui, area, txt)
	focused := ui.Focus() == p.fid
	if focused {
		p.keys(ui)
	}
	p.remap()
	p.updateMarks()
	p.updateInlays()
	p.refreshSignature()
	if p.reveal {
		p.revealCaret(font, area, txt)
		p.reveal = false
	}
	p.scrollY = mathx.Clamp(p.scrollY, 0, max(0, float32(count)*lh-area.H+lh))
	p.scrollX = max(0, p.scrollX)
	p.occurrences(ui.Time)
	p.brackets()
	p.updateBlock()
	p.updateLens()
	for i := range p.lens {
		p.lens[i].rect = mathx.Rect{}
	}
	ui.Rect(area, ct.Background)
	ui.Rect(mathx.Rect{X: area.X, Y: area.Y, W: gw, H: area.H}, ct.GutterBg)
	ui.PushClip(area)
	first := int(p.scrollY / lh)
	last := min(count, first+int(area.H/lh)+2)
	p.buildGuides(first, last)
	lo, hi := c.Sel.Range()
	caretLine := c.Line(c.Cursor())
	x0 := txt.X + p.pad - p.scrollX
	spaceW := font.Advance(' ')
	if ct.GuideColumn > 0 {
		gx := x0 + float32(ct.GuideColumn)*spaceW
		if gx >= txt.X && gx < txt.X+txt.W {
			ui.Rect(mathx.Rect{X: gx, Y: area.Y, W: 1, H: area.H}, ct.Guide)
		}
	}
	for l := first; l < last; l++ {
		v := p.view(l, font)
		y := area.Y + float32(l)*lh - p.scrollY
		ls, le := c.LineStart(l), c.LineEnd(l)
		ty := y + (lh-ui.LineHeight())/2
		ui.PushClip(txt)
		if p.inBlock(l) {
			ui.Rect(mathx.Rect{X: txt.X, Y: y, W: txt.W, H: lh}, ct.CurrentBlock)
		}
		if l == caretLine && focused {
			ui.Rect(mathx.Rect{X: txt.X, Y: y, W: txt.W, H: lh}, ct.CurrentLine)
		}
		if l == p.flashLine {
			if t := ui.Time - p.flashAt; t >= 0 && t < flashTime {
				col := ct.Occurrence
				col.A = float32(1 - t/flashTime)
				ui.Rect(mathx.Rect{X: txt.X, Y: y, W: txt.W, H: lh}, col)
			}
		}
		p.drawGuides(ui, v, x0, y, spaceW, l)
		p.drawOcc(ui, font, v, x0, y, ls, le)
		if hi > lo && hi >= ls && lo <= le {
			a, b := max(lo, ls)-ls, min(hi, le)-ls
			xa := x0 + v.x(font, v.idx[a])
			xb := x0 + v.x(font, v.idx[b])
			if hi > le {
				xb += spaceW
			}
			ui.Rect(mathx.Rect{X: xa, Y: y, W: xb - xa, H: lh}, th.Selection)
		}
		for _, h := range v.hints {
			hx := x0 + v.x(font, h.at) - h.w
			ui.Rect(mathx.Rect{X: hx + 1, Y: y + 2, W: h.w - 3, H: lh - 4}, th.Badge)
			ui.Text(h.label, mathx.Vec2{X: hx + hintPad/2, Y: ty}, th.Unit)
		}
		p.drawSpans(ui, font, v, x0, y, ty, ls, &pal)
		p.drawBrackets(ui, font, v, x0, y, ls, le)
		for i := range p.lens {
			if p.lens[i].line != l {
				continue
			}
			label := lensLabel(p.host.counts[p.lens[i].full])
			lx := x0 + v.x(font, len(v.disp)) + 3*spaceW
			lr := mathx.Rect{X: lx, Y: y, W: ui.TextSize(label).X, H: lh}
			p.lens[i].rect = lr
			col := ct.Comment
			if lr.Contains(ui.Mouse()) {
				col = th.Accent
			}
			ui.Text(label, mathx.Vec2{X: lx, Y: ty}, col)
		}
		p.underlines(ui, font, v, x0, y, ls, le)
		ui.PopClip()
		num := strconv.Itoa(l + 1)
		nc := ct.LineNo
		if l == caretLine {
			nc = ct.LineNoActive
		}
		ui.Text(num, mathx.Vec2{X: numEnd - ui.TextSize(num).X, Y: ty}, nc)
		if sev, ok := p.marks[l]; ok {
			col := th.Error
			if sev == diag.Warning {
				col = th.Warn
			}
			ui.Rect(mathx.Rect{X: area.X + max(0, (markerW-3)/2), Y: y + 3, W: 3, H: lh - 6}, col)
		}
	}
	if mw > 0 {
		p.mini.update(p, th)
		p.mini.draw(p, ui, mathx.Rect{X: area.X + area.W - mw, Y: area.Y, W: mw, H: area.H}, first)
	}
	l, cx := p.caretX(font)
	cy := area.Y + float32(l)*lh - p.scrollY
	p.caret = mathx.Vec2{X: x0 + cx, Y: cy + lh}
	if focused {
		if int((ui.Time-p.blinkAt)*2)%2 == 0 {
			ui.Rect(mathx.Rect{X: x0 + cx, Y: cy + 2, W: 2, H: lh - 4}, th.Text)
		}
		ui.WantTextInput(mathx.Rect{X: x0 + cx, Y: cy, W: 1, H: lh}, 0)
	}
	ui.PopClip()
	p.prune(first, last)
}

var openPairs = map[byte]byte{'(': ')', '[': ']', '{': '}', '"': '"'}

func isCloser(b byte) bool {
	return b == ')' || b == ']' || b == '}'
}

// autoPair pairs, types over or wraps brackets and quotes; false means insert s normally.
func (p *Panel) autoPair(c *code.Doc, s string) bool {
	if len(s) != 1 {
		return false
	}
	ch := s[0]
	lo, hi := c.Sel.Range()
	var prev, next byte
	if lo > 0 {
		if b := c.Slice(lo-1, 1, nil); len(b) == 1 {
			prev = b[0]
		}
	}
	if hi < c.Len() {
		if b := c.Slice(hi, 1, nil); len(b) == 1 {
			next = b[0]
		}
	}
	if lo == hi && next == ch && (isCloser(ch) || ch == '"') {
		c.SetCursor(hi+1, false)
		return true
	}
	cl, ok := openPairs[ch]
	if !ok {
		return false
	}
	if hi > lo {
		sel := string(c.Slice(lo, hi-lo, nil))
		c.Replace(lo, hi-lo, string(ch)+sel+string(cl))
		c.Select(lo+1, hi+1)
		return true
	}
	if ch == '"' {
		if identByte(prev) || identByte(next) || next == ch {
			return false
		}
	}
	if identByte(next) {
		return false
	}
	c.Replace(lo, 0, string(ch)+string(cl))
	c.SetCursor(lo+1, false)
	return true
}

// deletePair removes both halves of an empty pair on Backspace.
func (p *Panel) deletePair(c *code.Doc) bool {
	lo, hi := c.Sel.Range()
	if lo != hi || lo == 0 || hi >= c.Len() {
		return false
	}
	prev := c.Slice(lo-1, 1, nil)
	next := c.Slice(hi, 1, nil)
	if len(prev) == 1 && len(next) == 1 && openPairs[prev[0]] == next[0] {
		c.Replace(lo-1, 2, "")
		return true
	}
	return false
}
