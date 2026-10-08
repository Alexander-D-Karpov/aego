package icons

import "testing"

func TestMissingIconWarnsOnce(t *testing.T) {
	calls := 0
	s := &Set{OnMissing: func(string) { calls++ }}
	for i := 0; i < 5; i++ {
		if _, ok := s.Region("tool.nope", 16, 1); ok {
			t.Fatal("empty set returned a region")
		}
	}
	if calls != 1 {
		t.Fatalf("OnMissing called %d times", calls)
	}
	var nilSet *Set
	if _, ok := nilSet.Region("x", 16, 1); ok {
		t.Fatal("nil set returned a region")
	}
}
