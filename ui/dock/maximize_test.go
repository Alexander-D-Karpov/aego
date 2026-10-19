package dock

import "testing"

func TestMaximizeKeepsTree(t *testing.T) {
	tr := Columns([]string{"a"}, []string{"b"}, []string{"c", "d"}, []string{"e"}, []string{"f"})
	before := string(tr.Save())
	if !tr.Maximize("d") || tr.Maximized() != "d" {
		t.Fatal("maximize must take a docked panel")
	}
	if tr.Maximize("missing") {
		t.Fatal("maximize must refuse an unknown panel")
	}
	if string(tr.Save()) != before {
		t.Fatal("maximize must not change the saved layout")
	}
	tr.Restore()
	if tr.Maximized() != "" || string(tr.Save()) != before {
		t.Fatal("restore must bring back the same tree")
	}
	if !tr.ToggleMaximize("e") || tr.ToggleMaximize("e") {
		t.Fatal("toggle must maximize and then restore")
	}
	tr.ToggleMaximize("e")
	tr.Undock("e")
	if tr.Maximized() != "" {
		t.Fatal("closing the maximized panel must restore the layout")
	}
}
