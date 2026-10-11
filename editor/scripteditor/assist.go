package scripteditor

import (
	"strconv"
	"strings"
	"unicode"

	"aego/core/mathx"
	"aego/platform/input"
	"aego/script/diag"
	"aego/script/langsvc"
	"aego/ui/core"
	"aego/ui/draw"
)

const compRows = 10

func identByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func (p *Panel) wordBefore(off int) string {
	c := p.doc.Code
	ls := c.LineStart(c.Line(off))
	b := c.Slice(ls, off-ls, nil)
	i := len(b)
	for i > 0 && identByte(b[i-1]) {
		i--
	}
	return string(b[i:])
}

func tabStop(s string, i int) (int, string, int, bool) {
	if s[i] != '$' || i+1 >= len(s) {
		return 0, "", 0, false
	}
	j := i + 1
	brace := s[j] == '{'
	if brace {
		j++
	}
	k := j
	for k < len(s) && s[k] >= '0' && s[k] <= '9' {
		k++
	}
	if k == j {
		return 0, "", 0, false
	}
	n, _ := strconv.Atoi(s[j:k])
	switch {
	case !brace:
		return n, "", k, true
	case k < len(s) && s[k] == '}':
		return n, "", k + 1, true
	case k < len(s) && s[k] == ':':
		if e := strings.IndexByte(s[k:], '}'); e > 0 {
			return n, s[k+1 : k+e], k + e + 1, true
		}
	}
	return 0, "", 0, false
}

func expandSnippet(s string) (string, int, int) {
	var b strings.Builder
	best, lo, hi, final := -1, -1, -1, -1
	for i := 0; i < len(s); {
		if n, text, next, ok := tabStop(s, i); ok {
			at := b.Len()
			b.WriteString(text)
			switch {
			case n == 0:
				final = at
			case best < 0 || n < best:
				best, lo, hi = n, at, b.Len()
			}
			i = next
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	out := b.String()
	switch {
	case lo >= 0:
		return out, lo, hi
	case final >= 0:
		return out, final, final
	}
	return out, len(out), len(out)
}

func (p *Panel) typed(ui *core.Context, s string) {
	last := s[len(s)-1]
	switch {
	case last == '(' || last == ',':
		p.comp.open = false
		p.querySignature()
		if last == '(' && p.wordBefore(p.doc.Code.Cursor()-1) == "Connect" {
			p.requestCompletion(ui)
		}
	case last == ')':
		p.comp.open = false
		if p.sig.open {
			p.querySignature()
		}
	case last == '.' || last == '@':
		p.requestCompletion(ui)
	case last == '<':
		switch p.wordBefore(p.doc.Code.Cursor() - 1) {
		case "Get", "Has", "GetScript":
			p.requestCompletion(ui)
		}
	case last == ' ':
		p.comp.open = false
		switch p.wordBefore(p.doc.Code.Cursor() - 1) {
		case "start", "yield":
			p.requestCompletion(ui)
		}
	case identByte(last):
		if p.comp.open {
			p.refilter()
		} else if len(p.wordBefore(p.doc.Code.Cursor())) >= 2 {
			p.requestCompletion(ui)
		}
	default:
		p.comp.open = false
	}
}

func (p *Panel) requestCompletion(ui *core.Context) {
	gen := p.push()
	file, ok := p.file()
	if !ok {
		return
	}
	c := p.doc.Code
	p.host.lang.Complete(gen, file, uint32(c.Cursor()), func(cm langsvc.Completion) {
		start := int(cm.Prefix.Start)
		if start > p.doc.Code.Cursor() || len(cm.Items) == 0 {
			return
		}
		p.comp = compState{open: true, start: start, items: cm.Items}
		p.refilter()
	})
}

func matches(label, prefix string) bool {
	return len(prefix) <= len(label) && strings.EqualFold(label[:len(prefix)], prefix)
}

func (p *Panel) refilter() {
	if !p.comp.open {
		return
	}
	c := p.doc.Code
	cur := c.Cursor()
	if cur < p.comp.start || c.Line(cur) != c.Line(p.comp.start) {
		p.comp.open = false
		return
	}
	prefix := string(c.Slice(p.comp.start, cur-p.comp.start, nil))
	if strings.ContainsFunc(prefix, func(r rune) bool { return r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		p.comp.open = false
		return
	}
	p.comp.view = p.comp.view[:0]
	for i, it := range p.comp.items {
		if matches(it.Label, prefix) {
			p.comp.view = append(p.comp.view, i)
		}
	}
	if len(p.comp.view) == 0 {
		p.comp.open = false
		return
	}
	p.comp.sel = min(max(p.comp.sel, 0), len(p.comp.view)-1)
}

func (p *Panel) compKeys(ui *core.Context) bool {
	n := len(p.comp.view)
	switch {
	case ui.KeyRepeated(input.KeyDown):
		p.comp.sel = (p.comp.sel + 1) % n
	case ui.KeyRepeated(input.KeyUp):
		p.comp.sel = (p.comp.sel + n - 1) % n
	case ui.KeyPressed(input.KeyEnter), ui.KeyPressed(input.KeyKPEnter), ui.KeyPressed(input.KeyTab):
		p.accept(p.comp.sel)
	case ui.KeyPressed(input.KeyEscape):
		p.comp.open = false
	default:
		return false
	}
	return true
}

func (p *Panel) accept(i int) {
	if i < 0 || i >= len(p.comp.view) {
		return
	}
	it := p.comp.items[p.comp.view[i]]
	c := p.doc.Code
	ins, lo, hi := expandSnippet(it.Insert)
	start := p.comp.start
	c.Replace(start, c.Cursor()-start, ins)
	if lo == hi {
		c.SetCursor(start+lo, false)
	} else {
		c.Select(start+lo, start+hi)
	}
	p.comp.open = false
	p.reveal = true
	if strings.HasSuffix(ins, "(") {
		p.querySignature()
	}
}

func (p *Panel) jump(l langsvc.Location) {
	if l.Path == p.doc.Path && p.snap != nil {
		if s, _, ok := p.doc.Code.MapSpan(p.snapRev, int(l.Span.Start), int(l.Span.End)); ok {
			p.host.OpenAt(l.Path, s)
			return
		}
	}
	p.host.OpenAt(l.Path, int(l.Span.Start))
}

func (p *Panel) definition(ui *core.Context) {
	if p.doc.ReadOnly {
		p.stubSource(ui)
		return
	}
	file, ok := p.file()
	off, ok2 := p.snapOffset()
	if !ok || !ok2 {
		p.flash(ui.Time, analyzing)
		return
	}
	loc, ok := p.host.lang.DefinitionIn(p.snap, file, uint32(off))
	switch {
	case !ok:
		p.flash(ui.Time, "No definition found")
	case loc.Native && loc.Stub != "" && p.host.openStub(loc):
	case loc.Native && loc.Path != "" && p.host.openGo(loc.Path):
	case loc.Native:
		msg := "Defined in native code"
		if h, ok := p.host.lang.HoverIn(p.snap, file, uint32(off)); ok {
			msg = h.Signature
		}
		p.flash(ui.Time, msg)
	default:
		p.jump(loc)
	}
}

func box(ui *core.Context, at mathx.Vec2, lines []string, first mathx.Color) {
	th := &ui.Theme
	w, lh := float32(0), ui.LineHeight()+2
	for _, l := range lines {
		w = max(w, ui.TextSize(l).X)
	}
	r := mathx.Rect{X: at.X, Y: at.Y, W: w + 2*th.Padding, H: float32(len(lines))*lh + th.Padding}
	sz := ui.Size()
	r.X = max(0, min(r.X, sz.X-r.W))
	r.Y = max(0, min(r.Y, sz.Y-r.H))
	l := ui.List()
	l.PushLayer(draw.LayerTop)
	l.Rect(r, th.PanelAlt)
	l.RectOutline(r, th.Border, 1)
	for i, s := range lines {
		col := th.TextDim
		if i == 0 {
			col = first
		}
		l.Text(s, mathx.Vec2{X: r.X + th.Padding, Y: r.Y + th.Padding/2 + float32(i)*lh}, col)
	}
	l.PopLayer()
}

func (p *Panel) hoverLines(th *core.Theme) ([]string, mathx.Color) {
	var lines []string
	first := th.Text
	if p.hover.hasDiag {
		d := p.hover.dg
		first = th.Error
		if d.Severity == diag.Warning {
			first = th.Warn
		}
		lines = append(lines, d.Code+"  "+d.Message)
		for _, n := range d.Notes {
			lines = append(lines, "note: "+n.Message+p.noteWhere(n))
		}
		if h, ok := diag.Explain(d.Code); ok {
			lines = append(lines, h.What, "Fix: "+h.Fix)
		}
		if p.hover.ok {
			lines = append(lines, "")
		}
	}
	if p.hover.ok {
		lines = append(lines, p.hover.info.Signature)
		if p.hover.info.Doc != "" {
			lines = append(lines, strings.Split(p.hover.info.Doc, "\n")...)
		}
	}
	return lines, first
}

func (p *Panel) drawPopups(ui *core.Context) {
	th := &ui.Theme
	if p.comp.open {
		p.drawCompletion(ui)
	}
	p.drawSignature(ui)
	p.drawPeek(ui)
	p.drawOutline(ui)
	if p.hover.done && (p.hover.ok || p.hover.hasDiag) && !p.comp.open && !p.peek.open {
		lines, first := p.hoverLines(th)
		m := ui.Mouse()
		box(ui, mathx.Vec2{X: m.X + 12, Y: m.Y + 18}, lines, first)
	}
	if p.note != "" {
		if ui.Time-p.noteAt > noteTime {
			p.note = ""
		} else {
			box(ui, p.caret, strings.Split(p.note, "\n"), th.Text)
		}
	}
}

func (p *Panel) drawCompletion(ui *core.Context) {
	th := &ui.Theme
	rh := ui.LineHeight() + 6
	rows := min(len(p.comp.view), compRows)
	r := mathx.Rect{X: p.caret.X, Y: p.caret.Y + 2, W: 420, H: float32(rows)*rh + 4}
	sz := ui.Size()
	r.X = max(0, min(r.X, sz.X-r.W))
	if r.Y+r.H > sz.Y {
		r.Y = p.caret.Y - p.lineH - r.H - 2
	}
	top := min(max(p.comp.sel-rows+1, 0), len(p.comp.view)-rows)
	ui.BeginOverlay(r, draw.LayerTop)
	ui.Rect(r, th.PanelAlt)
	ui.Outline(r, th.Border)
	for i := 0; i < rows; i++ {
		k := top + i
		it := p.comp.items[p.comp.view[k]]
		rr := mathx.Rect{X: r.X + 2, Y: r.Y + 2 + float32(i)*rh, W: r.W - 4, H: rh}
		if ui.Pressed(core.IDSeq(p.fid, 100+i), rr) {
			p.accept(k)
			ui.SetFocus(p.fid)
		}
		if k == p.comp.sel {
			ui.Rect(rr, th.Selection)
		}
		ui.Text(it.Label, mathx.Vec2{X: rr.X + 6, Y: ui.TextY(rr)}, th.Text)
		if it.Detail != "" {
			dx := rr.X + 6 + ui.TextSize(it.Label).X + 16
			ui.PushClip(mathx.Rect{X: dx, Y: rr.Y, W: max(0, rr.X+rr.W-dx-4), H: rr.H})
			ui.Text(it.Detail, mathx.Vec2{X: dx, Y: ui.TextY(rr)}, th.TextDim)
			ui.PopClip()
		}
	}
	ui.EndOverlay()
	if it := p.comp.items[p.comp.view[p.comp.sel]]; it.Doc != "" {
		box(ui, mathx.Vec2{X: r.X, Y: r.Y + r.H + 2}, strings.Split(it.Doc, "\n"), th.TextDim)
	}
}
