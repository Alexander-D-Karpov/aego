package main

import "testing"

func TestParsePathAbsoluteAndRelative(t *testing.T) {
	segs, err := parsePath("M2 2 L10 2 l0 8 H2 Z m1 1 h2 v2 z")
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 9 {
		t.Fatalf("segments = %d", len(segs))
	}
	if segs[2].p[0] != (pt{10, 10}) || segs[3].p[0] != (pt{2, 10}) {
		t.Fatalf("relative points wrong: %+v", segs)
	}
	if segs[5].p[0] != (pt{3, 3}) {
		t.Fatalf("relative move after close = %+v", segs[5].p[0])
	}
}

func TestRasterFillsAndPunchesHoles(t *testing.T) {
	ic, err := parseSVG([]byte(doc(rect(2, 2, 20, 20, false), rect(8, 8, 8, 8, true))))
	if err != nil {
		t.Fatal(err)
	}
	a := raster(ic, 24)
	if a.AlphaAt(4, 4).A < 250 {
		t.Fatal("outer square not filled")
	}
	if a.AlphaAt(12, 12).A > 5 {
		t.Fatal("hole not punched")
	}
}

func TestPlaceholdersParse(t *testing.T) {
	for id, svg := range placeholders() {
		if _, err := parseSVG([]byte(svg)); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	if len(placeholders()) != 39 {
		t.Fatalf("placeholder count = %d", len(placeholders()))
	}
}
