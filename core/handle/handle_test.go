package handle

import "testing"

func TestZeroHandleInvalid(t *testing.T) {
	a := NewArena[int](4)
	if _, ok := a.Get(Handle[int]{}); ok {
		t.Fatal("zero handle resolved")
	}
	if !(Handle[int]{}).IsZero() {
		t.Fatal("zero handle not reported as zero")
	}
}

func TestGenerationInvalidation(t *testing.T) {
	a := NewArena[int](4)
	h1, v := a.Alloc()
	*v = 7
	if !a.Free(h1) {
		t.Fatal("free failed")
	}
	if _, ok := a.Get(h1); ok {
		t.Fatal("stale handle resolved")
	}
	if a.Free(h1) {
		t.Fatal("double free succeeded")
	}
	h2, _ := a.Alloc()
	if h2.Index != h1.Index {
		t.Fatalf("slot not reused: %d vs %d", h2.Index, h1.Index)
	}
	if h2.Gen == h1.Gen {
		t.Fatal("generation not bumped")
	}
	got, ok := a.Get(h2)
	if !ok || *got != 0 {
		t.Fatalf("reused slot not zeroed: ok=%v value=%v", ok, got)
	}
	if _, ok := a.Get(h1); ok {
		t.Fatal("old handle resolves to reused slot")
	}
}

func TestGrowEachClear(t *testing.T) {
	a := NewArena[int](1)
	grows := 0
	a.OnGrow = func(_, _ int) { grows++ }
	var hs []Handle[int]
	for i := 0; i < 10; i++ {
		h, v := a.Alloc()
		*v = i
		hs = append(hs, h)
	}
	if grows == 0 {
		t.Fatal("grow callback not called")
	}
	a.Free(hs[3])
	sum, n := 0, 0
	a.Each(func(_ Handle[int], v *int) {
		sum += *v
		n++
	})
	if n != 9 || sum != 42 {
		t.Fatalf("each: n=%d sum=%d", n, sum)
	}
	if a.Len() != 9 {
		t.Fatalf("len=%d", a.Len())
	}
	a.Clear()
	if a.Len() != 0 {
		t.Fatal("clear left items")
	}
	for _, h := range hs {
		if a.Valid(h) {
			t.Fatal("handle valid after clear")
		}
	}
	h, _ := a.Alloc()
	if h.Index != 0 {
		t.Fatalf("clear should reuse from index 0, got %d", h.Index)
	}
}
