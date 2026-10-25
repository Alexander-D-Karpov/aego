package integration

import (
	"testing"

	"aego/editor/core/workspace"
	edocs "aego/editor/panels/docs"
	"aego/editor/panels/scripteditor"
	"aego/ui/dock"
)

// Programming is code first: Assets on the left, scripts and documentation in the middle, Find on the right, Problems and Console below, no play toolbar.
func TestProgrammingLayout(t *testing.T) {
	s := workspaceSession(t)
	ws := s.App.Workspace()
	ws.Switch(workspace.Programming)
	s.Step()
	s.App.Context().OpenScript("scripts/player/player.aes", 1)
	for range 3 {
		s.Step()
	}
	if ws.Toolbar() {
		t.Fatal("Programming must hide the play toolbar")
	}
	for _, id := range []string{workspace.Scene, workspace.Inspector, workspace.Hierarchy, workspace.Game} {
		if ws.Tree.Has(id) {
			t.Fatalf("Programming shows %s", id)
		}
	}
	at := func(id string) dock.NodeID {
		n := ws.Tree.FindPanel(id)
		if n == 0 {
			t.Fatalf("Programming has no %s", id)
		}
		return n
	}
	code := at(scripteditor.PanelID("scripts/player/player.aes"))
	if code != at(edocs.ID) {
		t.Fatal("the script must open next to the documentation in the middle")
	}
	rect := func(id string) float32 {
		r, _ := ws.Tree.PanelRect(id)
		return r.X
	}
	if !(rect(workspace.Assets) < rect(scripteditor.PanelID("scripts/player/player.aes")) && rect(scripteditor.PanelID("scripts/player/player.aes")) < rect(scripteditor.FindID)) {
		t.Fatal("Assets, code and Find must go from left to right")
	}
	at(workspace.Problems)
	at(workspace.Console)
}
