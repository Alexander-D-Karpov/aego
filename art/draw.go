package art

import (
	"context"
	"fmt"
	"image"
	"math"
)

type Paint struct {
	Color    Color `json:"color"`
	Index    int   `json:"index"`
	UseIndex bool  `json:"useIndex"`
}
type Brush struct {
	Paint      Paint `json:"paint"`
	Background Paint `json:"background"`
	Size       int   `json:"size"`
	Round      bool  `json:"round"`
	Erase      bool  `json:"erase"`
	// EraseStrength 0 preserves the historical full-strength eraser for old
	// clients; 1–255 controls partial erasing or directional smudging.
	EraseStrength uint8      `json:"eraseStrength,omitempty"`
	Smudge        bool       `json:"smudge,omitempty"`
	PixelPerfect  bool       `json:"pixelPerfect"`
	MirrorX       bool       `json:"mirrorX"`
	MirrorY       bool       `json:"mirrorY"`
	Dither        int        `json:"dither"`
	Source        string     `json:"source,omitempty"`
	Stamp         bool       `json:"-"`
	Pattern       *Clipboard `json:"-"`
}

func paintValue(s *state, p Paint) (uint32, error) {
	if s.Canvas.Mode == Indexed && p.UseIndex {
		if p.Index < 0 || p.Index >= len(s.Palette) {
			return 0, fmt.Errorf("art: invalid palette index")
		}
		return uint32(p.Index), nil
	}
	return encode(s, p.Color), nil
}
func (e *Edit) Pixel(cel ID, x, y int) (Color, error) {
	if err := e.check(); err != nil {
		return Color{}, err
	}
	i, ok := e.celIndex(cel)
	if !ok {
		return Color{}, fmt.Errorf("art: missing cel")
	}
	c := e.work.Cels[i]
	return decode(e.work.forChannel(c.Channel), e.work.Images[c.Image].at(x-c.X, y-c.Y)), nil
}
func (d *Document) Pixel(cel ID, x, y int) (Color, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return Color{}, ErrClosed
	}
	i, ok := d.current.cel(cel)
	if !ok {
		return Color{}, fmt.Errorf("art: missing cel")
	}
	c := d.current.Cels[i]
	return decode(d.current.forChannel(c.Channel), d.current.Images[c.Image].at(x-c.X, y-c.Y)), nil
}

// SamplePaint reads one pixel from the active cel without changing history.
// In indexed albedo, retain the actual index even when palette colors repeat.
// An absent cel is transparent; coordinates outside the canvas are rejected.
func (d *Document) SamplePaint(cel ID, ch Channel, x, y int) (Paint, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return Paint{}, ErrClosed
	}
	if !validChannel(ch) || x < 0 || y < 0 || x >= d.current.Canvas.Width || y >= d.current.Canvas.Height {
		return Paint{}, fmt.Errorf("art: sample is outside the canvas or channel")
	}
	s := d.current.forChannel(ch)
	var value uint32
	if cel != "" {
		i, ok := d.current.cel(cel)
		if !ok || d.current.Cels[i].Channel != ch {
			return Paint{}, fmt.Errorf("art: missing cel in the selected channel")
		}
		c := d.current.Cels[i]
		value = d.current.Images[c.Image].at(x-c.X, y-c.Y)
	}
	paint := Paint{Color: decode(s, value)}
	if s.Canvas.Mode == Indexed {
		paint.UseIndex = true
		paint.Index = int(value)
	}
	return paint, nil
}
func (e *Edit) SetPixel(cel ID, x, y int, p Paint, sel *Selection, erase bool) error {
	c, err := e.celForWrite(cel)
	if err != nil {
		return err
	}
	if sel != nil && (sel.Width != e.work.Canvas.Width || sel.Height != e.work.Canvas.Height) {
		return fmt.Errorf("art: selection dimensions mismatch")
	}
	v, err := paintValue(e.work.forChannel(c.Channel), p)
	if err != nil {
		return err
	}
	return e.write(c, x, y, v, sel, erase)
}
func (e *Edit) write(c Cel, x, y int, v uint32, sel *Selection, erase bool) error {
	// Growing a cropped image shifts every linked cel's origin. Long-running
	// gestures keep stable IDs, never cached offsets from an earlier draft.
	ci, ok := e.celIndex(c.ID)
	if !ok {
		return fmt.Errorf("art: missing cel")
	}
	c = e.work.Cels[ci]
	if x < 0 || y < 0 || x >= e.work.Canvas.Width || y >= e.work.Canvas.Height || !sel.Contains(x, y) {
		return nil
	}
	li, _ := e.work.layer(c.Layer)
	l := e.work.Layers[li]
	p := e.work.Images[c.Image]
	lx, ly := x-c.X, y-c.Y
	if lx < 0 || ly < 0 || lx >= p.Width || ly >= p.Height {
		if erase || l.AlphaLock || v == 0 {
			return nil
		}
		var err error
		c, err = e.expandCel(c)
		if err != nil {
			return err
		}
		p = e.work.Images[c.Image]
		lx, ly = x-c.X, y-c.Y
	}
	old := p.at(lx, ly)
	if erase {
		if l.AlphaLock {
			return nil
		}
		return e.raw(c.Image, lx, ly, 0)
	}
	if l.AlphaLock {
		oc := decode(e.work.forChannel(c.Channel), old)
		if oc.A == 0 {
			return nil
		}
		color := decode(e.work.forChannel(c.Channel), v)
		color.A = oc.A
		if e.work.forChannel(c.Channel).Canvas.Mode == Indexed {
			v = nearest(e.work.Palette, color, &oc.A)
		} else {
			v = encode(e.work.forChannel(c.Channel), color)
		}
	}
	return e.raw(c.Image, lx, ly, v)
}

var bayer4 = [16]int{0, 8, 2, 10, 12, 4, 14, 6, 3, 11, 1, 9, 15, 7, 13, 5}

func validateBrush(b Brush) error {
	if b.Smudge && (!b.Erase || b.Pattern != nil || b.Stamp || b.PixelPerfect || b.Dither != 0 || b.Source != "" && b.Source != "solid") {
		return fmt.Errorf("art: smudge requires an eraser with a solid brush")
	}
	if b.Source != "" && b.Source != "solid" && b.Source != "pattern" && b.Source != "stamp" {
		return fmt.Errorf("art: invalid brush source")
	}
	if (b.Source == "stamp" || b.Source == "pattern") && b.Pattern == nil {
		return fmt.Errorf("art: capture a brush from a selection first")
	}
	if b.Size < 1 || b.Size > 128 || b.Dither < 0 || b.Dither > 16 {
		return fmt.Errorf("art: invalid brush")
	}
	if b.Pattern != nil && (b.Pattern.Width < 1 || b.Pattern.Height < 1 || len(b.Pattern.Pixels) != b.Pattern.Width*b.Pattern.Height) {
		return fmt.Errorf("art: invalid brush pattern")
	}
	return nil
}

// Gesture interpolates every input segment in the core, not in JS. Its draft
// is visible through Render but remains unpublished until Prepare/Apply.
type Gesture struct {
	edit      *Edit
	cel       Cel
	brush     Brush
	selection *Selection
	path      []image.Point
	hits      map[image.Point]int
	originals map[image.Point]uint32
	fg, bg    uint32
}

func (e *Edit) Stroke(cel ID, b Brush, sel *Selection) (*Gesture, error) {
	c, err := e.celForWrite(cel)
	if err != nil {
		return nil, err
	}
	if err = validateBrush(b); err != nil {
		return nil, err
	}
	if sel != nil && (sel.Width != e.work.Canvas.Width || sel.Height != e.work.Canvas.Height) {
		return nil, fmt.Errorf("art: selection dimensions mismatch")
	}
	fg, err := paintValue(e.work.forChannel(c.Channel), b.Paint)
	if err != nil {
		return nil, err
	}
	bg, err := paintValue(e.work.forChannel(c.Channel), b.Background)
	if err != nil {
		return nil, err
	}
	return &Gesture{edit: e, cel: c, brush: b, selection: sel, hits: make(map[image.Point]int), originals: make(map[image.Point]uint32), fg: fg, bg: bg}, nil
}
func (g *Gesture) Edit() *Edit { return g.edit }
func (g *Gesture) Point(p image.Point) error {
	if err := g.edit.check(); err != nil {
		return err
	}
	if !clampPoint(p) {
		return ErrLimit
	}
	if len(g.path) == 0 {
		return g.add(p)
	}
	last := g.path[len(g.path)-1]
	if last == p {
		return nil
	}
	first := true
	return line(last, p, func(v image.Point) error {
		if first {
			first = false
			return nil
		}
		return g.add(v)
	})
}
func (g *Gesture) add(p image.Point) error {
	n := len(g.path)
	if g.brush.Erase && g.brush.Smudge {
		g.path = append(g.path, p)
		if n == 0 {
			return nil // The first dab picks up pixels without changing them.
		}
		return g.smudge(g.path[n-1], p)
	}
	if g.brush.PixelPerfect && g.brush.Size == 1 && n >= 2 {
		a, b := g.path[n-2], g.path[n-1]
		if abs(a.X-p.X) == 1 && abs(a.Y-p.Y) == 1 && abs(a.X-b.X)+abs(a.Y-b.Y) == 1 && abs(b.X-p.X)+abs(b.Y-p.Y) == 1 {
			if err := g.stamp(b, -1); err != nil {
				return err
			}
			g.path = g.path[:n-1]
		}
	}
	g.path = append(g.path, p)
	return g.stamp(p, 1)
}
func (g *Gesture) stamp(p image.Point, delta int) error {
	b := g.brush
	e := g.edit
	s := e.work
	centers := []image.Point{p}
	if b.MirrorX {
		centers = append(centers, image.Pt(s.Canvas.Width-1-p.X, p.Y))
	}
	if b.MirrorY {
		n := len(centers)
		for i := 0; i < n; i++ {
			centers = append(centers, image.Pt(centers[i].X, s.Canvas.Height-1-centers[i].Y))
		}
	}
	unique := make(map[image.Point]bool, b.Size*b.Size*len(centers))
	for _, center := range centers {
		for by := 0; by < b.Size; by++ {
			for bx := 0; bx < b.Size; bx++ {
				if b.Round {
					dx, dy := 2*bx-b.Size+1, 2*by-b.Size+1
					if dx*dx+dy*dy > b.Size*b.Size {
						continue
					}
				}
				v := image.Pt(center.X+bx-b.Size/2, center.Y+by-b.Size/2)
				patternIndex := -1
				if b.Stamp && b.Pattern != nil {
					x, y := v.X-center.X+b.Pattern.Width/2, v.Y-center.Y+b.Pattern.Height/2
					if x < 0 || y < 0 || x >= b.Pattern.Width || y >= b.Pattern.Height {
						continue
					}
					patternIndex = y*b.Pattern.Width + x
					if b.Pattern.Pixels[patternIndex].A == 0 {
						continue
					}
				}
				if unique[v] || v.X < 0 || v.Y < 0 || v.X >= s.Canvas.Width || v.Y >= s.Canvas.Height || !g.selection.Contains(v.X, v.Y) {
					continue
				}
				unique[v] = true
				ci, ok := e.celIndex(g.cel.ID)
				if !ok {
					return fmt.Errorf("art: missing cel")
				}
				g.cel = e.work.Cels[ci]
				if delta < 0 {
					g.hits[v]--
					if g.hits[v] == 0 {
						if err := e.raw(g.cel.Image, v.X-g.cel.X, v.Y-g.cel.Y, g.originals[v]); err != nil {
							return err
						}
					}
					continue
				}
				g.hits[v]++
				if g.hits[v] > 1 && !b.Stamp {
					continue
				}
				if g.hits[v] == 1 {
					g.originals[v] = s.Images[g.cel.Image].at(v.X-g.cel.X, v.Y-g.cel.Y)
				}
				color := g.fg
				if b.Dither > 0 && bayer4[(v.Y&3)*4+(v.X&3)] >= b.Dither {
					color = g.bg
				}
				if b.Pattern != nil {
					i := (v.Y%b.Pattern.Height)*b.Pattern.Width + v.X%b.Pattern.Width
					if patternIndex >= 0 {
						i = patternIndex
					}
					color = encode(s.forChannel(g.cel.Channel), b.Pattern.Pixels[i])
				}
				if b.Erase && b.EraseStrength != 0 && b.EraseStrength != 255 {
					li, _ := s.layer(g.cel.Layer)
					if s.Layers[li].AlphaLock {
						continue
					}
					old := g.originals[v]
					channel := s.forChannel(g.cel.Channel)
					original := decode(channel, old)
					soft := original
					soft.A = uint8((uint32(original.A)*uint32(255-b.EraseStrength) + 127) / 255)
					value := encode(channel, soft)
					// Duplicate indexed swatches must not change their stored index
					// when the quantized color has not changed.
					if decode(channel, value) == original {
						value = old
					}
					if err := e.write(g.cel, v.X, v.Y, value, g.selection, false); err != nil {
						return err
					}
					continue
				}
				if err := e.write(g.cel, v.X, v.Y, color, g.selection, b.Erase); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// smudge moves the previous dab into the current one. All samples are read
// before writing a dab, so brush size, direction and mirrored overlaps do not
// depend on iteration order. Each completed drag is one normal history change.
func (g *Gesture) smudge(previous, current image.Point) error {
	e, s, b := g.edit, g.edit.work, g.brush
	ci, ok := e.celIndex(g.cel.ID)
	if !ok {
		return fmt.Errorf("art: missing cel")
	}
	g.cel = s.Cels[ci]
	channel := s.forChannel(g.cel.Channel)
	pixels := s.Images[g.cel.Image]
	type move struct{ from, to image.Point }
	centers := []move{{previous, current}}
	if b.MirrorX {
		centers = append(centers, move{image.Pt(s.Canvas.Width-1-previous.X, previous.Y), image.Pt(s.Canvas.Width-1-current.X, current.Y)})
	}
	if b.MirrorY {
		for _, pair := range append([]move(nil), centers...) {
			centers = append(centers, move{image.Pt(pair.from.X, s.Canvas.Height-1-pair.from.Y), image.Pt(pair.to.X, s.Canvas.Height-1-pair.to.Y)})
		}
	}
	strength := b.EraseStrength
	if strength == 0 {
		strength = 255
	}
	type pixel struct {
		point image.Point
		value uint32
	}
	var changes []pixel
	seen := make(map[image.Point]bool, b.Size*b.Size*len(centers))
	bounds := image.Rect(0, 0, s.Canvas.Width, s.Canvas.Height)
	for _, pair := range centers {
		for y := 0; y < b.Size; y++ {
			for x := 0; x < b.Size; x++ {
				if b.Round {
					dx, dy := 2*x-b.Size+1, 2*y-b.Size+1
					if dx*dx+dy*dy > b.Size*b.Size {
						continue
					}
				}
				offset := image.Pt(x-b.Size/2, y-b.Size/2)
				from, to := pair.from.Add(offset), pair.to.Add(offset)
				if seen[to] || !from.In(bounds) || !to.In(bounds) || !g.selection.Contains(from.X, from.Y) || !g.selection.Contains(to.X, to.Y) {
					continue
				}
				seen[to] = true
				old := pixels.at(to.X-g.cel.X, to.Y-g.cel.Y)
				picked := pixels.at(from.X-g.cel.X, from.Y-g.cel.Y)
				value := picked
				if strength != 255 {
					mixed := mixSmudge(decode(channel, old), decode(channel, picked), strength)
					value = encode(channel, mixed)
					if decode(channel, value) == decode(channel, old) {
						value = old
					}
				}
				changes = append(changes, pixel{to, value})
			}
		}
	}
	for _, change := range changes {
		if err := e.write(g.cel, change.point.X, change.point.Y, change.value, g.selection, false); err != nil {
			return err
		}
	}
	return nil
}

// Interpolate premultiplied channels to avoid dark fringes against transparent
// pixels, then return the straight-alpha bytes used by Art Core.
func mixSmudge(dst, src Color, strength uint8) Color {
	a, b := uint32(255-strength), uint32(strength)
	weight := uint32(dst.A)*a + uint32(src.A)*b
	if weight == 0 {
		return dst
	}
	mix := func(d, s uint8) uint8 {
		return uint8((uint32(d)*uint32(dst.A)*a + uint32(s)*uint32(src.A)*b + weight/2) / weight)
	}
	return Color{mix(dst.R, src.R), mix(dst.G, src.G), mix(dst.B, src.B), uint8((weight + 127) / 255)}
}
func line(a, b image.Point, visit func(image.Point) error) error {
	if !clampPoint(a) || !clampPoint(b) {
		return ErrLimit
	}
	dx, dy := abs(b.X-a.X), -abs(b.Y-a.Y)
	sx, sy := -1, -1
	if a.X < b.X {
		sx = 1
	}
	if a.Y < b.Y {
		sy = 1
	}
	err := dx + dy
	for {
		if e := visit(a); e != nil {
			return e
		}
		if a == b {
			return nil
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			a.X += sx
		}
		if e2 <= dx {
			err += dx
			a.Y += sy
		}
	}
}
func (e *Edit) Line(cel ID, a, b image.Point, brush Brush, sel *Selection) error {
	g, err := e.Stroke(cel, brush, sel)
	if err != nil {
		return err
	}
	if err = g.Point(a); err != nil {
		return err
	}
	return g.Point(b)
}

// Shape rasterizes inclusive endpoints. The view previews by discarding and
// recreating a private Edit; releasing the pointer publishes one Change.
func (e *Edit) Shape(cel ID, kind string, a, b image.Point, brush Brush, sel *Selection, filled bool) error {
	g, err := e.Stroke(cel, brush, sel)
	if err != nil {
		return err
	}
	if !clampPoint(a) || !clampPoint(b) {
		return ErrLimit
	}
	if kind == "line" {
		if err = g.Point(a); err != nil {
			return err
		}
		return g.Point(b)
	}
	x0, x1 := min(a.X, b.X), max(a.X, b.X)
	y0, y1 := min(a.Y, b.Y), max(a.Y, b.Y)
	if kind == "rectangle" {
		if filled {
			for y := max(0, y0); y <= min(e.work.Canvas.Height-1, y1); y++ {
				for x := max(0, x0); x <= min(e.work.Canvas.Width-1, x1); x++ {
					if err = g.stamp(image.Pt(x, y), 1); err != nil {
						return err
					}
				}
			}
			return nil
		}
		for _, seg := range [][2]image.Point{{{x0, y0}, {x1, y0}}, {{x1, y0}, {x1, y1}}, {{x1, y1}, {x0, y1}}, {{x0, y1}, {x0, y0}}} {
			if err = line(seg[0], seg[1], func(p image.Point) error { return g.stamp(p, 1) }); err != nil {
				return err
			}
		}
		return nil
	}
	if kind != "ellipse" {
		return fmt.Errorf("art: unknown shape %q", kind)
	}
	rx, ry := float64(x1-x0+1)/2, float64(y1-y0+1)/2
	cx, cy := float64(x0+x1)/2, float64(y0+y1)/2
	inside := func(x, y int) bool { dx, dy := (float64(x)-cx)/rx, (float64(y)-cy)/ry; return dx*dx+dy*dy <= 1 }
	for y := max(0, y0); y <= min(e.work.Canvas.Height-1, y1); y++ {
		for x := max(0, x0); x <= min(e.work.Canvas.Width-1, x1); x++ {
			if inside(x, y) && (filled || !inside(x-1, y) || !inside(x+1, y) || !inside(x, y-1) || !inside(x, y+1)) {
				if err = g.stamp(image.Pt(x, y), 1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func (e *Edit) Polygon(cel ID, points []image.Point, brush Brush, sel *Selection, filled bool) error {
	if len(points) < 2 || len(points) > 65536 {
		return fmt.Errorf("art: invalid polygon")
	}
	g, err := e.Stroke(cel, brush, sel)
	if err != nil {
		return err
	}
	if filled {
		mask, err := LassoSelection(e.work.Canvas.Width, e.work.Canvas.Height, points)
		if err != nil {
			return err
		}
		for y := 0; y < mask.Height; y++ {
			for x := 0; x < mask.Width; x++ {
				if mask.Contains(x, y) && sel.Contains(x, y) {
					if err = g.stamp(image.Pt(x, y), 1); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	for i, a := range points {
		if err = line(a, points[(i+1)%len(points)], func(p image.Point) error { return g.stamp(p, 1) }); err != nil {
			return err
		}
	}
	return nil
}
func (e *Edit) Curve(cel ID, a, b, c, d image.Point, brush Brush, sel *Selection) error {
	for _, p := range []image.Point{a, b, c, d} {
		if !clampPoint(p) {
			return ErrLimit
		}
	}
	n := max(1, 2*(abs(a.X-b.X)+abs(a.Y-b.Y)+abs(b.X-c.X)+abs(b.Y-c.Y)+abs(c.X-d.X)+abs(c.Y-d.Y)))
	n = min(n, 65536)
	g, err := e.Stroke(cel, brush, sel)
	if err != nil {
		return err
	}
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		u := 1 - t
		x := u*u*u*float64(a.X) + 3*u*u*t*float64(b.X) + 3*u*t*t*float64(c.X) + t*t*t*float64(d.X)
		y := u*u*u*float64(a.Y) + 3*u*u*t*float64(b.Y) + 3*u*t*t*float64(c.Y) + t*t*t*float64(d.Y)
		if err = g.Point(image.Pt(int(math.Round(x)), int(math.Round(y)))); err != nil {
			return err
		}
	}
	return nil
}
func (e *Edit) Fill(ctx context.Context, cel ID, x, y, tolerance int, paint Paint, sel *Selection, contiguous bool) error {
	c, err := e.celForWrite(cel)
	if err != nil {
		return err
	}
	value, err := paintValue(e.work.forChannel(c.Channel), paint)
	if err != nil {
		return err
	}
	mask, err := wand(e.work, cel, x, y, tolerance, contiguous)
	if err != nil {
		return err
	}
	for y := 0; y < mask.Height; y++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		for x := 0; x < mask.Width; x++ {
			if mask.Contains(x, y) && sel.Contains(x, y) {
				if err = e.write(c, x, y, value, sel, false); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func (e *Edit) Gradient(ctx context.Context, cel ID, a, b image.Point, from, to Paint, sel *Selection, dither bool) error {
	c, err := e.celForWrite(cel)
	if err != nil {
		return err
	}
	if !clampPoint(a) || !clampPoint(b) {
		return ErrLimit
	}
	av, err := paintValue(e.work.forChannel(c.Channel), from)
	if err != nil {
		return err
	}
	bv, err := paintValue(e.work.forChannel(c.Channel), to)
	if err != nil {
		return err
	}
	ac, bc := decode(e.work.forChannel(c.Channel), av), decode(e.work.forChannel(c.Channel), bv)
	dx, dy := int64(b.X-a.X), int64(b.Y-a.Y)
	den := dx*dx + dy*dy
	if den == 0 {
		den = 1
	}
	for y := 0; y < e.work.Canvas.Height; y++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		for x := 0; x < e.work.Canvas.Width; x++ {
			if !sel.Contains(x, y) {
				continue
			}
			t := int(max(0, min(65536, (int64(x-a.X)*dx+int64(y-a.Y)*dy)*65536/den)))
			v := uint32(0)
			if e.work.Canvas.Mode == Indexed {
				if dither {
					if t*16/65536 > bayer4[(y&3)*4+(x&3)] {
						v = bv
					} else {
						v = av
					}
				} else {
					v = uint32(int(av) + (int(bv)-int(av))*t/65536)
				}
			} else {
				lerp := func(a, b uint8) uint8 { return uint8((int(a)*(65536-t) + int(b)*t + 32768) / 65536) }
				v = encode(e.work.forChannel(c.Channel), Color{lerp(ac.R, bc.R), lerp(ac.G, bc.G), lerp(ac.B, bc.B), lerp(ac.A, bc.A)})
			}
			if err = e.write(c, x, y, v, sel, false); err != nil {
				return err
			}
		}
	}
	return nil
}
