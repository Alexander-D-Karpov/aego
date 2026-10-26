package pixelart

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestFramesMakeAnAtlas(t *testing.T) {
	pal, err := ParsePalette(`{"r": "#d0312d", "s": "#f4c7a1", ".": "transparent"}`)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Parse("idle:\n.rr.\nrssr\nrun1:\nr..r\n.ss.\n", pal, 2)
	if err != nil {
		t.Fatal(err)
	}
	if s.Image.Bounds() != image.Rect(0, 0, 16, 4) || len(s.Cells) != 2 || s.Cells[1] != image.Rect(8, 0, 16, 4) {
		t.Fatalf("atlas %v cells %v", s.Image.Bounds(), s.Cells)
	}
	if s.Names[0] != "idle" || s.Names[1] != "run1" {
		t.Fatalf("names %v", s.Names)
	}
	red := color.NRGBA{0xd0, 0x31, 0x2d, 255}
	if s.Image.NRGBAAt(2, 0) != red || s.Image.NRGBAAt(3, 1) != red || s.Image.NRGBAAt(0, 0).A != 0 {
		t.Fatal("scale 2 did not repeat the pixels")
	}
	if s.Image.NRGBAAt(8, 0) != red {
		t.Fatal("the second frame is not in its cell")
	}
}

func TestPairsPaletteAndSingleFrame(t *testing.T) {
	pal, err := ParsePalette("a=#000 b=white")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Parse("ab\nba", pal, 1)
	if err != nil || len(s.Names) != 1 || s.Names[0] != "" {
		t.Fatalf("%v %v", s, err)
	}
	if off, opaque := OffPalette(s.Image, []color.NRGBA{{0, 0, 0, 255}}); off != 2 || opaque != 4 {
		t.Fatalf("off %d of %d", off, opaque)
	}
}

func TestMistakesAreNamed(t *testing.T) {
	pal, _ := ParsePalette("a=#000")
	for in, want := range map[string]string{
		"ax":                 `'x' is not in the palette`,
		"a:\naa\naa\nb:\naa": "has 1 rows",
		"a:\naa\nb:\naaa":    "rows of 3 and 2",
		"":                   "empty",
		"a:\naa\na:\naa":     "twice",
	} {
		_, err := Parse(in, pal, 1)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", in, err, want)
		}
	}
	if _, err := ParsePalette("ab=#000"); err == nil {
		t.Error("a two-character key was accepted")
	}
}
