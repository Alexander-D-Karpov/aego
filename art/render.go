package art

import (
	"fmt"
	"image"
	"image/color"
)

type RenderOptions struct {
	Channel    Channel
	References bool
	// Isolate limits presentation to a layer/group and its ancestors. It never
	// changes serialized visibility, and normal exports leave this empty.
	Isolate ID
}

func options(o RenderOptions) RenderOptions {
	if o.Channel == "" {
		o.Channel = Albedo
	}
	return o
}
func (d *Document) Render(frame ID, o RenderOptions) (*image.NRGBA, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, ErrClosed
	}
	s := d.current
	d.mu.Unlock()
	out := image.NewNRGBA(image.Rect(0, 0, s.Canvas.Width, s.Canvas.Height))
	if err := renderRegion(s, frame, out, out.Bounds(), options(o)); err != nil {
		return nil, err
	}
	return out, nil
}
func (e *Edit) Render(frame ID, o RenderOptions) (*image.NRGBA, error) {
	if err := e.check(); err != nil {
		return nil, err
	}
	out := image.NewNRGBA(image.Rect(0, 0, e.work.Canvas.Width, e.work.Canvas.Height))
	if err := renderRegion(e.work, frame, out, out.Bounds(), options(o)); err != nil {
		return nil, err
	}
	return out, nil
}

// RenderRegion recomposites only the damaged canvas rectangle into a caller's
// retained image. Presentation owns this image, never the canonical pixels.
func (d *Document) RenderRegion(frame ID, dst *image.NRGBA, r image.Rectangle, o RenderOptions) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return ErrClosed
	}
	s := d.current
	d.mu.Unlock()
	return renderRegion(s, frame, dst, r, options(o))
}
func (e *Edit) RenderRegion(frame ID, dst *image.NRGBA, r image.Rectangle, o RenderOptions) error {
	if err := e.check(); err != nil {
		return err
	}
	return renderRegion(e.work, frame, dst, r, options(o))
}
func renderRegion(s *state, frame ID, dst *image.NRGBA, r image.Rectangle, o RenderOptions) error {
	if _, ok := s.frame(frame); !ok {
		return fmt.Errorf("art: missing frame")
	}
	if !validChannel(o.Channel) {
		return fmt.Errorf("art: invalid channel")
	}
	bounds := image.Rect(0, 0, s.Canvas.Width, s.Canvas.Height)
	if dst == nil || dst.Bounds() != bounds {
		return fmt.Errorf("art: composite dimensions mismatch")
	}
	r = r.Intersect(bounds)
	if r.Empty() {
		return nil
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		i := dst.PixOffset(r.Min.X, y)
		clear(dst.Pix[i : i+r.Dx()*4])
	}
	cels := make(map[ID]Cel)
	for _, c := range s.Cels {
		if c.Frame == frame && c.Channel == o.Channel {
			cels[c.Layer] = c
		}
	}
	var isolated map[ID]bool
	if o.Isolate != "" {
		index, found := s.layer(o.Isolate)
		if !found {
			return fmt.Errorf("art: missing isolated layer")
		}
		isolated = map[ID]bool{o.Isolate: true}
		// Include descendants of an isolated group, but not siblings of any
		// ancestor. Ancestors still supply their existing opacity and blend.
		for changed := true; changed; {
			changed = false
			for _, layer := range s.Layers {
				if layer.Parent != "" && isolated[layer.Parent] && !isolated[layer.ID] {
					isolated[layer.ID], changed = true, true
				}
			}
		}
		for parent := s.Layers[index].Parent; parent != ""; {
			if isolated[parent] {
				return fmt.Errorf("art: cyclic isolated groups")
			}
			isolated[parent] = true
			index, found = s.layer(parent)
			if !found {
				return fmt.Errorf("art: missing isolated group")
			}
			parent = s.Layers[index].Parent
		}
	}
	var layers func(ID, *image.NRGBA, int) error
	layers = func(parent ID, target *image.NRGBA, depth int) error {
		if depth > len(s.Layers) {
			return fmt.Errorf("art: cyclic groups")
		}
		for _, l := range s.Layers {
			if isolated != nil && !isolated[l.ID] {
				continue
			}
			if l.Parent != parent || !l.Visible || l.Opacity == 0 || (l.Kind == Reference && !o.References) {
				continue
			}
			var group *image.NRGBA
			var p *pixels
			var cel Cel
			if l.Kind == Group {
				group = image.NewNRGBA(r)
				if err := layers(l.ID, group, depth+1); err != nil {
					return err
				}
			} else {
				var ok bool
				cel, ok = cels[l.ID]
				if !ok {
					continue
				}
				p = s.Images[cel.Image]
			}
			composite := func(x, y int, src Color) {
				if src.A == 0 {
					// Normal, height and emission textures retain RGB independently
					// of alpha. When the composite is also transparent, the
					// topmost nonzero data pixel supplies its hidden RGB. This
					// must not cover any pixel with alpha. Albedo retains normal
					// transparent compositing semantics.
					if o.Channel != Albedo && (group != nil || cel.Opacity != 0) && (src.R != 0 || src.G != 0 || src.B != 0) {
						i := target.PixOffset(x, y)
						if target.Pix[i+3] == 0 {
							target.Pix[i], target.Pix[i+1], target.Pix[i+2] = src.R, src.G, src.B
						}
					}
					return
				}
				i := target.PixOffset(x, y)
				v := blendPixel(Color{target.Pix[i], target.Pix[i+1], target.Pix[i+2], target.Pix[i+3]}, src, l.Opacity, l.Blend)
				target.Pix[i], target.Pix[i+1], target.Pix[i+2], target.Pix[i+3] = v.R, v.G, v.B, v.A
			}
			if group != nil {
				for y := r.Min.Y; y < r.Max.Y; y++ {
					for x := r.Min.X; x < r.Max.X; x++ {
						v := group.NRGBAAt(x, y)
						composite(x, y, Color{v.R, v.G, v.B, v.A})
					}
				}
				continue
			}
			// Canonical pixels are sparse tiles. Tiles do not overlap, so their
			// map iteration order cannot affect source-over ordering between layers.
			// Empty canvas areas need neither sampling nor palette lookup.
			cs := s.forChannel(cel.Channel)
			for key, block := range p.Tiles {
				x0, y0 := key.X*TileSize, key.Y*TileSize
				area := image.Rect(x0+cel.X, y0+cel.Y,
					min(x0+TileSize, p.Width)+cel.X, min(y0+TileSize, p.Height)+cel.Y).Intersect(r)
				for y := area.Min.Y; y < area.Max.Y; y++ {
					for x := area.Min.X; x < area.Max.X; x++ {
						v := block[(y-cel.Y-y0)*TileSize+x-cel.X-x0]
						src := decode(cs, v)
						src.A = uint8((uint32(src.A)*uint32(cel.Opacity) + 127) / 255)
						composite(x, y, src)
					}
				}
			}
		}
		return nil
	}
	return layers("", dst, 0)
}

// FrameAt uses integer milliseconds and does not mutate the author document.
// Ping-pong does not double the endpoints (0,1,2,1 for three frames).
func (d *Document) FrameAt(tag ID, ms int64) (ID, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return "", ErrClosed
	}
	return frameAt(d.current, tag, ms)
}
func frameOrder(s *state, tag ID) ([]Frame, error) {
	start, end, dir := 0, len(s.Frames)-1, Forward
	if tag != "" {
		found := false
		for _, t := range s.Tags {
			if t.ID == tag {
				start, _ = s.frame(t.From)
				end, _ = s.frame(t.To)
				dir = t.Direction
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("art: missing animation tag")
		}
	}
	frames := append([]Frame(nil), s.Frames[start:end+1]...)
	switch dir {
	case Reverse:
		for i, j := 0, len(frames)-1; i < j; i, j = i+1, j-1 {
			frames[i], frames[j] = frames[j], frames[i]
		}
	case PingPong:
		for i := len(frames) - 2; i > 0; i-- {
			frames = append(frames, frames[i])
		}
	}
	return frames, nil
}
func frameAt(s *state, tag ID, ms int64) (ID, error) {
	frames, err := frameOrder(s, tag)
	if err != nil {
		return "", err
	}
	var total int64
	for _, f := range frames {
		total += int64(f.DurationMS)
	}
	if total == 0 {
		return "", fmt.Errorf("art: empty animation")
	}
	ms %= total
	if ms < 0 {
		ms += total
	}
	for _, f := range frames {
		if ms < int64(f.DurationMS) {
			return f.ID, nil
		}
		ms -= int64(f.DurationMS)
	}
	return frames[0].ID, nil
}
func (d *Document) Onion(frame ID, before, after int, o RenderOptions) (*image.NRGBA, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, ErrClosed
	}
	s := d.current
	d.mu.Unlock()
	fi, ok := s.frame(frame)
	if !ok {
		return nil, fmt.Errorf("art: missing frame")
	}
	if before < 0 || before > 8 || after < 0 || after > 8 {
		return nil, ErrLimit
	}
	out := image.NewNRGBA(image.Rect(0, 0, s.Canvas.Width, s.Canvas.Height))
	for side := 0; side < 2; side++ {
		count, step := before, -1
		tint := Color{255, 100, 100, 255}
		if side == 1 {
			count, step = after, 1
			tint = Color{100, 180, 255, 255}
		}
		for n := count; n >= 1; n-- {
			idx := fi + step*n
			if idx < 0 || idx >= len(s.Frames) {
				continue
			}
			im := image.NewNRGBA(out.Bounds())
			if err := renderRegion(s, s.Frames[idx].ID, im, im.Bounds(), options(o)); err != nil {
				return nil, err
			}
			for y := 0; y < s.Canvas.Height; y++ {
				for x := 0; x < s.Canvas.Width; x++ {
					c := im.NRGBAAt(x, y)
					if c.A == 0 {
						continue
					}
					a := uint8(int(c.A) / (n + 2))
					p := out.NRGBAAt(x, y)
					v := blendPixel(Color{p.R, p.G, p.B, p.A}, Color{tint.R, tint.G, tint.B, a}, 255, BlendNormal)
					out.SetNRGBA(x, y, color.NRGBA{R: v.R, G: v.G, B: v.B, A: v.A})
				}
			}
		}
	}
	return out, nil
}
