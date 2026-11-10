package richtext

import (
	"strings"
	"testing"

	"aego/core/mathx"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/theme"
)

func TestClipMarginsCoverGlyphOverhangAndRunButtons(t *testing.T) {
	f := text.NewFont(14, 2, 20, 14)
	f.SetGlyph('A', text.FontGlyph{Offset: mathx.Vec2{Y: -80}, Size: mathx.Vec2{Y: 180}})
	var fs fonts
	for i := range fs.face {
		fs.face[i] = f
	}
	top, bottom := clipMargins(&fs, 30)
	if top < 90.25 || bottom < 90.25 {
		t.Fatalf("glyph can overhang clipping bounds: top=%v bottom=%v", top, bottom)
	}
	f.SetGlyph('A', text.FontGlyph{})
	top, bottom = clipMargins(&fs, 200)
	if top < 90.25 || bottom < 202 {
		t.Fatalf("glyph or run button can be culled: top=%v bottom=%v", top, bottom)
	}
}

func TestCulledLinksKeepTheirIDsAcrossParentClipChange(t *testing.T) {
	f := text.NewFont(14, 1, 18, 14)
	for r := rune(32); r < 127; r++ {
		f.SetGlyph(r, text.FontGlyph{Advance: 8})
	}
	ui := core.New(f)
	ui.Theme = theme.Editor()
	var rt Text
	rt.CullClipped = true
	rt.Set(Parse(strings.Repeat("[earlier](ref:earlier)\n\n", 10) + "[target](ref:target)"))
	r := mathx.Rect{W: 260}
	r.H = rt.Measure(ui, r.W)
	doc := rt.Doc()
	var target *Line
	for i := range doc.Lines {
		for _, run := range doc.Lines[i].Runs {
			if run.Target == "ref:target" {
				target = &doc.Lines[i]
			}
		}
	}
	if target == nil {
		t.Fatal("target link not laid out")
	}
	pad := ui.Theme.Padding * 2
	at := mathx.Vec2{X: pad + 2, Y: pad + target.Y + target.H/2}
	var in input.State
	draw := func(clip mathx.Rect) Action {
		ui.Begin(&in, mathx.Vec2{X: 400, Y: r.H + 20}, 1)
		ui.PushClip(clip)
		act := rt.Draw(ui, ui.ID("rich"), r)
		ui.PopClip()
		ui.End()
		return act
	}
	in.BeginFrame()
	in.ApplyMouseMove(at, mathx.Vec2{})
	in.ApplyMouseDown(at, input.MouseLeft)
	clip := mathx.Rect{Y: pad + target.Y - 2, W: 300, H: target.H + 4}
	draw(clip)
	in.BeginFrame()
	in.ApplyMouseUp(at, input.MouseLeft)
	if act := draw(mathx.Rect{W: 300, H: r.H}); act.Target != "ref:target" {
		t.Fatalf("the target link lost its active ID after parent clipping: %+v", act)
	}
}

func TestCulledLinkStillReceivesReleaseAfterScrollingOut(t *testing.T) {
	f := text.NewFont(14, 1, 18, 14)
	for r := rune(32); r < 127; r++ {
		f.SetGlyph(r, text.FontGlyph{Advance: 8})
	}
	ui := core.New(f)
	ui.Theme = theme.Editor()
	var rt Text
	rt.CullClipped = true
	rt.Set(Parse("[target](ref:target)\n\n" + strings.Repeat("later\n\n", 20)))
	r := mathx.Rect{W: 260}
	r.H = rt.Measure(ui, r.W)
	pad := ui.Theme.Padding * 2
	at := mathx.Vec2{X: pad + 2, Y: pad + rt.Doc().Lines[0].H/2}
	var in input.State
	draw := func(clip mathx.Rect) Action {
		ui.Begin(&in, mathx.Vec2{X: 400, Y: r.H + 20}, 1)
		ui.PushClip(clip)
		act := rt.Draw(ui, ui.ID("rich"), r)
		ui.PopClip()
		ui.End()
		return act
	}
	in.BeginFrame()
	in.ApplyMouseMove(at, mathx.Vec2{})
	in.ApplyMouseDown(at, input.MouseLeft)
	draw(mathx.Rect{W: 300, H: r.H})
	if ui.Active() == 0 {
		t.Fatal("link did not receive the press")
	}
	in.BeginFrame()
	in.ApplyMouseUp(at, input.MouseLeft)
	if act := draw(mathx.Rect{Y: r.H / 2, W: 300, H: 50}); act.Target != "" || ui.Active() != 0 {
		t.Fatalf("offscreen release must cancel the link and clear active state: action=%+v active=%v", act, ui.Active())
	}
}
