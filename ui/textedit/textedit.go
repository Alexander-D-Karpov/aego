package textedit

import (
	"slices"
	"strings"
	"unicode"
)

type Dir int8

const (
	Left Dir = iota
	Right
	Home
	End
)

type op uint8

const (
	opNone op = iota
	opType
	opDelete
	opOther
)

type snapshot struct {
	runes  []rune
	cursor int
	anchor int
}

type Buffer struct {
	Multiline    bool
	MaxLen       int
	HistoryLimit int
	runes        []rune
	cursor       int
	anchor       int
	comp         []rune
	compCursor   int
	history      []snapshot
	last         op
	rev          uint64
}

func New(s string) *Buffer {
	b := &Buffer{HistoryLimit: 64}
	b.Set(s)
	return b
}

func (b *Buffer) Set(s string) {
	b.runes = append(b.runes[:0], []rune(s)...)
	b.cursor = len(b.runes)
	b.anchor = b.cursor
	b.comp = b.comp[:0]
	b.compCursor = 0
	clear(b.history)
	b.history = b.history[:0]
	b.last = opNone
	b.rev++
}

func (b *Buffer) String() string {
	return string(b.runes)
}

func (b *Buffer) Runes() []rune {
	return b.runes
}

func (b *Buffer) Len() int {
	return len(b.runes)
}

func (b *Buffer) Revision() uint64 {
	return b.rev
}

func (b *Buffer) Cursor() int {
	return b.cursor
}

func (b *Buffer) Anchor() int {
	return b.anchor
}

func (b *Buffer) Selection() (int, int) {
	if b.anchor < b.cursor {
		return b.anchor, b.cursor
	}
	return b.cursor, b.anchor
}

func (b *Buffer) HasSelection() bool {
	return b.anchor != b.cursor
}

func (b *Buffer) clamp(i int) int {
	if i < 0 {
		return 0
	}
	if i > len(b.runes) {
		return len(b.runes)
	}
	return i
}

func (b *Buffer) SetCursor(i int, extend bool) {
	b.cursor = b.clamp(i)
	if !extend {
		b.anchor = b.cursor
	}
	b.last = opOther
}

func (b *Buffer) SelectAll() {
	b.anchor = 0
	b.cursor = len(b.runes)
	b.last = opOther
}

func (b *Buffer) push(kind op) {
	if kind != opOther && kind == b.last {
		return
	}
	b.history = append(b.history, snapshot{
		runes:  slices.Clone(b.runes),
		cursor: b.cursor,
		anchor: b.anchor,
	})
	limit := b.HistoryLimit
	if limit <= 0 {
		limit = 64
	}
	if len(b.history) > limit {
		b.history = b.history[len(b.history)-limit:]
	}
	b.last = kind
}

func (b *Buffer) deleteSelection() bool {
	lo, hi := b.Selection()
	if lo == hi {
		return false
	}
	b.runes = slices.Delete(b.runes, lo, hi)
	b.cursor, b.anchor = lo, lo
	b.rev++
	return true
}

func sanitize(s string, multiline bool) []rune {
	if multiline {
		s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	}
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if multiline && (r == '\n' || r == '\t') {
			out = append(out, r)
			continue
		}
		if r == '\n' || r == '\r' {
			r = ' '
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		out = append(out, r)
	}
	return out
}

func (b *Buffer) Insert(s string) {
	ins := sanitize(s, b.Multiline)
	if len(ins) == 0 {
		return
	}
	kind := opType
	if len(ins) > 1 || unicode.IsSpace(ins[0]) || b.HasSelection() {
		kind = opOther
	}
	b.push(kind)
	b.deleteSelection()
	if b.MaxLen > 0 {
		room := b.MaxLen - len(b.runes)
		if room <= 0 {
			return
		}
		if len(ins) > room {
			ins = ins[:room]
		}
	}
	b.runes = slices.Insert(b.runes, b.cursor, ins...)
	b.cursor += len(ins)
	b.anchor = b.cursor
	b.comp = b.comp[:0]
	b.rev++
}

func (b *Buffer) Composition(s string, cursor int) {
	b.comp = append(b.comp[:0], []rune(s)...)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(b.comp) {
		cursor = len(b.comp)
	}
	b.compCursor = cursor
}

func (b *Buffer) CompositionRunes() []rune {
	return b.comp
}

func (b *Buffer) CompositionCursor() int {
	return b.compCursor
}

func isWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func (b *Buffer) wordLeft(i int) int {
	for i > 0 && !isWord(b.runes[i-1]) {
		i--
	}
	for i > 0 && isWord(b.runes[i-1]) {
		i--
	}
	return i
}

func (b *Buffer) wordRight(i int) int {
	n := len(b.runes)
	for i < n && !isWord(b.runes[i]) {
		i++
	}
	for i < n && isWord(b.runes[i]) {
		i++
	}
	return i
}

func (b *Buffer) Move(d Dir, word, extend bool) {
	if !extend && b.HasSelection() && (d == Left || d == Right) && !word {
		lo, hi := b.Selection()
		if d == Left {
			b.cursor = lo
		} else {
			b.cursor = hi
		}
		b.anchor = b.cursor
		b.last = opOther
		return
	}
	switch d {
	case Left:
		if word {
			b.cursor = b.wordLeft(b.cursor)
		} else {
			b.cursor--
		}
	case Right:
		if word {
			b.cursor = b.wordRight(b.cursor)
		} else {
			b.cursor++
		}
	case Home:
		b.cursor = 0
	case End:
		b.cursor = len(b.runes)
	}
	b.cursor = b.clamp(b.cursor)
	if !extend {
		b.anchor = b.cursor
	}
	b.last = opOther
}

func (b *Buffer) Delete(d Dir, word bool) bool {
	if b.HasSelection() {
		b.push(opOther)
		return b.deleteSelection()
	}
	from, to := b.cursor, b.cursor
	switch d {
	case Left, Home:
		if b.cursor == 0 {
			return false
		}
		from = b.cursor - 1
		if word {
			from = b.wordLeft(b.cursor)
		}
		if d == Home {
			from = 0
		}
	default:
		if b.cursor == len(b.runes) {
			return false
		}
		to = b.cursor + 1
		if word {
			to = b.wordRight(b.cursor)
		}
		if d == End {
			to = len(b.runes)
		}
	}
	b.push(opDelete)
	b.runes = slices.Delete(b.runes, from, to)
	b.cursor, b.anchor = from, from
	b.rev++
	return true
}

func (b *Buffer) Copy() string {
	lo, hi := b.Selection()
	return string(b.runes[lo:hi])
}

func (b *Buffer) Cut() string {
	s := b.Copy()
	if s != "" {
		b.push(opOther)
		b.deleteSelection()
	}
	return s
}

func (b *Buffer) Undo() bool {
	n := len(b.history)
	if n == 0 {
		return false
	}
	s := b.history[n-1]
	b.history = b.history[:n-1]
	b.runes = append(b.runes[:0], s.runes...)
	b.cursor = b.clamp(s.cursor)
	b.anchor = b.clamp(s.anchor)
	b.last = opNone
	b.rev++
	return true
}
