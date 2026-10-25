package integration

import (
	"testing"

	"aego/editor/core/workspace"
	"aego/editor/panels/scripteditor"
)

// A script opens in the biggest area, not next to Assets, also when the layout has no Scene View.
func TestScriptOpensInTheMainArea(t *testing.T) {
	s := workspaceSession(t)
	ws := s.App.Workspace()
	ws.Switch(workspace.Programming)
	ws.Tree.Undock(workspace.Scene)
	ws.Show(workspace.Assets)
	for range 3 {
		s.Step()
	}
	ctx := s.App.Context()
	for _, rel := range []string{"scripts/shared/shared.aes", "scripts/player/player.aes"} {
		ctx.OpenScript(rel, 1)
		for range 3 {
			s.Step()
		}
		at := ws.Tree.FindPanel(scripteditor.PanelID(rel))
		if at == 0 {
			t.Fatalf("%s did not open", rel)
		}
		if at == ws.Tree.FindPanel(workspace.Assets) {
			t.Fatalf("%s opened next to Assets", rel)
		}
	}
}
