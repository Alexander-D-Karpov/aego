package art

import (
	"context"
	"errors"
	"fmt"
	"image"
	"reflect"
	"testing"
)

func TestBatchEightFramesAliasesAndUserUndo(t *testing.T) {
	s, j := sessionForTest(t)
	base := s.Document.Info()
	brush := Brush{Size: 1, Paint: Paint{Color: Color{255, 20, 40, 255}}}
	ops := []Operation{{Kind: "stroke", Layer: s.Layer, Frame: s.Frame, Brush: brush, Points: []image.Point{{1, 1}}}}
	last := s.Frame
	for n := 1; n < 8; n++ {
		name := fmt.Sprintf("frame%d", n)
		ops = append(ops, Operation{Kind: "add-frame", Frame: last, DurationMS: 80 + n, As: name})
		last = ID("$" + name)
		ops = append(ops, Operation{Kind: "stroke", Layer: s.Layer, Frame: last, Brush: brush, Points: []image.Point{{n + 1, 1}}})
	}
	ops = append(ops, Operation{Kind: "tag", Tag: &Tag{Name: "walk", From: s.Frame, To: last, Direction: Forward}})
	change, ids, err := s.Document.Propose(context.Background(), base.Revision, "eight frames", ops)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(base, s.Document.Info()) {
		t.Fatal("proposal mutated visible state")
	}
	if len(ids) != len(ops) {
		t.Fatal("missing operation results")
	}
	if err := j.Exec(change); err != nil {
		t.Fatal(err)
	}
	info := s.Document.Info()
	if len(info.Frames) != 8 || len(info.Tags) != 1 || s.Info().Undo != 1 {
		t.Fatal("batch must be one complete undo step")
	}
	frame := info.Frames[4].ID
	im, err := s.Document.Render(frame, RenderOptions{})
	if err != nil || im.NRGBAAt(5, 1).A != 255 {
		t.Fatalf("frame pixels: %v", err)
	}
	stale, _, err := s.Document.Propose(context.Background(), info.Revision, "stale agent", []Operation{{Kind: "duration", Frame: frame, DurationMS: 300}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Execute(context.Background(), Operation{Kind: "stroke", Frame: frame, Layer: s.Layer, Brush: brush, Points: []image.Point{{12, 12}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Exec(stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale proposal: %v", err)
	}
	snapshot, err := s.Document.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	im, err = snapshot.Render(frame, RenderOptions{})
	if err != nil || im.NRGBAAt(12, 12).A != 255 {
		t.Fatalf("dirty capture: %v", err)
	}
	if err = s.Undo(); err != nil {
		t.Fatal(err)
	}
	if err = s.Undo(); err != nil {
		t.Fatal(err)
	}
	restored := s.Document.Info()
	restored.Revision = base.Revision
	if !reflect.DeepEqual(base, restored) {
		t.Fatal("batch undo did not restore content and links")
	}
}
func TestBatchRejectsBadReferencesAtomically(t *testing.T) {
	for _, ops := range [][]Operation{
		{{Kind: "add-frame", As: "same"}, {Kind: "add-frame", As: "same"}},
		{{Kind: "add-layer", Name: "x", LayerKind: Raster, As: "../bad"}},
		{{Kind: "add-layer", Name: "x", LayerKind: Raster, As: "ok"}, {Kind: "add-frame", Frame: "$missing"}},
	} {
		s, _ := sessionForTest(t)
		before := s.Document.Info()
		if _, _, err := s.Document.Propose(context.Background(), before.Revision, "invalid", ops); err == nil {
			t.Fatal("invalid batch accepted")
		}
		if !reflect.DeepEqual(before, s.Document.Info()) {
			t.Fatal("failed batch leaked changes")
		}
	}
}
func TestDecodeOperationsBoundsAndSchema(t *testing.T) {
	for _, raw := range []string{"null", "[]", `[{'kind':'stroke'}]`, `[{"kind":"add-frame","shell":"bad"}]`, `[{"kind":"add-frame"}] null`} {
		if _, err := DecodeOperations([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if _, err := DecodeOperations(make([]byte, MaxOperationBytes+1)); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if _, err := DecodeOperations([]byte(`[{"kind":"add-frame","as":"next","durationMS":100}]`)); err != nil {
		t.Fatal(err)
	}
}
