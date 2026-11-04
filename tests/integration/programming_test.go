package integration

import (
	"testing"

	"aego/app/agentui/backend"
	"aego/editor/core/workspace"
	"aego/editor/panels/agent"
	edocs "aego/editor/panels/docs"
	"aego/editor/panels/scripteditor"
	"aego/editor/panels/watch"
	"aego/ui/dock"
)

func TestProgrammingLayout(t *testing.T) {
	s, _ := agentSession(t, &backend.Fake{})
	ws := s.App.Workspace()
	ws.Switch(workspace.Programming)
	s.Step()
	const script = "scripts/player/player.aes"
	s.App.Context().OpenScript(script, 1)
	for range 3 {
		s.Step()
	}
	if !ws.Toolbar() {
		t.Fatal("Programming must show play controls by default")
	}
	for _, id := range []string{workspace.Scene, workspace.Inspector, workspace.Hierarchy} {
		if ws.Tree.Has(id) {
			t.Fatalf("Programming shows %s", id)
		}
	}
	at := func(id string) *dock.Node {
		n, ok := ws.Tree.Node(ws.Tree.FindPanel(id))
		if !ok {
			t.Fatalf("Programming has no %s", id)
		}
		return n
	}
	code := at(scripteditor.PanelID(script))
	if active, ok := code.ActivePanel(); !ok || active != scripteditor.PanelID(script) || code != at(edocs.ID) || code != at(workspace.Game) {
		t.Fatal("the open script must be active in the central Docs and Game tab group")
	}
	tools := at(workspace.Problems)
	if tools != at(workspace.Console) || tools != at(scripteditor.FindID) {
		t.Fatal("Problems, Console and Find must share the bottom group")
	}
	right := at(agent.ID)
	if right != at(watch.ID) || !(at(workspace.Assets).Rect.X < code.Rect.X && code.Rect.X < right.Rect.X && code.Rect.Y < tools.Rect.Y) {
		t.Fatal("expected Assets left, code in the center, tools below, Agent and Watch on the right")
	}

	ws.SetToolbar(false)
	ws.Toggle(watch.ID)
	if err := ws.SaveLayout(); err != nil {
		t.Fatal(err)
	}
	ws.Switch(workspace.Design)
	ws.LoadLayout()
	ws.Switch(workspace.Programming)
	if ws.Toolbar() || ws.Tree.Has(watch.ID) || !ws.Tree.Has(scripteditor.PanelID(script)) {
		t.Fatal("saved Programming toolbar, panel choice or open script was lost")
	}
}
