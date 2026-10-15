package source

// Interner deduplicates identifier strings across a project; not safe for concurrent use.
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
