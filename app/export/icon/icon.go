package icon

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"aego/core/errs"
	"aego/core/log"
)

var magic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

func IsPNG(data []byte) bool {
	return bytes.HasPrefix(data, magic)
}

func Load(path string) (image.Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errs.Wrap(err, errs.VFSNotFound, "the icon file cannot be read").With(log.F("path", path))
	}
	return Decode(data, path)
}

func Decode(data []byte, name string) (image.Image, error) {
	if !IsPNG(data) {
		return nil, errs.New(errs.AssetImportFailed, "the icon "+name+" is not a PNG file")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errs.Wrap(err, errs.AssetImportFailed, "the icon "+name+" is not a valid PNG")
	}
	if b := img.Bounds(); b.Dx() < 16 || b.Dy() < 16 {
		return nil, errs.New(errs.AssetImportFailed, "the icon "+name+" is smaller than 16x16")
	}
	return img, nil
}

func PNG(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&b, img); err != nil {
		return nil, errs.Wrap(err, errs.VFSIO, "cannot encode the icon")
	}
	return b.Bytes(), nil
}

func premultiplied(src image.Image) ([]float64, int, int) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]float64, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bb, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := (y*w + x) * 4
			out[i], out[i+1], out[i+2], out[i+3] = float64(r), float64(g), float64(bb), float64(a)
		}
	}
	return out, w, h
}

func area(pm []float64, sw, sh int, x0, y0, wx, wy float64) [4]float64 {
	var c [4]float64
	total := 0.0
	for sy := int(y0); sy < int(math.Ceil(y0+wy)) && sy < sh; sy++ {
		fy := math.Min(float64(sy+1), y0+wy) - math.Max(float64(sy), y0)
		if fy <= 0 {
			continue
		}
		for sx := int(x0); sx < int(math.Ceil(x0+wx)) && sx < sw; sx++ {
			fx := math.Min(float64(sx+1), x0+wx) - math.Max(float64(sx), x0)
			if fx <= 0 {
				continue
			}
			k := fx * fy
			i := (sy*sw + sx) * 4
			for j := range c {
				c[j] += pm[i+j] * k
			}
			total += k
		}
	}
	if total > 0 {
		for j := range c {
			c[j] /= total
		}
	}
	return c
}

func bilinear(pm []float64, sw, sh int, fx, fy float64) [4]float64 {
	fx = math.Max(0, math.Min(fx, float64(sw-1)))
	fy = math.Max(0, math.Min(fy, float64(sh-1)))
	x0, y0 := int(fx), int(fy)
	x1, y1 := min(x0+1, sw-1), min(y0+1, sh-1)
	tx, ty := fx-float64(x0), fy-float64(y0)
	var c [4]float64
	for j := 0; j < 4; j++ {
		a := pm[(y0*sw+x0)*4+j]*(1-tx) + pm[(y0*sw+x1)*4+j]*tx
		b := pm[(y1*sw+x0)*4+j]*(1-tx) + pm[(y1*sw+x1)*4+j]*tx
		c[j] = a*(1-ty) + b*ty
	}
	return c
}

func channel(v, a float64) uint8 {
	return uint8(math.Round(math.Max(0, math.Min(255, v/a*255))))
}

func Resize(src image.Image, w, h int) *image.NRGBA {
	pm, sw, sh := premultiplied(src)
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	sx, sy := float64(sw)/float64(w), float64(sh)/float64(h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var c [4]float64
			if sx >= 1 && sy >= 1 {
				c = area(pm, sw, sh, float64(x)*sx, float64(y)*sy, sx, sy)
			} else {
				c = bilinear(pm, sw, sh, (float64(x)+0.5)*sx-0.5, (float64(y)+0.5)*sy-0.5)
			}
			if c[3] <= 0 {
				continue
			}
			dst.SetNRGBA(x, y, color.NRGBA{
				R: channel(c[0], c[3]), G: channel(c[1], c[3]), B: channel(c[2], c[3]),
				A: uint8(math.Round(math.Min(255, c[3]/65535*255))),
			})
		}
	}
	return dst
}
