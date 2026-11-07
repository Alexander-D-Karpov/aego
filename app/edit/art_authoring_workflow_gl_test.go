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

	"aego/core/log"
	"aego/editor/core/artedit"
	"aego/editor/core/workspace"
	"aego/platform/input"
	"aego/platform/sdl"
)

func TestArtSceneAuthoringWorkflowNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cfg := Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()}
	frames := map[string][]byte{}
	ticks := map[string]uint64{}
	artSceneAuthoringWorkflow(t, cfg, func(t *testing.T, s *Session, panelID string, model *artedit.Document) {
		t.Helper()
		ws := s.App.Workspace()
		ws.Focus(panelID)
		if ws.Tree.Maximized() != panelID && !ws.ToggleMaximize(panelID) {
			t.Fatal("could not maximize the art panel")
		}
		for range 3 {
			if !s.Step() {
				t.Fatal("editor stopped before the pointer gesture")
			}
		}
		// A real SDL motion/down/up sequence goes through the window, docking,
		// native panel and shared History. No synthetic model stroke is used.
		for i := range 3 {
			if !sdl.TestPointerEvent(500, 400, i) {
				t.Fatal("SDL rejected the pointer event")
			}
			if !s.Step() {
				t.Fatal("editor stopped during the pointer gesture")
			}
		}
		if model.History.Cursor() != 1 {
			t.Fatalf("native gesture produced %d history entries", model.History.Cursor())
		}
		capture := func(name string) {
			t.Helper()
			dir := os.Getenv("AEGO_ART_WORKFLOW_SHOTS")
			if dir == "" {
				return
			}
			var capture *image.RGBA
			var captureErr error
			s.Engine.CaptureFrame(func(img *image.RGBA, err error) { capture, captureErr = img, err })
			s.Step()
			if captureErr != nil || capture == nil {
				t.Fatal("native capture failed", captureErr)
			}
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(filepath.Join(dir, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, capture)
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				t.Fatal(err, closeErr)
			}
		}
		capture("native-art")
		// Exercise the icon-bearing tool menu through SDL too. Opening it and
		// selecting the already active Pencil must not touch document history.
		for i := range 3 {
			if !sdl.TestPointerEvent(100, 122, i) || !s.Step() {
				t.Fatal("could not open the native tool menu")
			}
		}
		if !s.App.Context().UI.AnyPopup() {
			t.Fatal("native tool menu did not open")
		}
		capture("native-art-tools")
		for i := range 3 {
			if !sdl.TestPointerEvent(50, 150, i) || !s.Step() {
				t.Fatal("could not select a native tool")
			}
		}
		if s.App.Context().UI.AnyPopup() || model.History.Cursor() != 1 {
			t.Fatal("selecting the current tool changed the document or left the menu open")
		}
		// Select Eyedropper in the real tool menu, sample transparent and then
		// painted pixels with SDL events, and inspect the actual color swatch.
		for i := range 3 {
			if !sdl.TestPointerEvent(100, 122, i) || !s.Step() {
				t.Fatal("could not reopen the native tool menu")
			}
		}
		capture("native-art-eyedropper-menu")
		for i := range 3 {
			if !sdl.TestPointerEvent(50, 367, i) || !s.Step() {
				t.Fatal("could not select the native Eyedropper")
			}
		}
		if s.App.Context().UI.AnyPopup() {
			t.Fatal("Eyedropper selection left the native menu open")
		}
		colorSwatch := func() []byte {
			t.Helper()
			var shot *image.RGBA
			var shotErr error
			s.Engine.CaptureFrame(func(img *image.RGBA, err error) { shot, shotErr = img, err })
			if !s.Step() || shotErr != nil || shot == nil {
				t.Fatalf("capture color swatch: %v", shotErr)
			}
			var pixels []byte
			for y := 116; y < 130; y++ {
				for x := 858; x < 875; x++ {
					c := shot.RGBAAt(x, y)
					pixels = append(pixels, c.R, c.G, c.B, c.A)
				}
			}
			return pixels
		}
		white := colorSwatch()
		for i := range 3 {
			if !sdl.TestPointerEvent(600, 400, i) || !s.Step() {
				t.Fatal("could not sample the transparent native canvas")
			}
		}
		transparent := colorSwatch()
		for i := range 3 {
			if !sdl.TestPointerEvent(500, 400, i) || !s.Step() {
				t.Fatal("could not sample the painted native canvas")
			}
		}
		pickedWhite := colorSwatch()
		if bytes.Equal(white, transparent) || !bytes.Equal(white, pickedWhite) || model.History.Cursor() != 1 {
			t.Fatal("native Eyedropper did not sample both colors without changing history")
		}
		capture("native-art-eyedropper")
		selectTool := func(y float32) {
			t.Helper()
			for i := range 3 {
				if !sdl.TestPointerEvent(100, 122, i) || !s.Step() {
					t.Fatal("could not open the native selection tool menu")
				}
			}
			for i := range 3 {
				if !sdl.TestPointerEvent(50, y, i) || !s.Step() {
					t.Fatal("could not choose the native selection tool")
				}
			}
		}
		selectTool(391) // Ellipse selection follows Eyedropper in the menu.
		for _, e := range []struct {
			x, y float32
			kind int
		}{{600, 400, 0}, {600, 400, 1}, {760, 540, 0}, {760, 540, 2}} {
			if !sdl.TestPointerEvent(e.x, e.y, e.kind) || !s.Step() {
				t.Fatal("native ellipse selection gesture failed")
			}
		}
		if sel := model.Session.Selection; sel == nil || sel.Count() == 0 || sel.Count() >= sel.Bounds().Dx()*sel.Bounds().Dy() || model.History.Cursor() != 1 {
			t.Fatal("native ellipse selection was not a non-rectangular mask without history")
		}
		capture("native-art-ellipse-selection")
		selectTool(415) // Lasso follows elliptical selection.
		for _, e := range []struct {
			x, y float32
			kind int
		}{{600, 400, 0}, {600, 400, 1}, {760, 400, 0}, {680, 540, 0}, {680, 540, 2}} {
			if !sdl.TestPointerEvent(e.x, e.y, e.kind) || !s.Step() {
				t.Fatal("native lasso selection gesture failed")
			}
		}
		if sel := model.Session.Selection; sel == nil || sel.Count() == 0 || sel.Count() >= sel.Bounds().Dx()*sel.Bounds().Dy() || model.History.Cursor() != 1 {
			t.Fatal("native lasso selection was not a non-rectangular mask without history")
		}
		capture("native-art-lasso-selection")
		if !sdl.TestEditKey(input.KeyEscape, true, false, false) || !s.Step() || !sdl.TestEditKey(input.KeyEscape, false, false, false) || !s.Step() {
			t.Fatal("could not clear the native selection with Escape")
		}
		if model.Session.Selection != nil || model.History.Cursor() != 1 {
			t.Fatal("Escape did not clear selection without changing history")
		}
		if os.Getenv("AEGO_ART_WORKFLOW_SHOTS") != "" {
			ws.ToggleMaximize(panelID)
			for range 3 {
				s.Step()
			}
			capture("native-art-docks")
			ws.ToggleMaximize(panelID)
		}
	}, func(t *testing.T, s *Session, panelID, stage string) {
		t.Helper()
		view := "scene"
		target := workspace.Scene
		if stage == "game-painted" || stage == "game-undo" || stage == "game-redo" {
			view, target = "game", workspace.Game
			if stage == "game-painted" {
				s.App.Play().Pause() // Keep every Game View capture at one simulation tick.
			}
		}
		ws := s.App.Workspace()
		if current := ws.Tree.Maximized(); current != "" && current != target {
			ws.ToggleMaximize(current)
		}
		ws.Show(target)
		ws.Focus(target)
		if ws.Tree.Maximized() != target && !ws.ToggleMaximize(target) {
			t.Fatalf("could not maximize %s", target)
		}
		for range 4 {
			if !s.Step() {
				t.Fatal("editor stopped before view capture")
			}
		}
		png, tick, scenePath, err := s.App.CaptureAgentView(view)
		if err != nil || scenePath != "scenes/art-workflow.aescene" {
			t.Fatalf("%s capture: scene=%q err=%v", stage, scenePath, err)
		}
		frames[stage] = png
		ticks[stage] = tick
		if dir := os.Getenv("AEGO_ART_WORKFLOW_SHOTS"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, stage+".png"), png, 0644); err != nil {
				t.Fatal(err)
			}
		}
		ws.ToggleMaximize(target)
		ws.Focus(panelID)
	})
	if bytes.Equal(frames["scene-before"], frames["scene-painted"]) {
		t.Fatal("Scene View did not visibly change after the unsaved stroke")
	}
	if ticks["game-painted"] != ticks["game-undo"] || ticks["game-painted"] != ticks["game-redo"] || bytes.Equal(frames["game-painted"], frames["game-undo"]) || !bytes.Equal(frames["game-painted"], frames["game-redo"]) {
		t.Fatal("Game View did not visibly follow Undo and Redo at the same tick")
	}
}
