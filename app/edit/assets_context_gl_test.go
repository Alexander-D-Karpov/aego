//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aego/core/log"
	"aego/editor/core/workspace"
	"aego/platform/sdl"
)

// SDL events exercise the actual docked Assets grid, not an OS mouse device.
func TestAssetsEmptyGridContextMenuSDL(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("AEGO_LANG", "en")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	p := newProject(t)
	if err := os.MkdirAll(filepath.Join(p.Root, "assets", "only-folder"), 0755); err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Step() {
		t.Fatal("editor stopped during startup")
	}
	s.App.Workspace().Show(workspace.Assets)
	s.App.AssetsPanel().SetFolder("assets")
	for range 3 {
		if !s.Step() {
			t.Fatal("editor stopped before the Assets grid opened")
		}
	}
	r, ok := s.App.Workspace().Tree.PanelRect(workspace.Assets)
	if !ok || r.W < 260 {
		t.Fatalf("Assets grid is not visible or wide enough: %+v", r)
	}
	ui := s.App.Context().UI
	gridX := r.X + (r.W-ui.Theme.SplitterW)*0.28 + ui.Theme.SplitterW
	// The first row contains one 96px-wide folder tile. Click the empty cell next to it.
	x := gridX + 150
	y := r.Y + ui.Theme.Padding + ui.Theme.RowHeight + ui.Theme.Spacing + 40
	if ui.AnyPopup() {
		t.Fatal("unexpected popup before the right click")
	}
	for i := range 3 {
		if !sdl.TestRightPointerEvent(x, y, i) || !s.Step() {
			t.Fatal("could not send the SDL right click")
		}
	}
	if !ui.AnyPopup() {
		t.Fatalf("empty grid cell at %.0f,%.0f did not open a context menu (panel %+v)", x, y, r)
	}
}
