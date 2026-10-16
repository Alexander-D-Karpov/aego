package problems

import "testing"

func TestCollectDedupesAndCaches(t *testing.T) {
	s := New()
	rev := uint64(1)
	calls := 0
	s.Add(Source{
		Revision: func() uint64 { return rev },
		Append: func(dst []Diagnostic) []Diagnostic {
			calls++
			d := Diagnostic{Owner: "x", Code: "X001", Severity: Error, BlocksPlay: true}
			return append(dst, d, d)
		},
	})
	list, _ := s.Collect()
	if len(list) != 1 {
		t.Fatalf("want 1 after dedupe, got %d", len(list))
	}
	s.Collect()
	if calls != 1 {
		t.Fatalf("unchanged revision rebuilt the list %d times", calls)
	}
	rev = 2
	s.Collect()
	if calls != 2 {
		t.Fatal("new revision did not rebuild the list")
	}
	if _, n := s.Blocking(); n != 1 {
		t.Fatalf("want 1 blocking problem, got %d", n)
	}
}

func TestRemoveSource(t *testing.T) {
	s := New()
	drop := s.Add(Source{Revision: func() uint64 { return 1 }, Append: func(dst []Diagnostic) []Diagnostic {
		return append(dst, Diagnostic{Owner: "x", Code: "X001"})
	}})
	if list, _ := s.Collect(); len(list) != 1 {
		t.Fatal("source not collected")
	}
	drop()
	if list, _ := s.Collect(); len(list) != 0 {
		t.Fatal("removed source still reports")
	}
}
