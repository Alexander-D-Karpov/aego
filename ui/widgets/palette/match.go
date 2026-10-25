package palette

import (
	"unicode"
	"unicode/utf8"
)

type Tier uint8

const (
	TierPrefix Tier = iota
	TierInitials
	TierWord
	TierSubstring
	TierNone
)

// Text is a label prepared for matching: lowered runes, their byte offsets and the word starts.
type Text struct {
	low   []rune
	off   []int
	words []int
}

func Prepare(label string) Text {
	t := Text{}
	var prev rune
	for i, r := range label {
		start := false
		switch {
		case len(t.low) == 0:
			start = !separator(r)
		case separator(r):
		case separator(prev):
			start = true
		case unicode.IsUpper(r) && !unicode.IsUpper(prev):
			start = true
		case unicode.IsDigit(r) != unicode.IsDigit(prev):
			start = true
		}
		if start {
			t.words = append(t.words, len(t.low))
		}
		t.low = append(t.low, unicode.ToLower(r))
		t.off = append(t.off, i)
		prev = r
	}
	return t
}

func separator(r rune) bool {
	switch r {
	case ' ', '_', '-', '.', '/', ':', '(', ')':
		return true
	}
	return false
}

// Lower turns a query into lowered runes without separators, reusing dst.
func Lower(q string, dst []rune) []rune {
	dst = dst[:0]
	for _, r := range q {
		if r == utf8.RuneError || separator(r) {
			continue
		}
		dst = append(dst, unicode.ToLower(r))
	}
	return dst
}

// Match ranks q against t: name prefix, then initials, then word starts, then a substring. pos gets byte offsets of the matched runes.
func Match(q []rune, t *Text, pos []int) (Tier, []int) {
	pos = pos[:0]
	if len(q) == 0 {
		return TierPrefix, pos
	}
	if p, ok := t.prefixAt(q, 0, pos); ok {
		return TierPrefix, p
	}
	if p, ok := t.chain(q, 0, 0, pos, true); ok {
		return TierInitials, p
	}
	for w := 1; w < len(t.words); w++ {
		if p, ok := t.chain(q, w, 0, pos[:0], false); ok {
			return TierWord, p
		}
	}
	if p, ok := t.chain(q, 0, 0, pos[:0], false); ok {
		return TierWord, p
	}
	for i := 1; i+len(q) <= len(t.low); i++ {
		if p, ok := t.prefixAt(q, i, pos[:0]); ok {
			return TierSubstring, p
		}
	}
	return TierNone, pos[:0]
}

func (t *Text) prefixAt(q []rune, at int, pos []int) ([]int, bool) {
	if at+len(q) > len(t.low) {
		return pos, false
	}
	for i, r := range q {
		if t.low[at+i] != r {
			return pos, false
		}
	}
	for i := range q {
		pos = append(pos, t.off[at+i])
	}
	return pos, true
}

// chain matches q as prefixes of words taken in order from word w on; initials takes one rune per word and may skip words.
func (t *Text) chain(q []rune, w, qi int, pos []int, initials bool) ([]int, bool) {
	if qi == len(q) {
		return pos, true
	}
	for ; w < len(t.words); w++ {
		at := t.words[w]
		end := len(t.low)
		if w+1 < len(t.words) {
			end = t.words[w+1]
		}
		n := 0
		for at+n < end && qi+n < len(q) && t.low[at+n] == q[qi+n] {
			n++
		}
		if initials {
			n = min(n, 1)
		}
		for k := n; k >= 1; k-- {
			mark := len(pos)
			for i := 0; i < k; i++ {
				pos = append(pos, t.off[at+i])
			}
			if p, ok := t.chain(q, w+1, qi+k, pos, initials); ok {
				return p, true
			}
			pos = pos[:mark]
		}
		if !initials {
			return pos, false
		}
	}
	return pos, false
}
