package art

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxPaletteBytes = 128 << 10

type paletteFile struct {
	Version int      `json:"version"`
	Colors  []Swatch `json:"colors"`
}

func validatePalette(p []Swatch) error {
	if len(p) < 1 || len(p) > 256 {
		return fmt.Errorf("art: palette needs 1..256 colors")
	}
	if p[0].Color.A != 0 {
		return fmt.Errorf("art: palette index zero must be transparent")
	}
	for _, v := range p {
		if len(v.Name) > 256 || !utf8.ValidString(v.Name) {
			return fmt.Errorf("art: invalid swatch name")
		}
	}
	return nil
}

// EncodePalette is the lossless RGBA palette interchange. GPL is intentionally
// not used for export because it cannot represent alpha or transparent indices.
func EncodePalette(p []Swatch) ([]byte, error) {
	if err := validatePalette(p); err != nil {
		return nil, err
	}
	return json.MarshalIndent(paletteFile{Version: 1, Colors: p}, "", "  ")
}

// DecodePalette accepts versioned JSON or a GIMP Palette. GPL's opaque colors
// are preceded by Aego's reserved transparent entry. No pixel algorithm lives
// in browser import code.
func DecodePalette(data []byte, format string) ([]Swatch, error) {
	if len(data) == 0 || len(data) > MaxPaletteBytes {
		return nil, ErrLimit
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("art: palette must be UTF-8")
	}
	var p []Swatch
	switch format {
	case "json":
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		var f paletteFile
		if err := dec.Decode(&f); err != nil {
			return nil, fmt.Errorf("art: invalid palette: %w", err)
		}
		if f.Version != 1 {
			return nil, fmt.Errorf("art: unsupported palette version %d", f.Version)
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return nil, fmt.Errorf("art: trailing palette data")
		}
		p = f.Colors
	case "gpl":
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 1024), MaxPaletteBytes)
		if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "GIMP Palette" {
			return nil, fmt.Errorf("art: missing GIMP Palette header")
		}
		p = []Swatch{{Name: "Transparent"}}
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "Name:") || strings.HasPrefix(line, "Columns:") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 3 {
				return nil, fmt.Errorf("art: invalid GPL row")
			}
			var rgb [3]uint8
			for i := range rgb {
				v, err := strconv.ParseUint(fields[i], 10, 8)
				if err != nil {
					return nil, fmt.Errorf("art: invalid GPL channel")
				}
				rgb[i] = uint8(v)
			}
			if len(p) == 256 {
				return nil, ErrLimit
			}
			p = append(p, Swatch{Color: Color{rgb[0], rgb[1], rgb[2], 255}, Name: strings.Join(fields[3:], " ")})
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		if len(p) == 1 {
			return nil, fmt.Errorf("art: empty GPL palette")
		}
	default:
		return nil, fmt.Errorf("art: unsupported palette format %q", format)
	}
	if err := validatePalette(p); err != nil {
		return nil, err
	}
	return p, nil
}

// ReplacePalette preserves existing indexed colors by default, rejecting an
// insufficient replacement rather than silently recoloring art. Quantize is an
// explicit lossy operation; data channels are never quantized with albedo.
func (e *Edit) ReplacePalette(p []Swatch, quantize bool) error {
	if err := e.check(); err != nil {
		return err
	}
	if err := validatePalette(p); err != nil {
		return err
	}
	p = append([]Swatch(nil), p...)
	old := *e.work
	target := old
	target.Palette = p
	if old.Canvas.Mode == Indexed && !quantize {
		exact := map[Color]uint32{}
		for i, v := range p {
			if _, ok := exact[v.Color]; !ok {
				exact[v.Color] = uint32(i)
			}
		}
		used := map[uint32]bool{0: true}
		for _, c := range e.work.Cels {
			if c.Channel != Albedo {
				continue
			}
			for key, block := range e.work.Images[c.Image].Tiles {
				pixels := e.work.Images[c.Image]
				for i, v := range block {
					if key.X*TileSize+i%TileSize < pixels.Width && key.Y*TileSize+i/TileSize < pixels.Height {
						used[v] = true
					}
				}
			}
		}
		remap := make([]uint32, len(old.Palette))
		for v := range used {
			if v == 0 {
				continue
			}
			n, ok := exact[old.Palette[v].Color]
			if !ok {
				return fmt.Errorf("art: replacement omits used color %d; explicit quantization is required", v)
			}
			remap[v] = n
		}
		if err := e.mapPixels(func(v uint32) uint32 { return remap[v] }); err != nil {
			return err
		}
	} else if quantize {
		if err := e.mapPixels(func(v uint32) uint32 {
			c := decode(&old, v)
			if c.A == 0 {
				return 0
			}
			idx := nearest(p, c, nil)
			if old.Canvas.Mode == Indexed {
				return idx
			}
			return encode(&target, p[idx].Color)
		}); err != nil {
			return err
		}
	}
	e.work.Palette = p
	e.structural = true
	return nil
}

// ExtractPalette uses frequency-ranked exact colors, not a platform-dependent
// quantizer. Ties are sorted by RGBA value so native and WASM agree. The bounded
// histogram makes large true-color imports fail explicitly instead of growing
// memory without limit. The first result is always transparent.
func ExtractPalette(ctx context.Context, im *image.NRGBA, limit int) ([]Swatch, error) {
	if limit < 2 || limit > 256 || im == nil {
		return nil, fmt.Errorf("art: palette limit must be 2..256")
	}
	histogram := map[Color]int{}
	for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
			v := im.NRGBAAt(x, y)
			if v.A == 0 {
				continue
			}
			c := Color{v.R, v.G, v.B, v.A}
			if _, ok := histogram[c]; !ok && len(histogram) >= 65536 {
				return nil, fmt.Errorf("art: palette extraction exceeds 65536 distinct colors")
			}
			histogram[c]++
		}
	}
	colors := make([]Color, 0, len(histogram))
	for c := range histogram {
		colors = append(colors, c)
	}
	sort.Slice(colors, func(i, j int) bool {
		a, b := colors[i], colors[j]
		if histogram[a] != histogram[b] {
			return histogram[a] > histogram[b]
		}
		return a.packed() < b.packed()
	})
	out := []Swatch{{Name: "Transparent"}}
	for _, c := range colors[:min(len(colors), limit-1)] {
		out = append(out, Swatch{Color: c})
	}
	return out, nil
}

func (e *Edit) PaletteFromFrame(ctx context.Context, frame ID, limit int, quantize bool) error {
	im, err := e.Render(frame, RenderOptions{})
	if err != nil {
		return err
	}
	p, err := ExtractPalette(ctx, im, limit)
	if err != nil {
		return err
	}
	return e.ReplacePalette(p, quantize)
}

// ShadeRamp shifts selected albedo pixels along a palette range. It respects
// selection, linked cel topology and alpha lock via the ordinary write path.
func (e *Edit) ShadeRamp(ctx context.Context, cel ID, sel *Selection, first, last, step int) error {
	c, err := e.celForWrite(cel)
	if err != nil {
		return err
	}
	if c.Channel != Albedo {
		return fmt.Errorf("art: ramp shading is only defined for albedo")
	}
	if first < 1 || last < first || last >= len(e.work.Palette) || step == 0 || step < -255 || step > 255 {
		return fmt.Errorf("art: invalid palette ramp")
	}
	p := e.work.Images[c.Image]
	for y := 0; y < e.work.Canvas.Height; y++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		for x := 0; x < e.work.Canvas.Width; x++ {
			v := p.at(x-c.X, y-c.Y)
			col := decode(e.work, v)
			if col.A == 0 {
				continue
			}
			idx := int(v)
			if e.work.Canvas.Mode != Indexed {
				idx = int(nearest(e.work.Palette, col, nil))
			}
			if idx < first || idx > last {
				continue
			}
			idx = max(first, min(last, idx+step))
			if err := e.write(c, x, y, encode(e.work, e.work.Palette[idx].Color), sel, false); err != nil {
				return err
			}
		}
	}
	return nil
}
