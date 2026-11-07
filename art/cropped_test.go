package art

import (
	"image"
	"reflect"
	"testing"
)

func TestCroppedCelGrowsWithoutBreakingLinksOrCopies(t *testing.T) {
	d, layer, frame, cel := doc(t, RGBA)
	e := edit(t, d)
	ci, _ := e.work.cel(cel)
	original := e.work.Cels[ci]
	original.X, original.Y = 5, 6
	e.work.Cels[ci] = original
	e.work.Images[original.Image] = newPixels(2, 2)
	if err := e.SetPixel(cel, 5, 6, Paint{Color: red}, nil, false); err != nil {
		t.Fatal(err)
	}
	linkedFrame, err := e.AddFrame(frame, 100)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := e.CopyCel(cel, layer, linkedFrame, true)
	if err != nil {
		t.Fatal(err)
	}
	li, _ := e.work.cel(linked)
	e.work.Cels[li].X, e.work.Cels[li].Y = 3, 4
	copyFrame, err := e.AddFrame(linkedFrame, 100)
	if err != nil {
		t.Fatal(err)
	}
	independent, err := e.CopyCel(cel, layer, copyFrame, false)
	if err != nil {
		t.Fatal(err)
	}
	done(t, e)
	before := d.Info()
	e = edit(t, d)
	b := brush(blue)
	b.PixelPerfect = true
	g, err := e.Stroke(cel, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []image.Point{{2, 2}, {3, 2}, {3, 3}, {15, 15}} {
		if err := g.Point(p); err != nil {
			t.Fatal(err)
		}
	}
	ch := done(t, e)
	pixel(t, d, cel, 2, 2, blue)
	pixel(t, d, cel, 3, 2, Color{}) // Pixel-perfect restoration after growth.
	pixel(t, d, cel, 15, 15, blue)
	pixel(t, d, cel, 5, 6, red)
	pixel(t, d, linked, 0, 0, blue)
	pixel(t, d, linked, 3, 4, red)
	pixel(t, d, independent, 2, 2, Color{})
	pixel(t, d, independent, 5, 6, red)
	if err = ch.Undo(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Cels, d.Info().Cels) {
		t.Fatal("undo failed to restore cropped offsets and shared image identities")
	}
	if err = ch.Apply(); err != nil {
		t.Fatal(err)
	}
	pixel(t, d, cel, 15, 15, blue)
}

func TestCroppedCelEraserAndAlphaLockDoNotGrow(t *testing.T) {
	d, _, _, cel := doc(t, RGBA)
	e := edit(t, d)
	c := e.work.Cels[0]
	c.X, c.Y = 5, 5
	e.work.Cels[0] = c
	e.work.Images[c.Image] = newPixels(2, 2)
	done(t, e)
	for _, alphaLock := range []bool{false, true} {
		e = edit(t, d)
		e.work.Layers[0].AlphaLock = alphaLock
		if err := e.SetPixel(cel, 0, 0, Paint{Color: red}, nil, !alphaLock); err != nil {
			t.Fatal(err)
		}
		if p := e.work.Images[c.Image]; p.Width != 2 || p.Height != 2 {
			t.Fatal("erasing transparent pixels or alpha lock expanded the cel")
		}
		e.Cancel()
	}
}

func TestThumbnailUsesLivePixelsAndDoesNotMutateDocument(t *testing.T) {
	d, layer, frame, cel := doc(t, RGBA)
	e := edit(t, d)
	if err := e.Shape(cel, "rectangle", image.Pt(0, 0), image.Pt(15, 15), brush(red), nil, true); err != nil {
		t.Fatal(err)
	}
	e.work.Layers[0].Visible = false
	done(t, e)
	rev := d.Revision()
	im, err := d.Thumbnail(layer, frame, RenderOptions{}, 16)
	if err != nil {
		t.Fatal(err)
	}
	if got := im.NRGBAAt(8, 8); got.R != 255 || got.A != 255 {
		t.Fatal("hidden layer thumbnail lost its pixels", got)
	}
	if d.Revision() != rev {
		t.Fatal("thumbnail changed the document")
	}
}

func TestIndependentCopiesFreezeDraftOwnedTiles(t *testing.T) {
	for _, operation := range []string{"copy", "unlink", "duplicate-layer"} {
		t.Run(operation, func(t *testing.T) {
			d, layer, frame, cel := doc(t, RGBA)
			e := edit(t, d)
			if err := e.SetPixel(cel, 1, 1, Paint{Color: red}, nil, false); err != nil {
				t.Fatal(err)
			}
			var copied ID
			if operation == "duplicate-layer" {
				other, err := e.DuplicateLayer(layer)
				if err != nil {
					t.Fatal(err)
				}
				ci, _ := e.work.findCel(other, frame, Albedo)
				copied = e.work.Cels[ci].ID
			} else {
				other, err := e.AddFrame(frame, 100)
				if err != nil {
					t.Fatal(err)
				}
				copied, err = e.CopyCel(cel, layer, other, operation == "unlink")
				if err != nil {
					t.Fatal(err)
				}
				if operation == "unlink" {
					if err := e.Unlink(copied); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := e.SetPixel(cel, 1, 1, Paint{Color: blue}, nil, false); err != nil {
				t.Fatal(err)
			}
			if err := e.SetPixel(copied, 2, 2, Paint{Color: red}, nil, false); err != nil {
				t.Fatal(err)
			}
			done(t, e)
			pixel(t, d, cel, 1, 1, blue)
			pixel(t, d, copied, 1, 1, red)
			pixel(t, d, cel, 2, 2, Color{})
			pixel(t, d, copied, 2, 2, red)
		})
	}
}

func TestDraftPixelAllocationIsBoundedBeforePrepare(t *testing.T) {
	d, err := New(32, 32, RGBA, Limits{MaxPixelBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	cel := d.Info().Cels[0].ID
	e := edit(t, d)
	if err = e.SetPixel(cel, 0, 0, Paint{Color: red}, nil, false); err != nil {
		t.Fatal(err)
	}
	if err = e.SetPixel(cel, 31, 31, Paint{Color: red}, nil, false); err != ErrLimit {
		t.Fatal("draft must enforce the allocation budget before commit", err)
	}
	e.Cancel()
	pixel(t, d, cel, 0, 0, Color{})
}
