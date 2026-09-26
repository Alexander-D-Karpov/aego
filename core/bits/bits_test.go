package bits

import (
	"math/rand"
	"slices"
	"testing"
)

func TestSet(t *testing.T) {
	s := NewSet(130)
	s.Set(0)
	s.Set(64)
	s.Set(129)
	s.Set(200)
	if !s.Has(0) || !s.Has(64) || !s.Has(129) || s.Has(200) || s.Has(1) {
		t.Fatal("bit state wrong")
	}
	if s.Count() != 3 {
		t.Fatalf("count = %d", s.Count())
	}
	var got []int
	s.EachSet(func(i int) { got = append(got, i) })
	if !slices.Equal(got, []int{0, 64, 129}) {
		t.Fatalf("each = %v", got)
	}
	s.Clear(64)
	if s.Has(64) || s.Count() != 2 {
		t.Fatal("clear failed")
	}
	s.Reset()
	if s.Count() != 0 {
		t.Fatal("reset failed")
	}
}

func TestRadixSort(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for _, n := range []int{0, 1, 2, 17, 1000, 50000} {
		keys := make([]uint64, n)
		for i := range keys {
			keys[i] = rng.Uint64()
		}
		want := slices.Clone(keys)
		slices.Sort(want)
		got := RadixSort64(keys, make([]uint64, n))
		if !slices.Equal(got, want) {
			t.Fatalf("n=%d not sorted", n)
		}
	}
}

func TestRadixSortSkipsUniformDigits(t *testing.T) {
	keys := []uint64{5, 3, 9, 1}
	got := RadixSort64(keys, make([]uint64, 4))
	if !slices.Equal(got, []uint64{1, 3, 5, 9}) {
		t.Fatalf("got %v", got)
	}
}

func BenchmarkRadixSort50k(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	src := make([]uint64, 50000)
	for i := range src {
		src[i] = rng.Uint64()
	}
	keys := make([]uint64, len(src))
	scratch := make([]uint64, len(src))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(keys, src)
		RadixSort64(keys, scratch)
	}
}
