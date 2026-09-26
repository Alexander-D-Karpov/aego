package dock

import (
	"testing"

	"aego/core/mathx"
)

func standard() *Tree {
	return Standard(
		[]string{"hierarchy"},
		[]string{"scene", "game"},
		[]string{"assets", "console", "profiler"},
		[]string{"inspector"},
	)
}

func TestStandardLayoutRects(t *testing.T) {
	tr := standard()
	tr.Layout(mathx.Rect{W: 1280, H: 800})
	names := []string{"hierarchy", "scene", "assets", "inspector"}
	rects := map[string]mathx.Rect{}
	for _, n := range names {
		r, ok := tr.PanelRect(n)
		if !ok || r.W <= 0 || r.H <= 0 {
			t.Fatalf("panel %s has no rect: %+v", n, r)
		}
		rects[n] = r
	}
	if rects["hierarchy"].X >= rects["scene"].X || rects["scene"].X >= rects["inspector"].X {
		t.Fatalf("columns out of order: %+v", rects)
	}
	if rects["assets"].Y <= rects["scene"].Y {
		t.Fatal("bottom panel must be below the scene")
	}
	if _, ok := tr.PanelRect("game"); ok {
		t.Fatal("an inactive tab must not report a rect")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	tr := standard()
	tr.Activate("console")
	data := tr.Save()
	back, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if string(back.Save()) != string(data) {
		t.Fatalf("round trip differs:\n%s\n---\n%s", data, back.Save())
	}
	if len(back.Panels()) != 7 {
		t.Fatalf("panels = %v", back.Panels())
	}
}

func TestDockAllSides(t *testing.T) {
	for _, side := range []Side{SideLeft, SideRight, SideTop, SideBottom, SideCenter} {
		tr := standard()
		target := tr.FindPanel("scene")
		tr.DockPanel("hierarchy", target, side)
		if !tr.Has("hierarchy") {
			t.Fatalf("side %d: panel lost", side)
		}
		tr.Layout(mathx.Rect{W: 1000, H: 800})
		hr, _ := tr.PanelRect("hierarchy")
		sr, _ := tr.PanelRect("scene")
		switch side {
		case SideLeft:
			if hr.X >= sr.X {
				t.Fatal("left dock not on the left")
			}
		case SideRight:
			if hr.X <= sr.X {
				t.Fatal("right dock not on the right")
			}
		case SideTop:
			if hr.Y >= sr.Y {
				t.Fatal("top dock not above")
			}
		case SideBottom:
			if hr.Y <= sr.Y {
				t.Fatal("bottom dock not below")
			}
		case SideCenter:
			if tr.FindPanel("hierarchy") != tr.FindPanel("scene") {
				t.Fatal("center dock must join the tab group")
			}
		}
	}
}

func TestUndockLastTabCollapses(t *testing.T) {
	tr := standard()
	before := len(tr.nodes)
	tr.Undock("inspector")
	if tr.Has("inspector") {
		t.Fatal("panel still docked")
	}
	if len(tr.nodes) != before-2 {
		t.Fatalf("nodes %d -> %d, the tabs node and its split must go", before, len(tr.nodes))
	}
	for _, n := range tr.nodes {
		if n.Kind == KindSplit {
			if tr.nodes[n.First] == nil || tr.nodes[n.Second] == nil {
				t.Fatal("dangling child after collapse")
			}
		}
	}
}

func TestUndockEverythingKeepsRoot(t *testing.T) {
	tr := standard()
	for _, p := range tr.Panels() {
		tr.Undock(p)
	}
	root, ok := tr.Node(tr.Root())
	if !ok || root.Kind != KindTabs || len(root.Tabs) != 0 {
		t.Fatal("empty tree must keep an empty root tab group")
	}
	tr.EnsurePanel("scene")
	if !tr.Has("scene") {
		t.Fatal("ensure failed on an empty tree")
	}
}

func TestLoadRejectsDuplicates(t *testing.T) {
	bad := []byte("dock version=1 {\n  split axis=x ratio=0.5 {\n    tabs active=0 {\n      panels=[\"a\"]\n    }\n    tabs active=0 {\n      panels=[\"a\"]\n    }\n  }\n}\n")
	if _, err := Load(bad); err == nil {
		t.Fatal("duplicate panel accepted")
	}
}
