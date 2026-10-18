package play

import (
	"testing"

	"aego/platform/input"
)

func recorded(n int) *recording {
	h := newHistory([]byte("scene"), "test", 0)
	var in input.State
	for range n {
		h.add(1.0/60, 1, false, stepNone, &in)
	}
	return h
}

func TestEditAtCursorSurvivesResume(t *testing.T) {
	h := recorded(600)
	h.cutAt(200, "edit")
	h.truncate(200)
	if h.usable() != 200 || h.why != "edit" {
		t.Fatalf("usable %d, why %q; want 200 and the edit", h.usable(), h.why)
	}
	var in input.State
	if h.add(1.0/60, 1, false, stepNone, &in) {
		t.Fatal("a frame after the edit was recorded")
	}
}
