package scripteditor

import "testing"

func TestSettleWaits(t *testing.T) {
	var s settle
	k := mark{rev: 1, off: 5}
	if !s.changed(k, 1.0) {
		t.Fatal("first key not reported as changed")
	}
	if s.due(1.10, occDelay) {
		t.Fatal("fired before 150 ms")
	}
	if !s.due(1.16, occDelay) {
		t.Fatal("did not fire after 150 ms")
	}
	if s.due(1.30, occDelay) {
		t.Fatal("fired twice for one stop")
	}
	if s.changed(k, 1.4) {
		t.Fatal("same position restarted the timer")
	}
	s.changed(mark{rev: 1, off: 6}, 1.5)
	if s.due(1.6, occDelay) {
		t.Fatal("moving the caret did not restart the timer")
	}
	if !s.due(1.66, occDelay) {
		t.Fatal("did not fire after the caret stopped again")
	}
}

func TestIsWrite(t *testing.T) {
	cases := map[string]bool{
		"x = 1":   true,
		"x := 1":  true,
		"x += 1":  true,
		"x %= 2":  true,
		"x++":     true,
		"x--":     true,
		"x == 1":  false,
		"x <= y":  false,
		"x != y":  false,
		"x(1)":    false,
		"x.y = 1": false,
		"x":       false,
	}
	for src, want := range cases {
		if got := isWrite([]byte(src), 1); got != want {
			t.Errorf("%q: got %v, want %v", src, got, want)
		}
	}
}
