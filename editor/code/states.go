package code

import (
	"slices"

	"aego/editor/code/highlight"
)

const bracketScan = 1 << 16

var newline = []byte{'\n'}

func (d *Doc) initStates() {
	n := d.idx.Count()
	d.states = make([]highlight.State, n)
	for l := 0; l+1 < n; l++ {
		d.states[l+1] = highlight.Scan(d.lineText(l), d.states[l])
	}
}

func (d *Doc) LineState(l int) highlight.State {
	return d.states[d.line(l)]
}

func (d *Doc) spliceStates(l0, removed, added int) {
	d.states = slices.Replace(d.states, l0+1, l0+1+removed, make([]highlight.State, added)...)
	d.restate(l0, l0+added)
}

func (d *Doc) restate(from, last int) {
	n := d.idx.Count()
	for l := from; l+1 < n; l++ {
		out := highlight.Scan(d.lineText(l), d.states[l])
		if l >= last && d.states[l+1] == out {
			return
		}
		d.states[l+1] = out
	}
}

func (d *Doc) flatText() []byte {
	if d.flat == nil || d.flatRev != d.rev {
		d.flat, d.flatRev = d.buf.Bytes(), d.rev
	}
	return d.flat
}

func (d *Doc) Bracket(off int) (int, int, bool) {
	t := d.flatText()
	for _, at := range [2]int{off, off - 1} {
		if at < 0 || at >= len(t) {
			continue
		}
		if m, ok := matchAt(t, at); ok {
			return at, m, true
		}
	}
	return 0, 0, false
}

func matchAt(t []byte, at int) (int, bool) {
	var open, close byte
	dir := 1
	switch t[at] {
	case '(':
		open, close = '(', ')'
	case '[':
		open, close = '[', ']'
	case '{':
		open, close = '{', '}'
	case ')':
		open, close, dir = ')', '(', -1
	case ']':
		open, close, dir = ']', '[', -1
	case '}':
		open, close, dir = '}', '{', -1
	default:
		return 0, false
	}
	depth := 0
	for i, n := at, 0; i >= 0 && i < len(t) && n < bracketScan; i, n = i+dir, n+1 {
		switch t[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}
