package art

import (
	"context"
	"image"
	"reflect"
	"testing"
)

func TestOverlappingCelRangeMovesOriginalSnapshots(t *testing.T) {
	d, l, f0, c0 := doc(t, RGBA)
	defer d.Close()
	e := edit(t, d)
	_, _ = e.Execute(context.Background(), Operation{Kind: "stroke", Cel: c0, Points: []image.Point{{1, 1}}, Brush: brush(red)}, nil)
	f1, err := e.AddFrame(f0, 80)
	if err != nil {
		t.Fatal(err)
	}
	c1, _ := e.EnsureCel(l, f1, Albedo)
	_, _ = e.Execute(context.Background(), Operation{Kind: "stroke", Cel: c1, Points: []image.Point{{1, 1}}, Brush: brush(blue)}, nil)
	f2, _ := e.AddFrame(f1, 120)
	done(t, e)
	e = edit(t, d)
	if err = e.TransferCels([]CellSlot{{l, f0, Albedo}, {l, f1, Albedo}}, 0, 1, false, false, false); err != nil {
		t.Fatal(err)
	}
	change := done(t, e)
	info := d.Info()
	if len(info.Cels) != 2 {
		t.Fatal(info.Cels)
	}
	if info.Cels[0].ID != c0 || info.Cels[0].Frame != f1 || info.Cels[1].ID != c1 || info.Cels[1].Frame != f2 {
		t.Fatal("move did not preserve identities", info.Cels)
	}
	pixel(t, d, c0, 1, 1, red)
	pixel(t, d, c1, 1, 1, blue)
	if err = change.Undo(); err != nil {
		t.Fatal(err)
	}
	if d.Info().Cels[0].Frame != f0 {
		t.Fatal("undo did not restore source")
	}
}
func TestRangeCopyKeepsInternalLinksButNotSourceLinks(t *testing.T) {
	d, l, f0, c := doc(t, RGBA)
	defer d.Close()
	e := edit(t, d)
	f1, _ := e.AddFrame(f0, 100)
	_, _ = e.CopyCel(c, l, f1, true)
	f2, _ := e.AddFrame(f1, 100)
	_, _ = e.AddFrame(f2, 100)
	done(t, e)
	e = edit(t, d)
	if err := e.TransferCels([]CellSlot{{l, f0, Albedo}, {l, f1, Albedo}}, 0, 2, true, false, false); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	i := d.Info()
	if len(i.Cels) != 4 {
		t.Fatal(i.Cels)
	}
	if i.Cels[2].Image != i.Cels[3].Image || i.Cels[2].Image == i.Cels[0].Image {
		t.Fatal("copy link topology", i.Cels)
	}
	e = edit(t, d)
	g, _ := e.Stroke(i.Cels[2].ID, brush(red), nil)
	_ = g.Point(image.Pt(2, 2))
	done(t, e)
	pixel(t, d, i.Cels[0].ID, 2, 2, Color{})
	pixel(t, d, i.Cels[3].ID, 2, 2, red)
}
func TestEmptyRangeCellsAndOverwriteGuard(t *testing.T) {
	d, l, f0, c := doc(t, RGBA)
	defer d.Close()
	e := edit(t, d)
	f1, _ := e.AddFrame(f0, 100)
	done(t, e)
	before := d.Info()
	e = edit(t, d)
	if err := e.TransferCels([]CellSlot{{l, f1, Albedo}}, 0, -1, false, false, false); err == nil {
		t.Fatal("overwrote an occupied cell without confirmation")
	}
	e.Cancel()
	if !reflect.DeepEqual(before, d.Info()) {
		t.Fatal("failed operation published")
	}
	e = edit(t, d)
	if err := e.TransferCels([]CellSlot{{l, f1, Albedo}}, 0, -1, false, false, true); err != nil {
		t.Fatal(err)
	}
	change := done(t, e)
	if len(d.Info().Cels) != 0 {
		t.Fatal("moving empty slot must clear destination")
	}
	if err := change.Undo(); err != nil {
		t.Fatal(err)
	}
	if d.Info().Cels[0].ID != c {
		t.Fatal("lost cel ID")
	}
}
func TestRangePreflightLocksAndBounds(t *testing.T) {
	d, l, f, c := doc(t, RGBA)
	defer d.Close()
	e := edit(t, d)
	other, _ := e.AddLayer("Target", Raster, "")
	f2, _ := e.AddFrame(f, 100)
	_ = f2
	layer := e.Info().Layers[0]
	layer.Locked = true
	_ = e.SetLayer(layer)
	done(t, e)
	e = edit(t, d)
	if err := e.TransferCels([]CellSlot{{l, f, Albedo}}, 0, 1, false, false, false); err != ErrLocked {
		t.Fatal(err)
	}
	e.Cancel()
	e = edit(t, d)
	if err := e.TransferCels([]CellSlot{{l, f, Albedo}}, 1, 1, true, false, false); err != nil {
		t.Fatal("read-only source copy failed", err)
	}
	done(t, e)
	i := d.Info()
	if i.Cels[1].Layer != other || i.Cels[1].Image == i.Cels[0].Image {
		t.Fatal(i.Cels)
	}
	e = edit(t, d)
	if err := e.TransferCels([]CellSlot{{l, f, Albedo}}, 50, 0, true, false, false); err == nil {
		t.Fatal("out of bounds accepted")
	}
	e.Cancel()
	pixel(t, d, c, 0, 0, Color{})
}
func TestFrameReorderKeepsTagMembersAndRejectsSplit(t *testing.T) {
	d, _, a, _ := doc(t, RGBA)
	defer d.Close()
	e := edit(t, d)
	b, _ := e.AddFrame(a, 80)
	c, _ := e.AddFrame(b, 120)
	tag, _ := e.SetTag(Tag{Name: "AB", From: a, To: b, Direction: Forward})
	done(t, e)
	e = edit(t, d)
	if err := e.ReorderFrames([]ID{a, c, b}); err == nil {
		t.Fatal("split tag silently changed membership")
	}
	e.Cancel()
	e = edit(t, d)
	if err := e.ReorderFrames([]ID{c, b, a}); err != nil {
		t.Fatal(err)
	}
	change := done(t, e)
	i := d.Info()
	if i.Tags[0].From != b || i.Tags[0].To != a || i.Frames[0].DurationMS != 120 {
		t.Fatal(i)
	}
	if err := change.Undo(); err != nil {
		t.Fatal(err)
	}
	e = edit(t, d)
	if err := e.DeleteTag(tag); err != nil {
		t.Fatal(err)
	}
	change = done(t, e)
	if len(d.Info().Tags) != 0 {
		t.Fatal("tag not deleted")
	}
	if err := change.Undo(); err != nil {
		t.Fatal(err)
	}
	if d.Info().Tags[0].ID != tag {
		t.Fatal("tag ID changed on undo")
	}
}
