package search

import "bytes"

type Options struct {
	Case bool
	Word bool
}

func fold(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return out
}

func wordByte(c byte) bool {
	return c == '_' || c >= 0x80 || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func whole(text []byte, at, n int) bool {
	if at > 0 && wordByte(text[at-1]) {
		return false
	}
	return at+n >= len(text) || !wordByte(text[at+n])
}

func All(text []byte, q string, o Options) []int {
	if q == "" {
		return nil
	}
	t, p := text, []byte(q)
	if !o.Case {
		t, p = fold(text), fold(p)
	}
	var out []int
	for i := 0; i <= len(t)-len(p); {
		j := bytes.Index(t[i:], p)
		if j < 0 {
			break
		}
		at := i + j
		if !o.Word || whole(text, at, len(p)) {
			out = append(out, at)
			i = at + len(p)
			continue
		}
		i = at + 1
	}
	return out
}

func Find(text []byte, q string, from int, back bool, o Options) (int, bool) {
	hits := All(text, q, o)
	if len(hits) == 0 {
		return 0, false
	}
	if !back {
		for _, h := range hits {
			if h >= from {
				return h, true
			}
		}
		return hits[0], true
	}
	for i := len(hits) - 1; i >= 0; i-- {
		if hits[i] < from {
			return hits[i], true
		}
	}
	return hits[len(hits)-1], true
}
