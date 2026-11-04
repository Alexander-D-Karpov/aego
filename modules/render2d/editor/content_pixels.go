package editor

import (
	"errors"
	"fmt"
	"image"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/gpu"
)

func contentPixels(ed *context.Editor, id ids.AssetID, mode string) (image.Image, string, error) {
	c := loadImageContent(ed, id, id)
	var px *imagePixels
	var rect image.Rectangle
	role := asset.RoleColor
	if d := ed.Docs.AssetDocument(c.SourceID); mode == "source" && d != nil && d.Metadata != nil && d.Metadata.Dirty() {
		meta, err := asset.ParseSourceMeta(d.Metadata.Code.Bytes(), d.Metadata.Path)
		if err != nil {
			return nil, "", err
		}
		role, err = asset.TextureRoleOf(c.Path, meta.Settings)
		if err != nil {
			return nil, "", err
		}
		if id != c.SourceID {
			f, ok := meta.Frame(id)
			if !ok {
				return nil, "", fmt.Errorf("frame absent in current document")
			}
			c.SourceRect = f.Rect
		}
	} else if meta, ok := ed.Pipeline.MetaOf(c.SourceID); ok {
		var err error
		role, err = asset.TextureRoleOf(c.Path, meta.Settings)
		if err != nil {
			return nil, "", err
		}
	}
	switch mode {
	case "source":
		if c.Source == nil {
			return nil, "", errors.New(c.SourceError)
		}
		px = c.Source
		a := c.SourceRect
		rect = image.Rect(a[0], a[1], a[0]+a[2], a[1]+a[3])
	case "imported":
		if c.Imported == nil {
			return nil, "", errors.New(c.ImportedError)
		}
		px = c.Imported
		a := c.Region
		rect = image.Rect(a.X, a.Y, a.X+a.W, a.Y+a.H)
		if rect.Empty() && asset.IsBuiltin(id) {
			rect = image.Rect(0, 0, px.data.Width, px.data.Height)
		}
	default:
		return nil, "", fmt.Errorf("unknown image representation %q", mode)
	}
	d := px.data
	bounds := image.Rect(0, 0, d.Width, d.Height)
	if rect.Empty() || !rect.In(bounds) || len(d.Pixels) != 4*d.Width*d.Height {
		return nil, "", errors.New("invalid image region")
	}
	// PNG stores straight colour; imported colour textures are premultiplied in linear space.
	img := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for y := 0; y < rect.Dy(); y++ {
		row := img.Pix[y*img.Stride : (y+1)*img.Stride]
		i := 4 * ((rect.Min.Y+y)*d.Width + rect.Min.X)
		copy(row, d.Pixels[i:i+len(row)])
		if mode == "source" || d.NonColor || (!d.Premultiplied && d.Format == gpu.TexSRGBA8) {
			continue
		}
		for k := 0; k < len(row); k += 4 {
			a := row[k+3]
			if a == 0 || (a == 255 && d.Format == gpu.TexSRGBA8) {
				continue
			}
			for j := 0; j < 3; j++ {
				v := float32(row[k+j]) / 255
				if d.Format == gpu.TexSRGBA8 {
					v = mathx.SRGBToLinear(v)
				}
				if d.Premultiplied {
					v = mathx.Clamp(v*255/float32(a), 0, 1)
				}
				v = mathx.LinearToSRGB(v)
				row[k+j] = byte(v*255 + .5)
			}
		}
	}
	return img, string(role), nil
}
