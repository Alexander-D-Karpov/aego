package scripteditor

import (
	"bytes"
	"strings"

	"aego/core/mathx"
	"aego/editor/code/highlight"
	"aego/editor/code/search"
	"aego/render/text"
	"aego/ui/core"
)

const (
	occDelay   = 0.15
	occMaxText = 256
)

type settle struct {
	key   mark
	since float64
	done  bool
}

func (s *settle) changed(k mark, now float64) bool {
	if k == s.key {
		return false
	}
	s.key, s.since, s.done = k, now, false
	return true
}

func (s *settle) due(now, wait float64) bool {
	if s.done || now-s.since < wait {
		return false
	}
	s.done = true
	return true
}

func (p *Panel) clearOcc() {
	p.occ, p.occWrite, p.occCur = p.occ[:0], p.occWrite[:0], -1
}

func (p *Panel) occurrences(now float64) {
	c := p.doc.Code
	k := mark{rev: c.Rev(), off: c.Cursor(), anchor: c.Sel.Anchor, snap: p.snap}
	if p.occT.changed(k, now) {
		p.clearOcc()
	}
	if p.occT.due(now, occDelay) {
		p.findOcc()
	}
}

func (p *Panel) findOcc() {
	c := p.doc.Code
	p.clearOcc()
	if lo, hi := c.Sel.Range(); hi > lo {
		p.textOcc(lo, hi)
		return
	}
	file, ok := p.file()
	if !ok || p.snap == nil {
		return
	}
	cur := c.Cursor()
	off, ok := c.Unmap(p.snapRev, cur)
	if !ok {
		return
	}
	for _, l := range p.host.lang.ReferencesIn(p.snap, file, uint32(off)) {
		if l.File != file {
			continue
		}
		s, e, ok := c.MapSpan(p.snapRev, int(l.Span.Start), int(l.Span.End))
		if !ok {
			continue
		}
		if s <= cur && cur <= e {
			p.occCur = len(p.occ)
		}
		p.occ = append(p.occ, highlight.Sem{Start: s, End: e})
		p.occWrite = append(p.occWrite, p.writeAt(e))
	}
	if len(p.occ) < 2 {
		p.clearOcc()
	}
}

func (p *Panel) textOcc(lo, hi int) {
	c := p.doc.Code
	if hi-lo > occMaxText {
		return
	}
	q := c.Selected()
	if strings.TrimSpace(q) == "" || strings.ContainsRune(q, '\n') {
		return
	}
	for _, at := range search.All(c.Bytes(), q, search.Options{Case: true}) {
		if at == lo {
			p.occCur = len(p.occ)
		}
		p.occ = append(p.occ, highlight.Sem{Start: at, End: at + len(q)})
		p.occWrite = append(p.occWrite, false)
	}
	if len(p.occ) < 2 {
		p.clearOcc()
	}
}

func (p *Panel) writeAt(end int) bool {
	c := p.doc.Code
	l := c.Line(end)
	p.line = c.LineBytes(l, p.line[:0])
	return isWrite(p.line, end-c.LineStart(l))
}

func isWrite(line []byte, at int) bool {
	i := at
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	rest := line[i:]
	switch {
	case len(rest) == 0:
		return false
	case bytes.HasPrefix(rest, []byte("++")), bytes.HasPrefix(rest, []byte("--")):
		return true
	case rest[0] == '=':
		return len(rest) == 1 || rest[1] != '='
	case len(rest) > 1 && rest[1] == '=' && bytes.IndexByte([]byte(":+-*/%"), rest[0]) >= 0:
		return true
	}
	return false
}

func (p *Panel) drawOcc(ui *core.Context, font *text.Font, v *lineView, x0, y float32, ls, le int) {
	ct := &ui.Theme.Code
	for i, s := range p.occ {
		if s.End <= ls || s.Start > le {
			continue
		}
		a, b := max(s.Start, ls)-ls, min(s.End, le)-ls
		xa := x0 + v.x(font, v.idx[a])
		xb := x0 + v.x(font, v.idx[b])
		fill := ct.OccurrenceDim
		if i == p.occCur {
			fill = ct.Occurrence
		}
		ui.Rect(mathx.Rect{X: xa, Y: y + 1, W: xb - xa, H: p.lineH - 2}, fill)
		if i < len(p.occWrite) && p.occWrite[i] {
			ui.Rect(mathx.Rect{X: xa, Y: y + p.lineH - 2, W: xb - xa, H: 1}, ct.OccurrenceWrite)
		}
	}
}
