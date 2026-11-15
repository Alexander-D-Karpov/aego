//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aego/art"
	"aego/core/log"
	"aego/editor/core/artedit"
	"aego/platform/input"
	"aego/platform/sdl"
	"aego/project"
)

// This is an SDL/OpenGL editor workflow. It types into the real import prompt,
// opens the resulting indexed asset in Art Studio, paints with an SDL pointer,
// and undoes the stroke. It does not substitute a headless import API for UI.
func TestAsepriteImportNativeEditor(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	data, err := os.ReadFile(filepath.Join("..", "..", "art", "testdata", "aseprite", "upstream", "tags3.aseprite"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "real.aseprite")
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := project.Create(t.TempDir(), "Aseprite native import", false)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	capture := func(name string) {
		t.Helper()
		dir := os.Getenv("AEGO_ASE_IMPORT_SHOTS")
		if dir == "" {
			return
		}
		var shot *image.RGBA
		var shotErr error
		s.Engine.CaptureFrame(func(img *image.RGBA, err error) { shot, shotErr = img, err })
		if !s.Step() || shotErr != nil || shot == nil {
			t.Fatalf("capture native import: %v", shotErr)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, shot)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
	}
	for range 3 {
		if !s.Step() {
			t.Fatal("editor stopped before import")
		}
	}
	for i := range 3 {
		if !sdl.TestPointerEvent(272, 15, i) || !s.Step() {
			t.Fatal("could not open the native Assets menu")
		}
	}
	capture("native-assets-menu")
	for i := range 3 {
		if !sdl.TestPointerEvent(330, 314, i) || !s.Step() {
			t.Fatal("could not select Import Aseprite in the native menu")
		}
	}
	if !s.Step() {
		t.Fatal("editor stopped before the import prompt")
	}
	capture("native-import-prompt")
	for remaining := source; len(remaining) > 0; {
		n := min(20, len(remaining))
		if !sdl.TestTextInput(remaining[:n]) || !s.Step() {
			t.Fatal("native input could not enter the source path")
		}
		remaining = remaining[n:]
	}
	if !sdl.TestEditKey(input.KeyEnter, true, false, false) || !s.Step() ||
		!sdl.TestEditKey(input.KeyEnter, false, false, false) || !s.Step() {
		t.Fatal("native input could not submit the import prompt")
	}
	for range 4 {
		if !s.Step() {
			t.Fatal("editor stopped after import")
		}
	}
	id, ok := s.Pipeline.Lookup("assets/imported.aeart")
	if !ok {
		notice, _ := s.App.Context().LastNotice()
		t.Fatalf("native import did not create the indexed source: %+v", notice)
	}
	doc := s.App.Context().Docs.AssetDocument(id)
	if doc == nil {
		t.Fatal("native import did not open the document")
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok || len(model.Session.Document.Info().Frames) != 12 || len(model.Session.Document.Info().Tags) != 3 {
		t.Fatal("native editor did not open the real animation in Art Studio")
	}
	panel := "render2d.art/" + id.String()
	ws := s.App.Workspace()
	ws.Focus(panel)
	if ws.Tree.Maximized() != panel && !ws.ToggleMaximize(panel) {
		t.Fatal("could not maximize imported Art Studio")
	}
	for range 3 {
		s.Step()
	}
	frame := model.Session.Frame
	before, _, err := model.Capture(frame, art.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if !sdl.TestPointerEvent(500, 400, i) || !s.Step() {
			t.Fatal("native painting gesture failed")
		}
	}
	painted, _, err := model.Capture(frame, art.Albedo)
	if err != nil || bytes.Equal(before.Pix, painted.Pix) || model.History.Cursor() != 1 {
		t.Fatalf("native painting did not edit the imported source once: %v", err)
	}
	capture("native-aseprite-import")
	if err := model.SessionUndo(); err != nil {
		t.Fatal(err)
	}
	after, _, err := model.Capture(frame, art.Albedo)
	if err != nil || !bytes.Equal(before.Pix, after.Pix) {
		t.Fatalf("undo did not restore the imported pixels: %v", err)
	}
	if got, err := os.ReadFile(source); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("native import changed the original file: %v", err)
	}
}

func TestAsepriteImportNativeWarningsNeedConsent(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	data, err := os.ReadFile(filepath.Join("..", "..", "art", "testdata", "aseprite", "upstream", "tags3x123reps.aseprite"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "repeat.aseprite")
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := project.Create(t.TempDir(), "Aseprite diagnostics", false)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for range 3 {
		s.Step()
	}
	key := func(code input.Scancode) {
		t.Helper()
		if !sdl.TestEditKey(code, true, false, false) || !s.Step() || !sdl.TestEditKey(code, false, false, false) || !s.Step() {
			t.Fatal("native keyboard event failed")
		}
	}
	start := func() {
		t.Helper()
		for _, pos := range [][2]float32{{272, 15}, {330, 314}} {
			for i := range 3 {
				if !sdl.TestPointerEvent(pos[0], pos[1], i) || !s.Step() {
					t.Fatal("native Assets menu interaction failed")
				}
			}
		}
		for remaining := source; len(remaining) > 0; {
			n := min(20, len(remaining))
			if !sdl.TestTextInput(remaining[:n]) || !s.Step() {
				t.Fatal("native input could not enter the source path")
			}
			remaining = remaining[n:]
		}
		key(input.KeyEnter)
		for range 3 {
			s.Step()
		}
		if !s.App.Context().UI.ModalActive() {
			t.Fatal("metadata loss did not produce a native confirmation")
		}
		if _, ok := s.Pipeline.Lookup("assets/imported.aeart"); ok {
			t.Fatal("source was published before warning confirmation")
		}
	}
	start()
	if dir := os.Getenv("AEGO_ASE_IMPORT_SHOTS"); dir != "" {
		var shot *image.RGBA
		var shotErr error
		s.Engine.CaptureFrame(func(img *image.RGBA, err error) { shot, shotErr = img, err })
		if !s.Step() || shotErr != nil || shot == nil {
			t.Fatalf("capture native import warning: %v", shotErr)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(filepath.Join(dir, "native-import-warnings.png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(file, shot)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
	}
	key(input.KeyEscape)
	if _, ok := s.Pipeline.Lookup("assets/imported.aeart"); ok {
		t.Fatal("canceling the warning still created a source")
	}
	start()
	key(input.KeyEnter)
	for range 3 {
		s.Step()
	}
	id, ok := s.Pipeline.Lookup("assets/imported.aeart")
	if !ok || s.App.Context().Docs.AssetDocument(id) == nil {
		notice, _ := s.App.Context().LastNotice()
		t.Fatalf("native confirmation did not import the source: %+v", notice)
	}
	if got, err := os.ReadFile(source); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("native warning flow changed the original file: %v", err)
	}
}
