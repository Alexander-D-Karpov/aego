package bits

type Set struct {
	words []uint64
	n     int
}

func NewSet(n int) *Set {
	if n < 0 {
		n = 0
	}
	return &Set{words: make([]uint64, (n+63)/64), n: n}
}

func (s *Set) Len() int {
	return s.n
}

func (s *Set) Grow(n int) {
	if n <= s.n {
		return
	}
	need := (n + 63) / 64
	if need > len(s.words) {
		w := make([]uint64, need)
		copy(w, s.words)
		s.words = w
	}
	s.n = n
}

func (s *Set) Set(i int) {
	if i < 0 || i >= s.n {
		return
	}
	s.words[i>>6] |= 1 << uint(i&63)
}

func (s *Set) Clear(i int) {
	if i < 0 || i >= s.n {
		return
	}
	s.words[i>>6] &^= 1 << uint(i&63)
}

func (s *Set) Has(i int) bool {
	if i < 0 || i >= s.n {
		return false
	}
	return s.words[i>>6]&(1<<uint(i&63)) != 0
}

func (s *Set) Reset() {
	clear(s.words)
}

func (s *Set) Count() int {
	n := 0
	for _, w := range s.words {
		n += popcount(w)
	}
	return n
}

func (s *Set) EachSet(fn func(int)) {
	for wi, w := range s.words {
		for w != 0 {
			b := trailingZeros(w)
			fn(wi<<6 + b)
			w &= w - 1
		}
	}
}

func popcount(w uint64) int {
	n := 0
	for w != 0 {
		w &= w - 1
		n++
	}
	return n
}

func trailingZeros(w uint64) int {
	n := 0
	for w&1 == 0 {
		w >>= 1
		n++
	}
	return n
}
