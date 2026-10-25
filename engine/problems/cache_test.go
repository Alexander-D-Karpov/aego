package problems

import "testing"

// Collect asks a source again only when its own revision changed (D359).
func TestCollectReusesUnchangedSources(t *testing.T) {
	s := New()
	var still, moving int
	rev := uint64(1)
	s.Add(Source{
		Revision: func() uint64 { return 7 },
		Append: func(dst []Diagnostic) []Diagnostic {
			still++
			return append(dst, Diagnostic{Owner: "still", Code: "A"})
		},
	})
	s.Add(Source{
		Revision: func() uint64 { return rev },
		Append: func(dst []Diagnostic) []Diagnostic {
			moving++
			return append(dst, Diagnostic{Owner: "moving", Code: "B"})
		},
	})
	for i := range 5 {
		rev = uint64(i + 2)
		if list, _ := s.Collect(); len(list) != 2 {
			t.Fatalf("collect %d: %d diagnostics, want 2", i, len(list))
		}
	}
	if still != 1 || moving != 5 {
		t.Fatalf("the unchanged source was asked %d times and the changing one %d, want 1 and 5", still, moving)
	}
}
