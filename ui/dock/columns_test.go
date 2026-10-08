package dock

import "testing"

func tabsWith(t *Tree, panel string) *Node {
	for _, n := range t.nodes {
		if n.Kind != KindTabs {
			continue
		}
		for _, p := range n.Tabs {
			if p == panel {
				return n
			}
		}
	}
	return nil
}

func TestColumnsPutsAssetsUnderHierarchy(t *testing.T) {
	tr := Columns([]string{"hierarchy"}, []string{"assets"}, []string{"scene", "game"}, []string{"console", "profiler"}, []string{"inspector"})
	h, a := tabsWith(tr, "hierarchy"), tabsWith(tr, "assets")
	if h == nil || a == nil {
		t.Fatal("panels missing")
	}
	if h.Parent != a.Parent {
		t.Fatal("hierarchy and assets must share a split")
	}
	split := tr.nodes[h.Parent]
	if split.Kind != KindSplit || split.Axis != AxisY || split.First != h.ID || split.Second != a.ID {
		t.Fatalf("hierarchy must be above assets: %+v", split)
	}
	if split.Parent != tr.root {
		t.Fatal("left column must hang off the root split")
	}
	s, c := tabsWith(tr, "scene"), tabsWith(tr, "console")
	if s == nil || c == nil || s.Parent != c.Parent || tr.nodes[s.Parent].Axis != AxisY {
		t.Fatal("scene/game must sit above console/profiler")
	}
	data := tr.Save()
	back, err := Load(data)
	if err != nil || !back.Has("assets") || !back.Has("inspector") {
		t.Fatalf("round trip failed: %v", err)
	}
}
