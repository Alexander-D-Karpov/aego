package hotreload

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	rtex "aego/render/texture"
)

func encodePNG(d *rtex.Data) ([]byte, error) {
	img := image.NewNRGBA(image.Rect(0, 0, d.Width, d.Height))
	for y := 0; y < d.Height; y++ {
		for x := 0; x < d.Width; x++ {
			i := (y*d.Width + x) * 4
			img.SetNRGBA(x, y, color.NRGBA{
				R: d.Pixels[i],
				G: d.Pixels[i+1],
				B: d.Pixels[i+2],
				A: d.Pixels[i+3],
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
