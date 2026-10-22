package gameview

import (
	"testing"

	"aego/editor/core/play"
)

type countScrub struct {
	seeks, scrubs []int
}

func (c *countScrub) Timeline() play.Timeline { return play.Timeline{Frames: 100, Usable: 100} }
func (c *countScrub) State() play.State       { return play.Paused }
func (c *countScrub) Ticks() uint64           { return 0 }
func (c *countScrub) Seek(n int) error        { c.seeks = append(c.seeks, n); return nil }
func (c *countScrub) Scrub(n int) error       { c.scrubs = append(c.scrubs, n); return nil }

func TestDragSeeksOncePerFrame(t *testing.T) {
	src := &countScrub{}
	b := NewTimelineBar(src)
	b.dragging = true
	for i := range 20 {
		b.hold(10+i, true)
	}
	b.Apply()
	if len(src.scrubs) != 1 || src.scrubs[0] != 29 || len(src.seeks) != 0 {
		t.Fatalf("20 moves in a frame: scrubs %v seeks %v, want one scrub to 29", src.scrubs, src.seeks)
	}
	b.Apply()
	if len(src.scrubs) != 1 {
		t.Fatalf("a frame without moves scrubbed again: %v", src.scrubs)
	}
	b.hold(40, true)
	b.hold(41, false)
	b.Apply()
	if len(src.seeks) != 1 || src.seeks[0] != 41 || len(src.scrubs) != 1 {
		t.Fatalf("release: scrubs %v seeks %v, want one final seek to 41", src.scrubs, src.seeks)
	}
	b.dragging = true
	b.hold(50, true)
	b.Apply()
	b.hold(50, false)
	b.Apply()
	if len(src.seeks) != 1 {
		t.Fatalf("a release where the last scrub already is seeks again: %v", src.seeks)
	}
}
