//go:build aego_gltest

package richtext

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/platform/input"
	"aego/render/text"
	"aego/render/texture"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/theme"
)

func clipTestFont() *text.Font {
	f := text.NewFont(14, 1, 18, 14)
	whole := texture.Whole(gpu.TextureHandle{Index: 1, Gen: 1}, 16, 16)
	for r := rune(32); r < 127; r++ {
		f.SetGlyph(r, text.FontGlyph{Region: whole, Size: mathx.Vec2{X: 8, Y: 14}, Advance: 8})
	}
	return f
}

func drawClipItems(ui *core.Context, rt *Text, r, clip mathx.Rect, prefix string) []draw.TestDrawnItem {
	var in input.State
	ui.Begin(&in, mathx.Vec2{X: 400, Y: r.H + 20}, 1)
	ui.PushClip(clip)
	rt.Draw(ui, ui.ID("rich"), r)
	ui.PopClip()
	list := ui.End()
	return list.TestDrawnItems(mathx.Rect{W: 400, H: r.H + 20}, ui.Font(), prefix, 0, 0)
}

func TestCullClippedKeepsVisibleLinesAndDropsOffscreenCommands(t *testing.T) {
	ui := core.New(clipTestFont())
	ui.Theme = theme.Editor()
	var md strings.Builder
	for i := range 40 {
		fmt.Fprintf(&md, "row%02d\n\n", i)
	}
	var rt Text
	rt.Set(Parse(md.String()))
	r := mathx.Rect{W: 220}
	r.H = rt.Measure(ui, r.W)
	for _, y := range []float32{0, 61, r.H / 2, r.H - 99} {
		clip := mathx.Rect{Y: y, W: r.W, H: 99}
		rt.CullClipped = false
		old := drawClipItems(ui, &rt, r, clip, "row")
		rt.CullClipped = true
		now := drawClipItems(ui, &rt, r, clip, "row")
		visible := func(items []draw.TestDrawnItem) []draw.TestDrawnItem {
			var out []draw.TestDrawnItem
			for _, v := range items {
				if v.Bounds.Y+v.Bounds.H > clip.Y && v.Bounds.Y < clip.Y+clip.H {
					out = append(out, v)
				}
			}
			return out
		}
		if !reflect.DeepEqual(visible(old), visible(now)) {
			t.Fatalf("visible text or positions changed at y=%v: old=%+v new=%+v", y, visible(old), visible(now))
		}
		if len(now) >= len(old) {
			t.Fatalf("y=%v did not cull: old=%d new=%d", y, len(old), len(now))
		}
	}
}

func TestCullClippedRetainsOverhangingGlyphs(t *testing.T) {
	f := clipTestFont()
	whole := texture.Whole(gpu.TextureHandle{Index: 1, Gen: 1}, 16, 16)
	f.SetGlyph('A', text.FontGlyph{Region: whole, Size: mathx.Vec2{X: 8, Y: 100}, Advance: 8})
	f.SetGlyph('Z', text.FontGlyph{Region: whole, Offset: mathx.Vec2{Y: -100}, Size: mathx.Vec2{X: 8, Y: 110}, Advance: 8})
	ui := core.New(f)
	ui.Theme = theme.Editor()
	var rt Text
	rt.CullClipped = true
	rt.Set(Parse("A\n\n" + strings.Repeat("row\n\n", 12) + "Z"))
	r := mathx.Rect{W: 220}
	r.H = rt.Measure(ui, r.W)
	lines := rt.Doc().Lines
	pad := 2 * ui.Theme.Padding
	for _, tc := range []struct {
		prefix string
		y      float32
	}{
		{"A", pad + lines[0].Y + 70},
		{"Z", pad + lines[len(lines)-1].Y - 70},
	} {
		items := drawClipItems(ui, &rt, r, mathx.Rect{Y: tc.y, W: r.W, H: 10}, tc.prefix)
		if len(items) != 1 {
			t.Fatalf("%s glyph crossing the clip was dropped: %+v", tc.prefix, items)
		}
	}
}

func TestCullClippedRetainsRunButtonOutsideItsCodeLine(t *testing.T) {
	ui := core.New(clipTestFont())
	ui.Theme = theme.Editor()
	ui.Theme.RowHeight = 100
	var rt Text
	rt.RunButtons = true
	rt.Set(Parse("```aes run\nrun_me()\n```\n\n" + strings.Repeat("row\n\n", 8)))
	r := mathx.Rect{W: 220}
	r.H = rt.Measure(ui, r.W)
	line := rt.Doc().Lines[0]
	if !line.Run {
		t.Fatal("runnable code line not found")
	}
	buttonY := 2*ui.Theme.Padding + line.Y - CodePad + 4
	textY := buttonY + (ui.Theme.RowHeight+2-ui.LineHeight())/2
	clip := mathx.Rect{Y: textY, W: r.W, H: ui.LineHeight()}
	rt.CullClipped = false
	old := drawClipItems(ui, &rt, r, clip, "Run")
	rt.CullClipped = true
	now := drawClipItems(ui, &rt, r, clip, "Run")
	if len(old) != 1 || !reflect.DeepEqual(old, now) {
		t.Fatalf("button in parent clip was culled: old=%+v new=%+v", old, now)
	}
}
