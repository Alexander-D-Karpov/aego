package debug

import (
	"testing"

	"aego/contracts/debugdraw"
	"aego/core/mathx"
)

var a, b = mathx.Vec2{}, mathx.Vec2{X: 1}

func TestLifetimes(t *testing.T) {
	s := NewService(DefaultSettings())
	s.Line(a, b, &debugdraw.Style{Channel: "t"})
	s.Line(a, b, &debugdraw.Style{Channel: "t", Life: debugdraw.ForSeconds(2)})
	s.Line(a, b, &debugdraw.Style{Channel: "t", Life: debugdraw.ForUnscaled(2)})
	s.Advance(true, 0, 3)
	if s.Live() != 2 {
		t.Fatalf("paused: %d live", s.Live())
	}
	s.Advance(false, 1.0/60, 1.0/60)
	if s.Live() != 1 {
		t.Fatalf("resumed: %d live", s.Live())
	}
	s.Advance(false, 2, 2)
	if s.Live() != 0 {
		t.Fatalf("expired: %d live", s.Live())
	}
}

func TestFramesAndFade(t *testing.T) {
	s := NewService(DefaultSettings())
	s.Circle(a, 1, &debugdraw.Style{Life: debugdraw.ForFrames(3), Fade: debugdraw.FadeAlpha})
	s.Advance(false, 0, 0)
	if f := s.prims[0].factor(); f <= 0.6 || f >= 0.7 {
		t.Fatalf("fade factor %v", f)
	}
	s.Advance(false, 0, 0)
	s.Advance(false, 0, 0)
	if s.Live() != 0 {
		t.Fatal("frames lifetime did not end")
	}
}

func TestChannelLimit(t *testing.T) {
	cfg := DefaultSettings()
	cfg.MaxPerChannel = 2
	s := NewService(cfg)
	st := &debugdraw.Style{Channel: "c"}
	for range 3 {
		s.Line(a, b, st)
	}
	if s.Live() != 2 || s.Dropped() != 1 {
		t.Fatalf("live %d dropped %d", s.Live(), s.Dropped())
	}
}

func TestDisabledIsNoop(t *testing.T) {
	cfg := DefaultSettings()
	cfg.Enabled = false
	s := NewService(cfg)
	if h := s.Line(a, b, nil); h != 0 || s.Live() != 0 || s.Wants("x") {
		t.Fatal("disabled service recorded a primitive")
	}
	if n := testing.AllocsPerRun(100, func() { s.Line(a, b, nil) }); n != 0 {
		t.Fatalf("%v allocs", n)
	}
}

func TestClearAndVisibility(t *testing.T) {
	s := NewService(DefaultSettings())
	s.Line(a, b, &debugdraw.Style{Channel: "physics.shapes", Life: debugdraw.Forever()})
	s.Line(a, b, &debugdraw.Style{Channel: "ai.path", Life: debugdraw.Forever()})
	s.HideByDefault("physics.")
	if s.Wants("physics.shapes") || !s.Wants("ai.path") {
		t.Fatal("default visibility")
	}
	s.Hide("physics.shapes", false)
	if !s.Wants("physics.shapes") {
		t.Fatal("explicit show lost")
	}
	s.ClearChannel("physics.")
	s.Advance(false, 0, 0)
	if s.Live() != 1 {
		t.Fatalf("clear left %d", s.Live())
	}
}

func TestTenThousandWithoutAllocs(t *testing.T) {
	s := NewService(DefaultSettings())
	st := &debugdraw.Style{Channel: "bench", Life: debugdraw.ForSeconds(1)}
	pts := []mathx.Vec2{a, b, {Y: 1}}
	s.Line(a, b, st)
	s.Advance(false, 2, 2)
	n := testing.AllocsPerRun(20, func() {
		for range 3000 {
			s.Line(a, b, st)
			s.Circle(a, 1, st)
			s.Polygon(pts, true, st)
		}
		s.Advance(false, 2, 2)
	})
	if n != 0 {
		t.Fatalf("%v allocs per frame", n)
	}
}
