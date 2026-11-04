package pixelart

import (
	"image/color"
	"testing"
)

func TestPaintCopyUsesSnapshotAndUnassociatedAlpha(t *testing.T) {
	im, err := Paint(nil, 5, 2, nil, []Op{{Op: "line", X2: 2, Color: "#bf200181"}, {Op: "pixel", X: 0, Color: "#2103a083"}, {Op: "copy", W: 3, H: 1, ToX: 1, FlipX: true}, {Op: "fill", Y: 1, Color: "#040607ff"}})
	if err != nil {
		t.Fatal(err)
	}
	a, b, c := color.NRGBA{0x21, 3, 0xa0, 0x83}, color.NRGBA{0xbf, 0x20, 1, 0x81}, color.NRGBA{4, 6, 7, 255}
	for x, want := range []color.NRGBA{a, b, b, a, c} {
		if got := im.NRGBAAt(x, 0); got != want {
			t.Fatalf("x=%d got=%v want=%v", x, got, want)
		}
	}
	if _, err = Paint(im, 0, 0, nil, []Op{{Op: "rect", W: 5, H: 2, Color: "transparent"}, {Op: "copy", W: 5, H: 2, ToX: 1}}); err == nil || im.NRGBAAt(0, 0) != a {
		t.Fatal("invalid batch changed its input")
	}
}

func TestFloodLeavesDisconnectedRegion(t *testing.T) {
	im, err := Paint(nil, 4, 4, Palette{'g': {G: 255, A: 255}}, []Op{{Op: "rect", X: 2, W: 1, H: 4, Color: "g"}, {Op: "fill", Color: "#111111"}})
	if err != nil || im.NRGBAAt(0, 3).A != 255 || im.NRGBAAt(3, 3).A != 0 || im.NRGBAAt(2, 3).G != 255 {
		t.Fatal(im, err)
	}
}
