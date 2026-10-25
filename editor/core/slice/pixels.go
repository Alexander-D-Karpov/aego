package slice

import (
	"image"
	"image/draw"
	"image/png"
	"os"

	"aego/core/errs"
)

// Pixels reads a png sheet as straight RGBA rows for Alpha.
func Pixels(abs string) ([]byte, int, int, error) {
	f, err := os.Open(abs)
	if err != nil {
		return nil, 0, 0, errs.Wrap(err, errs.VFSIO, "cannot read the sheet")
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, 0, 0, errs.Wrap(err, errs.AssetImportFailed, "cannot decode the sheet")
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Rect, img, b.Min, draw.Src)
	return out.Pix, b.Dx(), b.Dy(), nil
}
