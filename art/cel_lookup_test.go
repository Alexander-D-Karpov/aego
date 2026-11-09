package art

import (
	"image"
	"testing"
)

func celLookupDocument(t *testing.T) (*Document, ID, []ID, []ID) {
	t.Helper()
	d, layer, frame, cel := doc(t, RGBA)
	frames, cels := []ID{frame}, []ID{cel}
	e := edit(t, d)
	for len(frames) < 3 {
		frame, err := e.AddFrame(frames[len(frames)-1], 100)
		if err != nil {
			t.Fatal(err)
		}
		cel, err := e.EnsureCel(layer, frame, Albedo)
		if err != nil {
			t.Fatal(err)
		}
		frames, cels = append(frames, frame), append(cels, cel)
	}
	done(t, e)
	t.Cleanup(d.Close)
	return d, layer, frames, cels
}

func TestCelLookupHintFollowsRangeReorder(t *testing.T) {
	d, layer, frames, cels := celLookupDocument(t)
	e := edit(t, d)
	if err := e.SetPixel(cels[0], 1, 1, Paint{Color: red}, nil, false); err != nil {
		t.Fatal(err)
	}
	// The most recently written cel starts at slot1. Moving slot0 over slot2
	// puts it in slot1 and shifts the hinted cel to slot0 within this draft.
	if err := e.SetPixel(cels[1], 2, 2, Paint{Color: red}, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := e.TransferCels([]CellSlot{{layer, frames[0], Albedo}}, 0, 2, false, false, true); err != nil {
		t.Fatal(err)
	}
	if err := e.SetPixel(cels[1], 3, 3, Paint{Color: blue}, nil, false); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	pixel(t, d, cels[0], 1, 1, red)
	pixel(t, d, cels[0], 3, 3, Color{})
	pixel(t, d, cels[1], 2, 2, red)
	pixel(t, d, cels[1], 3, 3, blue)
}

func TestCelLookupHintRejectsRemovedOrTruncatedSlot(t *testing.T) {
	d, _, frames, cels := celLookupDocument(t)
	e := edit(t, d)
	for _, index := range []int{0, 2} {
		if _, err := e.Pixel(cels[index], 1, 1); err != nil {
			t.Fatal(err)
		}
		// First deletion replaces a still-in-range slot; the second truncates
		// the hinted last slot. Neither may resolve to another cel or panic.
		if err := e.DeleteFrame(frames[index]); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Pixel(cels[index], 1, 1); err == nil {
			t.Fatal("read a deleted cel through an old slot")
		}
		if err := e.SetPixel(cels[index], 1, 1, Paint{Color: red}, nil, false); err == nil {
			t.Fatal("wrote to a deleted cel through an old slot")
		}
	}
	if err := e.SetPixel(cels[1], 1, 1, Paint{Color: blue}, nil, false); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	pixel(t, d, cels[1], 1, 1, blue)
}

func TestCelLookupHintReadsCurrentOffsetImageAndLock(t *testing.T) {
	d, layer, frame, cel := doc(t, RGBA)
	defer d.Close()
	e := edit(t, d)
	if err := e.SetPixel(cel, 1, 1, Paint{Color: red}, nil, false); err != nil {
		t.Fatal(err)
	}
	second, err := e.AddFrame(frame, 100)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := e.CopyCel(cel, layer, second, true)
	if err != nil {
		t.Fatal(err)
	}
	// Prime the lookup, then unlink (same cel ID and slot, different Image).
	if _, err = e.Pixel(linked, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err = e.Unlink(linked); err != nil {
		t.Fatal(err)
	}
	g, err := e.Stroke(linked, brush(blue), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.SetCelPosition(linked, 2, 3); err != nil {
		t.Fatal(err)
	}
	if v, err := e.Pixel(linked, 3, 4); err != nil || v != red {
		t.Fatal("read did not use the new cel origin", v, err)
	}
	if err = g.Point(image.Pt(4, 5)); err != nil {
		t.Fatal(err)
	}
	l := e.Info().Layers[0]
	l.Locked = true
	if err = e.SetLayer(l); err != nil {
		t.Fatal(err)
	}
	if err = e.SetPixel(linked, 4, 5, Paint{Color: red}, nil, false); err != ErrLocked {
		t.Fatal("lookup hint bypassed the current layer lock", err)
	}
	done(t, e)
	pixel(t, d, cel, 1, 1, red)
	pixel(t, d, cel, 2, 2, Color{})
	pixel(t, d, linked, 3, 4, red)
	pixel(t, d, linked, 4, 5, blue)
}
