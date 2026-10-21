package audio

import (
	"testing"

	"aego/core/ids"
)

func TestVariantDeterministic(t *testing.T) {
	ev := DefaultEvent()
	ev.Variants = []ClipRef{{ID: ids.AssetID{Lo: 1}}, {}, {ID: ids.AssetID{Lo: 2}}, {ID: ids.AssetID{Lo: 3}}}
	a, b := NewService(nil, nil, nil), NewService(nil, nil, nil)
	seen := map[ids.AssetID]bool{}
	for i := 0; i < 60; i++ {
		x, ok1 := a.variant(&ev)
		y, ok2 := b.variant(&ev)
		if !ok1 || !ok2 || x != y || x.IsZero() {
			t.Fatalf("pick %d: %v %v %v %v", i, x, y, ok1, ok2)
		}
		seen[x] = true
	}
	if len(seen) != 3 {
		t.Fatalf("picked %d different variants, want 3", len(seen))
	}
}

func TestStealAndCooldown(t *testing.T) {
	s := NewService(nil, nil, nil)
	key := ids.AssetID{Lo: 9}
	ev := DefaultEvent()
	ev.MaxVoices, ev.Steal = 2, StealOldest
	s.recs = []voiceRec{{event: key, born: 1}, {event: key, born: 2}}
	if !s.allow(key, &ev) {
		t.Fatal("Oldest refused the play")
	}
	if !s.recs[0].stopped || s.recs[1].stopped {
		t.Fatal("Oldest did not stop the oldest voice")
	}
	s.recs = append(s.recs, voiceRec{event: key, born: 3})
	ev.Steal = StealNone
	if s.allow(key, &ev) {
		t.Fatal("None allowed a play over Max Voices")
	}
	other := ids.AssetID{Lo: 10}
	ev.MaxVoices, ev.Cooldown = 0, 0.5
	if !s.allow(other, &ev) || s.allow(other, &ev) {
		t.Fatal("cooldown did not block the second play")
	}
	s.time = 0.6
	if !s.allow(other, &ev) {
		t.Fatal("cooldown blocked a play after the window")
	}
}
