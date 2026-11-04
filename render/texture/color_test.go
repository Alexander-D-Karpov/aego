package texture

import (
	"slices"
	"testing"

	"aego/core/mathx"
	"aego/gpu"
)

func TestSRGBPremultiplyUsesLinearLightOnce(t *testing.T) {
	d := New(1, 1, gpu.TexSRGBA8)
	copy(d.Pixels, []byte{255, 128, 0, 128})
	d.Premultiply()
	if d.Pixels[0] < 187 || d.Pixels[0] > 189 {
		t.Fatalf("white alpha .5 encoded as %d; expected sRGB(linear .5)", d.Pixels[0])
	}
	if d.Pixels[1] < 92 || d.Pixels[1] > 94 || d.Pixels[3] != 128 {
		t.Fatalf("wrong channels %v", d.Pixels)
	}
	once := slices.Clone(d.Pixels)
	d.Premultiply()
	if !slices.Equal(once, d.Pixels) {
		t.Fatal("premultiplied twice")
	}
}
func TestLinearAndDataTextureSemantics(t *testing.T) {
	d := New(1, 1, gpu.TexRGBA8)
	copy(d.Pixels, []byte{255, 128, 20, 128})
	d.Premultiply()
	if !slices.Equal(d.Pixels, []byte{128, 64, 10, 128}) {
		t.Fatal(d.Pixels)
	}
	raw := New(1, 1, gpu.TexRGBA8)
	raw.NonColor = true
	copy(raw.Pixels, []byte{255, 128, 20, 128})
	before := slices.Clone(raw.Pixels)
	raw.Premultiply()
	if !slices.Equal(raw.Pixels, before) {
		t.Fatal("data channels changed")
	}
}
func TestGeneratedSRGBTextureAcceptsRuntimeLinearColor(t *testing.T) {
	d := Solid(1, 1, mathx.Color{R: .5, G: .5, B: .5, A: 1})
	if d.Pixels[0] < 187 || d.Pixels[0] > 189 {
		t.Fatalf("runtime linear color was not encoded: %v", d.Pixels)
	}
}
