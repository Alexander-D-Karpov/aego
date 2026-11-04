package texture_test

import (
	"aego/asset"
	"aego/core/ids"
	"slices"
	"testing"
)

// A stable ID must win even after authoring order changes on either side.
// Check the persisted metadata and each reimported AERG, not only the in-memory list.
func TestAV08NormalFramePairsSurviveExplicitPermutation(t *testing.T) {
	pl, colorID, normalID := pairProject(t, false)
	colors, normals := cells(2), cells(2)
	want := map[ids.AssetID]ids.AssetID{colors[0].ID: normals[0].ID, colors[1].ID: normals[1].ID}
	if len(want) != 2 || normals[0].ID == normals[1].ID {
		t.Fatal("fixture must have two distinct pairs")
	}
	if err := pl.SetSubs(colorID, colors); err != nil {
		t.Fatal(err)
	}
	if err := pl.SetSubs(normalID, normals); err != nil {
		t.Fatal(err)
	}
	verify := func(stage string) {
		t.Helper()
		b, err := pl.ImportAll()
		if err != nil || len(b.Failed) != 0 {
			t.Fatalf("%s import: %v %+v", stage, err, b)
		}
		meta, ok := pl.MetaOf(colorID)
		if !ok {
			t.Fatal("missing albedo metadata")
		}
		for color, expected := range want {
			if got := regionFromPipeline(t, pl, color).Normal; got != expected {
				t.Fatalf("%s %s rebound to %s, want %s", stage, color, got, expected)
			}
			got, explicit, err := asset.TextureChannel(&meta, color, "normal")
			if err != nil || !explicit || got != expected {
				t.Fatalf("%s stored frame link %s: %s, explicit=%t, err=%v", stage, color, got, explicit, err)
			}
			if err := pl.VerifyCurrent(color); err != nil {
				t.Fatalf("%s stale artifact: %v", stage, err)
			}
		}
	}
	verify("initial")
	slices.Reverse(normals)
	if err := pl.SetSubs(normalID, normals); err != nil {
		t.Fatal(err)
	}
	nm, _ := pl.MetaOf(normalID)
	if nm.Subs[0].ID != normals[0].ID {
		t.Fatal("normal order was not actually changed")
	}
	verify("normal reversed, albedo unchanged")
	slices.Reverse(colors)
	if err := pl.SetSubs(colorID, colors); err != nil {
		t.Fatal(err)
	}
	verify("albedo also reversed")
	t.Log("two distinct normal IDs retained by both albedo frame IDs through separate normal- and albedo-sheet reversals/reimports")
}
