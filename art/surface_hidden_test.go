package art

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestSurfaceChannelsPreserveLinearRGBAtZeroAlpha(t *testing.T) {
	d, layer, frame, albedo := doc(t, RGBA)
	defer d.Close()
	e := edit(t, d)
	normal, err := e.EnsureCel(layer, frame, Normal)
	if err != nil {
		t.Fatal(err)
	}
	height, err := e.EnsureCel(layer, frame, Height)
	if err != nil {
		t.Fatal(err)
	}
	emission, err := e.EnsureCel(layer, frame, Emission)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		cel   ID
		color Color
	}{
		{albedo, Color{73, 24, 9, 0}}, {normal, Color{128, 129, 255, 0}},
		{height, Color{91, 91, 91, 0}}, {emission, Color{33, 144, 233, 0}},
	} {
		if err := e.SetPixel(v.cel, 2, 3, Paint{Color: v.color}, nil, false); err != nil {
			t.Fatal(err)
		}
	}
	change := done(t, e)
	check := func(ch Channel, want color.NRGBA) {
		t.Helper()
		img, err := d.Render(frame, RenderOptions{Channel: ch})
		if err != nil {
			t.Fatal(err)
		}
		if got := img.NRGBAAt(2, 3); got != want {
			t.Fatalf("%s: %v want %v", ch, got, want)
		}
		sheet, err := d.ExportSheet(1, ch)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := png.Decode(bytes.NewReader(sheet.PNG))
		if err != nil {
			t.Fatal(err)
		}
		if got := color.NRGBAModel.Convert(decoded.At(2, 3)).(color.NRGBA); got != want {
			t.Fatalf("%s PNG: %v want %v", ch, got, want)
		}
	}
	check(Albedo, color.NRGBA{})
	check(Normal, color.NRGBA{128, 129, 255, 0})
	check(Height, color.NRGBA{91, 91, 91, 0})
	check(Emission, color.NRGBA{33, 144, 233, 0})
	encoded, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer decoded.Close()
	for _, spec := range []struct {
		channel Channel
		want    color.NRGBA
	}{
		{Normal, color.NRGBA{128, 129, 255, 0}}, {Height, color.NRGBA{91, 91, 91, 0}}, {Emission, color.NRGBA{33, 144, 233, 0}},
	} {
		im, err := decoded.Render(frame, RenderOptions{Channel: spec.channel})
		if err != nil || im.NRGBAAt(2, 3) != spec.want {
			t.Fatalf("%s source round-trip: image=%v error=%v", spec.channel, im, err)
		}
	}
	if err := change.Undo(); err != nil {
		t.Fatal(err)
	}
	check(Normal, color.NRGBA{})
	if err := change.Apply(); err != nil {
		t.Fatal(err)
	}
	check(Normal, color.NRGBA{128, 129, 255, 0})
}

func TestHiddenSurfaceRGBDoesNotCoverVisibleData(t *testing.T) {
	d, layer, frame, _ := doc(t, RGBA)
	defer d.Close()
	e := edit(t, d)
	lower, err := e.EnsureCel(layer, frame, Emission)
	if err != nil {
		t.Fatal(err)
	}
	upperLayer, err := e.AddLayer("upper", Raster, "")
	if err != nil {
		t.Fatal(err)
	}
	upper, err := e.EnsureCel(upperLayer, frame, Emission)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []struct {
		cel   ID
		x     int
		color Color
	}{
		{lower, 0, Color{11, 22, 33, 200}}, {upper, 0, Color{99, 88, 77, 0}},
		{lower, 1, Color{99, 88, 77, 0}}, {upper, 1, Color{11, 22, 33, 200}},
	} {
		if err := e.SetPixel(p.cel, p.x, 0, Paint{Color: p.color}, nil, false); err != nil {
			t.Fatal(err)
		}
	}
	done(t, e)
	im, err := d.Render(frame, RenderOptions{Channel: Emission})
	if err != nil {
		t.Fatal(err)
	}
	for x := 0; x < 2; x++ {
		if got := im.NRGBAAt(x, 0); got != (color.NRGBA{11, 22, 33, 200}) {
			t.Fatalf("pixel %d: %v", x, got)
		}
	}
}

func TestHiddenSurfaceRGBRespectsLayerAndCelVisibility(t *testing.T) {
	d, layer, frame, _ := doc(t, RGBA)
	defer d.Close()
	e := edit(t, d)
	lower, err := e.EnsureCel(layer, frame, Normal)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetPixel(lower, 1, 1, Paint{Color: Color{128, 128, 255, 0}}, nil, false); err != nil {
		t.Fatal(err)
	}
	group, err := e.AddLayer("group", Group, "")
	if err != nil {
		t.Fatal(err)
	}
	upperLayer, err := e.AddLayer("upper", Raster, group)
	if err != nil {
		t.Fatal(err)
	}
	upper, err := e.EnsureCel(upperLayer, frame, Normal)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetPixel(upper, 1, 1, Paint{Color: Color{44, 55, 66, 0}}, nil, false); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	check := func(want color.NRGBA) {
		t.Helper()
		im, err := d.Render(frame, RenderOptions{Channel: Normal})
		if err != nil {
			t.Fatal(err)
		}
		if got := im.NRGBAAt(1, 1); got != want {
			t.Fatalf("got %v want %v", got, want)
		}
		part := image.NewNRGBA(im.Bounds())
		if err := d.RenderRegion(frame, part, image.Rect(1, 1, 2, 2), RenderOptions{Channel: Normal}); err != nil {
			t.Fatal(err)
		}
		if got := part.NRGBAAt(1, 1); got != want {
			t.Fatalf("partial: %v want %v", got, want)
		}
	}
	check(color.NRGBA{44, 55, 66, 0})
	e = edit(t, d)
	groupLayer := e.work.Layers[1]
	groupLayer.Visible = false
	if err := e.SetLayer(groupLayer); err != nil {
		t.Fatal(err)
	}
	hidden := done(t, e)
	check(color.NRGBA{128, 128, 255, 0})
	if err := hidden.Undo(); err != nil {
		t.Fatal(err)
	}
	e = edit(t, d)
	ci, _ := e.work.cel(upper)
	e.work.Cels[ci].Opacity = 0
	e.structural = true
	done(t, e)
	check(color.NRGBA{128, 128, 255, 0})
}
