package art

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestImportPNGFramesPreservesPixelsAndIndependentFrames(t *testing.T) {
	sheet := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			sheet.SetNRGBA(x, y, color.NRGBA{R: uint8(20 + x*40), G: uint8(30 + y*40), B: 90, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, sheet); err != nil {
		t.Fatal(err)
	}
	rects := []image.Rectangle{image.Rect(2, 0, 4, 2), image.Rect(0, 0, 2, 2)}
	doc, err := ImportPNGFrames(encoded.Bytes(), rects, 80, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	info := doc.Info()
	if info.Canvas.Width != 2 || info.Canvas.Height != 2 || len(info.Frames) != 2 || len(info.Cels) != 2 {
		t.Fatalf("unexpected frame document: %+v", info)
	}
	if info.Frames[0].ID == info.Frames[1].ID || info.Cels[0].Image == info.Cels[1].Image {
		t.Fatal("imported frames must have separate identities and pixels")
	}
	for i, frame := range info.Frames {
		if frame.DurationMS != 80 {
			t.Fatalf("frame %d duration is %d", i, frame.DurationMS)
		}
		got, err := doc.Render(frame.ID, RenderOptions{Channel: Albedo})
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				want := sheet.NRGBAAt(rects[i].Min.X+x, rects[i].Min.Y+y)
				if pixel := got.NRGBAAt(x, y); pixel != want {
					t.Fatalf("frame %d pixel (%d,%d): got %v, want %v", i, x, y, pixel, want)
				}
			}
		}
	}
	session, err := NewSession(doc, NewJournal(Limits{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.Execute(context.Background(), Operation{Kind: "stroke", Points: []image.Point{{0, 0}}, Brush: Brush{Size: 1, Paint: Paint{Color: Color{R: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	other, err := doc.Render(info.Frames[1].ID, RenderOptions{Channel: Albedo})
	if err != nil || other.NRGBAAt(0, 0) != sheet.NRGBAAt(0, 0) {
		t.Fatalf("editing one imported frame changed the other: %v", err)
	}
	data, err := doc.Encode()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Decode(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.Info(); len(got.Frames) != 2 || got.Frames[0].ID != info.Frames[0].ID || got.Frames[1].ID != info.Frames[1].ID {
		t.Fatal("saved import did not retain frame identities")
	}
}

func TestImportPNGFramesRejectsInvalidInputAndBudgets(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 4, 2))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		data     []byte
		rects    []image.Rectangle
		duration int
		limits   Limits
	}{
		{"invalid PNG", []byte("not a PNG"), []image.Rectangle{image.Rect(0, 0, 2, 2)}, 100, Limits{}},
		{"no frames", b.Bytes(), nil, 100, Limits{}},
		{"outside image", b.Bytes(), []image.Rectangle{image.Rect(3, 0, 5, 2)}, 100, Limits{}},
		{"negative origin", b.Bytes(), []image.Rectangle{image.Rect(-1, 0, 1, 2)}, 100, Limits{}},
		{"empty frame", b.Bytes(), []image.Rectangle{image.Rect(0, 0, 0, 2)}, 100, Limits{}},
		{"mixed dimensions", b.Bytes(), []image.Rectangle{image.Rect(0, 0, 2, 2), image.Rect(2, 0, 3, 2)}, 100, Limits{}},
		{"duration", b.Bytes(), []image.Rectangle{image.Rect(0, 0, 2, 2)}, 0, Limits{}},
		{"frame limit", b.Bytes(), []image.Rectangle{image.Rect(0, 0, 2, 2), image.Rect(2, 0, 4, 2)}, 100, Limits{MaxFrames: 1}},
		{"cel limit", b.Bytes(), []image.Rectangle{image.Rect(0, 0, 2, 2), image.Rect(2, 0, 4, 2)}, 100, Limits{MaxCels: 1}},
		{"sheet budget", b.Bytes(), []image.Rectangle{image.Rect(0, 0, 2, 2)}, 100, Limits{MaxCanvasPixels: 4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := ImportPNGFrames(tc.data, tc.rects, tc.duration, tc.limits)
			if doc != nil {
				doc.Close()
			}
			if err == nil {
				t.Fatal("invalid import succeeded")
			}
		})
	}
}
