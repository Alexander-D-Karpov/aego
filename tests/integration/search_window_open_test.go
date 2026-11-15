package integration

import (
	"testing"

	"aego/app/edit"
	"aego/editor/core/command"
	"aego/editor/core/favorites"
	"aego/editor/core/workspace"
	"aego/platform/input"
	"aego/ui/dock"
)

func searchWindow(t *testing.T, s *edit.Session, id string) {
	t.Helper()
	d, ok := s.App.Context().Commands.Get(command.WindowPrefix + id)
	if !ok {
		t.Fatalf("window command for %q is missing", id)
	}
	push(t, s, key(input.KeyLShift, input.ModShift)...)
	push(t, s, key(input.KeyLShift, input.ModShift)...)
	push(t, s, typed(d.Title())...)
	push(t, s, key(input.KeyEnter, 0)...)
}

func TestSearchFavoriteOpensWindow(t *testing.T) {
	s := workspaceSession(t)
	id := workspace.Console
	cmd := command.WindowPrefix + id
	d, ok := s.App.Context().Commands.Get(cmd)
	if !ok {
		t.Fatal("console command is missing")
	}
	push(t, s, key(input.KeyLShift, input.ModShift)...)
	push(t, s, key(input.KeyLShift, input.ModShift)...)
	push(t, s, typed(d.Title())...)
	push(t, s, key(input.KeyD, input.PrimaryMod())...)
	push(t, s, key(input.KeyEscape, 0)...)
	if !favorites.Load(s.Host.Project.Root).Has(favorites.Item{Kind: favorites.Command, Command: cmd}) {
		t.Fatal("console was not added to favorites")
	}
	ws := s.App.Workspace()
	ws.Show(id)
	searchWindow(t, s, id)
	requireActiveWindow(t, ws, id)
}

func requireActiveWindow(t *testing.T, ws *workspace.Workspace, id string) {
	t.Helper()
	n, ok := ws.Tree.Node(ws.Tree.FindPanel(id))
	if !ok {
		t.Fatalf("window %q is not open", id)
	}
	if active, ok := n.ActivePanel(); !ok || active != id {
		t.Fatalf("active window = %q, want %q", active, id)
	}
}

func TestSearchOpensWindowsInsteadOfToggling(t *testing.T) {
	for _, id := range []string{workspace.Console, workspace.Profiler} {
		t.Run(id, func(t *testing.T) {
			s := workspaceSession(t)
			ws := s.App.Workspace()
			peer := workspace.Console
			if id == peer {
				peer = workspace.Profiler
			}

			// An open but inactive tab must be activated, not removed from the layout.
			ws.Show(peer)
			ws.Show(id)
			ws.Tree.DockPanel(id, ws.Tree.FindPanel(peer), dock.SideCenter)
			ws.Show(peer)
			searchWindow(t, s, id)
			requireActiveWindow(t, ws, id)

			// Searching for the already active window must leave it open as well.
			searchWindow(t, s, id)
			requireActiveWindow(t, ws, id)

			// The Window menu command remains a toggle.
			if err := s.App.Context().Commands.Run(command.WindowPrefix + id); err != nil {
				t.Fatal(err)
			}
			if ws.Tree.Has(id) {
				t.Fatalf("window menu command did not close %q", id)
			}
			searchWindow(t, s, id)
			requireActiveWindow(t, ws, id)
		})
	}
}
