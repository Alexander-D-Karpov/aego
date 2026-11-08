package art

import (
	"context"
	"errors"
	"image"
	"testing"
)

func TestSnapshotStableAndRejectsStalePublish(t *testing.T) {
	d, err := New(8, 8, RGBA, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	before, _ := s.Encode()
	info := d.Info()
	change, _, err := d.Propose(context.Background(), d.Revision(), "paint", []Operation{{Kind: "stroke", Layer: info.Layers[0].ID, Frame: info.Frames[0].ID, Points: []image.Point{{1, 1}}, Brush: Brush{Size: 1, Paint: Paint{Color: Color{255, 0, 0, 255}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = change.Apply(); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Encode()
	if string(before) != string(after) {
		t.Fatal("immutable snapshot changed")
	}
	called := false
	if err = d.Publish(s, func() error { called = true; return nil }); !errors.Is(err, ErrConflict) || called {
		t.Fatalf("stale publication: %v callback=%v", err, called)
	}
	live, _ := d.Snapshot()
	if err = d.Publish(live, func() error { called = true; return nil }); err != nil || !called {
		t.Fatalf("current publication: %v", err)
	}
	d.Close()
	called = false
	if err = d.Publish(live, func() error { called = true; return nil }); !errors.Is(err, ErrClosed) || called {
		t.Fatalf("closed publication: %v", err)
	}
}
func TestSavedSnapshotDoesNotMarkLaterEditsClean(t *testing.T) {
	d, _ := New(8, 8, RGBA, Limits{})
	s, _ := NewSession(d, NewJournal(Limits{}))
	saved, _ := d.Snapshot()
	if _, err := s.Execute(context.Background(), Operation{Kind: "add-frame", DurationMS: 90}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkSnapshotSaved(saved); err != nil {
		t.Fatal(err)
	}
	if !s.Info().Dirty {
		t.Fatal("later edit marked clean")
	}
	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if s.Info().Dirty {
		t.Fatal("undo to saved snapshot remains dirty")
	}
	other, _ := New(8, 8, RGBA, Limits{})
	foreign, _ := other.Snapshot()
	if err := s.MarkSnapshotSaved(foreign); err == nil {
		t.Fatal("foreign snapshot accepted")
	}
}
func TestSnapshotUsesOriginalDurationAndTagOrder(t *testing.T) {
	d, _ := New(8, 8, RGBA, Limits{})
	e, _ := d.Begin(d.Revision(), "frames")
	a := d.Info().Frames[0].ID
	b, _ := e.AddFrame(a, 70)
	c, _ := e.AddFrame(b, 210)
	tag, _ := e.SetTag(Tag{Name: "walk", From: a, To: c, Direction: PingPong})
	change, err := e.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if err = change.Apply(); err != nil {
		t.Fatal(err)
	}
	snap, _ := d.Snapshot()
	order, err := snap.PlaybackFrames(tag)
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 4 || order[0].ID != a || order[1].ID != b || order[2].ID != c || order[3].ID != b || order[2].DurationMS != 210 {
		t.Fatalf("wrong sequence: %+v", order)
	}
	if _, err = snap.PlaybackFrames("missing"); err == nil {
		t.Fatal("missing tag accepted")
	}
}
