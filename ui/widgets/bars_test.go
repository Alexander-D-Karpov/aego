package widgets

import (
	"testing"

	"aego/core/mathx"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/theme"
)

func newCtx() *core.Context {
	c := core.New(text.NewFont(14, 1, 18, 14))
	c.Theme = theme.Editor()
	return c
}

func at(x, y float32) *input.State {
	in := &input.State{}
	in.ApplyMouseMove(mathx.Vec2{X: x, Y: y}, mathx.Vec2{})
	return in
}

func pressAt(x, y float32) *input.State {
	in := at(x, y)
	in.ApplyMouseDown(mathx.Vec2{X: x, Y: y}, input.MouseLeft)
	return in
}

func releaseAt(x, y float32) *input.State {
	in := at(x, y)
	in.ApplyMouseDown(mathx.Vec2{X: x, Y: y}, input.MouseLeft)
	in.ApplyMouseUp(mathx.Vec2{X: x, Y: y}, input.MouseLeft)
	return in
}

func runFrame(c *core.Context, in *input.State, fn func()) {
	c.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
	fn()
	c.End()
}

func TestMenuOpensAndCloses(t *testing.T) {
	c := newCtx()
	bar := mathx.Rect{W: 800, H: 28}
	open := false
	draw := func() {
		open = false
		mb := BeginMenuBar(c, bar)
		if mb.Menu("File", 200) {
			open = true
			MenuItem(c, "New", "", true, false)
			mb.EndMenu()
		}
	}
	runFrame(c, pressAt(12, 10), draw)
	if !open {
		t.Fatal("press on the menu title must open it")
	}
	runFrame(c, at(12, 10), draw)
	if !open {
		t.Fatal("menu closed by itself")
	}
	esc := at(12, 10)
	esc.ApplyKeyDown(input.KeyEscape, 0, 0, false)
	runFrame(c, esc, draw)
	if open {
		t.Fatal("Esc must close the menu")
	}
	runFrame(c, pressAt(12, 10), draw)
	runFrame(c, at(400, 400), draw)
	runFrame(c, pressAt(400, 400), draw)
	if open {
		t.Fatal("click outside must close the menu")
	}
}

func TestToolButtonStates(t *testing.T) {
	c := newCtx()
	r := mathx.Rect{Y: 28, W: 800, H: 28}
	clicked := [2]bool{}
	draw := func() {
		tb := BeginToolbar(c, r)
		clicked[0] = tb.Button(c.ID("on"), "play.start", "Play", false, true)
		clicked[1] = tb.Button(c.ID("off"), "play.step", "Step", false, false)
	}
	runFrame(c, pressAt(16, 40), draw)
	runFrame(c, releaseAt(16, 40), draw)
	if !clicked[0] {
		t.Fatal("enabled button did not click")
	}
	runFrame(c, pressAt(40, 40), draw)
	runFrame(c, releaseAt(40, 40), draw)
	if clicked[1] {
		t.Fatal("disabled button clicked")
	}
}
