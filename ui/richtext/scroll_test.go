package richtext

import (
	"strings"
	"testing"

	"aego/core/mathx"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/layout"
	"aego/ui/theme"
	"aego/ui/widgets"
)

func scrollFrame(flipped bool) (list, area, rich, zoom float32) {
	c := core.New(text.NewFont(14, 1, 18, 14))
	c.Theme = theme.Editor()
	in := &input.State{}
	at := mathx.Vec2{X: 50, Y: 50}
	in.ApplyMouseMove(at, mathx.Vec2{})
	in.ApplyScroll(at, mathx.Vec2{Y: -1}, false, flipped)
	list, area = 100, 100
	var t Text
	t.Set(Parse(strings.Repeat("paragraph\n\n", 200)))
	t.Scroll = 100
	r := mathx.Rect{W: 200, H: 100}
	c.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
	widgets.VirtualList(c, c.ID("list"), r, 100, 20, &list, func(int, mathx.Rect) {})
	layout.ScrollArea(c, c.ID("area"), 100, &area, func() {
		for range 40 {
			c.Cursor().Next(0)
		}
	})
	t.Draw(c, c.ID("rich"), r)
	zoom = c.ZoomY()
	c.End()
	return list, area, t.Scroll, zoom
}

func TestFlippedWheelScrollsEveryPanelTheSameWay(t *testing.T) {
	for _, flipped := range []bool{false, true} {
		list, area, rich, zoom := scrollFrame(flipped)
		if list <= 100 || area <= 100 || rich <= 100 {
			t.Fatalf("flipped=%v: list %v area %v rich %v, all must scroll down", flipped, list, area, rich)
		}
		want := float32(-1)
		if flipped {
			want = 1
		}
		if zoom != want {
			t.Fatalf("flipped=%v: zoom %v, want %v", flipped, zoom, want)
		}
	}
}

func TestPreciseScrollIsPixels(t *testing.T) {
	c := core.New(nil)
	in := &input.State{}
	in.ApplyScroll(mathx.Vec2{}, mathx.Vec2{Y: 0.5}, true, false)
	c.Begin(in, mathx.Vec2{X: 100, Y: 100}, 1)
	s := c.Scroll()
	c.End()
	if s.Lines.Y != 0 || s.Pixels.Y != 0.5*core.PrecisePixels {
		t.Fatalf("scroll %+v", s)
	}
}
