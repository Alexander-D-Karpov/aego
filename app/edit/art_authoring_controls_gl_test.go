//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

// Real native authoring controls. Fixture creation/opening is setup; every
// operation under test is sent through SDL controls. The Art model is read only
// for assertions and mapping source pixels to the actual Canvas draw rectangle.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"aego/art"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/platform/input"
	"aego/platform/sdl"
	"aego/ui/draw"
)

type artControlsArtUI struct {
	t     *testing.T
	s     *Session
	model *artedit.Document
	panel string
	dir   string
}

func artControlsOpenArt(t *testing.T, name string) *artControlsArtUI {
	t.Helper()
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
	const source = "assets/controls.aeart"
	if err := os.WriteFile(filepath.Join(p.Root, source), data, 0644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	interopStep(t, s)
	doc, err := s.App.Context().OpenAssetPath(source)
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok {
		s.Close()
		t.Fatal("supplement fixture did not open as an Art document")
	}
	f := &artControlsArtUI{t: t, s: s, model: model, panel: "render2d.art/" + doc.Asset.String()}
	if root := os.Getenv("AEGO_ART_CONTROLS_SHOTS"); root != "" {
		f.dir = filepath.Join(root, name)
		if err := os.MkdirAll(f.dir, 0755); err != nil {
			s.Close()
			t.Fatal(err)
		}
	}
	f.maximize(f.panel)
	t.Log("setup: blank RGBA16 document created/opened using existing host; subsequent tested authoring actions use SDL")
	return f
}

func (f *artControlsArtUI) close() {
	if f.t.Failed() && f.dir != "" {
		// A useful screenshot is captured while the actual editor is still alive.
		interopShot(f.t, f.s, filepath.Join(f.dir, "failure.png"))
	}
	f.s.Close()
}

func (f *artControlsArtUI) step() { f.t.Helper(); interopStep(f.t, f.s) }
func (f *artControlsArtUI) maximize(id string) mathx.Rect {
	f.t.Helper()
	return interopMaximize(f.t, f.s, id)
}
func (f *artControlsArtUI) click(p mathx.Vec2) {
	f.t.Helper()
	interopClick(f.t, f.s, p.X, p.Y)
}
func (f *artControlsArtUI) key(k input.Scancode, ctrl bool) {
	f.t.Helper()
	interopKey(f.t, f.s, k, ctrl)
}
func (f *artControlsArtUI) text(value string) {
	f.t.Helper()
	f.key(input.KeyA, true)
	// SDL_TEXTINPUT has a bounded payload. All test strings are ASCII.
	for len(value) > 0 {
		n := min(20, len(value))
		if !sdl.TestTextInput(value[:n]) {
			f.t.Fatal("SDL text input failed")
		}
		f.step()
		value = value[n:]
	}
}

func artControlsInside(r, clip mathx.Rect) bool {
	return r.X >= clip.X-.1 && r.Y >= clip.Y-.1 && r.X+r.W <= clip.X+clip.W+.1 && r.Y+r.H <= clip.Y+clip.H+.1
}

func (f *artControlsArtUI) label(text string) draw.TestDrawnItem {
	f.t.Helper()
	ui := f.s.App.Context().UI
	screen := mathx.Rect{W: ui.Size().X, H: ui.Size().Y}
	var matches []draw.TestDrawnItem
	for _, item := range f.s.App.UIList().TestDrawnItems(screen, ui.Font(), text, 0, 0) {
		if item.Text == text && (item.Clip != screen || ui.AnyPopup()) && artControlsInside(item.Bounds, item.Clip) {
			matches = append(matches, item)
		}
	}
	if len(matches) != 1 {
		f.t.Fatalf("expected one complete real label %q, got %d: %+v", text, len(matches), matches)
	}
	return matches[0]
}

func (f *artControlsArtUI) clickLabel(text string) {
	f.t.Helper()
	f.click(f.label(text).Bounds.Center())
}

func (f *artControlsArtUI) menu(panel, title, item string) {
	f.t.Helper()
	f.maximize(panel)
	f.clickLabel(title)
	if !f.s.App.Context().UI.AnyPopup() {
		f.t.Fatalf("actual menu %q did not open", title)
	}
	// BeginPopup deliberately uses a zero clip for its first size-probe frame.
	// Wait for the next real layout before locating a visible menu item.
	f.step()
	f.clickLabel(item)
	if f.s.App.Context().UI.AnyPopup() {
		f.t.Fatalf("actual menu entry %q did not close its popup", item)
	}
	// A selected action can enqueue the existing Question for the next frame.
	f.step()
}

func (f *artControlsArtUI) tool(name string) {
	f.t.Helper()
	f.maximize("art.tools")
	f.clickLabel(name)
	f.maximize(f.panel)
}

func (f *artControlsArtUI) brushToggle(label string) {
	f.t.Helper()
	f.maximize("art.tools")
	f.clickLabel(label)
	f.maximize(f.panel)
}

func (f *artControlsArtUI) swatch(index int) art.Color {
	f.t.Helper()
	if index < 0 || index > 7 {
		f.t.Fatal("this artControls palette locator intentionally tests the first visible page only")
	}
	r := f.maximize("art.properties")
	ui := f.s.App.Context().UI
	channel := f.label("Albedo")
	// Native properties put the real swatches one row below the surface Combo.
	// Derive Y from the actual rendered label, and X from the current panel.
	width := r.W - 2*ui.Theme.Padding
	point := mathx.V2(r.X+ui.Theme.Padding+width*(float32(index)+1.5)/10, channel.Bounds.Center().Y+ui.Theme.RowHeight+ui.Theme.Spacing)
	if !r.Contains(point) {
		f.t.Fatal("swatch is outside the actual properties panel")
	}
	want := f.model.Session.Document.Info().Palette[index].Color
	f.click(point)
	f.maximize(f.panel)
	return want
}

func (f *artControlsArtUI) canvasTexture() (mathx.Rect, mathx.Rect) {
	f.t.Helper()
	ui := f.s.App.Context().UI
	panel, visible := f.s.App.Workspace().Tree.PanelRect(f.panel)
	if !visible {
		f.t.Fatal("the actual Canvas is not visible")
	}
	size := f.model.Session.Document.Info().Canvas
	var found []draw.TestDrawnItem
	for _, item := range f.s.App.UIList().TestDrawnItems(mathx.Rect{W: ui.Size().X, H: ui.Size().Y}, ui.Font(), "", size.Width, size.Height) {
		if item.Bounds.W > float32(size.Width) && item.Bounds.H > float32(size.Height) && artControlsInside(item.Bounds, panel) {
			found = append(found, item)
		}
	}
	if len(found) != 1 {
		f.t.Fatalf("expected one real source-sized Canvas texture, got %d", len(found))
	}
	return found[0].Bounds, found[0].Clip
}

func (f *artControlsArtUI) point(p image.Point) mathx.Vec2 {
	f.t.Helper()
	r, clip := f.canvasTexture()
	c := f.model.Session.Document.Info().Canvas
	at := mathx.V2(r.X+(float32(p.X)+.5)*r.W/float32(c.Width), r.Y+(float32(p.Y)+.5)*r.H/float32(c.Height))
	if p.X < 0 || p.Y < 0 || p.X >= c.Width || p.Y >= c.Height || !clip.Contains(at) {
		f.t.Fatalf("source pixel %v is not visible in actual Canvas: %v %+v", p, at, clip)
	}
	return at
}

func (f *artControlsArtUI) stroke(points ...image.Point) {
	f.t.Helper()
	if len(points) == 0 {
		f.t.Fatal("empty SDL gesture")
	}
	at := f.point(points[0])
	for _, kind := range []int{0, 1} {
		if !sdl.TestPointerEvent(at.X, at.Y, kind) {
			f.t.Fatal("SDL stroke start failed")
		}
		f.step()
	}
	for _, p := range points[1:] {
		at = f.point(p)
		if !sdl.TestPointerEvent(at.X, at.Y, 0) {
			f.t.Fatal("SDL stroke motion failed")
		}
		f.step()
	}
	if !sdl.TestPointerEvent(at.X, at.Y, 2) {
		f.t.Fatal("SDL stroke release failed")
	}
	f.step()
}

func (f *artControlsArtUI) pixels() *image.NRGBA {
	f.t.Helper()
	pixels, _, err := f.model.Capture(f.model.Session.Frame, art.Albedo)
	if err != nil {
		f.t.Fatal(err)
	}
	return pixels
}

func (f *artControlsArtUI) snapshot() []byte {
	f.t.Helper()
	data, err := f.model.Session.Document.Encode()
	if err != nil {
		f.t.Fatal(err)
	}
	return data
}

func (f *artControlsArtUI) expectPixels(want map[image.Point]art.Color) {
	f.t.Helper()
	pixels := f.pixels()
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			point := image.Pt(x, y)
			c := want[point]
			expected := color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A}
			if got := pixels.NRGBAAt(x, y); got != expected {
				f.t.Fatalf("actual SDL result pixel %v=%+v, want %+v", point, got, expected)
			}
		}
	}
}

func (f *artControlsArtUI) undoTo(cursor int) {
	f.t.Helper()
	f.maximize(f.panel)
	for count := 0; f.model.History.Cursor() > cursor && count < 20; count++ {
		before := f.model.History.Cursor()
		f.key(input.KeyZ, true)
		if f.model.History.Cursor() != before-1 {
			f.t.Fatal("one native Ctrl+Z did not undo exactly one operation")
		}
	}
	if f.model.History.Cursor() != cursor {
		f.t.Fatal("native Undo did not reach requested history position")
	}
}

func (f *artControlsArtUI) modalField(label, current, value string) {
	f.t.Helper()
	ui := f.s.App.Context().UI
	if !ui.ModalActive() {
		f.t.Fatal("expected actual existing Question modal", label)
	}
	caption := f.label(label)
	var field mathx.Rect
	screen := mathx.Rect{W: ui.Size().X, H: ui.Size().Y}
	for _, item := range f.s.App.UIList().TestDrawnItems(screen, ui.Font(), current, 0, 0) {
		r := item.Clip
		if item.Text == current && artControlsInside(item.Bounds, r) && r.H == ui.Theme.RowHeight && r.X >= caption.Clip.X && r.X+r.W <= caption.Clip.X+caption.Clip.W && r.Y >= caption.Bounds.Y+caption.Bounds.H && r.Y+r.H <= caption.Clip.Y+caption.Clip.H && (field.H == 0 || r.Y < field.Y) {
			field = r
		}
	}
	if field.H == 0 {
		f.t.Fatalf("actual modal input below %q with value %q is not visible", label, current)
	}
	f.click(field.Center())
	if ui.Focus() == 0 {
		f.t.Fatal("actual input click did not acquire native text focus")
	}
	f.text(value)
}

func (f *artControlsArtUI) applyModal() {
	f.t.Helper()
	if !f.s.App.Context().UI.ModalActive() {
		f.t.Fatal("Question closed before Apply")
	}
	f.clickLabel("Apply")
	if f.s.App.Context().UI.ModalActive() {
		f.t.Fatal("real Apply button did not submit Question")
	}
}

func (f *artControlsArtUI) shot(name string) *image.RGBA {
	f.t.Helper()
	var img *image.RGBA
	var captureErr error
	f.s.Engine.CaptureFrame(func(value *image.RGBA, err error) { img, captureErr = value, err })
	f.step()
	if img == nil || captureErr != nil {
		f.t.Fatal("actual native GPU screenshot failed", captureErr)
	}
	if f.dir != "" {
		out, err := os.Create(filepath.Join(f.dir, name+".png"))
		if err != nil {
			f.t.Fatal(err)
		}
		writeErr := png.Encode(out, img)
		closeErr := out.Close()
		if writeErr != nil || closeErr != nil {
			f.t.Fatal(writeErr, closeErr)
		}
	}
	return img
}

func (f *artControlsArtUI) receipt(value any) {
	f.t.Helper()
	if f.dir == "" {
		return
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.dir, "assertions.json"), append(data, '\n'), 0644); err != nil {
		f.t.Fatal(err)
	}
}

func TestArtFillPixelPerfectNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f := artControlsOpenArt(t, "fill-pixel-perfect")
	defer f.close()
	border := f.swatch(6)
	f.tool("Rectangle")
	f.stroke(image.Pt(2, 2), image.Pt(7, 7))
	if f.model.History.Cursor() != 1 {
		t.Fatal("actual rectangle was not one operation")
	}
	borderOnly := map[image.Point]art.Color{}
	for y := 2; y <= 7; y++ {
		for x := 2; x <= 7; x++ {
			if x == 2 || x == 7 || y == 2 || y == 7 {
				borderOnly[image.Pt(x, y)] = border
			}
		}
	}
	f.expectPixels(borderOnly)
	f.shot("00-rectangle-before-fill")
	fill := f.swatch(2)
	f.tool("Fill")
	f.stroke(image.Pt(4, 4))
	want := map[image.Point]art.Color{}
	for y := 2; y <= 7; y++ {
		for x := 2; x <= 7; x++ {
			c := fill
			if x == 2 || x == 7 || y == 2 || y == 7 {
				c = border
			}
			want[image.Pt(x, y)] = c
		}
	}
	f.expectPixels(want)
	if f.model.History.Cursor() != 2 {
		t.Fatal("actual Fill did not create exactly one operation")
	}
	f.shot("01-closed-region-fill")
	f.undoTo(0)
	f.expectPixels(nil)
	white := f.swatch(5)
	f.tool("Pencil")
	// The existing default is perfect=true. Demonstrate both actual toggle states,
	// not just the default and not a direct write to artView or Brush.
	f.brushToggle("Pixel perfect")
	f.stroke(image.Pt(2, 2), image.Pt(3, 2), image.Pt(3, 3))
	f.expectPixels(map[image.Point]art.Color{image.Pt(2, 2): white, image.Pt(3, 2): white, image.Pt(3, 3): white})
	f.shot("02-ordinary-corner")
	f.undoTo(0)
	f.brushToggle("Pixel perfect")
	f.stroke(image.Pt(2, 2), image.Pt(3, 2), image.Pt(3, 3))
	f.expectPixels(map[image.Point]art.Color{image.Pt(2, 2): white, image.Pt(3, 3): white})
	if f.model.History.Cursor() != 1 {
		t.Fatal("pixel-perfect gesture did not commit once")
	}
	f.shot("03-pixel-perfect-corner")
	f.undoTo(0)
	f.expectPixels(nil)
	f.receipt(map[string]any{"result": "PASS", "input": "SDL only after blank fixture", "fillInterior": 16, "borderPixels": 20, "ordinaryCornerPixels": 3, "perfectCornerPixels": 2, "historyAfterUndo": 0})
}

func TestArtColorModesRemapNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f := artControlsOpenArt(t, "color-modes-remap")
	defer f.close()
	f.menu("art.properties", "Palette…", "Convert to indexed palette")
	if f.model.Session.Document.Info().Canvas.Mode != art.Indexed {
		t.Fatal("actual native palette menu did not convert to Indexed")
	}
	red := f.swatch(6)
	f.tool("Pencil")
	f.stroke(image.Pt(3, 3))
	paint, err := f.model.Session.SamplePaint(image.Pt(3, 3))
	if err != nil || !paint.UseIndex || paint.Index != 6 || paint.Color != red {
		t.Fatal("native indexed stroke did not store the selected palette index", paint, err)
	}
	before := bytes.Clone(f.pixels().Pix)
	cursor := f.model.History.Cursor()
	f.menu("art.properties", "Palette…", "Move swatch right (preserve pixels)")
	paint, err = f.model.Session.SamplePaint(image.Pt(3, 3))
	if err != nil || !paint.UseIndex || paint.Index != 7 || paint.Color != red || !bytes.Equal(before, f.pixels().Pix) || f.model.History.Cursor() != cursor+1 {
		t.Fatal("native reorder did not preserve rendered pixels while remapping actual indices", paint, err)
	}
	f.maximize(f.panel)
	f.shot("01-indexed-reorder-preserves-pixels")
	f.menu("art.properties", "Palette…", "Remap palette index…")
	f.modalField("Destination index", "7", "2")
	f.applyModal()
	paint, err = f.model.Session.SamplePaint(image.Pt(3, 3))
	want := f.model.Session.Document.Info().Palette[2].Color
	if err != nil || paint.Index != 2 || paint.Color != want || bytes.Equal(before, f.pixels().Pix) || f.model.History.Cursor() != cursor+2 {
		t.Fatal("native remap did not intentionally change index/color once", paint, err)
	}
	f.maximize(f.panel)
	f.shot("02-remapped-color")
	f.undoTo(cursor + 1)
	paint, err = f.model.Session.SamplePaint(image.Pt(3, 3))
	if err != nil || paint.Index != 7 || !bytes.Equal(before, f.pixels().Pix) {
		t.Fatal("common Undo did not restore the reordered indexed pixels", paint, err)
	}
	f.menu("art.properties", "Palette…", "Convert to grayscale")
	if f.model.Session.Document.Info().Canvas.Mode != art.Grayscale || f.pixels().NRGBAAt(3, 3) != (color.NRGBA{151, 151, 151, 255}) {
		t.Fatal("native grayscale conversion has incorrect mode/luminance")
	}
	chosen := f.swatch(2)
	if chosen != (art.Color{R: 52, G: 62, B: 85, A: 255}) {
		t.Fatal("fixture palette changed; review explicit grayscale expectation", chosen)
	}
	f.stroke(image.Pt(10, 10))
	if got := f.pixels().NRGBAAt(10, 10); got != (color.NRGBA{62, 62, 62, 255}) {
		t.Fatal("native grayscale painting did not apply its color-mode rules", got)
	}
	gray := bytes.Clone(f.pixels().Pix)
	f.shot("03-grayscale-painted")
	f.menu("art.properties", "Palette…", "Convert to RGBA")
	if f.model.Session.Document.Info().Canvas.Mode != art.RGBA || !bytes.Equal(gray, f.pixels().Pix) {
		t.Fatal("native RGBA conversion changed grayscale image")
	}
	f.maximize(f.panel)
	f.key(input.KeyZ, true)
	if f.model.Session.Document.Info().Canvas.Mode != art.Grayscale || !bytes.Equal(gray, f.pixels().Pix) {
		t.Fatal("common Undo did not restore grayscale mode and pixels")
	}
	f.receipt(map[string]any{"result": "PASS", "input": "SDL palette menu/swatches/dialogs/Pencil/Undo", "storedIndex": 6, "reorderedIndex": 7, "remappedIndex": 2, "reorderPixelInvariant": true, "grayscaleFromRed": 151, "grayscalePaint": 62, "rgbaPreservesGray": true})
}

func TestArtSelectionTransformNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f := artControlsOpenArt(t, "selection-transform")
	defer f.close()
	white := f.swatch(5)
	f.tool("Pencil")
	for _, point := range []image.Point{image.Pt(2, 2), image.Pt(2, 3), image.Pt(3, 3)} {
		f.stroke(point)
	}
	sentinel := f.swatch(6)
	f.stroke(image.Pt(12, 12))
	before := bytes.Clone(f.pixels().Pix)
	cursor := f.model.History.Cursor()
	f.tool("Selection")
	f.stroke(image.Pt(2, 2), image.Pt(3, 3))
	if f.model.Session.Selection == nil || f.model.Session.Selection.Bounds() != image.Rect(2, 2, 4, 4) || f.model.History.Cursor() != cursor {
		t.Fatal("actual rectangular selection is not the intended 2x2 mask")
	}
	f.menu("art.properties", "Pixels…", "Rotate 90° clockwise")
	f.expectPixels(map[image.Point]art.Color{image.Pt(2, 2): white, image.Pt(3, 2): white, image.Pt(2, 3): white, image.Pt(12, 12): sentinel})
	if f.model.History.Cursor() != cursor+1 {
		t.Fatal("actual selection rotation was not one operation")
	}
	f.maximize(f.panel)
	f.shot("01-rotated-selection")
	f.undoTo(cursor)
	if !bytes.Equal(before, f.pixels().Pix) {
		t.Fatal("native Undo lost pre-rotation pixels")
	}
	f.menu("art.properties", "Pixels…", "Scale pixels…")
	f.modalField("Width", "2", "4")
	f.modalField("Height", "2", "4")
	f.applyModal()
	want := map[image.Point]art.Color{image.Pt(12, 12): sentinel}
	// Explicit expected 2x nearest-neighbor expansion of [white,transparent;
	// white,white]. Outside the transformed area the sentinel is untouched.
	for y := 2; y < 6; y++ {
		for x := 2; x < 6; x++ {
			if x < 4 || y >= 4 {
				want[image.Pt(x, y)] = white
			}
		}
	}
	f.expectPixels(want)
	if f.model.History.Cursor() != cursor+1 {
		t.Fatal("actual Scale dialog was not one operation")
	}
	f.maximize(f.panel)
	f.shot("02-scaled-selection")
	f.undoTo(cursor)
	if !bytes.Equal(before, f.pixels().Pix) {
		t.Fatal("native Undo lost pre-scale pixels")
	}
	f.receipt(map[string]any{"result": "PASS", "input": "SDL selection/rotate/scale dialog/common Undo", "selection": image.Rect(2, 2, 4, 4), "scaledExtent": image.Rect(2, 2, 6, 6), "sentinelPreserved": image.Pt(12, 12), "all256PixelsChecked": true})
}

func (f *artControlsArtUI) setDuration(current, next string) {
	f.t.Helper()
	f.maximize("art.timeline")
	info := f.model.Session.Document.Info()
	f.t.Logf("duration input before editing: selected frame=%s canonical frames=%+v expected label=%s", f.model.Session.Frame, info.Frames, current)
	item := f.label(current)
	if item.Clip.H != f.s.App.Context().UI.Theme.RowHeight {
		f.t.Fatal("duration label does not belong to a real text input", item)
	}
	f.click(item.Clip.Center())
	f.text(next)
	f.key(input.KeyEnter, false)
}

func (f *artControlsArtUI) selectFrame(index, duration int) {
	f.t.Helper()
	f.maximize("art.timeline")
	// The real grid intentionally elides duration labels in its ~48px cells.
	// Locate the rendered frame-number prefix, never an assumed old tile point.
	ui := f.s.App.Context().UI
	prefix := fmt.Sprintf("%d · ", index)
	var headers []draw.TestDrawnItem
	for _, item := range f.s.App.UIList().TestDrawnItems(mathx.Rect{W: ui.Size().X, H: ui.Size().Y}, ui.Font(), prefix, 0, 0) {
		if strings.HasPrefix(item.Text, prefix) && artControlsInside(item.Bounds, item.Clip) {
			headers = append(headers, item)
		}
	}
	if len(headers) != 1 {
		f.t.Fatal("expected one visible real frame-header prefix", prefix, headers)
	}
	f.click(headers[0].Bounds.Center())
	info := f.model.Session.Document.Info()
	if f.model.Session.Frame != info.Frames[index-1].ID || info.Frames[index-1].DurationMS != duration {
		f.t.Fatal("actual Timeline header did not select requested frame")
	}
	f.maximize(f.panel)
}

type artControlsSignature [3]color.RGBA

func (f *artControlsArtUI) signature(img *image.RGBA) artControlsSignature {
	f.t.Helper()
	var out artControlsSignature
	for i, p := range []image.Point{image.Pt(2, 2), image.Pt(4, 4), image.Pt(6, 6)} {
		at := f.point(p)
		if !image.Pt(int(at.X), int(at.Y)).In(img.Rect) {
			f.t.Fatal("actual source-pixel position is outside GPU screenshot")
		}
		out[i] = img.RGBAAt(int(at.X), int(at.Y))
	}
	return out
}

func TestArtOnionTagsDurationNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f := artControlsOpenArt(t, "onion-tags-duration")
	defer f.close()
	f.swatch(6)
	f.tool("Pencil")
	f.stroke(image.Pt(2, 2))
	f.setDuration("100", "240")
	firstFrame := f.model.Session.Frame
	durationCursor := f.model.History.Cursor()
	f.maximize(f.panel)
	f.key(input.KeyZ, true)
	if current := f.model.Session.Document.Info().Frames[0]; current.ID != firstFrame || current.DurationMS != 100 || f.model.History.Cursor() != durationCursor-1 {
		t.Fatal("common native Undo did not restore duration100 on the same frame", current)
	}
	f.maximize("art.timeline")
	t.Log("same-frame duration Undo restored canonical100; checking the actual visible field")
	f.label("100")
	f.shot("00-duration-undo-100")
	f.maximize(f.panel)
	// The existing global EditRedo shortcut is Primary+Shift+Z.
	for _, down := range []bool{true, false} {
		if !sdl.TestEditKey(input.KeyZ, down, down, down) {
			t.Fatal("SDL shared Redo key failed")
		}
		f.step()
	}
	if current := f.model.Session.Document.Info().Frames[0]; current.ID != firstFrame || current.DurationMS != 240 || f.model.History.Cursor() != durationCursor {
		t.Fatal("common native Redo did not restore duration240 on the same frame", current)
	}
	f.maximize("art.timeline")
	f.label("240")
	f.shot("00-duration-redo-240")
	// Unconfirmed typing stays local to the real input buffer across redraws.
	field := f.label("240")
	f.click(field.Clip.Center())
	f.text("360")
	for range 3 {
		f.step()
	}
	f.label("360")
	if f.model.Session.Document.Info().Frames[0].DurationMS != 240 || f.model.History.Cursor() != durationCursor {
		t.Fatal("unconfirmed duration typing mutated the authoring document")
	}
	f.key(input.KeyEscape, false)
	f.label("240")
	f.clickLabel("+ Frame")
	f.swatch(5)
	f.tool("Pencil")
	f.stroke(image.Pt(4, 4))
	f.setDuration("100", "400")
	f.maximize("art.timeline")
	f.clickLabel("+ Frame")
	f.swatch(3)
	f.tool("Pencil")
	f.stroke(image.Pt(6, 6))
	f.setDuration("100", "240")
	info := f.model.Session.Document.Info()
	if len(info.Frames) != 3 || info.Frames[0].DurationMS != 240 || info.Frames[1].DurationMS != 400 || info.Frames[2].DurationMS != 240 {
		t.Fatal("actual Timeline duration fields did not apply independently", info.Frames)
	}
	frameIDs := []art.ID{info.Frames[0].ID, info.Frames[1].ID, info.Frames[2].ID}
	f.maximize(f.panel)
	off := f.signature(f.shot("01-onion-off"))
	snapshot, cursor := f.snapshot(), f.model.History.Cursor()
	f.maximize("art.timeline")
	f.clickLabel("Onion skin")
	f.maximize(f.panel)
	on := f.signature(f.shot("02-onion-on"))
	if off[1] == on[1] || off[2] != on[2] || !bytes.Equal(snapshot, f.snapshot()) || f.model.History.Cursor() != cursor {
		t.Fatal("actual Onion toggle did not add previous-frame GPU ghost without altering current pixels/history", off, on)
	}
	if c := f.pixels().NRGBAAt(4, 4); c.A != 0 {
		t.Fatal("Onion ghost leaked into canonical current-frame pixels", c)
	}
	f.maximize("art.timeline")
	f.clickLabel("Onion skin")
	f.maximize(f.panel)
	if restored := f.signature(f.shot("03-onion-off-restored")); restored != off {
		t.Fatal("disabling Onion did not restore actual Canvas pixels", restored, off)
	}
	f.menu("art.timeline", "Frames / Tags…", "New animation tag…")
	// Name starts focused in the existing Question; subsequent fields are located
	// by their real rendered captions and input clips, not old dialog coordinates.
	f.text("LoopProof")
	f.modalField("Direction: forward / reverse / ping-pong", "forward", "ping-pong")
	f.applyModal()
	info = f.model.Session.Document.Info()
	if len(info.Tags) != 1 || info.Tags[0].Name != "LoopProof" || info.Tags[0].From != frameIDs[0] || info.Tags[0].To != frameIDs[2] || info.Tags[0].Direction != art.PingPong || f.model.History.Cursor() != cursor+1 {
		t.Fatal("actual tag dialog did not publish one exact range/direction operation", info.Tags)
	}
	f.maximize(f.panel)
	f.shot("04-created-tag")
	var signatures [3]artControlsSignature
	for i, ms := range []int{240, 400, 240} {
		f.selectFrame(i+1, ms)
		signatures[i] = f.signature(f.shot(fmt.Sprintf("reference-frame-%d", i+1)))
	}
	if signatures[0] == signatures[1] || signatures[1] == signatures[2] || signatures[0] == signatures[2] {
		t.Fatal("actual GPU frame references are not distinguishable")
	}
	beforePlayback := f.snapshot()
	history := f.model.History.Cursor()
	f.maximize("art.timeline")
	f.clickLabel("Play")
	f.maximize(f.panel)
	type transition struct {
		Frame int     `json:"frame"`
		Time  float64 `json:"uiSeconds"`
	}
	var transitions []transition
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) && len(transitions) < 5 {
		var shot *image.RGBA
		var captureErr error
		f.s.Engine.CaptureFrame(func(img *image.RGBA, err error) { shot, captureErr = img, err })
		f.step()
		if captureErr != nil || shot == nil {
			t.Fatal("actual playback screenshot failed", captureErr)
		}
		value := f.signature(shot)
		index := -1
		for i, expected := range signatures {
			if value == expected {
				index = i + 1
			}
		}
		if index < 0 {
			t.Fatal("playback Canvas did not match any actual authored frame signature", value)
		}
		if len(transitions) == 0 || transitions[len(transitions)-1].Frame != index {
			transitions = append(transitions, transition{Frame: index, Time: f.s.App.Context().UI.Time})
		}
		runtime.Gosched()
		time.Sleep(5 * time.Millisecond)
	}
	want := []int{1, 2, 3, 2, 1}
	if len(transitions) != len(want) {
		t.Fatal("actual bounded ping-pong playback did not visit required frames", transitions)
	}
	for i, frame := range want {
		if transitions[i].Frame != frame {
			t.Fatal("actual tag playback direction/order is incorrect", transitions)
		}
	}
	// Observe the complete middle-frame dwell. The 400 ms user duration must
	// affect playback, rather than falling back to a fixed 100 ms interval.
	dwell := transitions[2].Time - transitions[1].Time
	if dwell < .30 || dwell > .60 {
		t.Fatalf("actual 400 ms duration was not reflected in playback: %.6fs %+v", dwell, transitions)
	}
	if !bytes.Equal(beforePlayback, f.snapshot()) || f.model.History.Cursor() != history {
		t.Fatal("preview playback changed canonical drawing or History")
	}
	f.maximize("art.timeline")
	f.clickLabel("Play")
	f.maximize(f.panel)
	f.shot("05-playback-stopped")
	f.receipt(map[string]any{"result": "PASS", "input": "SDL Timeline/frame/duration/Onion/tag/Play controls", "durationsMS": []int{240, 400, 240}, "direction": "ping-pong", "transitions": transitions, "observedMiddleDwellSeconds": dwell, "onionChangesGPUOnly": true, "canonicalBytesUnchangedByPreview": true, "durationUndoRedoUpdatesVisibleField": true, "unconfirmedDurationPreservedUntilEsc": true})
}
