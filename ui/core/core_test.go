package core

import (
	"testing"

	"aego/core/mathx"
	"aego/platform/input"
)

func newCtx() (*Context, *input.State) {
	in := &input.State{}
	c := New(nil)
	return c, in
}

func frame(c *Context, in *input.State) {
	c.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
}

func TestClickRequiresPressAndReleaseInside(t *testing.T) {
	c, in := newCtx()
	id := IDOf("btn")
	r := mathx.Rect{X: 10, Y: 10, W: 100, H: 20}

	in.BeginFrame()
	in.ApplyMouseMove(mathx.Vec2{X: 50, Y: 15}, mathx.Vec2{})
	in.ApplyMouseDown(mathx.Vec2{X: 50, Y: 15}, input.MouseLeft)
	frame(c, in)
	if c.Clicked(id, r) {
		t.Fatal("click must not fire on press")
	}
	if c.Active() != id {
		t.Fatal("press must capture active")
	}

	in.BeginFrame()
	in.ApplyMouseUp(mathx.Vec2{X: 50, Y: 15}, input.MouseLeft)
	frame(c, in)
	if !c.Clicked(id, r) {
		t.Fatal("click must fire on release inside")
	}
	if c.Active() != 0 {
		t.Fatal("active must clear on release")
	}
}

func TestReleaseOutsideDoesNotClick(t *testing.T) {
	c, in := newCtx()
	id := IDOf("btn")
	r := mathx.Rect{X: 10, Y: 10, W: 100, H: 20}

	in.BeginFrame()
	in.ApplyMouseDown(mathx.Vec2{X: 50, Y: 15}, input.MouseLeft)
	frame(c, in)
	c.Clicked(id, r)

	in.BeginFrame()
	in.ApplyMouseMove(mathx.Vec2{X: 500, Y: 400}, mathx.Vec2{})
	in.ApplyMouseUp(mathx.Vec2{X: 500, Y: 400}, input.MouseLeft)
	frame(c, in)
	if c.Clicked(id, r) {
		t.Fatal("release outside must not click")
	}
}

func TestHotAndWantsMouse(t *testing.T) {
	c, in := newCtx()
	r := mathx.Rect{X: 0, Y: 0, W: 50, H: 50}
	in.BeginFrame()
	in.ApplyMouseMove(mathx.Vec2{X: 25, Y: 25}, mathx.Vec2{})
	frame(c, in)
	if !c.MouseOver(IDOf("a"), r) || c.Hot() != IDOf("a") || !c.WantsMouse() {
		t.Fatal("hover state wrong")
	}

	in.BeginFrame()
	in.ApplyMouseMove(mathx.Vec2{X: 500, Y: 500}, mathx.Vec2{})
	frame(c, in)
	if c.MouseOver(IDOf("a"), r) || c.WantsMouse() {
		t.Fatal("hover must clear when the mouse leaves")
	}
}

func TestDrag(t *testing.T) {
	c, in := newCtx()
	id := IDOf("slider")
	r := mathx.Rect{X: 0, Y: 0, W: 100, H: 20}

	in.BeginFrame()
	in.ApplyMouseDown(mathx.Vec2{X: 10, Y: 10}, input.MouseLeft)
	frame(c, in)
	c.Drag(id, r)

	in.BeginFrame()
	in.ApplyMouseMove(mathx.Vec2{X: 40, Y: 500}, mathx.Vec2{X: 30})
	frame(c, in)
	delta, dragging := c.Drag(id, r)
	if !dragging || delta.X != 30 {
		t.Fatalf("drag must continue outside the rect: %v %v", delta, dragging)
	}
}

func TestIDStability(t *testing.T) {
	if IDOf("a") == IDOf("b") {
		t.Fatal("IDOf collision")
	}
	if IDSeq(IDOf("list"), 1) == IDSeq(IDOf("list"), 2) {
		t.Fatal("IDSeq collision")
	}
}

func TestIDDoesNotAllocate(t *testing.T) {
	if n := testing.AllocsPerRun(1000, func() {
		_ = IDSeq(IDOf("widget"), 7)
	}); n != 0 {
		t.Fatalf("IDOf/IDSeq allocate: %.1f", n)
	}
}
