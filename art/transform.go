package art

import (
	"fmt"
	"image"
)

type Clipboard struct {
	Width, Height int
	Origin        image.Point
	Pixels        []Color
	Mask          []bool
}

func (d *Document) Copy(cel ID, sel *Selection) (*Clipboard, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, ErrClosed
	}
	return copyPixels(d.current, cel, sel)
}
func copyPixels(s *state, cel ID, sel *Selection) (*Clipboard, error) {
	i, ok := s.cel(cel)
	if !ok {
		return nil, fmt.Errorf("art: missing cel")
	}
	c := s.Cels[i]
	p := s.Images[c.Image]
	r := image.Rect(0, 0, s.Canvas.Width, s.Canvas.Height)
	if sel != nil {
		if sel.Width != s.Canvas.Width || sel.Height != s.Canvas.Height {
			return nil, fmt.Errorf("art: selection dimensions mismatch")
		}
		r = sel.Bounds()
	}
	if r.Empty() {
		return nil, fmt.Errorf("art: selection is empty")
	}
	out := &Clipboard{Width: r.Dx(), Height: r.Dy(), Origin: r.Min, Pixels: make([]Color, r.Dx()*r.Dy()), Mask: make([]bool, r.Dx()*r.Dy())}
	for y := 0; y < out.Height; y++ {
		for x := 0; x < out.Width; x++ {
			at := y*out.Width + x
			out.Mask[at] = sel.Contains(x+r.Min.X, y+r.Min.Y)
			if out.Mask[at] {
				out.Pixels[at] = decode(s.forChannel(c.Channel), p.at(x+r.Min.X-c.X, y+r.Min.Y-c.Y))
			}
		}
	}
	return out, nil
}
func (e *Edit) Clear(cel ID, sel *Selection) error {
	c, err := e.celForWrite(cel)
	if err != nil {
		return err
	}
	for y := 0; y < e.work.Canvas.Height; y++ {
		for x := 0; x < e.work.Canvas.Width; x++ {
			if err = e.write(c, x, y, 0, sel, true); err != nil {
				return err
			}
		}
	}
	return nil
}
func (e *Edit) Paste(cel ID, clip *Clipboard, at image.Point, sel *Selection) error {
	c, err := e.celForWrite(cel)
	if err != nil {
		return err
	}
	if clip == nil || clip.Width < 1 || clip.Height < 1 || clip.Width > e.doc.limits.MaxDimension || clip.Height > e.doc.limits.MaxDimension || len(clip.Pixels) != clip.Width*clip.Height || (len(clip.Mask) != 0 && len(clip.Mask) != len(clip.Pixels)) {
		return fmt.Errorf("art: invalid clipboard")
	}
	if !clampPoint(at) {
		return ErrLimit
	}
	for y := 0; y < clip.Height; y++ {
		for x := 0; x < clip.Width; x++ {
			i := y*clip.Width + x
			if len(clip.Mask) > 0 && !clip.Mask[i] {
				continue
			}
			if err = e.write(c, x+at.X, y+at.Y, encode(e.work.forChannel(c.Channel), clip.Pixels[i]), sel, false); err != nil {
				return err
			}
		}
	}
	return nil
}
func (c *Clipboard) Transform(kind string, width, height int) (*Clipboard, error) {
	if c == nil || c.Width < 1 || c.Height < 1 || len(c.Pixels) != c.Width*c.Height || (len(c.Mask) != 0 && len(c.Mask) != len(c.Pixels)) {
		return nil, fmt.Errorf("art: invalid clipboard")
	}
	w, h := c.Width, c.Height
	switch kind {
	case "rotate-cw", "rotate-ccw":
		w, h = h, w
	case "flip-x", "flip-y":
	case "scale":
		w, h = width, height
	default:
		return nil, fmt.Errorf("art: unsupported transform %q", kind)
	}
	if w < 1 || h < 1 || w > 4096 || h > 4096 || int64(w)*int64(h) > 4194304 {
		return nil, ErrLimit
	}
	out := &Clipboard{Width: w, Height: h, Origin: c.Origin, Pixels: make([]Color, w*h), Mask: make([]bool, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx, sy := x, y
			switch kind {
			case "flip-x":
				sx = c.Width - 1 - x
			case "flip-y":
				sy = c.Height - 1 - y
			case "rotate-cw":
				sx, sy = y, c.Height-1-x
			case "rotate-ccw":
				sx, sy = c.Width-1-y, x
			case "scale":
				sx, sy = x*c.Width/w, y*c.Height/h
			}
			dst, src := y*w+x, sy*c.Width+sx
			out.Pixels[dst] = c.Pixels[src]
			out.Mask[dst] = len(c.Mask) == 0 || c.Mask[src]
		}
	}
	return out, nil
}
func (e *Edit) Transform(cel ID, sel *Selection, kind string, width, height, dx, dy int) error {
	if _, err := e.celForWrite(cel); err != nil {
		return err
	}
	clip, err := copyPixels(e.work, cel, sel)
	if err != nil {
		return err
	}
	out := clip
	if kind != "move" {
		out, err = clip.Transform(kind, width, height)
		if err != nil {
			return err
		}
	}
	if err = e.Clear(cel, sel); err != nil {
		return err
	}
	return e.Paste(cel, out, clip.Origin.Add(image.Pt(dx, dy)), nil)
}

// DuplicateLayer clones a raster/reference layer and its cels. Link groups are
// preserved within the duplicate but never share storage identity with source.
func (e *Edit) DuplicateLayer(id ID) (ID, error) {
	if err := e.check(); err != nil {
		return "", err
	}
	i, ok := e.work.layer(id)
	if !ok {
		return "", fmt.Errorf("art: missing layer")
	}
	source := e.work.Layers[i]
	if source.Kind == Group {
		return "", fmt.Errorf("art: duplicate a raster layer, not a group")
	}
	dest, err := e.AddLayer(source.Name+" copy", source.Kind, source.Parent)
	if err != nil {
		return "", err
	}
	di, _ := e.work.layer(dest)
	source.ID = dest
	source.Locked = false
	e.work.Layers[di] = source
	remap := make(map[ID]ID)
	old := append([]Cel(nil), e.work.Cels...)
	for _, c := range old {
		if c.Layer != id {
			continue
		}
		if len(e.work.Cels) >= e.doc.limits.MaxCels {
			return "", ErrLimit
		}
		im := remap[c.Image]
		if im == "" {
			im = e.doc.newID()
			remap[c.Image] = im
			e.shareIndependent(c.Image, im)
		}
		c.ID = e.doc.newID()
		c.Layer = dest
		c.Image = im
		e.work.Cels = append(e.work.Cels, c)
	}
	return dest, nil
}
