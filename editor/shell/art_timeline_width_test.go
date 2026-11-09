package shell

import (
	"fmt"
	"math"
	"testing"

	"aego/core/mathx"
	"aego/editor/core/workspace"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/theme"
)

func artTimelineWidthWorkspace(t *testing.T, root string) *workspace.Workspace {
	t.Helper()
	w := workspace.New(root, nil)
	for _, id := range []string{workspace.Assets, workspace.Scene, workspace.Game, "art.tools", "art.options", "art.timeline", "art.layers", "art.palette", "art.color", "art.preview"} {
		if err := w.Register(workspace.Desc{Panel: defaultLayoutPanel(id)}); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range BuiltinWorkspaces() {
		if err := w.RegisterPreset(p); err != nil {
			t.Fatal(err)
		}
	}
	w.LoadLayout()
	if !w.Switch(workspace.Art) {
		t.Fatal("Art workspace unavailable")
	}
	return w
}

func artTimelineWidthUI() *core.Context {
	font := text.NewFont(14, 1, 16, 12)
	for r := rune(32); r < 127; r++ {
		font.SetGlyph(r, text.FontGlyph{Advance: 8})
	}
	ui := core.New(font)
	ui.Theme = theme.Editor()
	return ui
}

// The dock receives logical coordinates; density affects font rendering, not
// these rectangles. Each case starts with a fresh factory tree, rather than
// reusing the ratio chosen for a different window size.
func TestArtFactoryTimelineWidths(t *testing.T) {
	for _, tc := range []struct {
		physicalW, physicalH int
		density              float32
		minTimeline          float32
		maxTimeline          float32
		minAssets            float32
	}{
		{1366, 768, 1, 543, 545, 380},
		{1920, 1080, 1, 543, 545, 760},
		{2560, 1440, 1, 543, 545, 1200},
		{3840, 2160, 1, 543, 545, 2000},
		{1920, 1080, 2, 440, 450, 200},
		{1366, 768, 2, 310, 320, 140},
		{600, 600, 1, 270, 280, 125},
	} {
		t.Run(fmt.Sprintf("%dx%d@%g", tc.physicalW, tc.physicalH, tc.density), func(t *testing.T) {
			w := artTimelineWidthWorkspace(t, t.TempDir())
			width := float32(tc.physicalW) / tc.density
			height := float32(tc.physicalH) / tc.density
			// Reserve space for the shell's menu, toolbar and status bar.
			body := mathx.Rect{Y: 56, W: width, H: height - 80}
			ui, in := artTimelineWidthUI(), &input.State{}
			ui.Begin(in, mathx.Vec2{X: width, Y: height}, tc.density)
			w.Tree.Draw(ui, body, func(string, mathx.Rect) {})
			ui.End()
			assets, aok := w.Tree.PanelRect(workspace.Assets)
			timeline, tok := w.Tree.PanelRect("art.timeline")
			canvas, cok := w.Tree.PanelRect(workspace.Scene)
			if !aok || !tok || !cok {
				t.Fatal("factory panels are not visible")
			}
			if timeline.W < tc.minTimeline || timeline.W > tc.maxTimeline || assets.W < tc.minAssets {
				t.Fatalf("logical widths: Assets=%.1f, Timeline=%.1f", assets.W, timeline.W)
			}
			if assets.X+assets.W >= timeline.X || assets.Y != timeline.Y || assets.H != timeline.H || canvas.Y+canvas.H > assets.Y {
				t.Fatalf("unexpected panel order: canvas=%+v assets=%+v timeline=%+v", canvas, assets, timeline)
			}
			if timeline.H > body.H*.22 || canvas.H <= timeline.H {
				t.Fatalf("compact bottom height was lost: body=%+v timeline=%+v canvas=%+v", body, timeline, canvas)
			}
			t.Logf("logical size %.0fx%.0f: Assets %.1f, Timeline %.1f, bottom content height %.1f", width, height, assets.W, timeline.W, timeline.H)
		})
	}
}

func TestArtFactoryTimelineEarlySaveAndUserSplitter(t *testing.T) {
	root := t.TempDir()
	w := artTimelineWidthWorkspace(t, root)
	// A workspace can be switched or saved before it has ever been drawn.
	w.Tree.Layout(mathx.Rect{W: 0, H: 0})
	if err := w.SaveLayout(); err != nil {
		t.Fatal(err)
	}
	w = artTimelineWidthWorkspace(t, root)
	ui, in := artTimelineWidthUI(), &input.State{}
	area := mathx.Rect{W: 1920, H: 1000}
	draw := func() {
		ui.Begin(in, mathx.Vec2{X: area.W, Y: area.H}, 1)
		w.Tree.Draw(ui, area, func(string, mathx.Rect) {})
		ui.End()
	}
	in.BeginFrame()
	draw()
	assets, _ := w.Tree.PanelRect(workspace.Assets)
	timeline, _ := w.Tree.PanelRect("art.timeline")
	if math.Abs(float64(timeline.W-544)) > .01 {
		t.Fatalf("early save lost the initial width: %.1f", timeline.W)
	}
	point := mathx.Vec2{X: assets.X + assets.W + 2, Y: assets.Y + 10}
	in.BeginFrame()
	in.ApplyMouseMove(point, mathx.Vec2{})
	in.ApplyMouseDown(point, input.MouseLeft)
	draw()
	in.BeginFrame()
	end := point.Add(mathx.Vec2{X: 120})
	in.ApplyMouseMove(end, mathx.Vec2{X: 120})
	draw()
	in.BeginFrame()
	in.ApplyMouseUp(end, input.MouseLeft)
	draw()
	resized, _ := w.Tree.PanelRect("art.timeline")
	if math.Abs(float64(resized.W-(timeline.W-120))) > .1 {
		t.Fatalf("splitter did not move normally: %.1f -> %.1f", timeline.W, resized.W)
	}
	if err := w.SaveLayout(); err != nil {
		t.Fatal(err)
	}
	w = artTimelineWidthWorkspace(t, root)
	draw()
	restored, _ := w.Tree.PanelRect("art.timeline")
	if math.Abs(float64(restored.W-resized.W)) > .1 {
		t.Fatalf("saved splitter was replaced: want %.1f, got %.1f", resized.W, restored.W)
	}
	w.ResetLayout()
	draw()
	reset, _ := w.Tree.PanelRect("art.timeline")
	if math.Abs(float64(reset.W-544)) > .1 {
		t.Fatalf("explicit reset did not restore the factory width: %.1f", reset.W)
	}
}
