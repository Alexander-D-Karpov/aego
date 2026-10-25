package animedit_test

import (
	"testing"

	"aego/editor/core/animedit"
)

func TestViewGroupsTracksAndKeepsFileOrderOfEvents(t *testing.T) {
	r := newRig(t)
	v := r.svc.View()
	if len(v.Rows) != 3 || v.Rows[0].Track != -1 || v.Rows[0].Label != "This node" {
		t.Fatalf("rows %+v", v.Rows)
	}
	if v.Rows[1].Label != "SpriteRenderer.tint" || len(v.Rows[1].Keys) != 2 {
		t.Fatalf("tint row %+v", v.Rows[1])
	}
	if len(v.Events) != 2 || v.Events[0].Name != "Mark" || v.Events[1].Index != 1 {
		t.Fatalf("events %+v", v.Events)
	}
}

func TestMoveKeysOrdersMovesAndUndoes(t *testing.T) {
	r := newRig(t)
	before := string(r.files[clipRel])
	keys := []animedit.KeyRef{{Track: 0, T: 0}, {Track: 0, T: 1}}
	moved, err := r.svc.MoveKeys(keys, -0.25)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := r.clip().Clip()
	if k := c.Tracks[0].Keys; len(k) != 2 || k[0].T != 0 || !near(k[1].T, 0.75) {
		t.Fatalf("keys %+v", k)
	}
	if len(moved) != 2 {
		t.Fatalf("moved %+v", moved)
	}
	if _, err := r.svc.MoveKeys([]animedit.KeyRef{{Track: 0, T: 0}}, 0.75); err == nil {
		t.Fatal("a key moved onto another key")
	}
	if err := r.d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if string(r.files[clipRel]) != before {
		t.Fatal("undo of the move did not restore the clip")
	}
	if err := r.svc.SetInterp([]animedit.KeyRef{{Track: 0, T: 0}}, "step"); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.RemoveKeys([]animedit.KeyRef{{Track: 1, T: 0}, {Track: 1, T: 1}}); err != nil {
		t.Fatal(err)
	}
	c, _ = r.clip().Clip()
	if len(c.Tracks) != 1 || c.Tracks[0].Keys[0].Interp != 2 {
		t.Fatalf("after interp and delete %+v", c.Tracks)
	}
}
