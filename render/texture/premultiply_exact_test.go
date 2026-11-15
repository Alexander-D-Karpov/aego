package texture

import (
	"bytes"
	"os"
	"sync"
	"testing"

	"aego/core/mathx"
	"aego/gpu"
)

// Check every byte/alpha pair against the original linear-light calculation.
// Transparent pixels may carry nonzero hidden RGB and must still become zero.
func TestPremultiplyAllByteValues(t *testing.T) {
	for _, format := range []gpu.TextureFormat{gpu.TexRGBA8, gpu.TexSRGBA8} {
		src := make([]byte, 256*256*4)
		want := make([]byte, len(src))
		for alpha := range 256 {
			for color := range 256 {
				i := (alpha*256 + color) * 4
				src[i], src[i+1], src[i+2], src[i+3] = byte(color), byte(255-color), byte((color*29+alpha*7)&255), byte(alpha)
				want[i+3] = byte(alpha)
				for c := range 3 {
					if alpha == 255 {
						want[i+c] = src[i+c]
						continue
					}
					v := float32(src[i+c]) / 255
					if format == gpu.TexSRGBA8 {
						v = mathx.SRGBToLinear(v)
					}
					v *= float32(alpha) / 255
					if format == gpu.TexSRGBA8 {
						v = mathx.LinearToSRGB(v)
					}
					want[i+c] = byte(mathx.Clamp(v, 0, 1)*255 + 0.5)
				}
			}
		}
		d := &Data{Width: 256, Height: 256, Format: format, Pixels: src}
		d.Premultiply()
		if !bytes.Equal(d.Pixels, want) {
			for i := range want {
				if d.Pixels[i] != want[i] {
					t.Fatalf("format %v byte %d = %d, want %d", format, i, d.Pixels[i], want[i])
				}
			}
		}
		if !d.Premultiplied {
			t.Fatal("texture not marked premultiplied")
		}
		d.Premultiply()
		if !bytes.Equal(d.Pixels, want) {
			t.Fatal("premultiplication was applied twice")
		}
	}
}

func TestConcurrentPremultiply(t *testing.T) {
	const workers = 16
	var wg sync.WaitGroup
	start := make(chan struct{})
	bad := make(chan []byte, workers)
	for range workers {
		wg.Go(func() {
			<-start
			d := &Data{Format: gpu.TexSRGBA8, Pixels: []byte{255, 128, 0, 128}}
			d.Premultiply()
			if !bytes.Equal(d.Pixels, []byte{188, 93, 0, 128}) {
				bad <- d.Pixels
			}
		})
	}
	close(start)
	wg.Wait()
	close(bad)
	for pixels := range bad {
		t.Errorf("unexpected concurrent result: %v", pixels)
	}
}

func BenchmarkPremultiplyFontAtlas(b *testing.B) {
	f, err := os.Open("../../engine/res/fonts/ui-16@2.png")
	if err != nil {
		b.Fatal(err)
	}
	d, err := DecodePNG(f, true)
	_ = f.Close()
	if err != nil {
		b.Fatal(err)
	}
	source := bytes.Clone(d.Pixels)
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for range b.N {
		copy(d.Pixels, source)
		d.Premultiplied = false
		d.Premultiply()
	}
}
