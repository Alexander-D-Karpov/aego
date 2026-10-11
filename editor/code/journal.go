package code

const journalLimit = 512

type edit struct {
	rev uint64
	off int
	n   int
	ins int
}

func (d *Doc) journal(off, n, ins int) {
	d.log = append(d.log, edit{rev: d.rev, off: off, n: n, ins: ins})
	if over := len(d.log) - journalLimit; over > 0 {
		d.log = append(d.log[:0], d.log[over:]...)
	}
}

func (d *Doc) covers(rev uint64) bool {
	return rev <= d.rev && d.rev-rev <= uint64(len(d.log))
}

func (d *Doc) MapSpan(from uint64, s, e int) (int, int, bool) {
	if from == d.rev {
		return s, e, true
	}
	if !d.covers(from) {
		return 0, 0, false
	}
	for _, x := range d.log {
		if x.rev <= from {
			continue
		}
		switch hi := x.off + x.n; {
		case hi <= s:
			delta := x.ins - x.n
			s += delta
			e += delta
		case x.off >= e:
		default:
			return 0, 0, false
		}
	}
	return s, e, true
}

func (d *Doc) Map(from uint64, off int) (int, bool) {
	s, _, ok := d.MapSpan(from, off, off)
	return s, ok
}

func (d *Doc) Unmap(to uint64, off int) (int, bool) {
	if to == d.rev {
		return off, true
	}
	if !d.covers(to) {
		return 0, false
	}
	for i := len(d.log) - 1; i >= 0; i-- {
		x := d.log[i]
		if x.rev <= to {
			break
		}
		switch {
		case off < x.off:
		case off >= x.off+x.ins:
			off += x.n - x.ins
		default:
			return 0, false
		}
	}
	return off, true
}
