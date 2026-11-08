package art

import (
	"bytes"
	"context"
	"image"
	"reflect"
	"testing"
)

func TestPaletteJSONAndGPLImport(t *testing.T) {
	p := []Swatch{{Name: "Transparent"}, {Color: red, Name: "Красный"}, {Color: Color{1, 2, 3, 96}, Name: "Alpha"}}
	data, err := EncodePalette(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodePalette(data, "json")
	if err != nil || !reflect.DeepEqual(p, got) {
		t.Fatal(got, err)
	}
	if _, err = DecodePalette(append(data, []byte(" {}")...), "json"); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	if _, err = DecodePalette(bytes.Replace(data, []byte(`"version": 1`), []byte(`"version": 99`), 1), "json"); err == nil {
		t.Fatal("future version accepted")
	}
	got, err = DecodePalette([]byte("GIMP Palette\nName: Test\nColumns: 4\n#\n255 0 0 Red\n0 0 255 Blue\n"), "gpl")
	if err != nil || len(got) != 3 || got[0].Color.A != 0 || got[1].Color != red {
		t.Fatal(got, err)
	}
	if _, err = DecodePalette([]byte("GIMP Palette\n256 0 0 Bad"), "gpl"); err == nil {
		t.Fatal("out of range GPL accepted")
	}
}
func TestIndexedPaletteReplacementPreservesPixelsAndLinks(t *testing.T) {
	d, l, f, c := doc(t, Indexed)
	defer d.Close()
	e := edit(t, d)
	p := []Swatch{{}, {Color: red}, {Color: blue}}
	if err := e.ReplacePalette(p, false); err != nil {
		t.Fatal(err)
	}
	g, _ := e.Stroke(c, brush(red), nil)
	_ = g.Point(image.Pt(1, 1))
	f2, _ := e.AddFrame(f, 100)
	c2, _ := e.CopyCel(c, l, f2, true)
	done(t, e)
	before := hash(t, d, f)
	imageID := d.Info().Cels[0].Image
	e = edit(t, d)
	if err := e.ReplacePalette([]Swatch{{}, {Color: blue}, {Color: red}}, false); err != nil {
		t.Fatal(err)
	}
	ch := done(t, e)
	if hash(t, d, f) != before || d.Info().Cels[1].Image != imageID {
		t.Fatal("palette reorder changed pixels or links")
	}
	pixel(t, d, c2, 1, 1, red)
	if err := ch.Undo(); err != nil {
		t.Fatal(err)
	}
	e = edit(t, d)
	if err := e.ReplacePalette([]Swatch{{}, {Color: blue}}, false); err == nil {
		t.Fatal("lossy palette replacement not explicit")
	}
	e.Cancel()
}
func TestPaletteExtractionQuantizationAndDataChannels(t *testing.T) {
	d, l, f, c := doc(t, RGBA)
	defer d.Close()
	e := edit(t, d)
	g, _ := e.Stroke(c, brush(red), nil)
	_ = g.Point(image.Pt(1, 1))
	_ = g.Point(image.Pt(3, 1))
	g, _ = e.Stroke(c, brush(blue), nil)
	_ = g.Point(image.Pt(5, 5))
	normal, _ := e.EnsureCel(l, f, Normal)
	g, _ = e.Stroke(normal, brush(Color{127, 128, 255, 255}), nil)
	_ = g.Point(image.Pt(0, 0))
	done(t, e)
	e = edit(t, d)
	if err := e.PaletteFromFrame(context.Background(), f, 2, true); err != nil {
		t.Fatal(err)
	}
	ch := done(t, e)
	if len(d.Info().Palette) != 2 || d.Info().Palette[1].Color != red {
		t.Fatal(d.Info().Palette)
	}
	pixel(t, d, normal, 0, 0, Color{127, 128, 255, 255})
	if err := ch.Undo(); err != nil {
		t.Fatal(err)
	}
	pixel(t, d, c, 5, 5, blue)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e = edit(t, d)
	if err := e.PaletteFromFrame(ctx, f, 8, false); err != context.Canceled {
		t.Fatal(err)
	}
	e.Cancel()
}
func TestRampShadeSelectionAndUndo(t *testing.T) {
	d, _, _, c := doc(t, Indexed)
	defer d.Close()
	e := edit(t, d)
	p := []Swatch{{}, {Color: Color{40, 40, 40, 255}}, {Color: Color{100, 100, 100, 255}}, {Color: Color{200, 200, 200, 255}}}
	_ = e.ReplacePalette(p, false)
	g, _ := e.Stroke(c, brush(p[1].Color), nil)
	_ = g.Point(image.Pt(1, 1))
	_ = g.Point(image.Pt(3, 1))
	done(t, e)
	selection, _ := RectSelection(16, 16, image.Rect(2, 1, 3, 2), false)
	e = edit(t, d)
	if err := e.ShadeRamp(context.Background(), c, selection, 1, 3, 1); err != nil {
		t.Fatal(err)
	}
	ch := done(t, e)
	pixel(t, d, c, 1, 1, p[1].Color)
	pixel(t, d, c, 2, 1, p[2].Color)
	if err := ch.Undo(); err != nil {
		t.Fatal(err)
	}
	pixel(t, d, c, 2, 1, p[1].Color)
}
