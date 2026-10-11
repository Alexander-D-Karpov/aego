package code

import (
	"bytes"
	"hash/fnv"
	"time"
	"unicode"
	"unicode/utf8"

	"aego/editor/code/buffer"
	"aego/editor/code/highlight"
	"aego/editor/code/lines"
	"aego/editor/code/search"
)

const (
	groupWindow  = 300 * time.Millisecond
	historyLimit = 1000
)

type Dir uint8

const (
	Left Dir = iota
	Right
	Up
	Down
	LineStart
	LineEnd
	DocStart
	DocEnd
)

type Kind uint8

const (
	KindOther Kind = iota
	KindType
	KindDelete
)

type Selection struct {
	Anchor int
	Head   int
}

func (s Selection) Range() (int, int) {
	if s.Anchor < s.Head {
		return s.Anchor, s.Head
	}
	return s.Head, s.Anchor
}

func (s Selection) Empty() bool {
	return s.Anchor == s.Head
}

type record struct {
	off      int
	removed  []byte
	inserted []byte
}

type group struct {
	kind   Kind
	at     time.Time
	recs   []record
	before Selection
	after  Selection
	sealed bool
}

type Doc struct {
	Sel     Selection
	Now     func() time.Time
	buf     *buffer.Table
	idx     *lines.Index
	states  []highlight.State
	log     []edit
	flat    []byte
	flatRev uint64
	goal    int
	undo    []group
	redo    []group
	batch   *group
	rev     uint64
	saved   uint64
	hash    uint64
	hashed  bool
	hashRev uint64
	scratch []byte
}

func normalize(b []byte) []byte {
	if bytes.IndexByte(b, '\r') < 0 {
		return b
	}
	return bytes.ReplaceAll(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")), []byte("\r"), []byte("\n"))
}

func New(src []byte) *Doc {
	src = normalize(src)
	d := &Doc{buf: buffer.New(src), idx: lines.New(src), goal: -1, Now: time.Now}
	d.initStates()
	d.MarkSaved()
	return d
}

func (d *Doc) Rev() uint64                         { return d.rev }
func (d *Doc) Len() int                            { return d.buf.Len() }
func (d *Doc) Bytes() []byte                       { return d.buf.Bytes() }
func (d *Doc) Slice(off, n int, dst []byte) []byte { return d.buf.Slice(off, n, dst) }
func (d *Doc) Lines() int                          { return d.idx.Count() }
func (d *Doc) Cursor() int                         { return d.Sel.Head }
func (d *Doc) CanUndo() bool                       { return len(d.undo) > 0 }
func (d *Doc) CanRedo() bool                       { return len(d.redo) > 0 }

func (d *Doc) clamp(off int) int {
	return min(max(off, 0), d.buf.Len())
}

func (d *Doc) line(l int) int {
	return min(max(l, 0), d.idx.Count()-1)
}

func (d *Doc) LineStart(l int) int {
	return d.idx.Offset(d.line(l))
}

func (d *Doc) LineEnd(l int) int {
	return d.idx.End(d.line(l))
}

func (d *Doc) LineBytes(l int, dst []byte) []byte {
	ls, le := d.LineStart(l), d.LineEnd(l)
	return d.buf.Slice(ls, le-ls, dst)
}

func (d *Doc) lineText(l int) []byte {
	d.scratch = d.LineBytes(l, d.scratch[:0])
	return d.scratch
}

func (d *Doc) Line(off int) int {
	return d.idx.Line(d.clamp(off))
}

func (d *Doc) LineCol(off int) (int, int) {
	off = d.clamp(off)
	l := d.idx.Line(off)
	text := d.lineText(l)
	return l, utf8.RuneCount(text[:off-d.idx.Offset(l)])
}

func (d *Doc) Offset(line, col int) int {
	l := d.line(line)
	text := d.lineText(l)
	i := 0
	for c := 0; c < col && i < len(text); c++ {
		_, s := utf8.DecodeRune(text[i:])
		i += s
	}
	return d.idx.Offset(l) + i
}

func (d *Doc) contentHash() uint64 {
	if !d.hashed || d.hashRev != d.rev {
		h := fnv.New64a()
		h.Write(d.buf.Bytes())
		d.hash, d.hashed, d.hashRev = h.Sum64(), true, d.rev
	}
	return d.hash
}

func (d *Doc) MarkSaved() {
	d.saved = d.contentHash()
	d.seal()
}

func (d *Doc) SetSaved(disk []byte) {
	h := fnv.New64a()
	h.Write(normalize(disk))
	d.saved = h.Sum64()
}

func (d *Doc) Dirty() bool {
	return d.contentHash() != d.saved
}

func (d *Doc) seal() {
	if n := len(d.undo); n > 0 {
		d.undo[n-1].sealed = true
	}
}

func (d *Doc) apply(off, n int, ins []byte) []byte {
	l0 := d.idx.Line(off)
	removed := d.buf.Slice(off, n, nil)
	d.buf.Delete(off, n)
	d.buf.Insert(off, ins)
	d.idx.Edit(off, n, ins)
	d.rev++
	d.journal(off, n, len(ins))
	d.spliceStates(l0, bytes.Count(removed, newline), bytes.Count(ins, newline))
	return removed
}

func (d *Doc) edit(kind Kind, off, n int, ins []byte, after Selection) {
	off = d.clamp(off)
	n = min(max(n, 0), d.buf.Len()-off)
	before := d.Sel
	rec := record{off: off, removed: d.apply(off, n, ins), inserted: append([]byte(nil), ins...)}
	d.Sel = Selection{Anchor: d.clamp(after.Anchor), Head: d.clamp(after.Head)}
	d.goal = -1
	d.push(kind, rec, before, d.Sel)
}

func (d *Doc) push(kind Kind, rec record, before, after Selection) {
	d.redo = d.redo[:0]
	now := d.Now()
	if n := len(d.undo); n > 0 && kind != KindOther && bytes.IndexByte(rec.inserted, '\n') < 0 {
		g := &d.undo[n-1]
		if !g.sealed && g.kind == kind && now.Sub(g.at) < groupWindow {
			g.recs = append(g.recs, rec)
			g.at, g.after = now, after
			return
		}
	}
	d.undo = append(d.undo, group{kind: kind, at: now, recs: []record{rec}, before: before, after: after})
	if over := len(d.undo) - historyLimit; over > 0 {
		d.undo = append(d.undo[:0], d.undo[over:]...)
	}
}

func shift(pos, off, n, ins int) int {
	switch {
	case pos <= off:
		return pos
	case pos >= off+n:
		return pos + ins - n
	}
	return off
}

func (d *Doc) begin() {
	d.batch = &group{kind: KindOther, before: d.Sel}
}

func (d *Doc) change(off, n int, ins []byte) {
	rec := record{off: off, removed: d.apply(off, n, ins), inserted: append([]byte(nil), ins...)}
	d.batch.recs = append(d.batch.recs, rec)
	d.Sel.Anchor = shift(d.Sel.Anchor, off, n, len(ins))
	d.Sel.Head = shift(d.Sel.Head, off, n, len(ins))
}

func (d *Doc) commit() {
	g := d.batch
	d.batch = nil
	if g == nil || len(g.recs) == 0 {
		return
	}
	g.at, g.after, g.sealed = d.Now(), d.Sel, true
	d.goal = -1
	d.redo = d.redo[:0]
	d.seal()
	d.undo = append(d.undo, *g)
}

func (d *Doc) Undo() bool {
	n := len(d.undo)
	if n == 0 {
		return false
	}
	g := d.undo[n-1]
	d.undo = d.undo[:n-1]
	for i := len(g.recs) - 1; i >= 0; i-- {
		r := g.recs[i]
		d.apply(r.off, len(r.inserted), r.removed)
	}
	d.Sel, d.goal = g.before, -1
	g.sealed = true
	d.redo = append(d.redo, g)
	d.seal()
	return true
}

func (d *Doc) Redo() bool {
	n := len(d.redo)
	if n == 0 {
		return false
	}
	g := d.redo[n-1]
	d.redo = d.redo[:n-1]
	for _, r := range g.recs {
		d.apply(r.off, len(r.removed), r.inserted)
	}
	d.Sel, d.goal = g.after, -1
	d.undo = append(d.undo, g)
	return true
}

func (d *Doc) SetCursor(off int, extend bool) {
	d.setHead(off, extend)
	d.goal = -1
	d.seal()
}

func (d *Doc) setHead(head int, extend bool) {
	d.Sel.Head = d.clamp(head)
	if !extend {
		d.Sel.Anchor = d.Sel.Head
	}
}

func (d *Doc) Select(lo, hi int) {
	d.Sel = Selection{Anchor: d.clamp(lo), Head: d.clamp(hi)}
	d.goal = -1
	d.seal()
}

func (d *Doc) SelectAll() {
	d.Select(0, d.buf.Len())
}

func (d *Doc) Selected() string {
	lo, hi := d.Sel.Range()
	return string(d.buf.Slice(lo, hi-lo, nil))
}

func isWord(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func punct(r rune) bool {
	return !isWord(r) && !unicode.IsSpace(r)
}

func (d *Doc) SelectWord(off int) {
	off = d.clamp(off)
	l := d.idx.Line(off)
	ls := d.idx.Offset(l)
	text := d.lineText(l)
	a, b := off-ls, off-ls
	for a > 0 {
		r, s := utf8.DecodeLastRune(text[:a])
		if !isWord(r) {
			break
		}
		a -= s
	}
	for b < len(text) {
		r, s := utf8.DecodeRune(text[b:])
		if !isWord(r) {
			break
		}
		b += s
	}
	d.Select(ls+a, ls+b)
}

func (d *Doc) prev(off int, word bool) int {
	l := d.idx.Line(off)
	ls := d.idx.Offset(l)
	if off == ls {
		return max(off-1, 0)
	}
	text := d.lineText(l)[:off-ls]
	if !word {
		_, s := utf8.DecodeLastRune(text)
		return off - s
	}
	i := len(text)
	skip := func(pred func(rune) bool) {
		for i > 0 {
			r, s := utf8.DecodeLastRune(text[:i])
			if !pred(r) {
				return
			}
			i -= s
		}
	}
	skip(unicode.IsSpace)
	if i > 0 {
		if r, _ := utf8.DecodeLastRune(text[:i]); isWord(r) {
			skip(isWord)
		} else {
			skip(punct)
		}
	}
	return ls + i
}

func (d *Doc) next(off int, word bool) int {
	l := d.idx.Line(off)
	ls, le := d.idx.Offset(l), d.idx.End(l)
	if off >= le {
		return min(off+1, d.buf.Len())
	}
	text := d.lineText(l)
	i := off - ls
	if !word {
		_, s := utf8.DecodeRune(text[i:])
		return off + s
	}
	skip := func(pred func(rune) bool) {
		for i < len(text) {
			r, s := utf8.DecodeRune(text[i:])
			if !pred(r) {
				return
			}
			i += s
		}
	}
	skip(unicode.IsSpace)
	if i < len(text) {
		if r, _ := utf8.DecodeRune(text[i:]); isWord(r) {
			skip(isWord)
		} else {
			skip(punct)
		}
	}
	return ls + i
}

func (d *Doc) Move(dir Dir, word, extend bool) {
	head := d.Sel.Head
	lo, hi := d.Sel.Range()
	goal := -1
	switch dir {
	case Left:
		switch {
		case !extend && !d.Sel.Empty():
			head = lo
		case head > 0:
			head = d.prev(head, word)
		}
	case Right:
		switch {
		case !extend && !d.Sel.Empty():
			head = hi
		case head < d.buf.Len():
			head = d.next(head, word)
		}
	case Up, Down:
		l, c := d.LineCol(head)
		if d.goal >= 0 {
			c = d.goal
		}
		goal = c
		switch {
		case dir == Up && l == 0:
			head = 0
		case dir == Up:
			head = d.Offset(l-1, c)
		case l >= d.idx.Count()-1:
			head = d.buf.Len()
		default:
			head = d.Offset(l+1, c)
		}
	case LineStart:
		l := d.idx.Line(head)
		ls := d.idx.Offset(l)
		text := d.lineText(l)
		first := ls + len(text) - len(bytes.TrimLeft(text, " \t"))
		if head == first {
			head = ls
		} else {
			head = first
		}
	case LineEnd:
		head = d.idx.End(d.idx.Line(head))
	case DocStart:
		head = 0
	case DocEnd:
		head = d.buf.Len()
	}
	d.setHead(head, extend)
	d.goal = goal
	d.seal()
}

func (d *Doc) MoveLines(n int, extend bool) {
	l, c := d.LineCol(d.Sel.Head)
	if d.goal >= 0 {
		c = d.goal
	}
	d.setHead(d.Offset(l+n, c), extend)
	d.goal = c
	d.seal()
}

func (d *Doc) Replace(off, n int, s string) {
	b := normalize([]byte(s))
	end := d.clamp(off) + len(b)
	d.edit(KindOther, off, n, b, Selection{Anchor: end, Head: end})
}

func (d *Doc) ReplaceSelection(s string) {
	lo, hi := d.Sel.Range()
	b := normalize([]byte(s))
	d.edit(KindOther, lo, hi-lo, b, Selection{Anchor: lo, Head: lo + len(b)})
}

func (d *Doc) Type(s string) {
	b := normalize([]byte(s))
	if len(b) == 0 {
		return
	}
	lo, hi := d.Sel.Range()
	kind := KindType
	if hi > lo || len(b) > 1 {
		kind = KindOther
	}
	if s == "}" && lo == hi {
		ls := d.idx.Offset(d.idx.Line(lo))
		pre := d.buf.Slice(ls, lo-ls, nil)
		if len(pre) > 0 && pre[len(pre)-1] == '\t' && len(bytes.TrimLeft(pre, " \t")) == 0 {
			lo--
		}
	}
	end := lo + len(b)
	d.edit(kind, lo, hi-lo, b, Selection{Anchor: end, Head: end})
}

func (d *Doc) Paste(s string) {
	d.Type(s)
	d.seal()
}

func (d *Doc) Cut() string {
	s := d.Selected()
	if s != "" {
		lo, hi := d.Sel.Range()
		d.edit(KindOther, lo, hi-lo, nil, Selection{Anchor: lo, Head: lo})
	}
	return s
}

func (d *Doc) Newline() {
	lo, hi := d.Sel.Range()
	l := d.idx.Line(lo)
	ls := d.idx.Offset(l)
	line := append([]byte(nil), d.lineText(l)...)
	local := lo - ls
	ws := line[:len(line)-len(bytes.TrimLeft(line, " \t"))]
	if len(ws) > local {
		ws = ws[:local]
	}
	before := bytes.TrimRight(line[:local], " \t")
	after := d.buf.Slice(hi, 1, nil)
	ins := "\n" + string(ws)
	cursor := len(ins)
	if len(before) > 0 && before[len(before)-1] == '{' {
		ins += "\t"
		cursor = len(ins)
		if len(after) == 1 && after[0] == '}' {
			ins += "\n" + string(ws)
		}
	}
	d.edit(KindOther, lo, hi-lo, []byte(ins), Selection{Anchor: lo + cursor, Head: lo + cursor})
}

func (d *Doc) Delete(dir Dir, word bool) {
	lo, hi := d.Sel.Range()
	if lo == hi {
		switch {
		case dir == Left && lo == 0, dir != Left && hi >= d.buf.Len():
			return
		case dir == Left:
			lo = d.prev(lo, word)
		default:
			hi = d.next(hi, word)
		}
	}
	d.edit(KindDelete, lo, hi-lo, nil, Selection{Anchor: lo, Head: lo})
}

func (d *Doc) lineRange() (int, int) {
	lo, hi := d.Sel.Range()
	l0, l1 := d.idx.Line(lo), d.idx.Line(hi)
	if hi > lo && l1 > l0 && hi == d.idx.Offset(l1) {
		l1--
	}
	return l0, l1
}

func (d *Doc) selectLines(l0, l1 int, had bool) {
	if had {
		d.Sel = Selection{Anchor: d.idx.Offset(l0), Head: d.idx.End(l1)}
	}
}

func (d *Doc) Indent() {
	l0, l1 := d.lineRange()
	had := !d.Sel.Empty()
	d.begin()
	for l := l0; l <= l1; l++ {
		d.change(d.idx.Offset(l), 0, []byte{'\t'})
	}
	d.selectLines(l0, l1, had)
	d.commit()
}

func (d *Doc) Unindent() {
	l0, l1 := d.lineRange()
	had := !d.Sel.Empty()
	d.begin()
	for l := l0; l <= l1; l++ {
		text := d.lineText(l)
		n := 0
		if len(text) > 0 && text[0] == '\t' {
			n = 1
		} else {
			for n < 4 && n < len(text) && text[n] == ' ' {
				n++
			}
		}
		if n > 0 {
			d.change(d.idx.Offset(l), n, nil)
		}
	}
	d.selectLines(l0, l1, had)
	d.commit()
}

func (d *Doc) ToggleComment() {
	l0, l1 := d.lineRange()
	all, any := true, false
	for l := l0; l <= l1; l++ {
		t := bytes.TrimLeft(d.lineText(l), " \t")
		if len(t) == 0 {
			continue
		}
		any = true
		if !bytes.HasPrefix(t, []byte("//")) {
			all = false
		}
	}
	if !any {
		return
	}
	had := !d.Sel.Empty()
	d.begin()
	for l := l0; l <= l1; l++ {
		text := d.lineText(l)
		t := bytes.TrimLeft(text, " \t")
		if len(t) == 0 {
			continue
		}
		at := d.idx.Offset(l) + len(text) - len(t)
		if all {
			n := 2
			if len(t) > 2 && t[2] == ' ' {
				n = 3
			}
			d.change(at, n, nil)
		} else {
			d.change(at, 0, []byte("// "))
		}
	}
	d.selectLines(l0, l1, had)
	d.commit()
}

func (d *Doc) ReplaceAll(q, repl string, o search.Options) int {
	hits := search.All(d.buf.Bytes(), q, o)
	if len(hits) == 0 {
		return 0
	}
	d.begin()
	for i := len(hits) - 1; i >= 0; i-- {
		d.change(hits[i], len(q), []byte(repl))
	}
	d.commit()
	return len(hits)
}
