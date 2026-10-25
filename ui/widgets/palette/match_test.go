package palette

import (
	"fmt"
	"testing"
)

var names = []string{
	"Virtual Camera 2D", "Camera Follow 2D", "Camera Shake 2D", "Camera Zone 2D", "Camera Brain 2D",
	"Box Collider 2D", "Circle Collider 2D", "Capsule Collider 2D", "Hitbox 2D", "Hurtbox 2D",
	"Motion 2D", "Rigid Body 2D", "Health", "State Machine", "Sprite Renderer", "Animator", "Animation Player",
}

func rank(q string) []string {
	p := New(0)
	items := make([]Item, len(names))
	for i, n := range names {
		items[i] = Item{Key: n, Label: n}
	}
	p.SetItems(items)
	p.SetQuery(q)
	var out []string
	for _, h := range p.hits {
		out = append(out, items[h.item].Label)
	}
	return out
}

func TestMatchRanks(t *testing.T) {
	for _, tc := range []struct {
		q, first string
		has      []string
	}{
		{"vc", "Virtual Camera 2D", nil},
		{"VC", "Virtual Camera 2D", nil},
		{"box", "Box Collider 2D", []string{"Hitbox 2D", "Hurtbox 2D"}},
		{"mot", "Motion 2D", nil},
		{"cf2", "Camera Follow 2D", nil},
		{"fol", "Camera Follow 2D", nil},
		{"camfol", "Camera Follow 2D", nil},
		{"rb", "Rigid Body 2D", nil},
	} {
		got := rank(tc.q)
		if len(got) == 0 || got[0] != tc.first {
			t.Errorf("%q: got %v, want %q first", tc.q, got, tc.first)
		}
		for _, h := range tc.has {
			found := false
			for _, g := range got {
				found = found || g == h
			}
			if !found {
				t.Errorf("%q: %q is missing from %v", tc.q, h, got)
			}
		}
	}
}

func TestMatchTiers(t *testing.T) {
	for _, tc := range []struct {
		q, label string
		tier     Tier
	}{
		{"mot", "Motion 2D", TierPrefix},
		{"vc", "Virtual Camera 2D", TierInitials},
		{"cam", "Virtual Camera 2D", TierWord},
		{"tion", "Motion 2D", TierSubstring},
		{"xyz", "Motion 2D", TierNone},
		{"body", "RigidBody2D", TierWord},
	} {
		txt := Prepare(tc.label)
		if got, _ := Match(Lower(tc.q, nil), &txt, nil); got != tc.tier {
			t.Errorf("%q in %q: tier %d, want %d", tc.q, tc.label, got, tc.tier)
		}
	}
}

func TestMatchPositionsAreBytes(t *testing.T) {
	txt := Prepare("Ёлка Box")
	tier, pos := Match(Lower("bo", nil), &txt, nil)
	if tier != TierWord || fmt.Sprint(pos) != "[9 10]" {
		t.Fatalf("tier %d pos %v", tier, pos)
	}
}

func TestMatchCyrillicQuery(t *testing.T) {
	if got := rank("ДВИЖ"); len(got) != 0 {
		t.Fatalf("a Cyrillic query matched %v", got)
	}
	txt := Prepare("Движение 2D")
	if tier, _ := Match(Lower("дВи", nil), &txt, nil); tier != TierPrefix {
		t.Fatalf("Cyrillic prefix: tier %d", tier)
	}
	rank("\xff\xfe")
}

func TestUsesBreakTies(t *testing.T) {
	p := New(0)
	p.SetItems([]Item{{Key: "a", Label: "Hurtbox 2D"}, {Key: "b", Label: "Hitbox 2D", Uses: 3}})
	p.SetQuery("box")
	if p.items[p.hits[0].item].Label != "Hitbox 2D" {
		t.Fatal("the more used item must come first within a tier")
	}
}

func BenchmarkFilter300(b *testing.B) {
	p := New(0)
	items := make([]Item, 300)
	for i := range items {
		n := names[i%len(names)] + fmt.Sprint(" ", i)
		items[i] = Item{Key: n, Label: n, Category: "Category"}
	}
	p.SetItems(items)
	qs := []string{"c", "ca", "cam", "camf", "box", "vc", "cf2"}
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		p.SetQuery(qs[i%len(qs)])
	}
}
