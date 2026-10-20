package clock

import "testing"

const frame = 1.0 / 60

func run(t *Time, real float64, frames int) int {
	ticks := 0
	for range frames {
		p := t.Begin(real)
		for range p.Steps {
			t.BeginTick()
		}
		ticks += p.Steps
	}
	return ticks
}

func TestScaleHalvesTicks(t *testing.T) {
	a, b := NewTime(Config{}), NewTime(Config{})
	b.SetScale(0.5)
	ta, tb := run(a, frame, 120), run(b, frame, 120)
	if ta != 120 || tb != 60 {
		t.Fatalf("ticks %d %d", ta, tb)
	}
	if a.FixedDelta() != b.FixedDelta() {
		t.Fatal("scale changed the fixed delta")
	}
}

func TestMaxStepsDropsTicks(t *testing.T) {
	tm := NewTime(Config{MaxFrameDelta: 1, MaxFixedSteps: 4})
	p := tm.Begin(0.1)
	if p.Steps != 4 || tm.DroppedSteps() != 2 {
		t.Fatalf("steps %d dropped %d", p.Steps, tm.DroppedSteps())
	}
	if a := tm.Alpha(); a < 0 || a >= 1 {
		t.Fatalf("alpha %v", a)
	}
}

func TestClampsFrameDelta(t *testing.T) {
	tm := NewTime(Config{})
	tm.Begin(5)
	if tm.Span(Real).Delta != DefaultMaxFrameDelta || tm.ClampedTime() < 4.89 {
		t.Fatalf("real %v clamped %v", tm.Span(Real).Delta, tm.ClampedTime())
	}
}

func TestStepFixed(t *testing.T) {
	tm := NewTime(Config{})
	tm.SetPaused(true)
	if p := tm.Begin(frame); p.Steps != 0 || p.Update || !p.Paused {
		t.Fatalf("paused plan %+v", p)
	}
	tm.StepFixed()
	if p := tm.Begin(frame); p.Steps != 1 || p.Update {
		t.Fatalf("step fixed plan %+v", p)
	}
	if p := tm.Begin(frame); p.Steps != 0 {
		t.Fatal("step repeated")
	}
}

func TestStepFrame(t *testing.T) {
	tm := NewTime(Config{})
	tm.Begin(frame)
	tm.SetPaused(true)
	tm.StepFrame()
	p := tm.Begin(frame)
	if !p.Update || p.Steps > 1 || tm.Span(Game).Delta <= 0 {
		t.Fatalf("step frame plan %+v", p)
	}
}

func TestPauseDoesNotAccumulate(t *testing.T) {
	tm := NewTime(Config{})
	tm.SetPaused(true)
	run(tm, frame, 600)
	tm.SetPaused(false)
	if p := tm.Begin(frame); p.Steps != 1 {
		t.Fatalf("resume steps %d", p.Steps)
	}
}

func TestDeterministic(t *testing.T) {
	a, b := NewTime(Config{}), NewTime(Config{})
	for i := range 500 {
		d := frame * float64(1+i%3) * 0.7
		pa, pb := a.Begin(d), b.Begin(d)
		if pa != pb || a.Alpha() != b.Alpha() {
			t.Fatalf("frame %d diverged", i)
		}
	}
}

func TestUpdatesFreezeWhilePaused(t *testing.T) {
	tm := NewTime(Config{})
	run(tm, frame, 10)
	tm.SetPaused(true)
	run(tm, frame, 30)
	if tm.Updates() != 10 || tm.Frame() != 40 {
		t.Fatalf("updates %d frames %d", tm.Updates(), tm.Frame())
	}
	tm.StepFrame()
	tm.Begin(frame)
	if tm.Updates() != 11 {
		t.Fatalf("step frame gave %d updates", tm.Updates())
	}
}

func TestStepFixedCountsAsFrame(t *testing.T) {
	tm := NewTime(Config{})
	tm.SetPaused(true)
	before, game := tm.Updates(), tm.Span(Game).Elapsed
	tm.StepFixed()
	tm.Begin(frame)
	if tm.Updates() != before+1 || tm.Span(Game).Elapsed <= game {
		t.Fatalf("updates %d, game %v", tm.Updates(), tm.Span(Game).Elapsed)
	}
	tm.Begin(frame)
	if tm.Updates() != before+1 {
		t.Fatal("a paused frame without a step counted")
	}
}
