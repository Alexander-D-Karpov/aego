package source

// Interner deduplicates identifier and keyword strings so that equal names
// across a whole project share one backing array. Resolvers can then compare
// names by pointer-backed string identity instead of re-hashing each file's
// copies.
//
// An Interner is not safe for concurrent use: lex a project's files on a single
// goroutine, or give each goroutine its own Interner.
type Interner struct {
	m map[string]string
}

func NewInterner() *Interner {
	return &Interner{m: map[string]string{}}
}

func (i *Interner) Intern(b []byte) string {
	if s, ok := i.m[string(b)]; ok {
		return s
	}
	s := string(b)
	i.m[s] = s
	return s
}
