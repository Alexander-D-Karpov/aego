package code

import (
	"slices"
	"sort"

	"aego/editor/code/highlight"
)

const TabWidth = 4

var newline = []byte{'\n'}

type BracketState uint8

const (
	BracketNone BracketState = iota
	BracketMatched
	BracketUnmatched
)

type Block struct {
	Open  int
	Close int
	Line0 int
	Line1 int
	Level int
	Depth int
}

type opener struct {
	off   int
	line  int
	ch    byte
	depth int
}

type bracketIndex struct {
	rev     uint64
	ok      bool
	partner map[int]int
	bad     []int
	blocks  []Block
	stack   []opener
	brs     []highlight.BracketPos
}

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
		d.restated++
		if l >= last && d.states[l+1] == out {
			return
		}
		d.states[l+1] = out
	}
}

func closerOf(c byte) byte {
	switch c {
	case '(':
		return ')'
	case '[':
		return ']'
	case '{':
		return '}'
	case '<':
		return '>'
	}
	return 0
}

func indentCols(line []byte) int {
	n := 0
	for _, c := range line {
		switch c {
		case ' ':
			n++
		case '\t':
			n += TabWidth - n%TabWidth
		default:
			return n
		}
	}
	return n
}

func (d *Doc) brackets() *bracketIndex {
	b := &d.br
	if b.ok && b.rev == d.rev {
		return b
	}
	b.rev, b.ok = d.rev, true
	if b.partner == nil {
		b.partner = map[int]int{}
	} else {
		clear(b.partner)
	}
	b.bad, b.blocks, b.stack = b.bad[:0], b.blocks[:0], b.stack[:0]
	n := d.idx.Count()
	for l := 0; l < n; l++ {
		ls := d.idx.Offset(l)
		b.brs, _ = highlight.Brackets(d.lineText(l), d.states[l], b.brs[:0])
		for _, br := range b.brs {
			at := ls + br.Off
			if br.Open {
				b.stack = append(b.stack, opener{off: at, line: l, ch: br.Ch, depth: br.Depth})
				continue
			}
			k := len(b.stack) - 1
			for k >= 0 && closerOf(b.stack[k].ch) != br.Ch {
				k--
			}
			if k < 0 {
				b.bad = append(b.bad, at)
				continue
			}
			for _, o := range b.stack[k+1:] {
				b.bad = append(b.bad, o.off)
			}
			o := b.stack[k]
			b.stack = b.stack[:k]
			b.partner[o.off], b.partner[at] = at, o.off
			if o.ch == '{' {
				b.blocks = append(b.blocks, Block{
					Open:  o.off,
					Close: at,
					Line0: o.line,
					Line1: l,
					Level: indentCols(d.LineBytes(o.line, nil)) / TabWidth,
					Depth: o.depth,
				})
			}
		}
	}
	for _, o := range b.stack {
		b.bad = append(b.bad, o.off)
	}
	slices.Sort(b.bad)
	slices.SortFunc(b.blocks, func(x, y Block) int { return x.Open - y.Open })
	return b
}

func (d *Doc) Unmatched(off int) bool {
	bad := d.brackets().bad
	i := sort.SearchInts(bad, off)
	return i < len(bad) && bad[i] == off
}

func (d *Doc) BracketAt(off int) (int, int, BracketState) {
	b := d.brackets()
	lone := -1
	for _, at := range [2]int{off, off - 1} {
		if m, ok := b.partner[at]; ok {
			return at, m, BracketMatched
		}
		if lone < 0 && d.Unmatched(at) {
			lone = at
		}
	}
	if lone >= 0 {
		return lone, -1, BracketUnmatched
	}
	return 0, 0, BracketNone
}

func (d *Doc) BlockAt(off int) (Block, bool) {
	var best Block
	found := false
	for _, b := range d.brackets().blocks {
		if b.Open >= off {
			break
		}
		if off <= b.Close && b.Line1 > b.Line0 {
			best, found = b, true
		}
	}
	return best, found
}

func (d *Doc) Guides() []Block {
	return d.brackets().blocks
}
