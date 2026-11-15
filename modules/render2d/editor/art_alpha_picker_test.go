package editor

import (
	"bytes"
	stdctx "context"
	"fmt"
	"image"
	"reflect"
	"testing"

	"aego/art"
	"aego/core/mathx"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/theme"
	"aego/ui/widgets"
)

// These drive the real picker with synthetic input. They do not replace a
// screenshot or physical OS/GPU acceptance test.
type artAlphaHarness struct {
	p           *artPanel
	ui          *core.Context
	state       *input.State
	area, clip  mathx.Rect
	bar, number mathx.Rect
}

func newArtAlphaHarness(t *testing.T, width float32, light bool) *artAlphaHarness {
	t.Helper()
	p := artActionFixture(t)
	ui := p.ctx.UI
	if light {
		ui.Theme = theme.Light()
	} else {
		ui.Theme = theme.Editor()
	}
	font := text.NewFont(14, 1, 16, 12)
	for r := rune(32); r <= 0x04ff; r++ {
		font.SetGlyph(r, text.FontGlyph{Advance: 8})
	}
	ui.SetFont(font)
	h := &artAlphaHarness{p: p, ui: ui, state: &input.State{}, area: mathx.Rect{W: width, H: 900}}
	h.clip = h.area
	// Measure the layout through the same label/field primitives as the full
	// picker, then drive the actual colorPicker in subsequent frames.
	ui.Begin(h.state, mathx.Vec2{X: width, Y: h.area.H}, 1)
	ui.Region(h.area)
	cur := ui.Cursor()
	cur.Next(min(float32(240), cur.W-24))
	h.bar, h.number = artAlphaRects(ui)
	ui.PopCursor()
	ui.End()
	return h
}

func (h *artAlphaHarness) draw() {
	h.ui.Begin(h.state, mathx.Vec2{X: h.area.W, Y: h.area.H}, 1)
	h.ui.Region(h.area)
	h.ui.PushClip(h.clip)
	h.p.colorPicker(h.ui)
	h.ui.PopClip()
	h.ui.PopCursor()
	h.ui.End()
}

func (h *artAlphaHarness) down(at mathx.Vec2) {
	h.state.BeginFrame()
	h.state.ApplyMouseDown(at, input.MouseLeft)
	h.draw()
}

func (h *artAlphaHarness) move(at mathx.Vec2) {
	h.state.BeginFrame()
	h.state.ApplyMouseMove(at, at.Sub(h.state.MousePos()))
	h.draw()
}

func (h *artAlphaHarness) up(at mathx.Vec2) {
	h.state.BeginFrame()
	h.state.ApplyMouseUp(at, input.MouseLeft)
	h.draw()
}

func (h *artAlphaHarness) click(at mathx.Vec2) {
	h.down(at)
	h.up(at)
}

func (h *artAlphaHarness) position(alpha uint8) mathx.Vec2 {
	return mathx.Vec2{X: h.bar.X + 2 + (h.bar.W-4)*float32(alpha)/255, Y: h.bar.Y + h.bar.H/2}
}

func (h *artAlphaHarness) typeNumber(value string) {
	h.click(h.number.Center())
	h.state.BeginFrame()
	h.state.ApplyText([]byte(value))
	h.draw()
	h.state.BeginFrame()
	h.state.ApplyKeyDown(input.KeyEnter, 0, 0, false)
	h.draw()
	h.state.BeginFrame()
	h.state.ApplyKeyUp(input.KeyEnter, 0, 0)
	h.draw()
}

func (h *artAlphaHarness) swap() {
	h.state.BeginFrame()
	h.state.ApplyKeyDown(input.KeyX, 0, 0, false)
	h.ui.Begin(h.state, mathx.Vec2{X: h.area.W, Y: h.area.H}, 1)
	h.p.toolKeyboard(h.ui, true)
	h.ui.End()
	h.state.BeginFrame()
	h.state.ApplyKeyUp(input.KeyX, 0, 0)
	h.draw()
}

func TestArtAlphaBarAllBytesAndEndpoints(t *testing.T) {
	bar := mathx.Rect{X: 13, W: 304}
	for i := range 256 {
		x := bar.X + 2 + (bar.W-4)*float32(i)/255
		if got := artAlphaAt(bar, x); got != uint8(i) {
			t.Fatalf("position for %d returned %d", i, got)
		}
	}
	if artAlphaAt(bar, bar.X-100) != 0 || artAlphaAt(bar, bar.X+bar.W+100) != 255 {
		t.Fatal("dragging outside the track must clamp to its endpoints")
	}
}

func TestArtAlphaPickerResponsiveAndClipped(t *testing.T) {
	for _, light := range []bool{false, true} {
		for _, width := range []float32{44, 80, 132, 220, 300, 600} {
			t.Run(fmt.Sprintf("light=%v/width=%.0f", light, width), func(t *testing.T) {
				h := newArtAlphaHarness(t, width, light)
				h.p.view.color = mathx.ColorRGBA8(41, 102, 183, 37)
				for _, r := range []mathx.Rect{h.bar, h.number} {
					if r.W <= 0 || r.H <= 0 || r.X < 0 || r.X+r.W > width || r.Y < 0 || r.Y+r.H > h.area.H {
						t.Fatalf("control outside dock: %+v", r)
					}
				}
				if h.bar.W > 240 || (h.bar.Y == h.number.Y && h.bar.X+h.bar.W > h.number.X) {
					t.Fatalf("controls stretched or overlap: bar=%+v number=%+v", h.bar, h.number)
				}
				h.click(mathx.Vec2{X: h.bar.X, Y: h.bar.Center().Y})
				if got := artColorBytes(h.p.view.color); got != (art.Color{R: 41, G: 102, B: 183, A: 0}) {
					t.Fatalf("left click changed the wrong color: %+v", got)
				}
				h.click(mathx.Vec2{X: h.bar.X + h.bar.W - .1, Y: h.bar.Center().Y})
				if got := artColorBytes(h.p.view.color); got != (art.Color{R: 41, G: 102, B: 183, A: 255}) {
					t.Fatalf("right click changed the wrong color: %+v", got)
				}
				// A clipped-off part cannot start a drag, but an existing drag
				// continues outside the visible region until release.
				h.clip = mathx.Rect{X: h.bar.X + h.bar.W/2, Y: h.bar.Y, W: h.bar.W / 2, H: h.bar.H}
				h.click(h.position(0))
				if got := artColorBytes(h.p.view.color).A; got != 255 {
					t.Fatalf("clipped track accepted a click: %d", got)
				}
				h.down(h.position(192))
				h.move(mathx.Vec2{X: h.bar.X - 40, Y: h.bar.Center().Y})
				h.up(mathx.Vec2{X: h.bar.X - 40, Y: h.bar.Center().Y})
				if got := artColorBytes(h.p.view.color).A; got != 0 {
					t.Fatalf("drag did not continue outside clip: %d", got)
				}
			})
		}
	}
}

func TestArtAlphaPickerInScrolledColorPanel(t *testing.T) {
	for _, light := range []bool{false, true} {
		for _, width := range []float32{132, 240} {
			t.Run(fmt.Sprintf("light=%v/width=%.0f", light, width), func(t *testing.T) {
				h := newArtAlphaHarness(t, width, light)
				area := mathx.Rect{W: width, H: 180}
				draw := func() {
					h.ui.Begin(h.state, mathx.Vec2{X: area.W, Y: area.H}, 1)
					h.p.colorPanel(h.ui, area, true)
					h.ui.End()
				}
				// Let the actual panel measure its scrollable content and reserve
				// the scrollbar before calculating the visible control positions.
				draw()
				draw()
				scroll := h.p.view.partScroll["art.color"]
				if scroll == nil || scroll.content <= area.H {
					t.Fatal("fixture did not produce a scrollable color panel")
				}
				measure := func() (mathx.Rect, mathx.Rect) {
					h.ui.Begin(h.state, mathx.Vec2{X: area.W, Y: area.H}, 1)
					h.ui.Region(area)
					cur := h.ui.Cursor()
					cur.W = max(0, cur.W-h.ui.Theme.ScrollbarW)
					cur.Y -= scroll.offset
					widgets.Field(h.ui, "Foreground")
					widgets.Field(h.ui, "Background")
					cur.Next(0) // swap colors
					cur.Next(0) // eyedropper
					cur.Next(min(float32(240), cur.W-24))
					bar, number := artAlphaRects(h.ui)
					h.ui.PopCursor()
					h.ui.End()
					return bar, number
				}
				bar, _ := measure()
				scroll.offset = max(0, bar.Y-area.H/2)
				draw()
				bar, number := measure()
				for _, r := range []mathx.Rect{bar, number} {
					if r.X < 0 || r.X+r.W > area.W-h.ui.Theme.ScrollbarW || r.Y < 0 || r.Y+r.H > area.H {
						t.Fatalf("scrolled control is not visible beside scrollbar: %+v", r)
					}
				}
				h.p.view.color = mathx.ColorRGBA8(41, 102, 183, 37)
				at := mathx.Vec2{X: bar.X + 2 + (bar.W-4)*128/255, Y: bar.Center().Y}
				h.state.BeginFrame()
				h.state.ApplyMouseDown(at, input.MouseLeft)
				draw()
				h.state.BeginFrame()
				h.state.ApplyMouseUp(at, input.MouseLeft)
				draw()
				if got := artColorBytes(h.p.view.color); got != (art.Color{R: 41, G: 102, B: 183, A: 128}) {
					t.Fatalf("scrolled track did not change foreground alpha: %+v", got)
				}
				at = number.Center()
				h.state.BeginFrame()
				h.state.ApplyMouseDown(at, input.MouseLeft)
				draw()
				h.state.BeginFrame()
				h.state.ApplyMouseUp(at, input.MouseLeft)
				draw()
				h.state.BeginFrame()
				h.state.ApplyText([]byte("77"))
				draw()
				h.state.BeginFrame()
				h.state.ApplyKeyDown(input.KeyEnter, 0, 0, false)
				draw()
				if got := artColorBytes(h.p.view.color); got != (art.Color{R: 41, G: 102, B: 183, A: 77}) {
					t.Fatalf("scrolled number field did not commit alpha: %+v", got)
				}
			})
		}
	}
}

func TestArtAlphaPickerNumericAndFocusTransition(t *testing.T) {
	h := newArtAlphaHarness(t, 200, false)
	h.p.view.color = mathx.ColorRGBA8(17, 83, 201, 10)
	for _, tc := range []struct {
		text string
		want uint8
	}{{"73", 73}, {"999", 255}, {"-42", 0}, {"128", 128}} {
		h.typeNumber(tc.text)
		if got := artColorBytes(h.p.view.color); got != (art.Color{R: 17, G: 83, B: 201, A: tc.want}) {
			t.Fatalf("typing %q produced %+v", tc.text, got)
		}
	}
	// Uncommitted text must not override a subsequent track drag on focus loss.
	h.click(h.number.Center())
	h.state.BeginFrame()
	h.state.ApplyText([]byte("31"))
	h.draw()
	h.click(h.position(210))
	h.state.BeginFrame()
	h.draw()
	if got := artColorBytes(h.p.view.color); got != (art.Color{R: 17, G: 83, B: 201, A: 210}) {
		t.Fatalf("losing number focus overrode the track: %+v", got)
	}
}

func TestArtAlphaPickerQuickClickAfterNumberEdit(t *testing.T) {
	h := newArtAlphaHarness(t, 200, false)
	h.p.view.color = mathx.ColorRGBA8(17, 83, 201, 10)
	h.click(h.number.Center())
	h.state.BeginFrame()
	h.state.ApplyText([]byte("31"))
	h.draw()

	// SDL may deliver a press and release in the same frame. An older number
	// edit must not be committed in the next frame over this track click.
	at := h.position(210)
	h.state.BeginFrame()
	h.state.ApplyMouseDown(at, input.MouseLeft)
	h.state.ApplyMouseUp(at, input.MouseLeft)
	h.draw()
	h.state.BeginFrame()
	h.draw()
	if got := artColorBytes(h.p.view.color); got != (art.Color{R: 17, G: 83, B: 201, A: 210}) {
		t.Fatalf("quick track click was overridden by an older number edit: %+v", got)
	}
	if got := h.p.view.recentColors[0]; got.A != 210 {
		t.Fatalf("recent color did not retain the track selection: %+v", got)
	}
}

func TestArtAlphaNumberDragAndEditing(t *testing.T) {
	h := newArtAlphaHarness(t, 200, false)
	// Preserve the actual linear RGB values, not just their displayed bytes.
	initial := mathx.Color{R: .1234567, G: .3456789, B: .7654321, A: 10.0 / 255}
	h.p.view.color = initial
	at := h.number.Center()
	h.down(at)
	h.move(mathx.Vec2{X: at.X + 30, Y: at.Y})
	h.up(mathx.Vec2{X: at.X + 30, Y: at.Y})
	if got := h.p.view.color; got.R != initial.R || got.G != initial.G || got.B != initial.B || artColorBytes(got).A != 40 {
		t.Fatalf("number drag changed RGB or lost alpha: %+v", got)
	}

	// Escape cancels a number edit; clicking elsewhere still commits it.
	h.click(h.number.Center())
	h.state.BeginFrame()
	h.state.ApplyText([]byte("77"))
	h.draw()
	h.state.BeginFrame()
	h.state.ApplyKeyDown(input.KeyEscape, 0, 0, false)
	h.draw()
	h.state.BeginFrame()
	h.state.ApplyKeyUp(input.KeyEscape, 0, 0)
	h.draw()
	if got := artColorBytes(h.p.view.color).A; got != 40 {
		t.Fatalf("Escape failed to cancel alpha text: %d", got)
	}
	h.click(h.number.Center())
	h.state.BeginFrame()
	h.state.ApplyText([]byte("77"))
	h.draw()
	h.click(mathx.Vec2{X: h.area.W - 1, Y: h.area.H - 1})
	if got := h.p.view.color; got.R != initial.R || got.G != initial.G || got.B != initial.B || artColorBytes(got).A != 77 {
		t.Fatalf("focus loss failed to commit alpha without changing RGB: %+v", got)
	}
	h.click(h.position(122))
	if got := h.p.view.color; got.R != initial.R || got.G != initial.G || got.B != initial.B || artColorBytes(got).A != 122 {
		t.Fatalf("track click changed linear RGB or lost alpha: %+v", got)
	}
}

func TestArtAlphaPickerPreservesForegroundBackgroundAndUndo(t *testing.T) {
	for _, clean := range []bool{false, true} {
		t.Run(fmt.Sprintf("clean=%v", clean), func(t *testing.T) {
			h := newArtAlphaHarness(t, 240, false)
			p := h.p
			if clean {
				if err := p.model.SessionUndo(); err != nil || p.doc.Dirty() {
					t.Fatalf("could not restore clean fixture: %v", err)
				}
			}
			fg := art.Color{R: 41, G: 102, B: 183, A: 37}
			bg := art.Color{R: 220, G: 17, B: 78, A: 49}
			p.view.color, p.view.background = artPickerColor(fg), artPickerColor(bg)
			beforeInfo := p.model.Session.Document.Info()
			before, _, err := p.model.Capture(p.model.Session.Frame, art.Albedo)
			if err != nil {
				t.Fatal(err)
			}
			cursor, dirty := p.model.History.Cursor(), p.doc.Dirty()
			h.down(h.position(45))
			h.move(h.position(128))
			h.up(h.position(128))
			fg.A = 128
			if artColorBytes(p.view.color) != fg || artColorBytes(p.view.background) != bg || p.view.recentColors[0] != fg {
				t.Fatal("drag lost foreground RGB, background or recent-color state")
			}
			h.swap()
			h.click(h.position(200))
			bg.A = 200
			h.swap()
			if artColorBytes(p.view.color) != fg || artColorBytes(p.view.background) != bg {
				t.Fatal("foreground/background swap lost alpha")
			}
			if p.model.History.Cursor() != cursor || p.doc.Dirty() != dirty || !reflect.DeepEqual(beforeInfo, p.model.Session.Document.Info()) {
				t.Fatal("choosing alpha modified the document, history or dirty state")
			}
			brush := p.currentBrush()
			if brush.Paint.Color != fg || brush.Background.Color != bg {
				t.Fatalf("brush did not receive both colors: %+v", brush)
			}
			if _, err := p.model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 4, Y: 4}}, Brush: brush}); err != nil {
				t.Fatal(err)
			}
			after, _, err := p.model.Capture(p.model.Session.Frame, art.Albedo)
			if err != nil || p.model.History.Cursor() != cursor+1 || !p.doc.Dirty() || after.NRGBAAt(4, 4).A != 128 {
				t.Fatalf("stroke did not use selected alpha: pixel=%v err=%v", after.NRGBAAt(4, 4), err)
			}
			if err := p.model.SessionUndo(); err != nil {
				t.Fatal(err)
			}
			undone, _, err := p.model.Capture(p.model.Session.Frame, art.Albedo)
			if err != nil || !bytes.Equal(before.Pix, undone.Pix) || p.doc.Dirty() != dirty {
				t.Fatalf("Undo did not restore previous pixels and dirty state: %v", err)
			}
			if err := p.model.SessionRedo(); err != nil {
				t.Fatal(err)
			}
			redone, _, err := p.model.Capture(p.model.Session.Frame, art.Albedo)
			if err != nil || !bytes.Equal(after.Pix, redone.Pix) {
				t.Fatalf("Redo did not restore the transparent stroke: %v", err)
			}
		})
	}
}

func TestArtAlphaPickerIndexedKeepsPaletteContract(t *testing.T) {
	h := newArtAlphaHarness(t, 240, false)
	p := h.p
	for _, a := range []uint8{128, 128, 255} {
		swatch := art.Swatch{Color: art.Color{R: 41, G: 102, B: 183, A: a}}
		if _, err := p.model.Execute(stdctx.Background(), art.Operation{Kind: "palette-add", Swatch: &swatch}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.model.Execute(stdctx.Background(), art.Operation{Kind: "mode", Mode: art.Indexed}); err != nil {
		t.Fatal(err)
	}
	palette := p.model.Session.Document.Info().Palette
	first, duplicate, opaque := len(palette)-3, len(palette)-2, len(palette)-1
	p.view.palette, p.view.backgroundPalette = opaque, duplicate
	p.view.color, p.view.background = artPickerColor(palette[opaque].Color), artPickerColor(palette[duplicate].Color)
	cursor, dirty := p.model.History.Cursor(), p.doc.Dirty()
	h.click(h.position(128))
	if p.view.palette != first || artColorBytes(p.view.color) != palette[first].Color {
		t.Fatal("matching alpha did not select the corresponding indexed swatch")
	}
	p.view.palette = duplicate
	h.click(h.position(128))
	if p.view.palette != duplicate {
		t.Fatal("matching duplicate silently changed the preferred index")
	}
	h.click(h.position(77))
	if p.view.palette != duplicate || artColorBytes(p.view.color).A != 77 {
		t.Fatal("unmatched alpha changed the indexed paint swatch")
	}
	brush := p.currentBrush()
	if !brush.Paint.UseIndex || brush.Paint.Index != duplicate || !brush.Background.UseIndex || brush.Background.Index != duplicate {
		t.Fatalf("indexed brush lost its foreground/background indices: %+v", brush)
	}
	h.swap()
	if p.view.palette != duplicate || p.view.backgroundPalette != duplicate || artColorBytes(p.view.background).A != 77 {
		t.Fatal("indexed swap lost background alpha or swatch identity")
	}
	if p.model.History.Cursor() != cursor || p.doc.Dirty() != dirty || !reflect.DeepEqual(p.model.Session.Document.Info().Palette, palette) {
		t.Fatal("alpha selection modified the indexed palette or document history")
	}
}
