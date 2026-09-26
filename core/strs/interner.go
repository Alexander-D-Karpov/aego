package strs

type StringID uint32

const Empty StringID = 0

type Interner struct {
	index map[string]StringID
	table []string
}

func NewInterner() *Interner {
	return &Interner{
		index: map[string]StringID{"": Empty},
		table: []string{""},
	}
}

func (i *Interner) Intern(s string) StringID {
	if id, ok := i.index[s]; ok {
		return id
	}
	id := StringID(len(i.table))
	i.table = append(i.table, s)
	i.index[s] = id
	return id
}

func (i *Interner) Lookup(s string) (StringID, bool) {
	id, ok := i.index[s]
	return id, ok
}

func (i *Interner) String(id StringID) string {
	if int(id) >= len(i.table) {
		return ""
	}
	return i.table[id]
}

func (i *Interner) Len() int {
	return len(i.table)
}
