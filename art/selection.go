package art

import (
	"fmt"
	"image"
	"math"
	"sort"
)

// Selection is a canvas-space mask, independent of the document's pixels.
// A nil selection means unrestricted; an allocated empty mask means no pixels.
type Selection struct {
	Width, Height int
	bits          []uint64
	bounds        image.Rectangle
	count         int
}

func NewSelection(w, h int) (*Selection, error) {
	if w < 1 || h < 1 || w > 4096 || h > 4096 || int64(w)*int64(h) > 4194304 {
		return nil, ErrLimit
	}
	return &Selection{Width: w, Height: h, bits: make([]uint64, (w*h+63)/64)}, nil
}
func (s *Selection) Contains(x, y int) bool {
	if s == nil {
		return true
	}
	if x < 0 || y < 0 || x >= s.Width || y >= s.Height {
		return false
	}
	i := y*s.Width + x
	return s.bits[i/64]&(1<<uint(i%64)) != 0
}
func (s *Selection) set(x, y int) {
	if x >= 0 && y >= 0 && x < s.Width && y < s.Height {
		i := y*s.Width + x
		mask := uint64(1) << uint(i%64)
		if s.bits[i/64]&mask == 0 {
			s.bits[i/64] |= mask
			s.bounds = s.bounds.Union(image.Rect(x, y, x+1, y+1))
			s.count++
		}
	}
}
func (s *Selection) Bounds() image.Rectangle {
	if s == nil {
		return image.Rectangle{}
	}
	return s.bounds
}
func (s *Selection) Count() int {
	if s == nil {
		return 0
	}
	return s.count
}
func RectSelection(w, h int, r image.Rectangle, ellipse bool) (*Selection, error) {
	s, err := NewSelection(w, h)
	if err != nil {
		return nil, err
	}
	r = r.Canon().Intersect(image.Rect(0, 0, w, h))
	rx, ry := int64(r.Dx()), int64(r.Dy())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			dx, dy := int64(2*x-r.Min.X-r.Max.X+1), int64(2*y-r.Min.Y-r.Max.Y+1)
			if !ellipse || dx*dx*ry*ry+dy*dy*rx*rx <= rx*rx*ry*ry {
				s.set(x, y)
			}
		}
	}
	return s, nil
}
func LassoSelection(w, h int, points []image.Point) (*Selection, error) {
	s, err := NewSelection(w, h)
	if err != nil {
		return nil, err
	}
	if len(points) < 3 || len(points) > 65536 {
		return nil, fmt.Errorf("art: lasso needs 3..65536 points")
	}
	for _, p := range points {
		if abs(p.X) > 16384 || abs(p.Y) > 16384 {
			return nil, ErrLimit
		}
	}
	// Scanline crossings preserve the same even/odd rule at pixel centers as
	// the point-in-polygon test, without retesting every edge for every pixel.
	// This matters for pointer lassos on large canvases.
	crossings := make([]float64, 0, len(points))
	for y := 0; y < h; y++ {
		crossings = crossings[:0]
		py := float64(y) + .5
		j := len(points) - 1
		for i, p := range points {
			q := points[j]
			if (float64(p.Y) > py) != (float64(q.Y) > py) {
				crossings = append(crossings, float64(q.X-p.X)*(py-float64(p.Y))/float64(q.Y-p.Y)+float64(p.X))
			}
			j = i
		}
		sort.Float64s(crossings)
		for i := 0; i+1 < len(crossings); i += 2 {
			start := max(0, int(math.Ceil(crossings[i]-.5)))
			end := min(w, int(math.Ceil(crossings[i+1]-.5)))
			for x := start; x < end; x++ {
				s.set(x, y)
			}
		}
	}
	return s, nil
}
func (d *Document) Wand(cel ID, x, y, tolerance int, contiguous bool) (*Selection, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, ErrClosed
	}
	return wand(d.current, cel, x, y, tolerance, contiguous)
}
func wand(st *state, cel ID, x, y, tolerance int, contiguous bool) (*Selection, error) {
	i, ok := st.cel(cel)
	if !ok {
		return nil, fmt.Errorf("art: missing cel")
	}
	c := st.Cels[i]
	p := st.Images[c.Image]
	w, h := st.Canvas.Width, st.Canvas.Height
	s, err := NewSelection(w, h)
	if err != nil {
		return nil, err
	}
	if x < 0 || y < 0 || x >= w || y >= h {
		return s, nil
	}
	if tolerance < 0 || tolerance > 255 {
		return nil, fmt.Errorf("art: invalid tolerance")
	}
	target := decode(st.forChannel(c.Channel), p.at(x-c.X, y-c.Y))
	match := func(x, y int) bool {
		v := decode(st.forChannel(c.Channel), p.at(x-c.X, y-c.Y))
		return abs(int(v.R)-int(target.R)) <= tolerance && abs(int(v.G)-int(target.G)) <= tolerance && abs(int(v.B)-int(target.B)) <= tolerance && abs(int(v.A)-int(target.A)) <= tolerance
	}
	if !contiguous {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if match(x, y) {
					s.set(x, y)
				}
			}
		}
		return s, nil
	}
	visited := make([]bool, w*h)
	queue := []image.Point{{x, y}}
	visited[y*w+x] = true
	for head := 0; head < len(queue); head++ {
		v := queue[head]
		if !match(v.X, v.Y) {
			continue
		}
		s.set(v.X, v.Y)
		for _, n := range []image.Point{{v.X - 1, v.Y}, {v.X + 1, v.Y}, {v.X, v.Y - 1}, {v.X, v.Y + 1}} {
			if n.X >= 0 && n.Y >= 0 && n.X < w && n.Y < h && !visited[n.Y*w+n.X] {
				visited[n.Y*w+n.X] = true
				queue = append(queue, n)
			}
		}
	}
	return s, nil
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func clampPoint(p image.Point) bool {
	return math.Abs(float64(p.X)) <= 16384 && math.Abs(float64(p.Y)) <= 16384
}
