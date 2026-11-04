package view

import (
	"aego/core/mathx"
	"math"
)

type cameraCoverage struct {
	viewport, clear mathx.Rect
	order           int32
	opaque          bool
}

// OutputCoverage records the rectangles one logical output will process and
// the clears that establish their destination alpha, in camera submission order.
type OutputCoverage struct{ cameras []cameraCoverage }

func (s *OutputCoverage) Reset() { s.cameras = s.cameras[:0] }

func (s *OutputCoverage) Add(v View, order int32) {
	c := cameraCoverage{viewport: v.Viewport, order: order}
	if v.DoClear {
		c.clear = v.ClearRegion()
		c.opaque = v.Clear.A >= 1
	}
	if v.TargetW > 0 && v.TargetH > 0 {
		c.viewport = RasterRect(c.viewport, v.TargetW, v.TargetH)
		if v.DoClear {
			// GL33 and WebGL2 outward-round scissor endpoints; viewport
			// origins and extents instead use integer truncation.
			r := c.clear
			x, y := float32(math.Floor(float64(r.X))), float32(math.Floor(float64(r.Y)))
			right, bottom := float32(math.Ceil(float64(r.X+r.W))), float32(math.Ceil(float64(r.Y+r.H)))
			c.clear = RasterRect(mathx.Rect{X: x, Y: y, W: right - x, H: bottom - y}, v.TargetW, v.TargetH)
		}
	}
	s.cameras = append(s.cameras, c)
	i := len(s.cameras) - 1
	for i > 0 && s.cameras[i-1].order > order {
		s.cameras[i] = s.cameras[i-1]
		i--
	}
	s.cameras[i] = c
}

// RasterRect returns the active integer-pixel rectangle, clipped to the attachment.
func RasterRect(r mathx.Rect, w, h int) mathx.Rect {
	x, y := int(r.X), int(r.Y)
	x1, y1 := min(w, x+int(r.W)), min(h, y+int(r.H))
	x, y = max(0, x), max(0, y)
	return mathx.Rect{X: float32(x), Y: float32(y), W: float32(max(0, x1-x)), H: float32(max(0, y1-y))}
}

func (s *OutputCoverage) Opaque() bool {
	for i := range s.cameras {
		if !coveredByClears(s.cameras[i].viewport, s.cameras) {
			return false
		}
	}
	return len(s.cameras) > 0
}

// The last clear touching a pixel decides its coverage. An opaque intersection
// is solved; inspect only the four remaining disjoint strips against earlier clears.
func coveredByClears(r mathx.Rect, history []cameraCoverage) bool {
	if r.W <= 0 || r.H <= 0 {
		return true
	}
	right, bottom := r.X+r.W, r.Y+r.H
	for i := len(history) - 1; i >= 0; i-- {
		c := history[i]
		x0, y0 := max(r.X, c.clear.X), max(r.Y, c.clear.Y)
		x1, y1 := min(right, c.clear.X+c.clear.W), min(bottom, c.clear.Y+c.clear.H)
		if x0 >= x1 || y0 >= y1 {
			continue
		}
		if !c.opaque {
			return false
		}
		if x0 == r.X && y0 == r.Y && x1 == right && y1 == bottom {
			return true
		}
		prior := history[:i]
		return coveredByClears(mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: y0 - r.Y}, prior) &&
			coveredByClears(mathx.Rect{X: r.X, Y: y1, W: r.W, H: bottom - y1}, prior) &&
			coveredByClears(mathx.Rect{X: r.X, Y: y0, W: x0 - r.X, H: y1 - y0}, prior) &&
			coveredByClears(mathx.Rect{X: x1, Y: y0, W: right - x1, H: y1 - y0}, prior)
	}
	return false
}
