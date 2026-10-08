package dock

import (
	"testing"

	"aego/core/mathx"
)

// Dropping on the tab strip, or anywhere away from an edge, must add a tab
// (SideCenter). Only the outer quarter — capped at 80px — splits the panel.
func TestDropTargetZones(t *testing.T) {
	tr := New()
	tr.Layout(mathx.Rect{W: 1000, H: 800})
	// tabH defaults to 24; content body is {0, 24, 1000, 776}; edge band is min(size*0.25, 80) = 80.
	cases := []struct {
		name string
		at   mathx.Vec2
		want Side
	}{
		{"tab strip", mathx.Vec2{X: 500, Y: 10}, SideCenter},
		{"body center", mathx.Vec2{X: 500, Y: 400}, SideCenter},
		{"past the left band", mathx.Vec2{X: 100, Y: 400}, SideCenter},
		{"left edge", mathx.Vec2{X: 20, Y: 400}, SideLeft},
		{"right edge", mathx.Vec2{X: 980, Y: 400}, SideRight},
		{"top edge of body", mathx.Vec2{X: 500, Y: 30}, SideTop},
		{"bottom edge", mathx.Vec2{X: 500, Y: 790}, SideBottom},
	}
	for _, c := range cases {
		_, side, ok := tr.dropTarget(c.at)
		if !ok {
			t.Fatalf("%s: no drop target", c.name)
		}
		if side != c.want {
			t.Fatalf("%s: side = %v, want %v", c.name, side, c.want)
		}
	}
}
