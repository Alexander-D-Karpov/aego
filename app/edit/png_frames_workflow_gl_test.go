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
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/editor/core/artedit"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/platform/sdl"
	"aego/scene"
	"aego/scene/sceneio"
)

// The example project is copied to a temporary directory. Project/scene and
// image opening use the Go API; first Save and drawing use SDL events. This
// is an offscreen/software-runner workflow, not physical OS acceptance.
func TestPlatformerSlicedPNGCopyNative(t *testing.T) {
	platformerSlicedPNGCopyNative(t, false)
}

// This extended route explicitly assigns a new controller in a disposable
// project; it never silently rewrites the original Player or its PNG assets.
func TestPlatformerSlicedPNGGameUndoExportNative(t *testing.T) {
	platformerSlicedPNGCopyNative(t, true)
}

func platformerSlicedPNGCopyNative(t *testing.T, fullWorkflow bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	p := platformerAuthoringProject(t)
	if fullWorkflow {
		preparePlatformerNativeExportProject(t, p)
	}
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()}, platformerAuthoringModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Step() {
		t.Fatal("editor stopped during startup")
	}
	ed := s.App.Context()
	sceneDoc, err := ed.Docs.Open("scenes/main.aescene")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Step() {
		t.Fatal("editor stopped after opening the scene")
	}
	beforeScene := sceneio.Save(sceneDoc.Scene())
	hero := sceneDoc.Scene().Find(sceneDoc.Scene().Root(), "Player")
	sprite, ok := scene.Get[render2d.SpriteRenderer](sceneDoc.Scene(), hero)
	if !ok {
		t.Fatal("the real platformer Player has no sprite")
	}
	beforeSprite := sprite.Sprite.ID
	paths := []string{"assets/surfaces/runner.png", "assets/surfaces/runner.png.meta", "assets/surfaces/runner_n.png", "assets/surfaces/runner_n.png.meta", "scenes/main.aescene"}
	before := make(map[string][]byte, len(paths))
	for _, rel := range paths {
		before[rel], err = os.ReadFile(filepath.Join(p.Root, rel))
		if err != nil {
			t.Fatal(err)
		}
	}
	metaBefore, err := pipeline.ParseMeta(before[paths[1]], paths[1])
	if err != nil || len(metaBefore.Subs) != 8 {
		t.Fatal("the original runner needs eight indexed frames", err)
	}
	imageDoc, err := ed.OpenAsset(beforeSprite)
	if err != nil || imageDoc == nil || imageDoc.Path != "assets/surfaces/runner.png" {
		t.Fatalf("Player sprite did not open its PNG source: %+v, %v", imageDoc, err)
	}
	model, ok := imageDoc.Binding.Model.(*artedit.Document)
	if !ok || !model.Draft || imageDoc.Dirty() || len(model.Session.Document.Info().Frames) != 8 {
		t.Fatal("Player sprite did not open as eight editable frames in memory")
	}
	for _, rel := range paths {
		after, err := os.ReadFile(filepath.Join(p.Root, rel))
		if err != nil || !bytes.Equal(before[rel], after) {
			t.Fatalf("opening the PNG changed %s: %v", rel, err)
		}
	}
	ws := s.App.Workspace()
	panelID := "render2d.art/" + imageDoc.Asset.String()
	ws.Focus(panelID)
	if ws.Tree.Maximized() != panelID && !ws.ToggleMaximize(panelID) {
		t.Fatal("could not maximize the Art Canvas")
	}
	for range 4 {
		if !s.Step() {
			t.Fatal("editor stopped before first Save")
		}
	}
	capture := func(name string) {
		t.Helper()
		dir := os.Getenv("AEGO_PNG_FRAMES_SHOTS")
		if dir == "" {
			return
		}
		var shot *image.RGBA
		var captureErr error
		s.Engine.CaptureFrame(func(img *image.RGBA, err error) { shot, captureErr = img, err })
		if !s.Step() || captureErr != nil || shot == nil {
			t.Fatalf("capture %s: %v", name, captureErr)
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, shot)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("write %s: %v, %v", name, err, closeErr)
		}
	}
	capture("art-draft")
	key := func(k input.Scancode, ctrl bool) {
		t.Helper()
		if !sdl.TestEditKey(k, true, ctrl, false) || !s.Step() || !sdl.TestEditKey(k, false, false, false) || !s.Step() {
			t.Fatal("native keyboard event failed")
		}
	}
	key(input.KeyS, true)
	if !ed.UI.ModalActive() {
		t.Fatal("first Save did not ask before linking the source")
	}
	capture("first-save")
	const copyPath = "assets/surfaces/runner.aeart"
	if data, err := s.Pipeline.SourceBytes(copyPath); err != nil || data != nil {
		t.Fatal("the editable source appeared before confirmation", err)
	}
	if current, err := os.ReadFile(filepath.Join(p.Root, paths[1])); err != nil || !bytes.Equal(current, before[paths[1]]) {
		t.Fatal("the PNG metadata changed before confirmation", err)
	}
	key(input.KeyEnter, false)
	id, ok := s.Pipeline.Lookup(copyPath)
	if !ok {
		notice, _ := ed.LastNotice()
		t.Fatalf("confirmed editable source was not imported: %+v", notice)
	}
	copyDoc := ed.Docs.AssetDocument(id)
	if copyDoc == nil {
		t.Fatal("the linked drawing was not opened")
	}
	if copyDoc.Binding.Model != model || model.Draft || copyDoc.Dirty() || len(model.Session.Document.Info().Frames) != 8 {
		t.Fatal("first Save did not transfer the same clean eight-frame session")
	}
	metaAfter, ok := s.Pipeline.MetaOf(imageDoc.Asset)
	link, err := artsource.TextureLinkOf(&metaAfter)
	if !ok || err != nil || link == nil || link.Source != id || len(link.Frames) != len(metaBefore.Subs) || len(metaAfter.Subs) != len(metaBefore.Subs) {
		t.Fatal("first Save did not create the guarded PNG frame link", err)
	}
	for i, original := range metaBefore.Subs {
		current := metaAfter.Subs[i]
		if original.ID != current.ID || original.Name != current.Name || original.Rect != current.Rect || original.Pivot != current.Pivot || original.HasPivot != current.HasPivot || link.Frames[i].Asset != original.ID {
			t.Fatalf("frame %d lost its source identity, slicing or pivot", i)
		}
		for _, channel := range []string{"normal", "height", "emission"} {
			oldID, oldExplicit, oldErr := asset.TextureChannel(metaBefore, original.ID, channel)
			newID, newExplicit, newErr := asset.TextureChannel(&metaAfter, current.ID, channel)
			if oldErr != nil || newErr != nil || oldID != newID || oldExplicit != newExplicit {
				t.Fatalf("frame %d lost its %s association: %v, %v", i, channel, oldErr, newErr)
			}
		}
		if _, present := s.Pipeline.Entry(original.ID); !present {
			t.Fatalf("the original runtime frame %s disappeared", original.ID)
		}
	}
	// Subsequent checks compare against the explicitly confirmed sidecar, while
	// all PNG bytes, surface sources and scene bytes remain unchanged.
	before[paths[1]], err = os.ReadFile(filepath.Join(p.Root, paths[1]))
	if err != nil {
		t.Fatal(err)
	}
	artPanel := "render2d.art/" + id.String()
	if current := ws.Tree.Maximized(); current != "" && current != artPanel {
		ws.ToggleMaximize(current)
	}
	ws.Focus(artPanel)
	if ws.Tree.Maximized() != artPanel && !ws.ToggleMaximize(artPanel) {
		t.Fatal("could not maximize Art Studio")
	}
	for range 3 {
		s.Step()
	}
	first := model.Session.Document.Info().Frames[0].ID
	if err := model.Session.Select("", first, art.Albedo); err != nil {
		t.Fatal(err)
	}
	original, _, err := model.Capture(first, art.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if !sdl.TestPointerEvent(500, 400, i) || !s.Step() {
			t.Fatal("could not paint the imported frame")
		}
	}
	painted, _, err := model.Capture(first, art.Albedo)
	if err != nil || bytes.Equal(painted.Pix, original.Pix) || model.History.Cursor() != 1 {
		t.Fatalf("native pointer stroke did not edit the imported frame: %v", err)
	}
	capture("painted")
	key(input.KeyZ, true)
	undone, _, err := model.Capture(first, art.Albedo)
	if err != nil || !bytes.Equal(undone.Pix, original.Pix) {
		t.Fatalf("native Undo did not restore the frame: %v", err)
	}
	for _, rel := range paths {
		after, err := os.ReadFile(filepath.Join(p.Root, rel))
		if err != nil || !bytes.Equal(before[rel], after) {
			t.Fatalf("conversion changed original %s: %v", rel, err)
		}
	}
	if sceneDoc.Dirty() || !bytes.Equal(sceneio.Save(sceneDoc.Scene()), beforeScene) || sprite.Sprite.ID != beforeSprite {
		t.Fatal("conversion changed the real Player scene or its sprite reference")
	}
	if fullWorkflow {
		platformerSlicedPNGGameUndoExport(t, p, s, sceneDoc, copyDoc, model, original.Pix, painted.Pix, key)
		// The test explicitly edits the copied scene's Animator, but the PNG,
		// its sidecar and normal channel must remain byte-identical throughout.
		for _, rel := range paths[:len(paths)-1] {
			after, err := os.ReadFile(filepath.Join(p.Root, rel))
			if err != nil || !bytes.Equal(before[rel], after) {
				t.Fatalf("Player game/export workflow changed original %s: %v", rel, err)
			}
		}
	}
}
