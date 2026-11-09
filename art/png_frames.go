package art

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

// ImportPNGFrames creates an independent RGBA document from equally sized
// source rectangles, in the supplied order. Rectangles may overlap or omit
// pixels. The caller must make that choice explicit: this does not import the
// PNG's metadata, surface channels, animation timing or engine asset identities.
func ImportPNGFrames(data []byte, rects []image.Rectangle, durationMS int, limits Limits) (*Document, error) {
	l := limitsOrDefault(limits)
	if int64(len(data)) > l.MaxFileBytes || len(rects) == 0 || len(rects) > l.MaxFrames || len(rects) > l.MaxCels {
		return nil, ErrLimit
	}
	if durationMS < 1 || durationMS > 600000 {
		return nil, fmt.Errorf("art: invalid frame duration")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if err := CheckPNGPrecision(cfg); err != nil {
		return nil, err
	}
	// Bound the complete decoded image, not only the output rectangles.
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > l.MaxDimension || cfg.Height > l.MaxDimension || int64(cfg.Width)*int64(cfg.Height) > int64(l.MaxCanvasPixels) {
		return nil, ErrLimit
	}
	w, h := 0, 0
	for i, r := range rects {
		if r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > cfg.Width || r.Max.Y > cfg.Height || r.Empty() {
			return nil, fmt.Errorf("art: frame %d is outside the PNG or empty", i+1)
		}
		if i == 0 {
			w, h = r.Dx(), r.Dy()
		} else if r.Dx() != w || r.Dy() != h {
			return nil, fmt.Errorf("art: PNG frames must have the same dimensions")
		}
	}
	// Account conservatively for dense output before decoding or allocating it;
	// allocateTile also enforces the actual tiled allocation limit.
	if int64(w)*int64(h) > l.MaxPixelBytes/4/int64(len(rects)) {
		return nil, ErrLimit
	}
	im, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	d, err := New(w, h, RGBA, l)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			d.Close()
		}
	}()
	e, err := d.Begin(d.Revision(), "Import PNG frames")
	if err != nil {
		return nil, err
	}
	defer e.Cancel()
	layer := e.work.Layers[0].ID
	frame := e.work.Frames[0].ID
	for i, r := range rects {
		if i == 0 {
			err = e.SetDuration(frame, durationMS)
		} else {
			frame, err = e.AddFrame(frame, durationMS)
		}
		if err != nil {
			return nil, err
		}
		cel, err := e.EnsureCel(layer, frame, Albedo)
		if err != nil {
			return nil, err
		}
		ci, _ := e.work.cel(cel)
		// This is a new document and the image is private to this transaction.
		// Build its tiles directly: calling raw for each pixel would repeatedly
		// scan every previously imported cel, quadratic in the frame count.
		p := newPixels(w, h)
		e.work.Images[e.work.Cels[ci].Image] = p
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := color.NRGBAModel.Convert(im.At(r.Min.X+x, r.Min.Y+y)).(color.NRGBA)
				v := (Color{c.R, c.G, c.B, c.A}).packed()
				if v == 0 {
					continue
				}
				key := tileKey{x / TileSize, y / TileSize}
				if p.Tiles[key] == nil {
					p.Tiles[key], err = e.allocateTile(nil)
					if err != nil {
						return nil, err
					}
				}
				p.Tiles[key][y%TileSize*TileSize+x%TileSize] = v
			}
		}
	}
	e.structural = true
	change, err := e.Prepare()
	if err != nil {
		return nil, err
	}
	if change != nil {
		if err := change.Apply(); err != nil {
			return nil, err
		}
	}
	success = true
	return d, nil
}
