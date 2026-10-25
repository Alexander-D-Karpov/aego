package anim

import (
	"slices"
	"testing"
)

func clip(loop LoopMode, ts ...float32) *Clip {
	c := &Clip{Length: 1, Loop: loop}
	for _, t := range ts {
		c.Events = append(c.Events, Event{T: t, Name: "e"})
	}
	c.Fix()
	return c
}

func evs(cs []Crossing) []int32 {
	out := make([]int32, len(cs))
	for i, c := range cs {
		out[i] = c.Event
	}
	return out
}

func cycles(cs []Crossing) []int64 {
	out := make([]int64, len(cs))
	for i, c := range cs {
		out[i] = c.Cycle
	}
	return out
}

func expect[T comparable](t *testing.T, name string, got, want []T) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s: got %v, want %v", name, got, want)
	}
}

func TestOnce(t *testing.T) {
	c := clip(LoopOnce, 0, 0.5, 1)
	expect(t, "start", evs(Crossings(c, LoopOnce, 0, 0.6, true, nil)), []int32{0, 1})
	expect(t, "end", evs(Crossings(c, LoopOnce, 0.6, 1.5, false, nil)), []int32{2})
	expect(t, "after", evs(Crossings(c, LoopOnce, 1, 2, false, nil)), []int32{})
}

func TestZeroKeyEveryCycle(t *testing.T) {
	c := clip(LoopRepeat, 0)
	expect(t, "start", cycles(Crossings(c, LoopRepeat, 0, 0.5, true, nil)), []int64{0})
	expect(t, "next", cycles(Crossings(c, LoopRepeat, 0.5, 1.5, false, nil)), []int64{1})
	expect(t, "two", cycles(Crossings(c, LoopRepeat, 1.5, 3.2, false, nil)), []int64{2, 3})
}

func TestLengthKeyOncePerCycle(t *testing.T) {
	c := clip(LoopRepeat, 1)
	expect(t, "start", evs(Crossings(c, LoopRepeat, 0, 0.5, true, nil)), []int32{})
	expect(t, "first", cycles(Crossings(c, LoopRepeat, 0.5, 1, false, nil)), []int64{0})
	expect(t, "second", cycles(Crossings(c, LoopRepeat, 1, 2, false, nil)), []int64{1})
}

func TestSameMoment(t *testing.T) {
	c := clip(LoopOnce, 0.5, 0.5)
	expect(t, "forward", evs(Crossings(c, LoopOnce, 0, 1, false, nil)), []int32{0, 1})
	expect(t, "backward", evs(Crossings(c, LoopOnce, 1, 0, false, nil)), []int32{0, 1})
}

func TestStillAndBackward(t *testing.T) {
	c := clip(LoopRepeat, 0.2, 0.8)
	expect(t, "speed 0", evs(Crossings(c, LoopRepeat, 0.3, 0.3, false, nil)), []int32{})
	got := Crossings(c, LoopRepeat, 1.5, 0.1, false, nil)
	expect(t, "backward events", evs(got), []int32{0, 1, 0})
	expect(t, "backward cycles", cycles(got), []int64{1, 0, 0})
}

func TestManyCycles(t *testing.T) {
	c := clip(LoopRepeat, 0.5)
	expect(t, "three", cycles(Crossings(c, LoopRepeat, 0, 3, true, nil)), []int64{0, 1, 2})
}

func TestPingPongTurns(t *testing.T) {
	c := clip(LoopPingPong, 0, 1)
	got := Crossings(c, LoopPingPong, 0, 4, true, nil)
	expect(t, "events", evs(got), []int32{0, 1, 0, 1, 0})
	expect(t, "cycles", cycles(got), []int64{0, 0, 2, 2, 4})
	mid := clip(LoopPingPong, 0.25)
	got = Crossings(mid, LoopPingPong, 0, 2, true, nil)
	if len(got) != 2 || got[0].T != 0.25 || got[1].T != 1.75 {
		t.Fatalf("mid %+v", got)
	}
}

func TestCrossingsNoAllocs(t *testing.T) {
	c := clip(LoopRepeat, 0, 0.25, 0.5, 0.75)
	buf := make([]Crossing, 0, 64)
	if n := testing.AllocsPerRun(100, func() { buf = Crossings(c, LoopRepeat, 0.1, 3.9, false, buf) }); n != 0 {
		t.Fatalf("Crossings allocates %v", n)
	}
}
