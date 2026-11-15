package dock

import (
	"math"
	"strings"
	"testing"

	"aego/core/mathx"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/theme"
)

func TestInitialSplitSizeIsOneTimeAndSurvivesAnEarlySave(t *testing.T) {
	for _, tc := range []struct {
		name  string
		axis  Axis
		first bool
		area  mathx.Rect
		want  float32
	}{
		{"right", AxisX, false, mathx.Rect{W: 1004, H: 400}, 544},
		{"left", AxisX, true, mathx.Rect{W: 1004, H: 400}, 544},
		{"bottom", AxisY, false, mathx.Rect{W: 800, H: 1004}, 544},
		{"narrow", AxisX, false, mathx.Rect{W: 404, H: 400}, 260},
		{"sibling-minimum", AxisX, false, mathx.Rect{W: 174, H: 400}, 90},
		{"wide-save-limit", AxisX, false, mathx.Rect{W: 20004, H: 400}, 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := newTree()
			a, b := tr.newTabs([]string{"a"}), tr.newTabs([]string{"b"})
			tr.root = tr.newSplit(tc.axis, .5, a, b)
			child := b
			if tc.first {
				child = a
			}
			if !tr.SetInitialSplitSize(tr.root, child, 544, .65) {
				t.Fatal("valid initial size rejected")
			}
			tr.Layout(mathx.Rect{W: 0, H: 0})
			data := tr.Save()
			if !strings.Contains(string(data), "initial_size") {
				t.Fatal("early save lost the unresolved factory hint")
			}
			back, err := Load(data)
			if err != nil {
				t.Fatal(err)
			}
			back.Layout(tc.area)
			which := "b"
			if tc.first {
				which = "a"
			}
			n, _ := back.Node(back.FindPanel(which))
			got := n.Rect.W
			if tc.axis == AxisY {
				got = n.Rect.H
			}
			if math.Abs(float64(got-tc.want)) > .01 {
				t.Fatalf("initial size = %v, want %v", got, tc.want)
			}
			if strings.Contains(string(back.Save()), "initial_size") {
				t.Fatal("resolved factory size was not reduced to a normal ratio")
			}
			ratio := back.nodes[back.root].Ratio
			restored, err := Load(back.Save())
			if err != nil || math.Abs(float64(restored.nodes[restored.root].Ratio-ratio)) > .00001 {
				t.Fatalf("resolved split did not round-trip: ratio=%v, err=%v", ratio, err)
			}
			back.Layout(mathx.Rect{W: tc.area.W * 2, H: tc.area.H * 2})
			if back.nodes[back.root].Ratio != ratio {
				t.Fatal("resizing reapplied the factory hint")
			}
		})
	}
}

func TestInitialSplitSizeRejectsInvalidHints(t *testing.T) {
	tr := Standard([]string{"a"}, []string{"b"}, nil, nil)
	root, _ := tr.Node(tr.Root())
	for _, tc := range []struct {
		split, child NodeID
		size, share  float32
	}{
		{root.ID, root.ID, 544, .65},
		{root.First, root.Second, 544, .65},
		{root.ID, root.First, 0, .65},
		{root.ID, root.First, float32(math.Inf(1)), .65},
		{root.ID, root.First, 544, .4},
		{root.ID, root.First, 544, 1},
	} {
		if tr.SetInitialSplitSize(tc.split, tc.child, tc.size, tc.share) {
			t.Fatalf("accepted invalid initial size: %+v", tc)
		}
	}
}

func TestDragOverridesUnresolvedInitialSize(t *testing.T) {
	tr := Standard([]string{"a"}, []string{"b"}, nil, nil)
	split, _ := tr.Node(tr.Root())
	if !tr.SetInitialSplitSize(split.ID, split.Second, 544, .68) {
		t.Fatal("valid initial size rejected")
	}
	font := text.NewFont(14, 1, 16, 12)
	ui, in := core.New(font), &input.State{}
	ui.Theme = theme.Editor()
	area := mathx.Rect{W: 120, H: 300} // too narrow to resolve both 80px minima
	draw := func() {
		ui.Begin(in, mathx.Vec2{X: area.W, Y: area.H}, 1)
		tr.Draw(ui, area, func(string, mathx.Rect) {})
		ui.End()
	}
	in.BeginFrame()
	draw()
	if split.initial == nil {
		t.Fatal("hint was consumed at an unusable width")
	}
	first, _ := tr.Node(split.First)
	point := mathx.Vec2{X: first.Rect.X + first.Rect.W + 2, Y: 100}
	in.BeginFrame()
	in.ApplyMouseMove(point, mathx.Vec2{})
	in.ApplyMouseDown(point, input.MouseLeft)
	draw()
	in.BeginFrame()
	in.ApplyMouseMove(point.Add(mathx.Vec2{X: 10}), mathx.Vec2{X: 10})
	draw()
	if split.initial != nil {
		t.Fatal("user drag did not cancel the pending factory size")
	}
	ratio := split.Ratio
	tr.Layout(mathx.Rect{W: 1000, H: 300})
	if split.Ratio != ratio {
		t.Fatal("factory size overwrote a user drag after the window grew")
	}
}
