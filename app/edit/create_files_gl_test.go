//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"aego/art"
	"aego/asset/importers/artsource"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/artedit"
	"aego/editor/core/templates"
	"aego/platform/input"
	"aego/platform/sdl"
	"aego/scene"
)

// An Enter used to select Create must not also submit the following prompt.
func TestCreatePixelArtNativeEnterWaitsForConfirmation(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := newProject(t)
	const scenePath = "scenes/create-files.aescene"
	writeScene(t, p, scenePath, func(*scene.Scene) {})
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Step() {
		t.Fatal("editor stopped during startup")
	}
	if _, err := s.App.Context().Docs.Open(scenePath); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if !s.Step() {
			t.Fatal("editor stopped during startup")
		}
	}
	pal := s.App.AddPalette()
	pal.CreateNode(ids.NodeID{}, templates.Place{}, nil, nil)
	pal.Palette().SetQuery("Pixel Art")
	if i := pal.Palette().Selected(); i < 0 || !strings.Contains(pal.Palette().Item(i).Label, "Pixel Art") {
		t.Fatal("Create cannot find the pixel art file action")
	}
	if !s.Step() {
		t.Fatal("Create palette did not draw")
	}
	key := func(down bool) {
		t.Helper()
		if !sdl.TestEditKey(input.KeyEnter, down, false, false) || !s.Step() {
			t.Fatal("could not send Enter to the native editor")
		}
	}
	key(true)
	key(false)
	if _, err := os.Stat(filepath.Join(p.Root, "assets", "sprite.aeart")); !os.IsNotExist(err) {
		t.Fatalf("selecting Create submitted the new-file prompt: %v", err)
	}
	if !sdl.TestTextInput("assets/enter-test.aeart") || !s.Step() {
		t.Fatal("could not type the new asset path")
	}
	key(true)
	key(false)
	id, ok := s.Pipeline.Lookup("assets/enter-test.aeart")
	if !ok {
		notice, _ := s.App.Context().LastNotice()
		t.Fatalf("confirmed drawing was not created: %+v", notice)
	}
	doc := s.App.Context().Docs.AssetDocument(id)
	if doc == nil {
		t.Fatal("confirmed drawing did not open")
	}
	if _, ok := doc.Binding.Model.(*artedit.Document); !ok {
		t.Fatal("confirmed drawing is not editable")
	}
}

func TestDropAsepriteIntoNativeAssets(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	original, err := os.ReadFile(filepath.Join("..", "..", "art", "testdata", "aseprite", "upstream", "tags3.aseprite"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "native-drop.aseprite")
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}
	p := newProject(t)
	const scenePath = "scenes/drop.aescene"
	writeScene(t, p, scenePath, func(*scene.Scene) {})
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Step() {
		t.Fatal("editor stopped during startup")
	}
	if _, err := s.App.Context().Docs.Open(scenePath); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if !s.Step() {
			t.Fatal("editor stopped before the drop")
		}
	}
	if !sdl.TestDropFile(source, 180, 680) || !s.Step() {
		t.Fatal("could not drop the Aseprite file on the Assets panel")
	}
	for range 3 {
		if !s.Step() {
			t.Fatal("editor stopped after the drop")
		}
	}
	id, ok := s.Pipeline.Lookup("assets/native-drop.aeart")
	if !ok {
		notice, _ := s.App.Context().LastNotice()
		t.Fatalf("native drop did not produce an editable asset: %+v", notice)
	}
	doc, err := s.App.Context().OpenAsset(id)
	if err != nil || doc == nil {
		t.Fatalf("dropped asset did not open: %v", err)
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok || len(model.Session.Document.Info().Frames) != 12 {
		t.Fatal("dropped file did not open as an editable 12-frame drawing")
	}
	if data, err := os.ReadFile(source); err != nil || !bytes.Equal(data, original) {
		t.Fatalf("native import modified the original Aseprite file: %v", err)
	}
}

func TestDropAsepriteNativeWarningsNeedConsent(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	original, err := os.ReadFile(filepath.Join("..", "..", "art", "testdata", "aseprite", "upstream", "tags3x123reps.aseprite"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "warning.aseprite")
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}
	p := newProject(t)
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for range 3 {
		if !s.Step() {
			t.Fatal("editor stopped during startup")
		}
	}
	drop := func() {
		t.Helper()
		if !sdl.TestDropFile(source, 180, 680) || !s.Step() {
			t.Fatal("could not drop the Aseprite file on Assets")
		}
		for range 3 {
			s.Step()
		}
		if !s.App.Context().UI.ModalActive() {
			t.Fatal("metadata warnings did not open a native confirmation")
		}
		if _, ok := s.Pipeline.Lookup("assets/warning.aeart"); ok {
			t.Fatal("source was created before consent")
		}
	}
	key := func(k input.Scancode) {
		t.Helper()
		if !sdl.TestEditKey(k, true, false, false) || !s.Step() || !sdl.TestEditKey(k, false, false, false) || !s.Step() {
			t.Fatal("native keyboard event failed")
		}
	}
	drop()
	key(input.KeyEscape)
	if _, ok := s.Pipeline.Lookup("assets/warning.aeart"); ok {
		t.Fatal("cancel still created a converted source")
	}
	drop()
	key(input.KeyEnter)
	id, ok := s.Pipeline.Lookup("assets/warning.aeart")
	if !ok || s.App.Context().Docs.AssetDocument(id) == nil {
		notice, _ := s.App.Context().LastNotice()
		t.Fatalf("confirmation did not create and open the editable copy: %+v", notice)
	}
	if data, err := os.ReadFile(source); err != nil || !bytes.Equal(data, original) {
		t.Fatalf("native warning flow modified the original file: %v", err)
	}
}

func TestInitializeBlankTextArtInNativeEditor(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("AEGO_LANG", "en")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	p := newProject(t)
	const rel = "assets/test.aeart"
	file := filepath.Join(p.Root, filepath.FromSlash(rel))
	if err := os.WriteFile(file, []byte("\n"), 0o600); err != nil {
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
	id, ok := s.Pipeline.Lookup(rel)
	if !ok {
		t.Fatal("blank source was not indexed")
	}
	if _, err := s.App.Context().OpenAsset(id); err != nil {
		t.Fatalf("blank source did not open in the art panel: %v", err)
	}
	for range 3 {
		if !s.Step() {
			t.Fatal("editor stopped before initialization")
		}
	}
	controls := &artControlsArtUI{t: t, s: s}
	controls.maximize("render2d.art/" + id.String())
	controls.clickLabel("Initialize empty drawing…")
	for range 2 {
		controls.step()
	}
	if !s.App.Context().UI.ModalActive() {
		t.Fatal("native Initialize action did not open a confirmation")
	}
	if data, err := os.ReadFile(file); err != nil || !bytes.Equal(data, []byte("\n")) {
		t.Fatalf("source changed before confirmation: %q, %v", data, err)
	}
	if !sdl.TestEditKey(input.KeyEnter, true, false, false) || !s.Step() ||
		!sdl.TestEditKey(input.KeyEnter, false, false, false) || !s.Step() {
		t.Fatal("could not confirm blank-source initialization")
	}
	doc := s.App.Context().Docs.AssetDocument(id)
	if doc == nil {
		t.Fatal("initialized document is missing")
	}
	if _, ok := doc.Binding.Model.(*artedit.Document); !ok {
		notice, _ := s.App.Context().LastNotice()
		t.Fatalf("blank file did not become editable: %+v", notice)
	}
	backup := filepath.Join(p.Root, ".aego", "art-blank-"+id.String()+".original")
	if data, err := os.ReadFile(backup); err != nil || !bytes.Equal(data, []byte("\n")) {
		t.Fatalf("original text was not preserved: %q, %v", data, err)
	}
	if same, ok := s.Pipeline.Lookup(rel); !ok || same != id {
		t.Fatal("initialization changed the asset ID")
	}
}

func TestPNGEditableCopyNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	p := newProject(t)
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	want := color.NRGBA{R: 200, G: 120, B: 80, A: 200}
	img.SetNRGBA(1, 1, want)
	var original bytes.Buffer
	if err := png.Encode(&original, img); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(p.Root, "assets", "sheet.png")
	if err := os.WriteFile(source, original.Bytes(), 0o600); err != nil {
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
	id, ok := s.Pipeline.Lookup("assets/sheet.png")
	if !ok {
		t.Fatal("PNG source was not indexed")
	}
	ed := s.App.Context()
	doc, err := ed.OpenAsset(id)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok || !model.Draft || doc.Dirty() {
		t.Fatal("PNG did not open directly as a clean in-memory drawing")
	}
	metaBefore, err := os.ReadFile(source + ".meta")
	if err != nil {
		t.Fatal(err)
	}
	frame := model.Session.Frame
	pixels, _, err := model.Capture(frame, art.Albedo)
	if err != nil || pixels.NRGBAAt(1, 1) != want {
		t.Fatalf("PNG pixels were not preserved on open: %v", err)
	}
	panelID := "render2d.art/" + id.String()
	ws := s.App.Workspace()
	ws.Focus(panelID)
	if ws.Tree.Maximized() != panelID && !ws.ToggleMaximize(panelID) {
		t.Fatal("could not maximize Art Canvas")
	}
	for range 3 {
		if !s.Step() {
			t.Fatal("editor stopped before painting PNG")
		}
	}
	if name := os.Getenv("AEGO_PNG_COPY_SHOT"); name != "" {
		var shot *image.RGBA
		var shotErr error
		s.Engine.CaptureFrame(func(img *image.RGBA, err error) { shot, shotErr = img, err })
		if !s.Step() || shotErr != nil || shot == nil {
			t.Fatalf("could not capture Art Canvas: %v", shotErr)
		}
		file, err := os.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, shot); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 3 {
		if !sdl.TestPointerEvent(640, 460, i) || !s.Step() {
			t.Fatal("could not paint on the native Art Canvas")
		}
	}
	painted, _, err := model.Capture(frame, art.Albedo)
	if err != nil || bytes.Equal(painted.Pix, pixels.Pix) || model.History.Cursor() != 1 || !doc.Dirty() {
		t.Fatal("native stroke did not update the in-memory draft once", err)
	}
	interopKey(t, s, input.KeyS, true)
	if !ed.UI.ModalActive() {
		t.Fatal("first Save did not request a linked source path")
	}
	if _, ok := s.Pipeline.Lookup("assets/sheet.aeart"); ok {
		t.Fatal("source was created before confirmation")
	}
	if data, err := os.ReadFile(source + ".meta"); err != nil || !bytes.Equal(data, metaBefore) {
		t.Fatal("PNG metadata changed before confirmation", err)
	}
	if !sdl.TestEditKey(input.KeyEnter, true, false, false) || !s.Step() ||
		!sdl.TestEditKey(input.KeyEnter, false, false, false) || !s.Step() {
		t.Fatal("could not confirm the linked source")
	}
	copyID, ok := s.Pipeline.Lookup("assets/sheet.aeart")
	if !ok || copyID == id {
		t.Fatal("native confirmation did not create a separate source")
	}
	saved := ed.Docs.AssetDocument(copyID)
	if saved == nil || saved.Binding.Model != model || model.Draft || saved.Dirty() || model.History.Cursor() != 1 {
		t.Fatal("Save did not retain the same drawing and its history")
	}
	meta, ok := s.Pipeline.MetaOf(id)
	link, err := artsource.TextureLinkOf(&meta)
	if !ok || err != nil || link == nil || link.Source != copyID || len(link.Frames) != 1 || link.Frames[0].Asset != id || link.Frames[0].Frame != frame {
		t.Fatal("confirmed Save changed the original image identity or frame mapping", err)
	}
	if err := model.SessionUndo(); err != nil {
		t.Fatal("Undo after the first Save failed", err)
	}
	undone, _, err := model.Capture(frame, art.Albedo)
	if err != nil || !bytes.Equal(undone.Pix, pixels.Pix) || !saved.Dirty() {
		t.Fatal("Undo after Save did not restore the PNG pixels", err)
	}
	if err := model.SessionRedo(); err != nil || saved.Dirty() {
		t.Fatal("Redo after Save did not restore the saved drawing", err)
	}
	if data, err := os.ReadFile(source); err != nil || !bytes.Equal(data, original.Bytes()) {
		t.Fatalf("the original PNG was modified: %v", err)
	}
}
