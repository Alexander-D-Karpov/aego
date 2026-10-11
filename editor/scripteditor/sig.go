package scripteditor

import (
	"strings"

	"aego/core/mathx"
	"aego/script/langsvc"
	"aego/ui/core"
	"aego/ui/draw"
)

type sigState struct {
	open bool
	info langsvc.Signature
}

type inlay struct {
	off   int
	label string
}

func (p *Panel) querySignature() bool {
	file, ok := p.file()
	if !ok || p.snap == nil || p.doc.ReadOnly {
		p.sig.open = false
		return false
	}
	c := p.doc.Code
	s, ok := p.host.lang.SignatureAt(p.snap, file, c.Bytes(), c.Cursor())
	p.sig = sigState{open: ok, info: s}
	p.sigKey = mark{rev: c.Rev(), off: c.Cursor(), snap: p.snap}
	return ok
}

func (p *Panel) refreshSignature() {
	c := p.doc.Code
	if !p.sig.open {
		return
	}
	if k := (mark{rev: c.Rev(), off: c.Cursor(), snap: p.snap}); k != p.sigKey {
		p.querySignature()
	}
}

func (p *Panel) drawSignature(ui *core.Context) {
	if !p.sig.open {
		return
	}
	th := &ui.Theme
	s := p.sig.info
	lh := ui.LineHeight() + 2
	doc := s.Doc
	if i := strings.IndexByte(doc, '\n'); i >= 0 {
		doc = doc[:i]
	}
	w := max(ui.TextSize(s.Label).X, ui.TextSize(doc).X) + 2*th.Padding
	h := lh + th.Padding
	if doc != "" {
		h += lh
	}
	r := mathx.Rect{X: p.caret.X - 20, Y: p.caret.Y - p.lineH - h - 2, W: w, H: h}
	sz := ui.Size()
	r.X = max(0, min(r.X, sz.X-r.W))
	if r.Y < p.area.Y {
		r.Y = p.caret.Y + 2
	}
	l := ui.List()
	l.PushLayer(draw.LayerTop)
	l.Rect(r, th.PanelAlt)
	l.RectOutline(r, th.Border, 1)
	x, y := r.X+th.Padding, r.Y+th.Padding/2
	if s.Active >= 0 && s.Active < len(s.Params) {
		a, b := s.Params[s.Active][0], s.Params[s.Active][1]
		l.Text(s.Label[:a], mathx.Vec2{X: x, Y: y}, th.Text)
		ax := x + ui.TextSize(s.Label[:a]).X
		l.Text(s.Label[a:b], mathx.Vec2{X: ax, Y: y}, th.Accent)
		bx := ax + ui.TextSize(s.Label[a:b]).X
		l.Rect(mathx.Rect{X: ax, Y: y + ui.LineHeight(), W: bx - ax, H: 1}, th.Accent)
		l.Text(s.Label[b:], mathx.Vec2{X: bx, Y: y}, th.Text)
	} else {
		l.Text(s.Label, mathx.Vec2{X: x, Y: y}, th.Text)
	}
	if doc != "" {
		l.Text(doc, mathx.Vec2{X: x, Y: y + lh}, th.TextDim)
	}
	l.PopLayer()
}

func (p *Panel) quickDoc(ui *core.Context) {
	c := p.doc.Code
	h := hoverState{off: c.Cursor(), done: true}
	if d, ok := p.diagAt(h.off); ok {
		h.dg, h.hasDiag = d, true
	}
	if file, ok := p.file(); ok && p.snap != nil {
		if old, ok := c.Unmap(p.snapRev, h.off); ok {
			h.info, h.ok = p.host.lang.HoverIn(p.snap, file, uint32(old))
		}
	}
	if !h.ok && !h.hasDiag {
		p.flash(ui.Time, "No documentation at the cursor")
		return
	}
	saved := p.hover
	p.hover = h
	lines, _ := p.hoverLines(&ui.Theme)
	p.hover = saved
	p.note, p.noteAt = strings.Join(lines, "\n"), ui.Time+1e9
}

func (p *Panel) updateInlays() {
	c := p.doc.Code
	if !p.host.Hints || p.doc.ReadOnly {
		if len(p.inlays) > 0 {
			clear(p.inlays)
			p.inlayVer++
		}
		p.inlayKey = mark{}
		return
	}
	k := mark{rev: c.Rev(), snap: p.snap}
	if k == p.inlayKey {
		return
	}
	p.inlayKey = k
	clear(p.inlays)
	p.inlayVer++
	file, ok := p.file()
	if !ok || p.snap == nil {
		return
	}
	for _, h := range p.host.lang.HintsIn(p.snap, file) {
		at, _, ok := c.MapSpan(p.snapRev, int(h.Off), int(h.Off))
		if !ok {
			continue
		}
		l := c.Line(at)
		p.inlays[l] = append(p.inlays[l], inlay{off: at - c.LineStart(l), label: h.Label})
	}
}
