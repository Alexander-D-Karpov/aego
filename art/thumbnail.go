package art

import (
	"fmt"
	"image"
)

// Thumbnail samples a layer (or isolated group) using the same CPU color and
// blend rules as the canvas. It never allocates or decodes a full-size frame.
// Visibility of the selected layer is ignored so hidden layers remain visible
// in the layer list; visibility of children and reference policy are respected.
func (d *Document) Thumbnail(layer, frame ID, o RenderOptions, size int) (*image.NRGBA, error) {
	if size < 1 || size > 256 {
		return nil, ErrLimit
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, ErrClosed
	}
	s := d.current
	d.mu.Unlock()
	o = options(o)
	if _, ok := s.frame(frame); !ok || !validChannel(o.Channel) {
		return nil, fmt.Errorf("art: invalid thumbnail frame or channel")
	}
	li, ok := s.layer(layer)
	if !ok {
		return nil, fmt.Errorf("art: missing thumbnail layer")
	}
	cels := make(map[ID]Cel)
	for _, c := range s.Cels {
		if c.Frame == frame && c.Channel == o.Channel {
			cels[c.Layer] = c
		}
	}
	children := make(map[ID][]int)
	for i, l := range s.Layers {
		children[l.Parent] = append(children[l.Parent], i)
	}
	var sample func(int, int, int, int) Color
	sample = func(index, x, y, depth int) Color {
		if depth > len(s.Layers) {
			return Color{}
		}
		l := s.Layers[index]
		if (index != li && !l.Visible) || (l.Kind == Reference && !o.References) {
			return Color{}
		}
		v := Color{}
		if l.Kind == Group {
			for _, child := range children[l.ID] {
				v = blendPixel(v, sample(child, x, y, depth+1), s.Layers[child].Opacity, s.Layers[child].Blend)
			}
		} else if c, exists := cels[l.ID]; exists {
			v = decode(s.forChannel(c.Channel), s.Images[c.Image].at(x-c.X, y-c.Y))
			v.A = uint8((uint32(v.A)*uint32(c.Opacity) + 127) / 255)
		}
		return v
	}
	w, h := s.Canvas.Width, s.Canvas.Height
	dw, dh := size, size
	if w > h {
		dh = max(1, h*size/w)
	} else {
		dw = max(1, w*size/h)
	}
	ox, oy := (size-dw)/2, (size-dh)/2
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			v := sample(li, min(w-1, (2*x+1)*w/(2*dw)), min(h-1, (2*y+1)*h/(2*dh)), 0)
			v = blendPixel(Color{}, v, s.Layers[li].Opacity, s.Layers[li].Blend)
			i := out.PixOffset(x+ox, y+oy)
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = v.R, v.G, v.B, v.A
		}
	}
	return out, nil
}
