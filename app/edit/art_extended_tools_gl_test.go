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
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/platform/input"
	"aego/platform/sdl"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/ui/draw"
)

// This exercises the actual SDL controls and dialogs. Model reads below are
// assertions only; drawing, layer edits, transformation and export use the UI.
func TestArtExtendedToolsNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("AEGO_LANG", "en")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	p := newProject(t)
	d, err := art.New(16, 16, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	data, err := d.Encode()
	d.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, "assets/tools.aeart"), data, 0644); err != nil {
		t.Fatal(err)
	}
	writeScene(t, p, "scenes/keyboard.aescene", func(sc *scene.Scene) {
		sc.Create(sc.Root(), "Keep this scene object")
	})
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	step := func() {
		t.Helper()
		if !s.Step() {
			t.Fatal("editor stopped during extended tool workflow")
		}
	}
	step()
	ed, ws := s.App.Context(), s.App.Workspace()
	sceneDoc, err := ed.Docs.Open("scenes/keyboard.aescene")
	if err != nil {
		t.Fatal(err)
	}
	sc := sceneDoc.Scene()
	ed.Select(sc.ID(sc.Find(sc.Root(), "Keep this scene object")))
	sceneBefore := sceneio.Save(sc)
	doc, err := ed.OpenAssetPath("assets/tools.aeart")
	if err != nil {
		t.Fatal(err)
	}
	model := doc.Binding.Model.(*artedit.Document)
	canvasID := "render2d.art/" + doc.Asset.String()
	maximize := func(id string) mathx.Rect {
		t.Helper()
		if ws.Tree.Maximized() != "" {
			ws.Tree.Restore()
		}
		ws.Show(id)
		ws.Focus(id)
		if !ws.ToggleMaximize(id) {
			t.Fatal("cannot maximize", id)
		}
		for range 3 {
			step()
		}
		r, ok := ws.Tree.PanelRect(id)
		if !ok {
			t.Fatal("panel is not visible", id)
		}
		return r
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
	key := func(k input.Scancode, ctrl bool) {
		t.Helper()
		if !sdl.TestEditKey(k, true, ctrl, false) {
			t.Fatal("SDL key failed")
		}
		step()
		if !sdl.TestEditKey(k, false, false, false) {
			t.Fatal("SDL key release failed")
		}
		step()
	}
	typeText := func(text string) {
		t.Helper()
		key(input.KeyA, true)
		if !sdl.TestTextInput(text) {
			t.Fatal("SDL text failed")
		}
		step()
	}
	capture := func(name string) {
		t.Helper()
		var pixels *image.RGBA
		var captureErr error
		s.Engine.CaptureFrame(func(img *image.RGBA, err error) { pixels, captureErr = img, err })
		step()
		if pixels == nil || captureErr != nil {
			t.Fatal("GPU screenshot failed", captureErr)
		}
		if dir := os.Getenv("AEGO_ART_EXTENDED_SHOTS"); dir != "" {
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
	}
	canvas := maximize(canvasID)
	click(canvas.X+canvas.W*.4, canvas.Y+canvas.H*.4)
	if model.History.Cursor() != 1 {
		t.Fatal("initial SDL stroke did not paint")
	}
	key(input.KeyE, false)
	click(canvas.X+canvas.W*.4, canvas.Y+canvas.H*.4)
	if model.History.Cursor() != 2 {
		t.Fatal("native E shortcut did not activate Eraser")
	}
	key(input.KeyZ, true)
	if model.History.Cursor() != 1 {
		t.Fatal("keyboard erase did not participate in common Undo")
	}
	key(input.KeyA, true)
	key(input.KeyC, true)
	if model.Session.Clipboard == nil || model.Session.Selection == nil || model.History.Cursor() != 1 {
		t.Fatal("common Select All/Copy did not target Art pixels")
	}
	key(input.KeyX, true)
	if model.History.Cursor() != 2 {
		t.Fatal("common Cut did not create exactly one Art history operation")
	}
	key(input.KeyV, true)
	if model.History.Cursor() != 3 {
		t.Fatal("common Paste did not create exactly one Art history operation")
	}
	key(input.KeyZ, true)
	if model.History.Cursor() != 2 {
		t.Fatal("common Undo reverted more than one clipboard operation")
	}
	key(input.KeyZ, true)
	key(input.KeyDelete, false)
	if model.History.Cursor() != 2 {
		t.Fatal("common Delete did not target the Art selection")
	}
	key(input.KeyZ, true)
	key(input.KeyD, true)
	if model.History.Cursor() != 1 || model.Session.Selection != nil || !bytes.Equal(sceneio.Save(sc), sceneBefore) {
		t.Fatal("Art keyboard actions changed the selected scene object, its history, or the wrong gestures")
	}
	key(input.KeyB, false)
	props := maximize("art.properties")
	ui := ed.UI
	row, gap, pad := ui.Theme.RowHeight, ui.Theme.Spacing, ui.Theme.Padding
	promptField := func(label, current, value string) {
		t.Helper()
		if !ui.ModalActive() {
			t.Fatal("the actual properties prompt is not open", label)
		}
		screen := mathx.Rect{W: ui.Size().X, H: ui.Size().Y}
		visible := func(item draw.TestDrawnItem) bool {
			r, c := item.Bounds, item.Clip
			return r.X >= c.X && r.Y >= c.Y && r.X+r.W <= c.X+c.W && r.Y+r.H <= c.Y+c.H
		}
		var caption draw.TestDrawnItem
		for _, item := range s.App.UIList().TestDrawnItems(screen, ui.Font(), label, 0, 0) {
			if item.Text == label && item.Clip != screen && visible(item) {
				caption = item
			}
		}
		if caption.Text == "" {
			t.Fatal("properties field label is missing or clipped", label)
		}
		var field mathx.Rect
		for _, item := range s.App.UIList().TestDrawnItems(screen, ui.Font(), current, 0, 0) {
			r := item.Clip
			// TextField's own clip is the real input rectangle. Pair it with
			// the visible label above it inside the same scrollable modal body.
			if item.Text == current && visible(item) && r.H == row && r.X >= caption.Clip.X && r.X+r.W <= caption.Clip.X+caption.Clip.W && r.Y >= caption.Bounds.Y+caption.Bounds.H && r.Y+r.H <= caption.Clip.Y+caption.Clip.H && (field.H == 0 || r.Y < field.Y) {
				field = r
			}
		}
		if field.H == 0 {
			t.Fatal("properties input is missing below its actual label", label, current)
		}
		at := field.Center()
		click(at.X, at.Y)
		if ui.Focus() == 0 {
			t.Fatal("clicking the actual properties input did not focus it", label)
		}
		typeText(value)
	}
	applyPrompt := func() {
		t.Helper()
		if !ui.ModalActive() {
			t.Fatal("properties prompt closed before Apply")
		}
		screen := mathx.Rect{W: ui.Size().X, H: ui.Size().Y}
		var button mathx.Rect
		for _, item := range s.App.UIList().TestDrawnItems(screen, ui.Font(), "Apply", 0, 0) {
			if item.Text == "Apply" && item.Clip != screen && item.Bounds.X >= item.Clip.X && item.Bounds.Y >= item.Clip.Y && item.Bounds.X+item.Bounds.W <= item.Clip.X+item.Clip.W && item.Bounds.Y+item.Bounds.H <= item.Clip.Y+item.Clip.H {
				button = item.Bounds
			}
		}
		if button.H == 0 {
			t.Fatal("the full visible Apply button is missing")
		}
		at := button.Center()
		click(at.X, at.Y)
		if ui.ModalActive() {
			t.Fatal("clicking the actual Apply button did not submit properties")
		}
	}
	menu := func(menuIndex, itemIndex int) {
		t.Helper()
		// With docked Tools: channel, palette and add-layer rows, followed
		// by the layer rows and the compact actions menus.
		layers := min(8, len(model.Session.Document.Info().Layers))
		y := props.Y + pad + float32(3+layers+menuIndex)*(row+gap)
		x := props.X + pad + 110
		click(x, y+row/2)
		if !ui.AnyPopup() {
			capture("failure-menu")
			t.Fatalf("action menu %d did not open at %g,%g", menuIndex, x, y)
		}
		capture("actions-menu")
		click(x, y+row+pad+float32(itemIndex)*(row+gap)+row/2)
		if ui.AnyPopup() {
			t.Fatalf("action %d remained open", itemIndex)
		}
	}
	menu(0, 0)
	// Prompt fields are real text widgets. The first starts selected.
	typeText("Foreground")
	promptField("Opacity (0–255)", "255", "128")
	promptField("Blend: normal / multiply / screen / add", "normal", "multiply")
	capture("layer-properties-dialog")
	applyPrompt()
	info := model.Session.Document.Info()
	if info.Layers[0].Name != "Foreground" || info.Layers[0].Opacity != 128 || info.Layers[0].Blend != art.BlendMultiply {
		capture("failure-properties")
		t.Fatalf("SDL layer properties did not apply: %+v", info.Layers[0])
	}
	menu(0, 4)
	if !model.Session.Document.Info().Layers[0].AlphaLock {
		t.Fatal("alpha lock control did not apply")
	}
	menu(0, 4)
	if model.Session.Document.Info().Layers[0].AlphaLock {
		t.Fatal("alpha lock control did not disable")
	}
	menu(0, 1)
	info = model.Session.Document.Info()
	if len(info.Layers) != 2 || info.Layers[1].Kind != art.Group {
		t.Fatal("new group UI did not create a group")
	}
	// Return to the visible raster layer using its actual row.
	click(props.X+pad+100, props.Y+pad+3*(row+gap)+row/2)
	if model.Session.Layer != info.Layers[0].ID {
		t.Fatal("layer row did not select the raster")
	}
	before, _, err := model.Capture(model.Session.Frame, art.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	cursor := model.History.Cursor()
	menu(1, 4)
	after, _, err := model.Capture(model.Session.Frame, art.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	if model.History.Cursor() != cursor+1 || bytes.Equal(before.Pix, after.Pix) {
		t.Fatal("native flip did not make one visible reversible change")
	}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if before.NRGBAAt(x, y) != after.NRGBAAt(15-x, y) {
				t.Fatal("native horizontal flip has wrong pixels")
			}
		}
	}
	// Save Copy uses a dialog and the normal source transaction, not os.WriteFile.
	menu(3, 0)
	key(input.KeyEnter, false)
	copyBytes, err := os.ReadFile(filepath.Join(p.Root, "assets/tools-export.aeart"))
	if err != nil {
		capture("failure-export")
		t.Fatal(err)
	}
	copyDoc, err := art.Decode(copyBytes, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	copyPixels, err := copyDoc.Render(copyDoc.Info().Frames[0].ID, art.RenderOptions{Channel: art.Albedo})
	copyDoc.Close()
	if err != nil || !bytes.Equal(copyPixels.Pix, after.Pix) || !doc.Dirty() {
		t.Fatal("native Save Copy lost unsaved pixels or cleared source dirty state", err)
	}
	capture("properties-and-export")
	canvas = maximize(canvasID)
	tool := func(index int) {
		t.Helper()
		y := canvas.Y + pad + (row + gap)
		click(canvas.X+100, y+row/2)
		if !ui.AnyPopup() {
			t.Fatal("tool picker did not open")
		}
		click(canvas.X+50, y+row+pad+float32(index)*(row+gap)+row/2)
		if ui.AnyPopup() {
			t.Fatal("tool picker did not close")
		}
	}
	cursor = model.History.Cursor()
	tool(12) // Curve: start, end, two controls.
	click(430, 340)
	click(760, 340)
	click(500, 530)
	if !model.Session.Info().Gesture || model.History.Cursor() != cursor {
		t.Fatal("curve published before all four points")
	}
	capture("curve-preview")
	click(690, 530)
	if model.Session.Info().Gesture || model.History.Cursor() != cursor+1 {
		t.Fatal("curve did not commit once on its fourth click")
	}
	tool(13) // Polygon: arbitrary vertices, Enter commits.
	cursor = model.History.Cursor()
	click(430, 330)
	click(760, 350)
	click(610, 520)
	if !model.Session.Info().Gesture || model.History.Cursor() != cursor {
		t.Fatal("polygon published before confirmation")
	}
	capture("polygon-preview")
	key(input.KeyEnter, false)
	if model.Session.Info().Gesture || model.History.Cursor() != cursor+1 {
		t.Fatal("polygon did not commit one operation")
	}
	cursor = model.History.Cursor()
	pixels, _, err := model.Capture(model.Session.Frame, art.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	click(430, 350)
	click(750, 540)
	key(input.KeyEscape, false)
	canceled, _, err := model.Capture(model.Session.Frame, art.Albedo)
	if err != nil || model.Session.Info().Gesture || model.History.Cursor() != cursor || !bytes.Equal(pixels.Pix, canceled.Pix) {
		t.Fatal("Esc committed or retained an unfinished polygon", err)
	}
	capture("extended-tools-complete")
	timeline := maximize("art.timeline")
	width := timeline.W - 2*pad
	click(timeline.X+pad+width*5/8, timeline.Y+pad+(row+gap)+row/2)
	info = model.Session.Document.Info()
	if len(info.Frames) != 2 {
		t.Fatal("SDL Link did not create a linked frame")
	}
	first, second := info.Frames[0].ID, info.Frames[1].ID
	celImage := func(frame art.ID) art.ID {
		for _, cel := range model.Session.Document.Info().Cels {
			if cel.Frame == frame && cel.Layer == info.Layers[0].ID && cel.Channel == art.Albedo {
				return cel.Image
			}
		}
		return ""
	}
	if celImage(first) == "" || celImage(first) != celImage(second) {
		t.Fatal("SDL linked frame lost shared image identity")
	}
	capture("timeline-linked-cels")
	// The first raster cel is selected through the actual Layers x Frames grid.
	labelW := min(float32(150), max(float32(86), width*.24))
	columns := min(32, max(1, int((width-labelW-ui.Theme.ScrollbarW)/48)))
	cellW := (width - labelW - ui.Theme.ScrollbarW) / float32(columns)
	click(timeline.X+pad+labelW+cellW/2, timeline.Y+pad+4*(row+gap)+row/2)
	if model.Session.Frame != first || model.Session.Layer != info.Layers[0].ID {
		t.Fatal("cel grid did not select the real layer/frame")
	}
	click(timeline.X+pad+width*7/8, timeline.Y+pad+(row+gap)+row/2)
	if celImage(first) == celImage(second) {
		t.Fatal("SDL Unlink did not make an independent image")
	}
	key(input.KeyZ, true)
	if celImage(first) != celImage(second) {
		t.Fatal("common Undo did not restore linked identity")
	}
	// Frame pivot is a typed core operation through the real properties prompt.
	menuY := timeline.Y + pad + 2*(row+gap)
	click(timeline.X+pad+width*5/6, menuY+row/2)
	if !ui.AnyPopup() {
		t.Fatal("frame properties menu did not open")
	}
	click(timeline.X+pad+width*2/3+50, menuY+row+pad+7*(row+gap)+row/2)
	typeText("3")
	promptField("Y", "8", "12")
	cursor = model.History.Cursor()
	applyPrompt()
	if pivot, ok := model.Session.Document.Info().FramePivot(first); !ok || pivot != image.Pt(3, 12) || model.History.Cursor() != cursor+1 {
		t.Fatal("native pivot prompt did not create one reversible frame operation", pivot, ok)
	}
	capture("timeline-frame-pivot")
	// The shared File > Save As shortcut must save the Art document, without
	// silently saving or renaming the still-selected scene instead.
	originalHistory := model.History.Cursor()
	snapshot, err := model.Session.Document.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !sdl.TestEditKey(input.KeyS, true, true, true) {
		t.Fatal("SDL Save As shortcut failed")
	}
	step()
	if !sdl.TestEditKey(input.KeyS, false, false, false) {
		t.Fatal("SDL Save As release failed")
	}
	step()
	typeText("assets/saved-as.aeart")
	key(input.KeyEnter, false)
	for range 3 {
		step()
	}
	savedAs, err := os.ReadFile(filepath.Join(p.Root, "assets/saved-as.aeart"))
	if err != nil || !bytes.Equal(savedAs, snapshot) {
		t.Fatal("shared Save As did not preserve the complete current Art snapshot", err)
	}
	copyID, ok := ed.Pipeline.Lookup("assets/saved-as.aeart")
	if !ok || copyID == doc.Asset || ws.Focused() != "render2d.art/"+copyID.String() {
		t.Fatal("Save As did not open an independently identified Art document")
	}
	if copyDoc := ed.Docs.AssetDocument(copyID); copyDoc == nil || copyDoc.Dirty() || !doc.Dirty() || model.History.Cursor() != originalHistory || !bytes.Equal(sceneio.Save(sc), sceneBefore) {
		t.Fatal("Save As damaged the original dirty document, its history or the scene")
	}
	capture("shared-save-as-copy")
	t.Log("SDL single-gesture Undo, Cut/Copy/Paste/Delete isolated from scene, Save As, layer properties/group/alpha lock, flip pixels, safe export, curve/polygon preview, Layers x Frames grid, linked/unlink Undo and frame pivot PASS")
}
