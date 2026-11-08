package texture

import (
	"image"
	"image/png"
	"io"
	"sync"

	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/gpu"
)

type Data struct {
	Width, Height int
	Format        gpu.TextureFormat
	Pixels        []byte
	Premultiplied bool
	// NonColor keeps noise, masks and other data channels unchanged.
	NonColor bool
}

var premultipliedSRGB struct {
	once sync.Once
	data [256][256]byte // alpha, straight sRGB channel
}

func premultipliedSRGBTable() *[256][256]byte {
	premultipliedSRGB.once.Do(func() {
		for alpha := 1; alpha < 255; alpha++ {
			for color := range 256 {
				v := mathx.SRGBToLinear(float32(color) / 255)
				v *= float32(alpha) / 255
				v = mathx.LinearToSRGB(v)
				premultipliedSRGB.data[alpha][color] = byte(mathx.Clamp(v, 0, 1)*255 + 0.5)
			}
		}
	})
	return &premultipliedSRGB.data
}

func New(w, h int, format gpu.TextureFormat) *Data {
	return &Data{Width: w, Height: h, Format: format, Pixels: make([]byte, w*h*4)}
}

func DecodePNG(r io.Reader, srgb bool) (*Data, error) {
	img, err := png.Decode(r)
	if err != nil {
		return nil, errs.Wrap(err, errs.VFSIO, "png decode failed")
	}
	b := img.Bounds()
	format := gpu.TexRGBA8
	if srgb {
		format = gpu.TexSRGBA8
	}
	d := New(b.Dx(), b.Dy(), format)
	if src, ok := img.(*image.NRGBA); ok {
		for y := 0; y < d.Height; y++ {
			copy(d.Pixels[y*d.Width*4:(y+1)*d.Width*4], src.Pix[y*src.Stride:])
		}
		return d, nil
	}
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			if a16 > 0 && a16 < 0xffff {
				r16 = r16 * 0xffff / a16
				g16 = g16 * 0xffff / a16
				b16 = b16 * 0xffff / a16
			}
			d.Pixels[i] = byte(r16 >> 8)
			d.Pixels[i+1] = byte(g16 >> 8)
			d.Pixels[i+2] = byte(b16 >> 8)
			d.Pixels[i+3] = byte(a16 >> 8)
			i += 4
		}
	}
	return d, nil
}

func (d *Data) Premultiply() {
	if d.Premultiplied || d.NonColor {
		return
	}
	var table *[256][256]byte
	for i := 0; i+3 < len(d.Pixels); i += 4 {
		a := uint32(d.Pixels[i+3])
		if a == 255 {
			continue
		}
		if a == 0 {
			// Every finite color becomes zero at zero alpha, including hidden
			// RGB in transparent PNG pixels. Avoid costly sRGB conversions.
			d.Pixels[i], d.Pixels[i+1], d.Pixels[i+2] = 0, 0, 0
			continue
		}
		if d.Format == gpu.TexSRGBA8 {
			if table == nil {
				table = premultipliedSRGBTable()
			}
			row := &table[a]
			d.Pixels[i] = row[d.Pixels[i]]
			d.Pixels[i+1] = row[d.Pixels[i+1]]
			d.Pixels[i+2] = row[d.Pixels[i+2]]
			continue
		}
		for c := 0; c < 3; c++ {
			v := float32(d.Pixels[i+c]) / 255
			v *= float32(a) / 255
			d.Pixels[i+c] = byte(mathx.Clamp(v, 0, 1)*255 + 0.5)
		}
	}
	d.Premultiplied = true
}

func (d *Data) SetPixel(x, y int, c mathx.Color) {
	if x < 0 || y < 0 || x >= d.Width || y >= d.Height {
		return
	}
	i := (y*d.Width + x) * 4
	if d.Format == gpu.TexSRGBA8 {
		c.R = mathx.LinearToSRGB(c.R)
		c.G = mathx.LinearToSRGB(c.G)
		c.B = mathx.LinearToSRGB(c.B)
	}
	packed := c.PackRGBA8()
	d.Pixels[i] = byte(packed)
	d.Pixels[i+1] = byte(packed >> 8)
	d.Pixels[i+2] = byte(packed >> 16)
	d.Pixels[i+3] = byte(packed >> 24)
}

func Solid(w, h int, c mathx.Color) *Data {
	d := New(w, h, gpu.TexSRGBA8)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			d.SetPixel(x, y, c)
		}
	}
	return d
}

func Checker(w, h, cell int, a, b mathx.Color) *Data {
	if cell < 1 {
		cell = 1
	}
	d := New(w, h, gpu.TexSRGBA8)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := a
			if ((x/cell)+(y/cell))%2 == 1 {
				c = b
			}
			d.SetPixel(x, y, c)
		}
	}
	return d
}

func (d *Data) Validate() error {
	if d.Width <= 0 || d.Height <= 0 {
		return errs.New(errs.GPUResource, "texture data has non-positive size",
			log.F("w", d.Width), log.F("h", d.Height))
	}
	if len(d.Pixels) < d.Width*d.Height*4 {
		return errs.New(errs.GPUResource, "texture data too small", log.F("len", len(d.Pixels)))
	}
	return nil
}
