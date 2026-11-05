package sceneview

import (
	"testing"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/platform/input"
	"aego/render/text"
	"aego/scene"
	"aego/ui/core"
	"aego/ui/theme"
)

func TestToolbarHoverSurvivesSceneInput(t *testing.T) {
	for _, mode := range []string{"2D", "3D"} {
		t.Run(mode, func(t *testing.T) {
			types := testTypes(t)
			s := scene.New(types, nil, scene.Options{})
			mgr := document.NewManager(document.Options{Root: t.TempDir(), Types: types})
			doc := mgr.New("test.aescene", s)
			ed := &context.Editor{Docs: mgr}
			ui := core.New(text.NewFont(14, 1, 18, 14))
			ui.Theme = theme.Editor()
			p := New(nil, Deps{Docs: ed, Tools: &context.SceneSettings{}, Viewport: &services.Viewport{Overlay: &context.Overlays{}}})
			p.f = &services.Frame{UI: ui}
			p.cur = &world{scene: s}
			p.scale = 1
			if mode == "3D" {
				p.viewMode = Mode3D
			}
			in := &input.State{}
			for i := 0; i < 3; i++ {
				in.BeginFrame()
				in.ApplyMouseMove(mathx.V2(18, 18), mathx.Vec2{})
				ui.Time = float64(i) * 0.4
				ui.Begin(in, mathx.V2(800, 600), 1)
				r := mathx.Rect{W: 800, H: 600}
				p.toolbar(doc, r)
				if mode == "3D" {
					p.input3(doc, r)
				} else {
					p.input(doc, r)
				}
				id := ui.IDInt(300)
				if ui.Hot() != id {
					t.Fatalf("scene input stole toolbar hover: got %v, want %v", ui.Hot(), id)
				}
				if i == 2 && ui.HoverDuration(id) < 0.5 {
					t.Fatal("hover never lasted long enough to show the tooltip")
				}
				ui.End()
			}
		})
	}
}
