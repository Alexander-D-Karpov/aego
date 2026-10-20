package saves

import "testing"

func TestNextSlot(t *testing.T) {
	cases := []struct {
		prefix string
		taken  []string
		want   string
	}{
		{"slot", nil, "slot1"},
		{"slot", []string{"slot1", "slot2"}, "slot3"},
		{"slot", []string{"slot7", "quick", "slot2"}, "slot8"},
		{"run", []string{"slot3", "run9"}, "run10"},
		{"", []string{"quick"}, "slot1"},
	}
	for _, c := range cases {
		if got := NextSlot(c.prefix, c.taken); got != c.want {
			t.Fatalf("NextSlot(%q, %v) = %q, want %q", c.prefix, c.taken, got, c.want)
		}
	}
}
