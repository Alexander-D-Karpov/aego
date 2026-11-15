package art

import (
	"context"
	"image"
	"testing"
)

func TestAnimationClipboardPreservesCelsGroupsAndChannels(t *testing.T) {
	src, l, f, c := doc(t, RGBA)
	e := edit(t, src)
	group, _ := e.AddLayer("Group", Group, "")
	layer := e.Info().Layers[0]
	layer.Parent = group
	_ = e.SetLayer(layer)
	g, _ := e.Stroke(c, brush(red), nil)
	_ = g.Point(image.Pt(1, 1))
	f2, _ := e.DuplicateFrame(f, true)
	_ = e.SetDuration(f2, 80)
	tag, _ := e.SetTag(Tag{Name: "Walk", From: f, To: f2, Direction: PingPong})
	_ = tag
	normal, _ := e.EnsureCel(l, f, Normal)
	g, _ = e.Stroke(normal, brush(Color{128, 128, 255, 255}), nil)
	_ = g.Point(image.Pt(1, 1))
	done(t, e)
	clip, err := src.CopyAnimation([]ID{f, f2})
	if err != nil {
		t.Fatal(err)
	}
	// Editing after Copy and then closing the source cannot change the clipboard.
	e = edit(t, src)
	g, _ = e.Stroke(c, brush(blue), nil)
	_ = g.Point(image.Pt(1, 1))
	done(t, e)
	src.Close()
	dst, _, df, _ := doc(t, RGBA)
	defer dst.Close()
	e = edit(t, dst)
	frames, err := e.PasteAnimation(context.Background(), clip, df)
	if err != nil {
		t.Fatal(err)
	}
	ch := done(t, e)
	info := dst.Info()
	if len(frames) != 2 || len(info.Frames) != 3 || len(info.Layers) != 3 || info.Frames[2].DurationMS != 80 {
		t.Fatal(info)
	}
	if len(info.Tags) != 1 || info.Tags[0].From != frames[0] || info.Tags[0].Direction != PingPong {
		t.Fatal(info.Tags)
	}
	var copied []Cel
	for _, cel := range info.Cels {
		if cel.Channel == Albedo && cel.Frame != df {
			copied = append(copied, cel)
		}
	}
	if len(copied) != 2 || copied[0].Image != copied[1].Image {
		t.Fatal(copied)
	}
	pixel(t, dst, copied[0].ID, 1, 1, red)
	if err = ch.Undo(); err != nil {
		t.Fatal(err)
	}
	if len(dst.Info().Frames) != 1 {
		t.Fatal("paste is not one undo")
	}
	if err = ch.Apply(); err != nil {
		t.Fatal(err)
	}
	pixel(t, dst, copied[1].ID, 1, 1, red)
}
func TestAnimationClipboardRejectsImplicitQuantizationAndCanvasMismatch(t *testing.T) {
	src, _, f, c := doc(t, RGBA)
	defer src.Close()
	e := edit(t, src)
	g, _ := e.Stroke(c, brush(Color{17, 19, 23, 255}), nil)
	_ = g.Point(image.Pt(1, 1))
	done(t, e)
	clip, _ := src.CopyAnimation([]ID{f})
	dst, _, df, _ := doc(t, Indexed)
	defer dst.Close()
	e = edit(t, dst)
	if _, err := e.PasteAnimation(context.Background(), clip, df); err == nil {
		t.Fatal("silently quantized imported animation")
	}
	e.Cancel()
	if len(dst.Info().Frames) != 1 {
		t.Fatal("failed import mutated target")
	}
	small, _ := New(8, 8, RGBA, Limits{})
	defer small.Close()
	e = edit(t, small)
	if _, err := e.PasteAnimation(context.Background(), clip, small.Info().Frames[0].ID); err == nil {
		t.Fatal("silently cropped imported animation")
	}
	e.Cancel()
}
func TestImportedCOWPixelsAndDeletedPixelsCountAgainstHistoryBudget(t *testing.T) {
	src, _ := New(128, 128, RGBA, Limits{})
	defer src.Close()
	si := src.Info()
	e := edit(t, src)
	if err := e.Shape(si.Cels[0].ID, "rectangle", image.Pt(0, 0), image.Pt(127, 127), brush(red), nil, true); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	clip, _ := src.CopyAnimation([]ID{si.Frames[0].ID})
	dst, _ := New(128, 128, RGBA, Limits{})
	defer dst.Close()
	e = edit(t, dst)
	if _, err := e.PasteAnimation(context.Background(), clip, dst.Info().Frames[0].ID); err != nil {
		t.Fatal(err)
	}
	change, err := e.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if change.Bytes() < 65536 {
		t.Fatalf("imported immutable tiles not charged: %d", change.Bytes())
	}
	j := NewJournal(Limits{MaxHistoryBytes: 32768})
	if err = j.Exec(change); err == nil {
		t.Fatal("undo budget bypassed by cross-document COW")
	}
	if len(dst.Info().Frames) != 1 {
		t.Fatal("rejected history transaction applied")
	}
	if err = change.Apply(); err != nil {
		t.Fatal(err)
	}
	e = edit(t, dst)
	if err = e.DeleteLayer(dst.Info().Layers[1].ID); err != nil {
		t.Fatal(err)
	}
	deleted, err := e.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Bytes() < 65536 {
		t.Fatal("deleted pixel retention not charged")
	}
}
