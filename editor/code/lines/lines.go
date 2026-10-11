package lines

import (
	"slices"
	"sort"
)

type Index struct {
	starts []int
	length int
}

func New(src []byte) *Index {
	i := &Index{starts: []int{0}, length: len(src)}
	for k, c := range src {
		if c == '\n' {
			i.starts = append(i.starts, k+1)
		}
	}
	return i
}

func (i *Index) Count() int {
	return len(i.starts)
}

func (i *Index) Len() int {
	return i.length
}

func (i *Index) clamp(line int) int {
	return min(max(line, 0), len(i.starts)-1)
}

func (i *Index) Offset(line int) int {
	return i.starts[i.clamp(line)]
}

func (i *Index) End(line int) int {
	line = i.clamp(line)
	if line+1 < len(i.starts) {
		return i.starts[line+1] - 1
	}
	return i.length
}

func (i *Index) Line(off int) int {
	off = max(off, 0)
	return sort.Search(len(i.starts), func(k int) bool { return i.starts[k] > off }) - 1
}

func (i *Index) Edit(off, removed int, text []byte) {
	lo := sort.Search(len(i.starts), func(k int) bool { return i.starts[k] > off })
	hi := sort.Search(len(i.starts), func(k int) bool { return i.starts[k] > off+removed })
	delta := len(text) - removed
	for k := hi; k < len(i.starts); k++ {
		i.starts[k] += delta
	}
	var add []int
	for k, c := range text {
		if c == '\n' {
			add = append(add, off+k+1)
		}
	}
	i.starts = slices.Replace(i.starts, lo, hi, add...)
	i.length += delta
}
