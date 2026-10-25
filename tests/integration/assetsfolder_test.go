package integration

import (
	"testing"

	"aego/editor/core/workspace"
)

// Programming opens Assets on scripts; every workspace reopens Assets where it was left.
func TestAssetsFolderPerWorkspace(t *testing.T) {
	s := workspaceSession(t)
	ws, as := s.App.Workspace(), s.App.AssetsPanel()
	ws.Switch(workspace.Programming)
	s.Step()
	if got := as.Folder(); got != "scripts" {
		t.Fatalf("Programming opened %q", got)
	}
	as.SetFolder("scripts/player")
	ws.Switch(workspace.Design)
	s.Step()
	if got := as.Folder(); got != "assets" {
		t.Fatalf("Design opened %q", got)
	}
	ws.Switch(workspace.Programming)
	s.Step()
	if got := as.Folder(); got != "scripts/player" {
		t.Fatalf("Programming came back to %q", got)
	}
}
