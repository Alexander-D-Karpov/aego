//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"aego/art"
	"aego/core/log"
	"aego/editor/core/artedit"
	"aego/editor/core/workspace"
	"aego/platform/input"
	"aego/platform/sdl"
)

// Opens a test project through Go, then drives the native timeline through
// synthetic SDL pointer/text/key events; this is not physical OS/GPU input.
func TestArtTimelineDurationNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	p := newProject(t)
	drawing, err := art.New(16, 16, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	initial, err := drawing.Encode()
	drawing.Close()
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(p.Root, "assets", "duration.aeart")
	if err := os.WriteFile(filename, initial, 0644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1600, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s != nil {
			s.Close()
		}
	}()
	if !s.Step() {
		t.Fatal("editor stopped during startup")
	}
	ed, ws := s.App.Context(), s.App.Workspace()
	if !ws.Switch(workspace.Art) {
		t.Fatal("Art workspace is unavailable")
	}
	id, ok := ed.Pipeline.Lookup("assets/duration.aeart")
	if !ok {
		t.Fatal("drawing was not imported")
	}
	doc, err := ed.OpenAsset(id)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok {
		t.Fatal("drawing model is unavailable")
	}
	for range 8 {
		if !s.Step() {
			t.Fatal("editor stopped while laying out Art")
		}
	}
	timeline, visible := ws.Tree.PanelRect("art.timeline")
	if !visible {
		t.Fatal("Art timeline is not visible")
	}
	pad, row, gap := ed.UI.Theme.Padding, ed.UI.Theme.RowHeight, ed.UI.Theme.Spacing
	fieldX := timeline.X + pad + (timeline.W-2*pad)/6
	fieldY := timeline.Y + pad + 3*(row+gap) + row/2
	addX := timeline.X + pad + (timeline.W-2*pad)/8
	addY := timeline.Y + pad + 2*(row+gap) + row/2
	click := func(x, y float32) {
		t.Helper()
		for i := range 3 {
			if !sdl.TestPointerEvent(x, y, i) || !s.Step() {
				t.Fatal("SDL pointer input failed")
			}
		}
	}
	typeText := func(text string) {
		t.Helper()
		if !sdl.TestTextInput(text) || !s.Step() {
			t.Fatal("SDL text input failed")
		}
	}
	key := func(k input.Scancode, ctrl, shift bool) {
		t.Helper()
		if !sdl.TestEditKey(k, true, ctrl, shift) || !s.Step() || !sdl.TestEditKey(k, false, false, false) || !s.Step() {
			t.Fatal("SDL key input failed")
		}
	}
	capture := func(r image.Rectangle) []byte {
		t.Helper()
		var img *image.RGBA
		var captureErr error
		s.Engine.CaptureFrame(func(p *image.RGBA, err error) { img, captureErr = p, err })
		if !s.Step() || captureErr != nil || img == nil {
			t.Fatalf("timeline capture failed: %v", captureErr)
		}
		r = r.Intersect(img.Bounds())
		if r.Empty() {
			t.Fatal("timeline capture is empty")
		}
		var pixels []byte
		for y := r.Min.Y; y < r.Max.Y; y++ {
			start := img.PixOffset(r.Min.X, y)
			pixels = append(pixels, img.Pix[start:start+4*r.Dx()]...)
		}
		return pixels
	}
	// Compare the stable inside of the duration field, not its hover border.
	fieldW := (timeline.W - 2*pad - 8) / 3
	fieldRect := image.Rect(int(timeline.X+pad+5), int(fieldY-row/2+3), int(timeline.X+pad+min(fieldW-5, 80)), int(fieldY+row/2-3))
	want100 := capture(fieldRect)
	click(fieldX, fieldY)
	if !ed.UI.WantsKeyboard() {
		t.Fatal("duration field did not take keyboard focus")
	}
	typeText("250")
	key(input.KeyEnter, false, false)
	if frames := model.Session.Info().Document.Frames; len(frames) != 1 || frames[0].DurationMS != 250 {
		t.Fatalf("typing duration did not update the first frame: %+v", frames)
	}
	// Blur an uncommitted edit by clicking Add Frame. It belongs to the first
	// frame even though the new frame is selected on the same mouse release.
	click(fieldX, fieldY)
	typeText("275")
	click(addX, addY)
	frames := model.Session.Info().Document.Frames
	if len(frames) != 2 || frames[0].DurationMS != 275 || frames[1].DurationMS != 100 {
		t.Fatalf("SDL Add Frame moved the pending duration to the wrong frame: %+v", frames)
	}
	if got := capture(fieldRect); !bytes.Equal(got, want100) {
		t.Fatal("selected new frame's displayed duration does not match the original 100 ms field")
	}
	playY := timeline.Y + pad + row/2
	playW := (timeline.W - 2*pad - 12) / 4
	playRect := image.Rect(int(timeline.X+pad+4), int(playY-row/2+4), int(timeline.X+pad+min(playW-4, 70)), int(playY+row/2-4))
	moveAway := func() {
		t.Helper()
		if !sdl.TestPointerEvent(10, 10, 0) || !s.Step() {
			t.Fatal("SDL pointer move failed")
		}
	}
	moveAway()
	paused := capture(playRect)
	click(addX, playY)
	moveAway()
	playing := capture(playRect)
	if bytes.Equal(paused, playing) {
		t.Fatal("Play did not change its native toggle")
	}
	click(addX, playY)
	moveAway()
	if got := capture(playRect); !bytes.Equal(got, paused) {
		t.Fatal("Pause did not restore its native toggle")
	}
	// Focus the drawing before routing the ordinary Undo/Redo and Save shortcuts.
	ws.Focus("render2d.art/" + id.String())
	key(input.KeyZ, true, false)
	if frames = model.Session.Info().Document.Frames; len(frames) != 1 || frames[0].DurationMS != 275 {
		t.Fatalf("SDL Undo did not remove the added frame: %+v", frames)
	}
	key(input.KeyZ, true, true)
	if frames = model.Session.Info().Document.Frames; len(frames) != 2 || frames[0].DurationMS != 275 || frames[1].DurationMS != 100 {
		t.Fatalf("SDL Redo did not restore both durations: %+v", frames)
	}
	// The visible trash button removes the current frame, protects the final
	// frame, and participates in the same keyboard Undo history as Add Frame.
	beforeDelete := slices.Clone(frames)
	deletedID := model.Session.Frame
	remaining := slices.DeleteFunc(slices.Clone(frames), func(f art.Frame) bool { return f.ID == deletedID })
	contentWidth := timeline.W - 2*pad
	minimumHeight := 2*pad + 6*(row+gap) + 2*gap
	if timeline.H < minimumHeight {
		contentWidth -= ed.UI.Theme.ScrollbarW + gap
	}
	if contentWidth < 140 {
		t.Fatal("the factory timeline is unexpectedly narrow for the native delete test")
	}
	controlsWidth := min(contentWidth, 480)
	buttonWidth := (controlsWidth - 16) / 5
	deleteX := timeline.X + pad + 4*(buttonWidth+4) + buttonWidth/2
	deleteY := timeline.Y + pad + 2*(row+gap) + row/2
	beforeCursor := model.History.Cursor()
	click(deleteX, deleteY)
	if got := model.Session.Info().Document.Frames; !slices.Equal(got, remaining) || model.History.Cursor() != beforeCursor+1 {
		t.Fatalf("native Delete Frame did not remove the selected frame once: %+v", got)
	}
	click(deleteX, deleteY)
	if got := model.Session.Info().Document.Frames; !slices.Equal(got, remaining) || model.History.Cursor() != beforeCursor+1 {
		t.Fatalf("native Delete Frame did not protect the last frame: %+v", got)
	}
	ws.Focus("render2d.art/" + id.String())
	key(input.KeyZ, true, false)
	if got := model.Session.Info().Document.Frames; !slices.Equal(got, beforeDelete) || model.History.Cursor() != beforeCursor {
		t.Fatalf("native Undo did not restore the deleted frame: %+v", got)
	}
	// Seed a pixel through the fixture API, then exercise only real SDL
	// shortcuts for Copy/Cut/Paste. Each mutation must reach one history stack.
	_, err = model.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 4, Y: 4}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}})
	if err != nil {
		t.Fatal(err)
	}
	cursor := model.History.Cursor()
	key(input.KeyC, true, false)
	if model.History.Cursor() != cursor {
		t.Fatal("SDL Copy changed drawing history")
	}
	key(input.KeyX, true, false)
	cut, _, err := model.Capture(model.Session.Frame, art.Albedo)
	if err != nil || cut == nil || model.History.Cursor() != cursor+1 || cut.NRGBAAt(4, 4).A != 0 {
		t.Fatalf("SDL Cut did not create one art edit: %v", err)
	}
	key(input.KeyV, true, false)
	pasted, _, err := model.Capture(model.Session.Frame, art.Albedo)
	if err != nil || pasted == nil || model.History.Cursor() != cursor+2 || pasted.NRGBAAt(4, 4).A != 255 {
		t.Fatalf("SDL Paste did not create one art edit: %v", err)
	}
	key(input.KeyS, true, false)
	if doc.Dirty() {
		t.Fatal("SDL Save left the drawing dirty")
	}
	saved, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := art.Decode(saved, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	frames = reopened.Info().Frames
	if len(frames) != 2 || frames[0].DurationMS != 275 || frames[1].DurationMS != 100 {
		t.Fatalf("saved drawing lost its frame durations: %+v", frames)
	}
	s.Close()
	s = nil
	again, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1600, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if !again.Step() {
		t.Fatal("editor stopped after reopening the project")
	}
	reopenedDoc, err := again.App.Context().OpenAsset(id)
	if err != nil {
		t.Fatal(err)
	}
	reopenedModel, ok := reopenedDoc.Binding.Model.(*artedit.Document)
	if !ok {
		t.Fatal("reopened drawing did not bind to the native editor")
	}
	frames = reopenedModel.Session.Info().Document.Frames
	if len(frames) != 2 || frames[0].DurationMS != 275 || frames[1].DurationMS != 100 {
		t.Fatalf("reopened editor lost its frame durations: %+v", frames)
	}
}
