package art

import (
	"encoding/binary"
	"fmt"
	"image"
	"reflect"

	"aego/art/internal/aseprite"
)

// ImportDiagnostic identifies discarded non-pixel metadata. Unsupported visual
// semantics are errors, not a best-effort image presented as a lossless import.
type ImportDiagnostic struct {
	Code    string `json:"code"`
	Frame   int    `json:"frame"`
	Chunk   uint16 `json:"chunk"`
	Message string `json:"message"`
}

// Keep parsing vocabulary local to the Art adapter while wire mechanics live
// in the internal codec; no Art document or state crosses that boundary.
type aseReader struct{ *aseprite.Reader }

func (r *aseReader) take(n int) []byte { return r.Take(n) }
func (r *aseReader) u8() uint8         { return r.U8() }
func (r *aseReader) u16() uint16       { return r.U16() }
func (r *aseReader) u32() uint32       { return r.U32() }
func (r *aseReader) i16() int          { return r.I16() }
func (r *aseReader) i32() int          { return r.I32() }
func (r *aseReader) text() string      { return r.Text() }

type aseLayer struct {
	flags       uint16
	kind, level int
	blend       Blend
	opacity     uint8
	name        string
}
type aseCel struct {
	layer, frame, x, y, w, h, link int
	opacity                        uint8
	raw                            []byte
	image                          ID
	resolving                      bool
}
type aseTag struct {
	name      string
	from, to  int
	direction Direction
}
type aseSlice struct {
	name   string
	frame  int
	rect   image.Rectangle
	pivot  image.Point
	global bool
}
type aseImport struct {
	layers        []aseLayer
	cels          []*aseCel
	frames        []int
	tags          []aseTag
	slices        []aseSlice
	palette       []Swatch
	modernPalette bool
	paletteSeen   bool
	decoded       int64
	diagnostics   []ImportDiagnostic
	flags         uint32
	depth         int
	lim           Limits
}

func (a *aseImport) note(frame int, chunk uint16, code, msg string) {
	if len(a.diagnostics) < 512 {
		a.diagnostics = append(a.diagnostics, ImportDiagnostic{code, frame, chunk, msg})
	}
}

// ImportAseprite supports raster/group/reference layers, raw/compressed/linked
// cels, RGBA/grayscale/indexed pixels, static palettes, tags and basic slices.
// Tilemaps, ICC profiles, animated palettes, nonzero cel z-index and unsupported
// blend modes fail explicitly. It never executes external links or extracts files.
func ImportAseprite(data []byte, limits Limits) (*Document, []ImportDiagnostic, error) {
	l := limitsOrDefault(limits)
	a := &aseImport{lim: l}
	fail := func(err error) (*Document, []ImportDiagnostic, error) {
		return nil, a.diagnostics, fmt.Errorf("art: Aseprite import: %w", err)
	}
	if len(data) < 128 || int64(len(data)) > l.MaxFileBytes {
		return fail(ErrLimit)
	}
	h := aseReader{aseprite.NewReader(data[:128])}
	size := h.u32()
	magic := h.u16()
	frames := int(h.u16())
	w, hei := int(h.u16()), int(h.u16())
	a.depth = int(h.u16())
	a.flags = h.u32()
	speed := int(h.u16())
	h.take(8)
	transparent := int(h.u8())
	h.take(3)
	colors := int(h.u16())
	if int64(size) != int64(len(data)) || magic != 0xa5e0 {
		return fail(fmt.Errorf("invalid header or file size"))
	}
	if frames < 1 || frames > l.MaxFrames || w < 1 || hei < 1 || w > l.MaxDimension || hei > l.MaxDimension || int64(w)*int64(hei) > int64(l.MaxCanvasPixels) {
		return fail(ErrLimit)
	}
	mode := RGBA
	switch a.depth {
	case 32:
		mode = RGBA
	case 16:
		mode = Grayscale
	case 8:
		mode = Indexed
	default:
		return fail(fmt.Errorf("unsupported color depth %d", a.depth))
	}
	if colors == 0 {
		colors = 256
	}
	if colors > 256 {
		return fail(fmt.Errorf("palette exceeds 256 entries"))
	}
	a.palette = make([]Swatch, colors)
	for i := range a.palette {
		a.palette[i].Color.A = 255
	}
	if err := aseprite.ReadFrames(data, frames, speed, func(duration int) {
		a.frames = append(a.frames, duration)
	}, a.chunk); err != nil {
		return fail(err)
	}
	if len(a.layers) == 0 {
		return fail(fmt.Errorf("no layers"))
	}
	if mode == Indexed && (!a.paletteSeen || transparent >= len(a.palette)) {
		return fail(fmt.Errorf("indexed import requires an explicit valid palette"))
	}
	d, err := New(w, hei, mode, l)
	if err != nil {
		return fail(err)
	}
	s := &state{Canvas: Canvas{w, hei, mode, "srgb"}, Palette: a.palette, Images: make(map[ID]*pixels), Properties: make(map[string]string)}
	if mode != Indexed && !a.paletteSeen {
		s.Palette = DefaultPalette()
	}
	if mode == Indexed {
		s.Palette[0], s.Palette[transparent] = s.Palette[transparent], s.Palette[0]
		s.Palette[0].Color.A = 0
	}
	parents := []ID{}
	for _, v := range a.layers {
		if v.level > len(parents) {
			return fail(fmt.Errorf("invalid layer hierarchy"))
		}
		if mode == Indexed && v.flags&8 != 0 {
			return fail(fmt.Errorf("indexed background layer is not supported; convert it to a normal raster layer"))
		}
		parents = parents[:v.level]
		parent := ID("")
		if v.level > 0 {
			parent = parents[v.level-1]
		}
		kind := Raster
		if v.kind == 1 {
			kind = Group
		} else if v.flags&64 != 0 {
			kind = Reference
		}
		layer := Layer{ID: d.allocate(), Parent: parent, Name: v.name, Kind: kind, Visible: v.flags&1 != 0, Locked: v.flags&2 == 0, Opacity: v.opacity, Blend: v.blend}
		s.Layers = append(s.Layers, layer)
		if kind == Group {
			parents = append(parents, layer.ID)
		}
	}
	for _, duration := range a.frames {
		s.Frames = append(s.Frames, Frame{d.allocate(), duration})
	}
	cells := make(map[[2]int]*aseCel)
	for _, c := range a.cels {
		if c.layer < 0 || c.layer >= len(s.Layers) || s.Layers[c.layer].Kind == Group {
			return fail(fmt.Errorf("invalid cel layer"))
		}
		key := [2]int{c.frame, c.layer}
		if cells[key] != nil {
			return fail(fmt.Errorf("duplicate cel"))
		}
		cells[key] = c
	}
	var resolve func(*aseCel) (ID, error)
	resolve = func(c *aseCel) (ID, error) {
		if c.image != "" {
			return c.image, nil
		}
		if c.resolving {
			return "", fmt.Errorf("linked cel cycle")
		}
		c.resolving = true
		if c.link >= 0 {
			target := cells[[2]int{c.link, c.layer}]
			if target == nil {
				return "", fmt.Errorf("linked cel target is missing")
			}
			id, err := resolve(target)
			if err != nil {
				return "", err
			}
			c.image = id
			c.resolving = false
			return id, nil
		}
		p := newPixels(c.w, c.h)
		for y := 0; y < c.h; y++ {
			for x := 0; x < c.w; x++ {
				at := (y*c.w + x) * (a.depth / 8)
				var value uint32
				switch mode {
				case RGBA:
					value = binary.LittleEndian.Uint32(c.raw[at:])
				case Grayscale:
					g := c.raw[at]
					value = Color{g, g, g, c.raw[at+1]}.packed()
				case Indexed:
					idx := int(c.raw[at])
					if idx >= len(s.Palette) {
						return "", fmt.Errorf("pixel refers to missing palette entry")
					}
					if idx == transparent {
						idx = 0
					} else if idx == 0 {
						idx = transparent
					}
					value = uint32(idx)
				}
				if value != 0 {
					key := tileKey{x / TileSize, y / TileSize}
					t := p.Tiles[key]
					if t == nil {
						t = new(tile)
						p.Tiles[key] = t
					}
					t[(y%TileSize)*TileSize+x%TileSize] = value
				}
			}
		}
		c.image = d.allocate()
		s.Images[c.image] = p
		c.raw = nil
		c.resolving = false
		return c.image, nil
	}
	for _, c := range a.cels {
		id, err := resolve(c)
		if err != nil {
			return fail(err)
		}
		s.Cels = append(s.Cels, Cel{ID: d.allocate(), Layer: s.Layers[c.layer].ID, Frame: s.Frames[c.frame].ID, Image: id, Channel: Albedo, X: c.x, Y: c.y, Opacity: c.opacity})
	}
	for _, v := range a.tags {
		if v.from < 0 || v.to >= len(s.Frames) || v.from > v.to {
			return fail(fmt.Errorf("invalid tag range"))
		}
		s.Tags = append(s.Tags, Tag{d.allocate(), v.name, s.Frames[v.from].ID, s.Frames[v.to].ID, v.direction})
	}
	for _, v := range a.slices {
		if v.frame < 0 || v.frame >= len(s.Frames) {
			return fail(fmt.Errorf("invalid slice frame"))
		}
		frame := ID("")
		if !v.global {
			frame = s.Frames[v.frame].ID
		}
		s.Slices = append(s.Slices, Slice{d.allocate(), v.name, frame, v.rect, v.pivot})
	}
	if err = s.validate(l); err != nil {
		return fail(err)
	}
	d.current = s
	return d, a.diagnostics, nil
}
func (a *aseImport) chunk(frame int, kind uint16, b []byte) error {
	r := aseReader{aseprite.NewReader(b)}
	switch kind {
	case 0x2004:
		if frame != 0 {
			return fmt.Errorf("animated layer definitions are unsupported")
		}
		if len(a.layers) >= a.lim.MaxLayers {
			return ErrLimit
		}
		v := aseLayer{flags: r.u16(), kind: int(r.u16()), level: int(r.u16())}
		r.take(4)
		blend := r.u16()
		v.opacity = r.u8()
		r.take(3)
		v.name = r.text()
		if v.kind > 1 {
			return fmt.Errorf("tilemap layers are unsupported")
		}
		switch blend {
		case 0:
			v.blend = BlendNormal
		case 1:
			v.blend = BlendMultiply
		case 2:
			v.blend = BlendScreen
		case 16:
			v.blend = BlendAdd
		default:
			return fmt.Errorf("blend mode %d is unsupported", blend)
		}
		if a.flags&1 == 0 {
			v.opacity = 255
		}
		if v.kind == 1 && a.flags&2 == 0 {
			v.blend = BlendNormal
			v.opacity = 255
		}
		if a.flags&4 != 0 {
			r.take(16)
			a.note(frame, kind, "layer-uuid", "Aseprite UUID is replaced by a stable Art Document entity ID")
		}
		a.layers = append(a.layers, v)
	case 0x2005:
		if len(a.cels) >= a.lim.MaxCels {
			return ErrLimit
		}
		c := &aseCel{layer: int(r.u16()), frame: frame, x: r.i16(), y: r.i16(), opacity: r.u8(), link: -1}
		typ := r.u16()
		z := r.i16()
		r.take(5)
		if z != 0 {
			return fmt.Errorf("per-cel z-index is unsupported")
		}
		switch typ {
		case 1:
			c.link = int(r.u16())
		case 0, 2:
			c.w, c.h = int(r.u16()), int(r.u16())
			n := int64(c.w) * int64(c.h) * int64(a.depth/8)
			if c.w < 1 || c.h < 1 || c.w > a.lim.MaxDimension || c.h > a.lim.MaxDimension || int64(c.w)*int64(c.h) > int64(a.lim.MaxCanvasPixels) || n > a.lim.MaxPixelBytes-a.decoded {
				return ErrLimit
			}
			a.decoded += n
			if r.Err() != nil {
				return r.Err()
			}
			raw := r.take(len(b) - r.Offset())
			if typ == 0 {
				if int64(len(raw)) != n {
					return fmt.Errorf("raw cel size mismatch")
				}
				c.raw = append([]byte(nil), raw...)
			} else {
				var err error
				c.raw, err = aseprite.InflateExact(raw, n)
				if err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("cel type %d is unsupported", typ)
		}
		a.cels = append(a.cels, c)
	case 0x2019:
		size, first, last := int64(r.u32()), int64(r.u32()), int64(r.u32())
		r.take(8)
		if size < 1 || size > 256 || first < 0 || last < first || last >= size {
			return fmt.Errorf("invalid palette range")
		}
		p := make([]Swatch, int(size))
		copy(p, a.palette)
		for i := first; i <= last; i++ {
			flags := r.u16()
			color := Color{r.u8(), r.u8(), r.u8(), r.u8()}
			name := ""
			if flags&1 != 0 {
				name = r.text()
			}
			if flags&^uint16(1) != 0 {
				return fmt.Errorf("unknown palette flags")
			}
			p[i] = Swatch{color, name}
		}
		if frame > 0 && !reflect.DeepEqual(p, a.palette) {
			return fmt.Errorf("animated palettes are unsupported")
		}
		a.palette = p
		a.modernPalette = true
		a.paletteSeen = true
	case 0x0004, 0x0011:
		count := int(r.u16())
		index := 0
		p := append([]Swatch(nil), a.palette...)
		for n := 0; n < count; n++ {
			index += int(r.u8())
			colors := int(r.u8())
			if colors == 0 {
				colors = 256
			}
			if index+colors > len(p) {
				return fmt.Errorf("legacy palette exceeds bounds")
			}
			for j := 0; j < colors; j++ {
				c := Color{r.u8(), r.u8(), r.u8(), 255}
				if kind == 0x0011 {
					if c.R > 63 || c.G > 63 || c.B > 63 {
						return fmt.Errorf("invalid six-bit palette")
					}
					c.R = uint8((int(c.R)*255 + 31) / 63)
					c.G = uint8((int(c.G)*255 + 31) / 63)
					c.B = uint8((int(c.B)*255 + 31) / 63)
				}
				p[index+j].Color = c
			}
			index += colors
		}
		if !a.modernPalette {
			if frame > 0 && !reflect.DeepEqual(p, a.palette) {
				return fmt.Errorf("animated palettes are unsupported")
			}
			a.palette = p
			a.paletteSeen = true
		}
	case 0x2018:
		count := int(r.u16())
		r.take(8)
		if count > a.lim.MaxFrames-len(a.tags) {
			return ErrLimit
		}
		for i := 0; i < count; i++ {
			v := aseTag{from: int(r.u16()), to: int(r.u16())}
			dir := r.u8()
			repeat := r.u16()
			r.take(6)
			r.take(4)
			v.name = r.text()
			switch dir {
			case 0:
				v.direction = Forward
			case 1:
				v.direction = Reverse
			case 2:
				v.direction = PingPong
			default:
				return fmt.Errorf("ping-pong reverse tags are unsupported")
			}
			if repeat != 0 {
				a.note(frame, kind, "tag-repeat", "Finite tag repeat count is not retained; imported tag loops")
			}
			a.tags = append(a.tags, v)
		}
	case 0x2007:
		typ, flags := r.u16(), r.u16()
		gamma := r.u32()
		r.take(8)
		if typ > 1 || flags&1 != 0 && gamma != 0 {
			return fmt.Errorf("ICC/custom gamma color profiles are unsupported")
		}
	case 0x2022:
		count, flags := int64(r.u32()), r.u32()
		r.take(4)
		name := r.text()
		if count > int64(a.lim.MaxFrames-len(a.slices)) || flags&^uint32(3) != 0 {
			return ErrLimit
		}
		for i := int64(0); i < count; i++ {
			v := aseSlice{name: name, frame: int(r.u32())}
			v.global = count == 1 && v.frame == 0
			x, y := r.i32(), r.i32()
			w, h := int64(r.u32()), int64(r.u32())
			if w > int64(a.lim.MaxDimension) || h > int64(a.lim.MaxDimension) || x < -a.lim.MaxDimension || y < -a.lim.MaxDimension || x > a.lim.MaxDimension || y > a.lim.MaxDimension {
				return ErrLimit
			}
			v.rect = image.Rect(x, y, x+int(w), y+int(h))
			if flags&1 != 0 {
				r.take(16)
			}
			if flags&2 != 0 {
				// Aseprite pivots are relative to the slice origin; Aego's
				// frame pivot is in canvas coordinates. Bound the sum even on
				// 32-bit hosts before converting it back to an int.
				px, py := int64(x)+int64(r.i32()), int64(y)+int64(r.i32())
				if px < -1<<31 || px > 1<<31-1 || py < -1<<31 || py > 1<<31-1 {
					return ErrLimit
				}
				v.pivot = image.Pt(int(px), int(py))
			}
			a.slices = append(a.slices, v)
		}
		if count > 1 || count == 1 && a.slices[len(a.slices)-1].frame != 0 {
			a.note(frame, kind, "slice-keyframe-timing", "Aseprite slice keys persist until the next key; Aego currently retains each key only on its specified frame")
		}
		if flags&1 != 0 {
			a.note(frame, kind, "slice-ninepatch", "Nine-patch center is not retained; slice bounds and pivot are retained")
		}
	case 0x2006, 0x2008, 0x2016, 0x2017, 0x2020:
		// These chunks do not replace raster pixels. Keep the loss visible to users.
		a.note(frame, kind, "metadata-skipped", fmt.Sprintf("Chunk %#x metadata is not imported", kind))
		r.take(len(b))
	case 0x2023:
		return fmt.Errorf("tile sets are unsupported")
	default:
		return fmt.Errorf("unsupported chunk %#x", kind)
	}
	if r.Err() != nil {
		return r.Err()
	}
	if r.Offset() != len(b) {
		return fmt.Errorf("unexpected chunk trailing data")
	}
	return nil
}
