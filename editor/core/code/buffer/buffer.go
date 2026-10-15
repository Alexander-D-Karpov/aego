package buffer

import "slices"

const compactAt = 2048

type piece struct {
	add bool
	off int
	n   int
}

type Table struct {
	orig   []byte
	add    []byte
	pieces []piece
	length int
}

func New(src []byte) *Table {
	t := &Table{orig: append([]byte(nil), src...), length: len(src)}
	if len(src) > 0 {
		t.pieces = []piece{{n: len(src)}}
	}
	return t
}

func clamp(v, lo, hi int) int {
	return min(max(v, lo), hi)
}

func (t *Table) Len() int {
	return t.length
}

func (t *Table) data(p piece) []byte {
	if p.add {
		return t.add[p.off : p.off+p.n]
	}
	return t.orig[p.off : p.off+p.n]
}

func (t *Table) find(off int) (int, int) {
	pos := 0
	for i, p := range t.pieces {
		if off < pos+p.n {
			return i, off - pos
		}
		pos += p.n
	}
	return len(t.pieces), 0
}

func (t *Table) Insert(off int, s []byte) {
	if len(s) == 0 {
		return
	}
	off = clamp(off, 0, t.length)
	start := len(t.add)
	t.add = append(t.add, s...)
	np := piece{add: true, off: start, n: len(s)}
	i, in := t.find(off)
	switch {
	case in > 0:
		p := t.pieces[i]
		t.pieces = slices.Replace(t.pieces, i, i+1,
			piece{add: p.add, off: p.off, n: in}, np, piece{add: p.add, off: p.off + in, n: p.n - in})
	case i > 0 && t.pieces[i-1].add && t.pieces[i-1].off+t.pieces[i-1].n == start:
		t.pieces[i-1].n += len(s)
	default:
		t.pieces = slices.Insert(t.pieces, i, np)
	}
	t.length += len(s)
	if len(t.pieces) > compactAt {
		t.Compact()
	}
}

func (t *Table) Delete(off, n int) {
	off = clamp(off, 0, t.length)
	n = clamp(n, 0, t.length-off)
	if n == 0 {
		return
	}
	end := off + n
	out := make([]piece, 0, len(t.pieces)+1)
	pos := 0
	for _, p := range t.pieces {
		ps, pe := pos, pos+p.n
		pos = pe
		if pe <= off || ps >= end {
			out = append(out, p)
			continue
		}
		if ps < off {
			out = append(out, piece{add: p.add, off: p.off, n: off - ps})
		}
		if pe > end {
			out = append(out, piece{add: p.add, off: p.off + end - ps, n: pe - end})
		}
	}
	t.pieces = out
	t.length -= n
}

func (t *Table) Slice(off, n int, dst []byte) []byte {
	off = clamp(off, 0, t.length)
	n = clamp(n, 0, t.length-off)
	end := off + n
	pos := 0
	for _, p := range t.pieces {
		ps, pe := pos, pos+p.n
		pos = pe
		if pe <= off {
			continue
		}
		if ps >= end {
			break
		}
		d := t.data(p)
		dst = append(dst, d[max(off, ps)-ps:min(end, pe)-ps]...)
	}
	return dst
}

func (t *Table) Bytes() []byte {
	return t.Slice(0, t.length, make([]byte, 0, t.length))
}

func (t *Table) Compact() {
	b := t.Bytes()
	t.orig, t.add, t.pieces = b, nil, nil
	if len(b) > 0 {
		t.pieces = []piece{{n: len(b)}}
	}
}
