package clip

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func pattern() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(40 * x), G: uint8(90 * y), B: 200, A: 255})
		}
	}
	return img
}

func TestImageAndFilesRoundTrip(t *testing.T) {
	src := pattern()
	path := filepath.Join(t.TempDir(), "a b#1.png")
	var m Memory
	if !SetImage(&m, src, []string{path}) {
		t.Fatal("set image failed")
	}
	if !HasImage(&m) || !HasFiles(&m) {
		t.Fatal("image and files must both be offered")
	}
	got, ok := Image(&m)
	if !ok || got.Rect != src.Rect || !bytes.Equal(got.Pix, src.Pix) {
		t.Fatal("pixels changed through the clipboard")
	}
	files, ok := Files(&m)
	if !ok || len(files) != 1 || files[0] != path {
		t.Fatalf("files = %v", files)
	}
	if m.Text() != path {
		t.Fatalf("text = %q", m.Text())
	}
}

func TestURIListParsing(t *testing.T) {
	data := []byte("# comment\r\nfile:///tmp/x%20y.png\r\nhttp://example.com/a.png\r\nfile://remote/z.png\r\nfile://localhost/tmp/w.png\r\n")
	got := ParseURIList(data)
	want := []string{filepath.FromSlash("/tmp/x y.png"), filepath.FromSlash("/tmp/w.png")}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("parsed = %v", got)
	}
	gnome := ParseURIList(GnomeFiles([]string{filepath.FromSlash("/tmp/a.png")}))
	if len(gnome) != 1 || gnome[0] != filepath.FromSlash("/tmp/a.png") {
		t.Fatalf("gnome = %v", gnome)
	}
}

func TestTextPaths(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x.png")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var m Memory
	m.SetText(f + "\n")
	if files, ok := Files(&m); !ok || len(files) != 1 || files[0] != f {
		t.Fatalf("existing path not taken: %v", files)
	}
	for _, s := range []string{"hello", filepath.Join(dir, "missing.png"), "relative/x.png"} {
		m.SetText(s)
		if _, ok := Files(&m); ok {
			t.Fatalf("%q taken as a file", s)
		}
	}
}

func bmp(w, h, bpp int, topDown bool, px func(x, y int) [4]byte) []byte {
	le := binary.LittleEndian
	stride := (w*bpp/8 + 3) &^ 3
	size := 54 + stride*h
	b := make([]byte, size)
	b[0], b[1] = 'B', 'M'
	le.PutUint32(b[2:], uint32(size))
	le.PutUint32(b[10:], 54)
	le.PutUint32(b[14:], 40)
	le.PutUint32(b[18:], uint32(w))
	hh := int32(h)
	if topDown {
		hh = -hh
	}
	le.PutUint32(b[22:], uint32(hh))
	le.PutUint16(b[26:], 1)
	le.PutUint16(b[28:], uint16(bpp))
	for y := 0; y < h; y++ {
		row := h - 1 - y
		if topDown {
			row = y
		}
		for x := 0; x < w; x++ {
			c := px(x, y)
			o := 54 + row*stride + x*bpp/8
			b[o], b[o+1], b[o+2] = c[2], c[1], c[0]
			if bpp == 32 {
				b[o+3] = c[3]
			}
		}
	}
	return b
}

func TestBMP(t *testing.T) {
	px := func(x, y int) [4]byte { return [4]byte{byte(10 + x), byte(20 + y), 30, 0} }
	for _, tc := range []struct {
		bpp int
		top bool
	}{{24, false}, {24, true}, {32, false}, {32, true}} {
		img, err := DecodeBMP(bmp(3, 2, tc.bpp, tc.top, px))
		if err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		for y := 0; y < 2; y++ {
			for x := 0; x < 3; x++ {
				c := img.NRGBAAt(x, y)
				if c.R != byte(10+x) || c.G != byte(20+y) || c.B != 30 || c.A != 255 {
					t.Fatalf("%+v: pixel %d,%d = %+v", tc, x, y, c)
				}
			}
		}
	}
	if _, err := DecodeBMP([]byte("BM")); err == nil {
		t.Fatal("truncated bmp accepted")
	}
}
