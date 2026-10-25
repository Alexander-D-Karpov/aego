package palette

import (
	"testing"

	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
)

// The click on a button that opens the palette must not count as a click outside it.
func TestOpeningClickKeepsItOpen(t *testing.T) {
	c := core.New(nil)
	in := &input.State{}
	at := mathx.Vec2{X: 50, Y: 15}
	in.BeginFrame()
	in.ApplyMouseMove(at, mathx.Vec2{})
	in.ApplyMouseDown(at, input.MouseLeft)
	c.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
	p := New(core.IDOf("pal"))
	p.SetItems([]Item{{Key: "a", Label: "Alpha"}})
	p.Show(c)
	if res := p.Draw(c, mathx.Rect{X: 10, Y: 40, W: 400, H: 300}); res.Close {
		t.Fatal("the opening press closed the palette")
	}
	c.End()
	in.BeginFrame()
	in.ApplyMouseUp(at, input.MouseLeft)
	c.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
	if res := p.Draw(c, mathx.Rect{X: 10, Y: 40, W: 400, H: 300}); res.Close {
		t.Fatal("releasing the opening press closed the palette")
	}
	c.End()
	in.BeginFrame()
	in.ApplyMouseDown(at, input.MouseLeft)
	c.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
	if res := p.Draw(c, mathx.Rect{X: 10, Y: 40, W: 400, H: 300}); !res.Close {
		t.Fatal("a later click outside must close it")
	}
}
