//go:build aego_gltest && linux && cgo && !aego_nonative

package integration

import (
	"image"
	"image/png"
	"os"
	"runtime"
	"testing"

	"aego/app/edit"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/templates"
	"aego/editor/shell/addpalette"
	"aego/platform/sdl"
	"aego/project"
	"aego/ui/core"
)

func TestCreateMenuThroughSDL(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := project.Load(copyExample(t, "platformer"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Hidden: true, Width: 1100, Height: 820, NoWatch: true, Logger: log.Nop()}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	if _, err := s.App.Context().Docs.Open(bridgeScene); err != nil {
		t.Fatal(err)
	}
	s.Step()
	at := createMenuPoint(s)
	for i := range 3 {
		if !sdl.TestPointerEvent(at.X, at.Y, i) {
			t.Fatal("cannot enqueue SDL pointer event")
		}
		s.Step()
	}
	ui := s.App.Context().UI
	if !ui.PopupOpen(ui.ID("menu/Create")) || ui.ModalActive() || !s.App.AddPalette().Active() {
		t.Fatal("Create did not open as a non-modal menu")
	}
	if out := os.Getenv("AEGO_CREATE_MENU_SHOT"); out != "" {
		var screen *image.RGBA
		s.Engine.CaptureFrame(func(im *image.RGBA, err error) {
			if err != nil {
				t.Error(err)
			}
			screen = im
		})
		s.Step()
		if screen == nil {
			t.Fatal("no captured window")
		}
		f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, screen); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("SDL Create menu rendered with %s", s.Engine.Caps().Renderer)
}

func TestCreateEffectsIconsThroughSDL(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := project.Load(copyExample(t, "platformer"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Hidden: true, Width: 1100, Height: 820, NoWatch: true, Logger: log.Nop()}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	if _, err := s.App.Context().Docs.Open(bridgeScene); err != nil {
		t.Fatal(err)
	}
	s.Step()
	ui := s.App.Context().UI
	if got := addpalette.CategoryIcon("Effects"); got != "cat.particles" || !ui.Icons.Has(got) {
		t.Fatalf("Effects category icon is missing: %q", got)
	}
	click := createMenuPoint(s)
	for i := range 3 {
		if !sdl.TestPointerEvent(click.X, click.Y, i) {
			t.Fatal("cannot enqueue SDL pointer event")
		}
		s.Step()
	}
	pal := s.App.AddPalette().Palette()
	categories := []string{}
	want := map[string]string{"Dust": "particles.dust", "Spark": "particles.spark", "Smoke": "particles.smoke", "Rain": "particles.rain", "Explosion": "particles.explosion"}
	for _, item := range pal.Items() {
		if len(categories) == 0 || !containsCategory(categories, item.Category) {
			categories = append(categories, item.Category)
		}
		if icon, ok := want[item.Label]; ok && item.Category == "Effects" {
			if item.Icon != icon || !ui.Icons.Has(icon) {
				t.Fatalf("%s uses missing or wrong icon %q", item.Label, item.Icon)
			}
			delete(want, item.Label)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing effect templates: %v", want)
	}
	index := -1
	for i, c := range categories {
		if c == "Effects" {
			index = i
		}
	}
	if index < 0 || index >= 8 {
		t.Fatalf("Effects category not visible: %v", categories)
	}
	rowH := ui.Theme.RowHeight + 2
	at := mathx.Vec2{X: click.X, Y: ui.Theme.BarHeight + ui.Theme.Padding + ui.Theme.RowHeight + ui.Theme.Spacing + (float32(index)+0.5)*rowH}
	if !sdl.TestPointerEvent(at.X, at.Y, 0) {
		t.Fatal("cannot hover Effects")
	}
	for range 3 {
		s.Step()
	}
	if !ui.PopupOpen(core.IDSeq(core.IDOf("addpalette"), 4)) {
		t.Fatal("hovering Effects did not open its submenu")
	}
	writeCreateCapture(t, s, os.Getenv("AEGO_CREATE_SUBMENU_SHOT"))
	s.App.AddPalette().CreateNode(ids.NodeID{}, templates.Place{}, nil, nil)
	pal.Open["Effects"] = true
	pal.Refresh()
	for range 3 {
		s.Step()
	}
	if pal.Describe != nil || !ui.ModalActive() {
		t.Fatal("Create palette must have no description panel")
	}
	writeCreateCapture(t, s, os.Getenv("AEGO_CREATE_EFFECTS_SHOT"))
}

func containsCategory(categories []string, want string) bool {
	for _, c := range categories {
		if c == want {
			return true
		}
	}
	return false
}

func writeCreateCapture(t *testing.T, s *edit.Session, out string) {
	t.Helper()
	if out == "" {
		return
	}
	var screen *image.RGBA
	s.Engine.CaptureFrame(func(im *image.RGBA, err error) {
		if err != nil {
			t.Error(err)
		}
		screen = im
	})
	s.Step()
	if screen == nil {
		t.Fatal("no captured window")
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, screen)
	if closeErr := f.Close(); err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
}
