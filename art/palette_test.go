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

func TestAddPaletteEntryPreservesIndexedIdentityAndUndo(t *testing.T) {
	d, layer, frame, cel := doc(t, Indexed)
	defer d.Close()
	e := edit(t, d)
	duplicate := e.Info().Palette[1]
	duplicate.Name = "second instance"
	if err := e.SetPaletteEntry(2, duplicate); err != nil {
		t.Fatal(err)
	}
	if err := e.SetPixel(cel, 2, 3, Paint{UseIndex: true, Index: 2}, nil, false); err != nil {
		t.Fatal(err)
	}
	frame2, err := e.AddFrame(frame, 100)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := e.CopyCel(cel, layer, frame2, true)
	if err != nil {
		t.Fatal(err)
	}
	normal, err := e.EnsureCel(layer, frame, Normal)
	if err != nil {
		t.Fatal(err)
	}
	normalColor := Color{128, 129, 255, 255}
	if err := e.SetPixel(normal, 2, 3, Paint{Color: normalColor}, nil, false); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	before := d.Info()
	s, err := NewSession(d, NewJournal(DefaultLimits()))
	if err != nil {
		t.Fatal(err)
	}
	added := Swatch{Name: "extra", Color: Color{31, 91, 151, 190}}
	if _, err := s.Execute(context.Background(), Operation{Kind: "palette-add", Swatch: &added}); err != nil {
		t.Fatal(err)
	}
	check := func(wantCount int) {
		t.Helper()
		info := d.Info()
		if len(info.Palette) != wantCount || info.Palette[0] != before.Palette[0] || !reflect.DeepEqual(info.Cels, before.Cels) {
			t.Fatal("palette append changed transparent entry or cel links")
		}
		for _, id := range []ID{cel, linked} {
			value, err := d.SamplePaint(id, Albedo, 2, 3)
			if err != nil || !value.UseIndex || value.Index != 2 {
				t.Fatalf("duplicate palette index was remapped: %+v, %v", value, err)
			}
		}
		pixel(t, d, normal, 2, 3, normalColor)
	}
	check(len(before.Palette) + 1)
	if !s.Info().Dirty || d.Info().Palette[len(before.Palette)] != added {
		t.Fatal("palette append did not create a dirty swatch")
	}
	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	check(len(before.Palette))
	if s.Info().Dirty {
		t.Fatal("one Undo did not restore the saved state")
	}
	if err := s.Redo(); err != nil {
		t.Fatal(err)
	}
	check(len(before.Palette) + 1)
}

func TestAddPaletteEntryRejectsLimitWithoutMutation(t *testing.T) {
	d, _, _, _ := doc(t, Indexed)
	defer d.Close()
	e := edit(t, d)
	for len(e.Info().Palette) < 256 {
		if err := e.AddPaletteEntry(Swatch{Color: red}); err != nil {
			t.Fatal(err)
		}
	}
	before := e.Info().Palette
	if err := e.AddPaletteEntry(Swatch{Color: blue}); err == nil {
		t.Fatal("palette exceeded its 256-entry limit")
	}
	if !reflect.DeepEqual(before, e.Info().Palette) {
		t.Fatal("rejected palette append changed the pending edit")
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
