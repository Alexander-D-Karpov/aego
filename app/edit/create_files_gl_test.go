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
	for i := range 3 {
		if !sdl.TestPointerEvent(600, 122, i) || !s.Step() {
			t.Fatal("could not select initialize in the native art panel")
		}
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
	if _, err := s.App.Context().OpenAsset(id); err != nil {
		t.Fatal(err)
	}
	panelID := "render2d.image/" + id.String()
	ws := s.App.Workspace()
	ws.Focus(panelID)
	if ws.Tree.Maximized() != panelID && !ws.ToggleMaximize(panelID) {
		t.Fatal("could not maximize Image Viewer")
	}
	for range 3 {
		if !s.Step() {
			t.Fatal("editor stopped before copying PNG")
		}
	}
	if name := os.Getenv("AEGO_PNG_COPY_SHOT"); name != "" {
		var shot *image.RGBA
		var shotErr error
		s.Engine.CaptureFrame(func(img *image.RGBA, err error) { shot, shotErr = img, err })
		if !s.Step() || shotErr != nil || shot == nil {
			t.Fatalf("could not capture Image Viewer: %v", shotErr)
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
		if !sdl.TestPointerEvent(400, 147, i) || !s.Step() {
			t.Fatal("could not click the native editable-copy action")
		}
	}
	if !s.App.Context().UI.ModalActive() {
		t.Fatal("editable-copy action did not open a native confirmation")
	}
	if _, ok := s.Pipeline.Lookup("assets/sheet.aeart"); ok {
		t.Fatal("copy was created before confirmation")
	}
	if !sdl.TestEditKey(input.KeyEnter, true, false, false) || !s.Step() ||
		!sdl.TestEditKey(input.KeyEnter, false, false, false) || !s.Step() {
		t.Fatal("could not confirm the editable copy")
	}
	copyID, ok := s.Pipeline.Lookup("assets/sheet.aeart")
	if !ok || copyID == id {
		t.Fatal("native confirmation did not create a separate source")
	}
	doc := s.App.Context().Docs.AssetDocument(copyID)
	if doc == nil {
		t.Fatal("copy did not open in Art Studio")
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok {
		t.Fatal("copy is not editable")
	}
	frame := model.Session.Document.Info().Frames[0].ID
	pixels, _, err := model.Capture(frame, art.Albedo)
	if err != nil || pixels.NRGBAAt(1, 1) != want {
		t.Fatalf("PNG pixels were not preserved: %v", err)
	}
	if data, err := os.ReadFile(source); err != nil || !bytes.Equal(data, original.Bytes()) {
		t.Fatalf("the original PNG was modified: %v", err)
	}
}
