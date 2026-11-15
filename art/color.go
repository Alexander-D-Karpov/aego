package art

import "fmt"

// Data channels are not quantized or gamma-converted when albedo changes mode.
// Their byte payload is linear data; the runtime importer owns texture color space.
func (s *state) forChannel(ch Channel) *state {
	if ch == Albedo || ch == "" {
		return s
	}
	q := *s
	q.Canvas.Mode = RGBA
	if ch == Height {
		q.Canvas.Mode = Grayscale
	}
	return &q
}
func decode(s *state, v uint32) Color {
	if s.Canvas.Mode == Indexed {
		if int(v) < len(s.Palette) {
			return s.Palette[v].Color
		}
		return Color{}
	}
	return unpack(v)
}
func nearest(p []Swatch, c Color, alpha *uint8) uint32 {
	best := uint32(0)
	distance := int64(1 << 62)
	for i, s := range p {
		v := s.Color
		if alpha != nil && v.A != *alpha {
			continue
		}
		dr, dg, db, da := int64(c.R)-int64(v.R), int64(c.G)-int64(v.G), int64(c.B)-int64(v.B), int64(c.A)-int64(v.A)
		n := dr*dr + dg*dg + db*db + 2*da*da
		if n < distance {
			distance = n
			best = uint32(i)
		}
	}
	return best
}
func encode(s *state, c Color) uint32 {
	switch s.Canvas.Mode {
	case Indexed:
		return nearest(s.Palette, c, nil)
	case Grayscale:
		y := uint8((77*uint32(c.R) + 150*uint32(c.G) + 29*uint32(c.B) + 128) >> 8)
		return Color{y, y, y, c.A}.packed()
	default:
		return c.packed()
	}
}
func blendPixel(dst, src Color, opacity uint8, mode Blend) Color {
	sa := uint32(src.A) * uint32(opacity) / 255
	da := uint32(dst.A)
	if sa == 0 {
		if da == 0 {
			return Color{}
		}
		return dst
	}
	if da == 0 || (sa == 255 && mode == BlendNormal) {
		return Color{src.R, src.G, src.B, uint8(sa)}
	}
	outA := sa + (da*(255-sa)+127)/255
	if outA == 0 {
		return Color{}
	}
	channel := func(s, d uint8) uint8 {
		sv, dv := uint32(s), uint32(d)
		b := sv
		switch mode {
		case BlendMultiply:
			b = (sv*dv + 127) / 255
		case BlendScreen:
			b = 255 - ((255-sv)*(255-dv)+127)/255
		case BlendAdd:
			b = min(255, sv+dv)
		}
		// Porter-Duff source-over plus the standard separated blend function.
		numer := sv*sa*(255-da) + b*sa*da + dv*da*(255-sa)
		return uint8(min(255, (numer+outA*127)/(outA*255)))
	}
	return Color{channel(src.R, dst.R), channel(src.G, dst.G), channel(src.B, dst.B), uint8(outA)}
}
func (e *Edit) SetPaletteEntry(index int, value Swatch) error {
	if err := e.check(); err != nil {
		return err
	}
	if index < 0 || index >= len(e.work.Palette) {
		return fmt.Errorf("art: invalid palette index")
	}
	if index == 0 && e.work.Canvas.Mode == Indexed && value.Color.A != 0 {
		return fmt.Errorf("art: entry zero must stay transparent")
	}
	e.work.Palette[index] = value
	e.structural = true
	return nil
}

// ReorderPalette accepts new-position -> old-position. Index data is remapped
// too, preserving the displayed image; this is not a color-replacement command.
func (e *Edit) ReorderPalette(order []int) error {
	if err := e.check(); err != nil {
		return err
	}
	n := len(e.work.Palette)
	if len(order) != n {
		return fmt.Errorf("art: invalid palette permutation")
	}
	seen := make([]bool, n)
	inverse := make([]uint32, n)
	palette := make([]Swatch, n)
	for i, old := range order {
		if old < 0 || old >= n || seen[old] {
			return fmt.Errorf("art: invalid palette permutation")
		}
		seen[old] = true
		inverse[old] = uint32(i)
		palette[i] = e.work.Palette[old]
	}
	if e.work.Canvas.Mode == Indexed && order[0] != 0 {
		return fmt.Errorf("art: keep transparent entry at index zero")
	}
	if e.work.Canvas.Mode == Indexed {
		if err := e.mapPixels(func(v uint32) uint32 { return inverse[v] }); err != nil {
			return err
		}
	}
	e.work.Palette = palette
	e.structural = true
	return nil
}

// RemapPalette changes the indexed image intentionally; table is old -> new.
func (e *Edit) RemapPalette(table []int) error {
	if err := e.check(); err != nil {
		return err
	}
	if e.work.Canvas.Mode != Indexed {
		return fmt.Errorf("art: remap requires indexed mode")
	}
	if len(table) != len(e.work.Palette) || table[0] != 0 {
		return fmt.Errorf("art: invalid remap table")
	}
	for _, i := range table {
		if i < 0 || i >= len(e.work.Palette) {
			return fmt.Errorf("art: invalid remap index")
		}
	}
	if err := e.mapPixels(func(v uint32) uint32 { return uint32(table[v]) }); err != nil {
		return err
	}
	e.structural = true
	return nil
}
func (e *Edit) mapPixels(fn func(uint32) uint32) error {
	albedo := make(map[ID]bool)
	for _, c := range e.work.Cels {
		if c.Channel == Albedo {
			albedo[c.Image] = true
		}
	}
	for id, p := range e.work.Images {
		if !albedo[id] {
			continue
		}
		keys := make([]tileKey, 0, len(p.Tiles))
		for k := range p.Tiles {
			keys = append(keys, k)
		}
		for _, k := range keys {
			t := p.Tiles[k]
			for i, v := range t {
				x, y := k.X*TileSize+i%TileSize, k.Y*TileSize+i/TileSize
				if x < p.Width && y < p.Height {
					if err := e.raw(id, x, y, fn(v)); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
func (e *Edit) ConvertMode(mode Mode) error {
	if err := e.check(); err != nil {
		return err
	}
	if mode != RGBA && mode != Grayscale && mode != Indexed {
		return fmt.Errorf("art: invalid mode")
	}
	if mode == e.work.Canvas.Mode {
		return nil
	}
	if mode == Indexed && e.work.Palette[0].Color.A != 0 {
		return fmt.Errorf("art: indexed mode needs transparent entry zero")
	}
	old := *e.work
	target := *e.work
	target.Canvas.Mode = mode
	if err := e.mapPixels(func(v uint32) uint32 { return encode(&target, decode(&old, v)) }); err != nil {
		return err
	}
	e.work.Canvas.Mode = mode
	e.structural = true
	return nil
}
