package art

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"testing"
)

type aseFixture struct{ bytes.Buffer }

func (w *aseFixture) u8(v uint8) { w.WriteByte(v) }
func (w *aseFixture) u16(v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	w.Write(b[:])
}
func (w *aseFixture) u32(v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	w.Write(b[:])
}
func (w *aseFixture) str(v string) { w.u16(uint16(len(v))); w.WriteString(v) }
func aseChunk(kind uint16, b []byte) []byte {
	w := aseFixture{}
	w.u32(uint32(len(b) + 6))
	w.u16(kind)
	w.Write(b)
	return w.Bytes()
}
func aseFrame(duration int, chunks ...[]byte) []byte {
	w := aseFixture{}
	w.u32(0)
	w.u16(0xf1fa)
	w.u16(uint16(len(chunks)))
	w.u16(uint16(duration))
	w.u16(0)
	w.u32(uint32(len(chunks)))
	for _, b := range chunks {
		w.Write(b)
	}
	b := w.Bytes()
	binary.LittleEndian.PutUint32(b, uint32(len(b)))
	return b
}
func aseFile(depth int, transparent byte, frames ...[]byte) []byte {
	w := aseFixture{}
	w.Write(make([]byte, 128))
	for _, f := range frames {
		w.Write(f)
	}
	b := w.Bytes()
	binary.LittleEndian.PutUint32(b, uint32(len(b)))
	binary.LittleEndian.PutUint16(b[4:], 0xa5e0)
	binary.LittleEndian.PutUint16(b[6:], uint16(len(frames)))
	binary.LittleEndian.PutUint16(b[8:], 8)
	binary.LittleEndian.PutUint16(b[10:], 8)
	binary.LittleEndian.PutUint16(b[12:], uint16(depth))
	binary.LittleEndian.PutUint32(b[14:], 3)
	b[28] = transparent
	return b
}
func aseLayerChunk(name string, kind, level uint16, opacity byte) []byte {
	w := aseFixture{}
	w.u16(3)
	w.u16(kind)
	w.u16(level)
	w.u16(0)
	w.u16(0)
	w.u16(0)
	w.u8(opacity)
	w.Write(make([]byte, 3))
	w.str(name)
	return aseChunk(0x2004, w.Bytes())
}
func aseCelChunk(layer uint16, x, y int16, opacity byte, typ uint16, raw []byte) []byte {
	w := aseFixture{}
	w.u16(layer)
	w.u16(uint16(x))
	w.u16(uint16(y))
	w.u8(opacity)
	w.u16(typ)
	w.u16(0)
	w.Write(make([]byte, 5))
	if typ == 1 {
		w.u16(binary.LittleEndian.Uint16(raw))
	} else {
		w.u16(2)
		w.u16(2)
		if typ == 2 {
			var b bytes.Buffer
			z := zlib.NewWriter(&b)
			z.Write(raw)
			z.Close()
			w.Write(b.Bytes())
		} else {
			w.Write(raw)
		}
	}
	return aseChunk(0x2005, w.Bytes())
}
func asePaletteChunk() []byte {
	w := aseFixture{}
	w.u32(3)
	w.u32(0)
	w.u32(2)
	w.Write(make([]byte, 8))
	for _, c := range []Color{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}} {
		w.u16(0)
		w.Write([]byte{c.R, c.G, c.B, c.A})
	}
	return aseChunk(0x2019, w.Bytes())
}
func TestAsepriteCompressedLinkedCelsOpacityAndTags(t *testing.T) {
	raw := []byte{255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 0, 0, 0, 0}
	tag := aseFixture{}
	tag.u16(1)
	tag.Write(make([]byte, 8))
	tag.u16(0)
	tag.u16(1)
	tag.u8(2)
	tag.u16(0)
	tag.Write(make([]byte, 10))
	tag.str("Idle")
	data := aseFile(32, 0, aseFrame(100, aseLayerChunk("Body", 0, 0, 255), aseCelChunk(0, 2, 3, 255, 2, raw), aseChunk(0x2018, tag.Bytes())), aseFrame(180, aseCelChunk(0, 3, 3, 128, 1, []byte{0, 0})))
	d, notes, err := ImportAseprite(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 {
		t.Fatal(notes)
	}
	i := d.Info()
	if len(i.Frames) != 2 || i.Frames[1].DurationMS != 180 || len(i.Tags) != 1 || i.Tags[0].Direction != PingPong {
		t.Fatal(i)
	}
	if i.Cels[0].Image != i.Cels[1].Image {
		t.Fatal("linked cel lost")
	}
	im, err := d.Render(i.Frames[1].ID, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if c := im.NRGBAAt(3, 3); c.R != 255 || c.A != 128 {
		t.Fatalf("offset/opacity %v", c)
	}
	b, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	q, err := Decode(b, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if q.Info().Cels[0].Image != q.Info().Cels[1].Image {
		t.Fatal("source lost links")
	}
}
func TestAsepriteIndexedTransparencyNormalizationAndGray(t *testing.T) {
	d, _, err := ImportAseprite(aseFile(8, 2, aseFrame(100, aseLayerChunk("Indexed", 0, 0, 255), asePaletteChunk(), aseCelChunk(0, 0, 0, 255, 0, []byte{0, 1, 2, 0}))), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	i := d.Info()
	if i.Canvas.Mode != Indexed || i.Palette[0].Color.A != 0 {
		t.Fatal(i)
	}
	im, err := d.Render(i.Frames[0].ID, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if im.NRGBAAt(0, 0).R != 255 || im.NRGBAAt(1, 0).G != 255 || im.NRGBAAt(0, 1).A != 0 {
		t.Fatal("indexed remap changed pixels")
	}
	d, _, err = ImportAseprite(aseFile(16, 0, aseFrame(75, aseLayerChunk("Gray", 0, 0, 255), aseCelChunk(0, 0, 0, 255, 2, []byte{128, 255, 64, 128, 0, 0, 255, 255}))), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	c, err := d.Pixel(d.Info().Cels[0].ID, 1, 0)
	if err != nil || c != (Color{64, 64, 64, 128}) {
		t.Fatalf("gray %v %v", c, err)
	}
}
func TestAsepriteRejectsUnsupportedAndMalformed(t *testing.T) {
	base := aseFile(32, 0, aseFrame(100, aseLayerChunk("Body", 0, 0, 255), aseCelChunk(0, 0, 0, 255, 0, make([]byte, 16))))
	for _, data := range [][]byte{nil, base[:len(base)-1], append(append([]byte{}, base...), 0), aseFile(32, 0, aseFrame(100, aseLayerChunk("Tilemap", 2, 0, 255))), aseFile(32, 0, aseFrame(100, aseLayerChunk("Cycle", 0, 0, 255), aseCelChunk(0, 0, 0, 255, 1, []byte{0, 0}))), aseFile(32, 0, aseFrame(100, aseLayerChunk("Bad raw", 0, 0, 255), aseCelChunk(0, 0, 0, 255, 0, []byte{1}))), aseFile(32, 0, aseFrame(100, aseLayerChunk("Unknown", 0, 0, 255), aseChunk(0xffff, nil)))} {
		if _, _, err := ImportAseprite(data, Limits{}); err == nil {
			t.Fatal("accepted invalid input")
		}
	}
	if _, _, err := ImportAseprite(base, Limits{MaxPixelBytes: 4}); err == nil {
		t.Fatal("decoded-byte budget")
	}
}
func TestAsepriteGroupsMetadataAndEmptyRoundTrip(t *testing.T) {
	data := aseFile(32, 0, aseFrame(100, aseLayerChunk("Group", 1, 0, 128), aseLayerChunk("Raster", 0, 1, 255), aseChunk(0x2020, []byte{0, 0, 0, 0})))
	d, notes, err := ImportAseprite(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	i := d.Info()
	if i.Layers[1].Parent != i.Layers[0].ID || len(notes) != 1 {
		t.Fatal("group/diagnostic")
	}
	b, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Decode(b, Limits{}); err != nil {
		t.Fatal("empty round trip", err)
	}
}
func FuzzAseprite(f *testing.F) {
	f.Add(aseFile(32, 0, aseFrame(100, aseLayerChunk("Body", 0, 0, 255), aseCelChunk(0, 0, 0, 255, 2, make([]byte, 16)))))
	f.Add([]byte("not an aseprite file"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			t.Skip()
		}
		d, _, err := ImportAseprite(b, Limits{MaxDimension: 128, MaxCanvasPixels: 16384, MaxPixelBytes: 1 << 20, MaxFileBytes: 1 << 20, MaxFrames: 64, MaxLayers: 32, MaxCels: 2048})
		if err == nil {
			if err = d.current.validate(d.limits); err != nil {
				t.Fatal(err)
			}
		}
	})
}
