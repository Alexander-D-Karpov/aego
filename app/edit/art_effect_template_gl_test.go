//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"aego/art"
	"aego/asset/importers/artsource"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/artedit"
	"aego/editor/core/templates"
	"aego/editor/core/workspace"
	"aego/modules/particles"
	"aego/platform/input"
	"aego/platform/sdl"
	"aego/scene"
	"aego/scene/sceneio"
)

// Creation, choice selection, painting and both kinds of Undo use actual SDL
// UI input. The preview is the existing renderer and CPU ParticleSystem2D.
func TestArtEffectTemplateNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("AEGO_LANG", "en")
	p := newProject(t)
	const scenePath = "scenes/effect-templates.aescene"
	writeScene(t, p, scenePath, func(sc *scene.Scene) { sc.Create(sc.Root(), "Unchanged gameplay") })
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1600, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s != nil {
			s.Close()
		}
	}()
	step := func() {
		t.Helper()
		if !s.Step() {
			t.Fatal("editor stopped during New Effect workflow")
		}
	}
	key := func(k input.Scancode, ctrl, shift bool) {
		t.Helper()
		if !sdl.TestEditKey(k, true, ctrl, shift) {
			t.Fatal("SDL key failed")
		}
		step()
		if !sdl.TestEditKey(k, false, false, false) {
			t.Fatal("SDL key release failed")
		}
		step()
	}
	click := func(x, y float32) {
		t.Helper()
		for kind := range 3 {
			if !sdl.TestPointerEvent(x, y, kind) {
				t.Fatal("SDL pointer failed")
			}
			step()
		}
	}
	capture := func(name string) *image.RGBA {
		t.Helper()
		var pixels *image.RGBA
		var captureErr error
		s.Engine.CaptureFrame(func(img *image.RGBA, err error) { pixels, captureErr = img, err })
		step()
		if pixels == nil || captureErr != nil {
			t.Fatal("GPU effect screenshot failed", captureErr)
		}
		if dir := os.Getenv("AEGO_EFFECT_TEMPLATE_SHOTS"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(filepath.Join(dir, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, pixels); err != nil {
				f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
		}
		return pixels
	}
	step()
	ed, ws := s.App.Context(), s.App.Workspace()
	sceneDoc, err := ed.Docs.Open(scenePath)
	if err != nil {
		t.Fatal(err)
	}
	sceneBefore := sceneio.Save(sceneDoc.Scene())
	for range 3 {
		step()
	}
	palette := s.App.AddPalette()
	palette.CreateNode(ids.NodeID{}, templates.Place{}, nil, nil)
	palette.Palette().SetQuery("Animated Effect")
	if at := palette.Palette().Selected(); at < 0 || !strings.Contains(palette.Palette().Item(at).Label, "Animated Effect") {
		t.Fatal("Create palette cannot find the existing New Effect action")
	}
	step()
	key(input.KeyEnter, false, false)
	const artPath, presetPath = "assets/fire-starter.aeart", "assets/fire-starter.aepreset"
	if _, err := os.Stat(filepath.Join(p.Root, "assets", "effect.aeart")); !os.IsNotExist(err) {
		t.Fatalf("selecting the action submitted its prompt early: %v", err)
	}
	if !sdl.TestTextInput(artPath) {
		t.Fatal("cannot type the effect source path")
	}
	step()
	// The initial choice is Explosion; keyboard navigation selects Fire. No
	// source or scene mutation happens until the separate confirmation.
	key(input.KeyTab, false, false)
	key(input.KeyDown, false, false)
	capture("00-new-effect-choice")
	if _, ok := ed.Pipeline.Lookup(artPath); ok {
		t.Fatal("changing the effect template created files before confirmation")
	}
	key(input.KeyEnter, false, false)
	artID, found := ed.Pipeline.Lookup(artPath)
	presetID, presetFound := ed.Pipeline.Lookup(presetPath)
	if !found || !presetFound {
		notice, _ := ed.LastNotice()
		t.Fatalf("New Effect did not create its complete pair: %+v", notice)
	}
	doc, presetDoc := ed.Docs.AssetDocument(artID), ed.Docs.AssetDocument(presetID)
	if doc == nil || presetDoc == nil || presetDoc.Handler != "particles.preset" {
		t.Fatal("New Effect did not open its existing drawing and preset documents")
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok || model.Session.Document.Info().Properties[art.EffectTemplateProperty] != "fire" || len(model.Session.Document.Info().Frames) != 8 {
		t.Fatal("native keyboard choice did not create the real eight-frame Fire document")
	}
	if ws.Current().ID != workspace.VFX {
		t.Fatal("New Effect did not select the VFX workspace")
	}
	for range 8 {
		step()
	}
	canvasID, previewID := "render2d.art/"+artID.String(), "particles.preset/"+presetID.String()
	canvas, visible := ws.Tree.PanelRect(canvasID)
	preview, previewVisible := ws.Tree.PanelRect(previewID)
	if !visible || !previewVisible || canvas.W < 300 || canvas.H < 200 || preview.W < 140 || preview.H < 200 {
		t.Fatalf("New Effect has no usable canvas/particle layout: %+v %+v", canvas, preview)
	}
	configured, err := particles.LoadPreset(ed.Types, ed.Loader, presetID)
	if err != nil || len(configured.Frames) != 8 || configured.Rate != 16 || !configured.Loop || configured.FrameFPS != 10 {
		t.Fatalf("new preset is not the real Fire emitter with the authored frames: %+v %v", configured, err)
	}
	if err := particles.CheckResources(&configured, ed.Loader); err != nil {
		t.Fatal("New Effect has unresolved prepared dependencies", err)
	}
	for i, frame := range model.Session.Document.Info().Frames {
		want := artsource.StableID(artID, artsource.FrameName(frame.ID, art.Albedo))
		if configured.Frames[i].ID != want {
			t.Fatal("new particle frame uses an unrelated asset identity")
		}
	}
	before := capture("01-fire-vfx-start")
	for range 30 {
		time.Sleep(15 * time.Millisecond)
		step()
	}
	after := capture("02-fire-vfx-playing")
	changed, flame := 0, 0
	for y := int(preview.Y + 5); y < int(preview.Y+155); y++ {
		for x := int(preview.X + 5); x < int(preview.X+min(preview.W-5, 155)); x++ {
			pixel := after.RGBAAt(x, y)
			if pixel != before.RGBAAt(x, y) {
				changed++
			}
			if pixel.R > 80 && int(pixel.R) > 2*int(pixel.B) {
				flame++
			}
		}
	}
	if changed < 20 || flame < 5 {
		t.Fatalf("real particle preview did not show the animated Fire drawing: changed=%d warm=%d", changed, flame)
	}
	original := map[string][]byte{}
	for _, rel := range []string{artPath, artPath + ".meta", presetPath, presetPath + ".meta"} {
		original[rel], err = ed.Pipeline.SourceBytes(rel)
		if err != nil || original[rel] == nil {
			t.Fatalf("created source pair is incomplete: %s %v", rel, err)
		}
	}
	click(canvas.X+canvas.W/2, canvas.Y+canvas.H/2)
	if model.History.Cursor() != 1 || !doc.Dirty() {
		t.Fatal("new template is not editable with the native Pencil")
	}
	capture("03-fire-native-paint")
	focusAssets := func() {
		t.Helper()
		ws.Show(workspace.Assets)
		step()
		area, visible := ws.Tree.PanelRect(workspace.Assets)
		if !visible {
			t.Fatal("Assets is unavailable for file history")
		}
		// File commands use the actual panel under the pointer as their scope.
		// Focus it through SDL, as a user does, rather than changing only the
		// workspace's focus bookkeeping while leaving the mouse over Canvas.
		click(area.X+area.W-20, area.Y+area.H-20)
		if ws.Focused() != workspace.Assets || ws.Hovered() != workspace.Assets {
			t.Fatalf("SDL did not focus Assets: focus=%s hover=%s", ws.Focused(), ws.Hovered())
		}
	}
	focusAssets()
	key(input.KeyZ, true, false)
	if notice, ok := ed.LastNotice(); !ok || !strings.Contains(notice.Text, "unsaved edits") {
		t.Fatalf("file Undo did not report the dirty-source guard: %+v", notice)
	}
	if ed.Docs.AssetDocument(artID) != doc || !doc.Dirty() {
		t.Fatal("Assets creation Undo discarded dirty template pixels")
	}
	for rel, want := range original {
		got, _ := ed.Pipeline.SourceBytes(rel)
		if !bytes.Equal(got, want) {
			t.Fatal("dirty guard partially changed the source pair", rel)
		}
	}
	// Click the real canvas toolbar's Undo once to remove the single stroke.
	pad, row := ed.UI.Theme.Padding, ed.UI.Theme.RowHeight
	click(canvas.X+pad+row+4+row/2, canvas.Y+pad+row/2)
	if doc.Dirty() || model.History.Cursor() != 0 {
		t.Fatal("document Undo did not undo exactly the native gesture")
	}
	focusAssets()
	key(input.KeyZ, true, false)
	for rel := range original {
		if data, err := ed.Pipeline.SourceBytes(rel); err != nil || data != nil {
			notice, _ := ed.LastNotice()
			t.Fatalf("creation Undo left part of the pair: %s %v; focus=%s hover=%s notice=%+v", rel, err, ws.Focused(), ws.Hovered(), notice)
		}
	}
	if ed.Docs.AssetDocument(artID) != nil || ed.Docs.AssetDocument(presetID) != nil {
		t.Fatal("deleted clean effect documents remained open")
	}
	key(input.KeyZ, true, true)
	for rel, want := range original {
		got, err := ed.Pipeline.SourceBytes(rel)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("creation Redo changed or omitted a source: %s %v", rel, err)
		}
	}
	if got, ok := ed.Pipeline.Lookup(artPath); !ok || got != artID {
		t.Fatal("creation Redo changed the drawing AssetID")
	}
	if !bytes.Equal(sceneBefore, sceneio.Save(sceneDoc.Scene())) || sceneDoc.Dirty() {
		t.Fatal("effect authoring or preview changed the gameplay scene")
	}
	if _, err = ed.OpenAsset(presetID); err != nil {
		t.Fatal(err)
	}
	if _, err = ed.OpenAsset(artID); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		step()
	}
	capture("04-creation-redone")
	s.Close()
	s = nil
	s, err = Open(p, Config{Hidden: true, NoWatch: true, Width: 1600, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	step()
	ed = s.App.Context()
	loaded, err := ed.OpenAsset(artID)
	if err != nil {
		t.Fatal(err)
	}
	if model, ok := loaded.Binding.Model.(*artedit.Document); !ok || model.Session.Document.Info().Properties[art.EffectTemplateProperty] != "fire" {
		t.Fatal("effect drawing or template metadata did not survive restart")
	}
	actual, err := particles.LoadPreset(ed.Types, ed.Loader, presetID)
	if err != nil || len(actual.Frames) != 8 {
		t.Fatalf("linked effect did not reopen: %+v %v", actual, err)
	}
	for i := range configured.Frames {
		if actual.Frames[i].ID != configured.Frames[i].ID {
			t.Fatal("restarting changed an authored particle frame reference")
		}
	}
	if _, err = ed.OpenAsset(presetID); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		step()
	}
	capture("05-reopened")
}
