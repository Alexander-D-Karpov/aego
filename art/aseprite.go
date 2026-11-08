package art

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"reflect"
	"unicode/utf8"
)

// ImportDiagnostic identifies discarded non-pixel metadata. Unsupported visual
// semantics are errors, not a best-effort image presented as a lossless import.
type ImportDiagnostic struct {
	Code    string `json:"code"`
	Frame   int    `json:"frame"`
	Chunk   uint16 `json:"chunk"`
	Message string `json:"message"`
}
type aseReader struct {
	b   []byte
	at  int
	err error
}

func (r *aseReader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.b)-r.at {
		r.err = io.ErrUnexpectedEOF
		return nil
	}
	v := r.b[r.at : r.at+n]
	r.at += n
	return v
}
func (r *aseReader) u8() uint8 {
	b := r.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}
func (r *aseReader) u16() uint16 {
	b := r.take(2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}
func (r *aseReader) u32() uint32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}
func (r *aseReader) i16() int { return int(int16(r.u16())) }
func (r *aseReader) i32() int { return int(int32(r.u32())) }
func (r *aseReader) text() string {
	n := int(r.u16())
	b := r.take(n)
	if !utf8.Valid(b) {
		r.err = fmt.Errorf("invalid UTF-8")
	}
	return string(b)
}

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
	name  string
	frame int
	rect  image.Rectangle
	pivot image.Point
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
	h := aseReader{b: data[:128]}
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
	off := 128
	totalChunks := uint64(0)
	for frame := 0; frame < frames; frame++ {
		if off > len(data)-16 {
			return fail(io.ErrUnexpectedEOF)
		}
		r := aseReader{b: data[off:]}
		frameSize := int64(r.u32())
		fm := r.u16()
		oldCount := r.u16()
		duration := int(r.u16())
		r.take(2)
		newCount := r.u32()
		if frameSize < 16 || frameSize > int64(len(data)-off) || fm != 0xf1fa {
			return fail(fmt.Errorf("invalid frame %d", frame))
		}
		count := uint32(oldCount)
		if newCount != 0 {
			count = newCount
		}
		totalChunks += uint64(count)
		if totalChunks > 65536 || uint64(count) > uint64(frameSize-16)/6 {
			return fail(fmt.Errorf("invalid chunk count"))
		}
		if duration == 0 {
			duration = speed
		}
		if duration < 1 {
			duration = 100
		}
		a.frames = append(a.frames, duration)
		end := off + int(frameSize)
		pos := off + 16
		for ci := uint32(0); ci < count; ci++ {
			if pos > end-6 {
				return fail(io.ErrUnexpectedEOF)
			}
			n := int64(binary.LittleEndian.Uint32(data[pos:]))
			kind := binary.LittleEndian.Uint16(data[pos+4:])
			if n < 6 || n > int64(end-pos) {
				return fail(fmt.Errorf("invalid chunk size at frame %d", frame))
			}
			if err := a.chunk(frame, kind, data[pos+6:pos+int(n)]); err != nil {
				return fail(fmt.Errorf("frame %d chunk %#x: %w", frame, kind, err))
			}
			pos += int(n)
		}
		if pos != end {
			return fail(fmt.Errorf("trailing frame bytes"))
		}
		off = end
	}
	if off != len(data) {
		return fail(fmt.Errorf("trailing file bytes"))
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
		s.Slices = append(s.Slices, Slice{d.allocate(), v.name, s.Frames[v.frame].ID, v.rect, v.pivot})
	}
	if err = s.validate(l); err != nil {
		return fail(err)
	}
	d.current = s
	return d, a.diagnostics, nil
}
func (a *aseImport) chunk(frame int, kind uint16, b []byte) error {
	r := aseReader{b: b}
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
			if r.err != nil {
				return r.err
			}
			raw := r.take(len(b) - r.at)
			if typ == 0 {
				if int64(len(raw)) != n {
					return fmt.Errorf("raw cel size mismatch")
				}
				c.raw = append([]byte(nil), raw...)
			} else {
				source := bytes.NewReader(raw)
				z, err := zlib.NewReader(source)
				if err != nil {
					return err
				}
				c.raw, err = io.ReadAll(io.LimitReader(z, n+1))
				closeErr := z.Close()
				if err != nil {
					return err
				}
				if closeErr != nil {
					return closeErr
				}
				if int64(len(c.raw)) != n || source.Len() != 0 {
					return fmt.Errorf("compressed cel size mismatch or trailing data")
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
				v.pivot = image.Pt(r.i32(), r.i32())
			}
			a.slices = append(a.slices, v)
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
	if r.err != nil {
		return r.err
	}
	if r.at != len(b) {
		return fmt.Errorf("unexpected chunk trailing data")
	}
	return nil
}
