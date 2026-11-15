//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aego/core/log"
	"aego/platform/input"
	"aego/platform/sdl"
)

// The menu is opened with SDL pointer events and the file action and prompt
// are operated with SDL keyboard events, with no scene open.
func TestCreateMenuFilePromptKeepsNativeKeyboardFocus(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("AEGO_LANG", "en")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	p := newProject(t)
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Step() {
		t.Fatal("editor stopped during startup")
	}
	ed := s.App.Context()
	for len(ed.Docs.Docs()) > 0 {
		if err := ed.Docs.Close(ed.Docs.Docs()[0].ID, true); err != nil {
			t.Fatal(err)
		}
	}
	s.App.AssetsPanel().SetFolder("assets")
	if !s.Step() || ed.Doc() != nil {
		t.Fatal("expected an open project without a scene")
	}
	ui := ed.UI
	width := func(label string) float32 { return ui.TextSize(label).X + 2*ui.Theme.Padding + 4 }
	x, y := 4+width("File")+width("Edit")+width("Create")/2, ui.Theme.BarHeight/2
	for i := range 3 {
		if !sdl.TestPointerEvent(x, y, i) || !s.Step() {
			t.Fatal("could not open Create from the native menu")
		}
	}
	pal := s.App.AddPalette().Palette()
	if !s.App.AddPalette().Active() || !ui.PopupOpen(ui.ID("menu/Create")) {
		t.Fatal("Create menu did not open without a scene")
	}
	if !sdl.TestTextInput("animation clip") || !s.Step() {
		t.Fatal("could not search the Create menu")
	}
	if i := pal.Selected(); i < 0 || pal.Item(i).Key != "act:file.animation" {
		t.Fatalf("animation clip action was not selected: %d", i)
	}
	enter := func() {
		t.Helper()
		for _, down := range []bool{true, false} {
			if !sdl.TestEditKey(input.KeyEnter, down, false, false) || !s.Step() {
				t.Fatal("could not send Enter to the native editor")
			}
		}
	}
	enter()
	if !ui.ModalActive() || ui.Focus() == 0 {
		t.Fatal("new-file prompt did not take keyboard focus")
	}
	if _, err := os.Stat(filepath.Join(p.Root, "assets", "untitled.aeanim")); !os.IsNotExist(err) {
		t.Fatalf("the selection Enter submitted the default file: %v", err)
	}
	if !sdl.TestTextInput("native-no-scene") || !s.Step() {
		t.Fatal("could not enter the file name")
	}
	enter()
	id, ok := s.Pipeline.Lookup("assets/native-no-scene.aeanim")
	if !ok || s.Pipeline.VerifyCurrent(id) != nil || ed.Doc() != nil {
		notice, _ := ed.LastNotice()
		t.Fatalf("native file creation failed or opened a scene: %+v", notice)
	}
}
