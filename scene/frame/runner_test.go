package frame_test

import (
	"testing"

	"aego/core/mathx"
	"aego/render/view"
	"aego/scene/frame"
)

func TestScreenToWorldFollowsViewportSize(t *testing.T) {
	r := frame.New()
	cam := view.Camera2D{Position: mathx.Vec2{X: 3, Y: -2}, Zoom: 48}
	for _, sz := range [][2]int{{800, 600}, {1240, 500}, {333, 777}} {
		r.SetViewportSize(sz[0], sz[1])
		r.Prepare(1, 1, 1.25)
		r.AddView(view.View{
			Viewport:   r.Viewport(mathx.Rect{}),
			Camera:     cam,
			PixelScale: 1.25,
			Scale:      view.ScaleFitVirtual,
			Virtual:    mathx.Vec2{X: 320, Y: 180},
		}, 0)
		center := mathx.Vec2{X: float32(sz[0]) / 2, Y: float32(sz[1]) / 2}
		got, ok := r.ScreenToWorld(center)
		if !ok || got.Sub(cam.Position).Len() > 1e-3 {
			t.Fatalf("%v: center maps to %v, want %v", sz, got, cam.Position)
		}
		if vp := r.Views()[0].LetterboxViewport(); vp.X > 1 {
			if _, ok := r.ScreenToWorld(mathx.Vec2{X: 0.5, Y: center.Y}); ok {
				t.Fatalf("%v: a click on the letterbox bar must not hit the camera", sz)
			}
		}
	}
}
