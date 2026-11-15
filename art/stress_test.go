package art

import (
	"image"
	"testing"
)

func stressDocument(b testing.TB) (*Document, ID, ID) {
	b.Helper()
	d, err := New(256, 256, RGBA, Limits{})
	if err != nil {
		b.Fatal(err)
	}
	info := d.Info()
	frame := info.Frames[0].ID
	e := edit(b, d)
	for l := 0; l < 16; l++ {
		layer := info.Layers[0].ID
		if l > 0 {
			layer, err = e.AddLayer("Stress", Raster, "")
			if err != nil {
				b.Fatal(err)
			}
		}
		cel, err := e.EnsureCel(layer, frame, Albedo)
		if err != nil {
			b.Fatal(err)
		}
		if err = e.Shape(cel, "rectangle", image.Pt(l*8, l*8), image.Pt(l*8+31, l*8+31), brush(red), nil, true); err != nil {
			b.Fatal(err)
		}
	}
	for f := 1; f < 64; f++ {
		frame, err = e.DuplicateFrame(frame, false)
		if err != nil {
			b.Fatal(err)
		}
	}
	done(b, e)
	info = d.Info()
	if len(info.Layers) != 16 || len(info.Frames) != 64 || len(info.Cels) != 1024 {
		b.Fatal("stress fixture dimensions are wrong")
	}
	for _, c := range info.Cels {
		if c.Frame == frame {
			return d, frame, c.ID
		}
	}
	b.Fatal("stress cel is missing")
	return nil, "", ""
}

func TestStressDocumentUsesCopyOnWrite(t *testing.T) {
	d, frame, cel := stressDocument(t)
	seen := make(map[*tile]bool)
	for _, im := range d.current.Images {
		for _, block := range im.Tiles {
			seen[block] = true
		}
	}
	if len(seen) > 16*9 {
		t.Fatal("frame duplication copied all pixel blocks", len(seen))
	}
	if _, err := d.Render(frame, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	e := edit(t, d)
	if err := e.SetPixel(cel, 255, 255, Paint{Color: blue}, nil, false); err != nil {
		t.Fatal(err)
	}
	ch := done(t, e)
	if err := ch.Undo(); err != nil {
		t.Fatal(err)
	}
	pixel(t, d, cel, 255, 255, Color{})
}

func BenchmarkStressStroke16x64(b *testing.B) {
	d, frame, cel := stressDocument(b)
	im, err := d.Render(frame, RenderOptions{})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := edit(b, d)
		br := brush(red)
		if i%2 == 0 {
			br = brush(blue)
		}
		if err = e.Line(cel, image.Pt(0, 230), image.Pt(255, 230), br, nil); err != nil {
			b.Fatal(err)
		}
		ch := done(b, e)
		if ch != nil {
			if err = d.RenderRegion(frame, im, ch.DirtyRect(), RenderOptions{}); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkStressFrame16x64(b *testing.B) {
	d, _, _ := stressDocument(b)
	frames := d.Info().Frames
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := d.Render(frames[i%len(frames)].ID, RenderOptions{}); err != nil {
			b.Fatal(err)
		}
	}
}
