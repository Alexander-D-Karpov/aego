package code

func (d *Doc) ReplaceRanges(rs [][2]int, texts [][]byte) int {
	if len(rs) == 0 || len(rs) != len(texts) {
		return 0
	}
	d.begin()
	for i := len(rs) - 1; i >= 0; i-- {
		d.change(rs[i][0], rs[i][1]-rs[i][0], normalize(texts[i]))
	}
	d.commit()
	return len(rs)
}

func (d *Doc) DuplicateLines() {
	l0, l1 := d.lineRange()
	s, e := d.idx.Offset(l0), d.idx.End(l1)
	text := d.buf.Slice(s, e-s, nil)
	a, h := d.Sel.Anchor, d.Sel.Head
	shift := e - s + 1
	d.begin()
	d.change(e, 0, append([]byte{'\n'}, text...))
	d.Sel = Selection{Anchor: a + shift, Head: h + shift}
	d.commit()
}

func (d *Doc) DeleteLines() {
	l0, l1 := d.lineRange()
	s, e := d.idx.Offset(l0), d.idx.End(l1)
	switch {
	case e < d.buf.Len():
		e++
	case s > 0:
		s--
	}
	_, col := d.LineCol(d.Sel.Head)
	d.begin()
	d.change(s, e-s, nil)
	off := d.Offset(min(l0, d.idx.Count()-1), col)
	d.Sel = Selection{Anchor: off, Head: off}
	d.commit()
}

func (d *Doc) ShiftLines(dir int) {
	l0, l1 := d.lineRange()
	if (dir < 0 && l0 == 0) || (dir > 0 && l1 >= d.idx.Count()-1) {
		return
	}
	s, e := d.idx.Offset(l0), d.idx.End(l1)
	block := d.buf.Slice(s, e-s, nil)
	a, h := d.Sel.Anchor, d.Sel.Head
	d.begin()
	if dir < 0 {
		ps := d.idx.Offset(l0 - 1)
		prev := d.buf.Slice(ps, s-1-ps, nil)
		repl := append(append(block, '\n'), prev...)
		d.change(ps, e-ps, repl)
		delta := ps - s
		d.Sel = Selection{Anchor: a + delta, Head: h + delta}
	} else {
		ne := d.idx.End(l1 + 1)
		next := d.buf.Slice(e+1, ne-e-1, nil)
		repl := append(append(next, '\n'), block...)
		d.change(s, ne-s, repl)
		delta := ne - e
		d.Sel = Selection{Anchor: a + delta, Head: h + delta}
	}
	d.commit()
}
