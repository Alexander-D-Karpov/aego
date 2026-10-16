package vectors

import (
	"testing"

	"aego/editor/core/context"
	"aego/editor/core/shape"
)

func TestOffDoesNothing(t *testing.T) {
	ctx := &context.Editor{}
	ctx.Vectors = []context.VectorLayer{{ID: "a", Draw: func(*context.VectorSink) { t.Fatal("drawn while off") }}}
	fn := Overlay(ctx)
	c := &shape.OverlayCtx{}
	if n := testing.AllocsPerRun(100, func() { fn(c) }); n != 0 {
		t.Fatalf("%v allocations with the overlay off", n)
	}
}

func TestGameOverlayFollowsVectorsToggle(t *testing.T) {
	ctx := &context.Editor{}
	drawn := 0
	ctx.Vectors = []context.VectorLayer{{ID: "a", Draw: func(*context.VectorSink) { drawn++ }}}
	ctx.Overlay.Set(Prefix+"a", true)
	fn := Overlay(ctx)
	fn(&shape.OverlayCtx{Game: true})
	if drawn != 0 {
		t.Fatal("vectors must stay hidden until the Vectors toggle is on")
	}
	ctx.Overlay.Set(CommandToggle, true)
	fn(&shape.OverlayCtx{Game: true})
	if drawn != 1 {
		t.Fatal("game view overlay must draw enabled vector layers")
	}
	ctx.Overlay.Set(Prefix+"a", false)
	fn(&shape.OverlayCtx{Game: true})
	if drawn != 1 {
		t.Fatal("a disabled layer must stay hidden in the game view")
	}
}
