package propclaim

import (
	"testing"

	"aego/scene"
	"aego/scene/propref"
)

type services map[string]any

func (s services) ServiceAny(id string) (any, bool) {
	v, ok := s[id]
	return v, ok
}

func (s services) SetView(id string, v any) {
	s[id] = v
}

func TestRoundsAndRevision(t *testing.T) {
	tb := New()
	a := tb.Register("anim")
	n := scene.NodeHandle{Index: 1, Gen: 1}
	pos := propref.Transform(false, propref.PartPosition)
	tb.Begin(a)
	tb.Claim(a, n, &pos)
	tb.End(a)
	rev := tb.Revision()
	if rev == 0 {
		t.Fatal("a new claim must bump the revision")
	}
	tb.Begin(a)
	tb.Claim(a, n, &pos)
	tb.Claim(a, n, &pos)
	tb.End(a)
	if tb.Revision() != rev {
		t.Fatal("an unchanged round must keep the revision")
	}
	tb.Begin(a)
	tb.End(a)
	if _, ok := tb.Held(n, &pos, NoOwner); ok || tb.Revision() == rev {
		t.Fatal("an empty round must drop the claim")
	}
}

func TestHeldOverlapsOtherOwners(t *testing.T) {
	tb := New()
	a, g := tb.Register("anim"), tb.Register("gameplay")
	if tb.Register("anim") != a || tb.Name(g) != "gameplay" {
		t.Fatal("owners are registered once by name")
	}
	n := scene.NodeHandle{Index: 2, Gen: 1}
	pos := propref.Transform(false, propref.PartPosition)
	x, y := pos.WithLane(0), pos.WithLane(1)
	scale := propref.Transform(false, propref.PartScale)
	tb.Begin(a)
	tb.Claim(a, n, &x)
	tb.End(a)
	if o, ok := tb.Held(n, &pos, g); !ok || o != a {
		t.Fatal("the whole position overlaps a claimed lane")
	}
	if _, ok := tb.Held(n, &y, g); ok {
		t.Fatal("another lane is free")
	}
	if _, ok := tb.Held(n, &x, a); ok {
		t.Fatal("an owner does not conflict with itself")
	}
	if _, ok := tb.Held(n, &scale, g); ok {
		t.Fatal("scale is another property")
	}
	if _, ok := tb.Held(scene.NodeHandle{Index: 3, Gen: 1}, &x, g); ok {
		t.Fatal("another node is free")
	}
}

func TestEnsure(t *testing.T) {
	s := services{}
	if Of(s) != nil {
		t.Fatal("no table yet")
	}
	tb := Ensure(s)
	if Ensure(s) != tb || Of(s) != tb {
		t.Fatal("Ensure keeps one table per world")
	}
}

func TestSlotFollowsRevision(t *testing.T) {
	tb := New()
	a, g := tb.Register("anim"), tb.Register("gameplay")
	n := scene.NodeHandle{Index: 4, Gen: 1}
	pos := propref.Transform(false, propref.PartPosition)
	k := KeyOf(n, &pos)
	var slot Slot
	round := func() {
		tb.Begin(a)
		tb.ClaimAt(a, k, &pos, &slot)
		tb.End(a)
	}
	round()
	rev := tb.Revision()
	for range 3 {
		round()
	}
	if tb.Revision() != rev {
		t.Fatal("rounds through a slot must keep the revision")
	}
	tb.Begin(g)
	tb.Claim(g, n, &pos)
	tb.End(g)
	round()
	if o, ok := tb.HeldKey(k, &pos, g); !ok || o != a {
		t.Fatal("the claim must survive a stale slot")
	}
	if o, ok := tb.HeldKey(k, &pos, a); !ok || o != g {
		t.Fatal("the other owner must be seen")
	}
	tb.Drop(g)
	if _, ok := tb.HeldKey(k, &pos, a); ok {
		t.Fatal("an owner alone holds nothing against itself")
	}
}
