package shapes2d

import (
	"aego/core/mathx"
	"errors"
	"math"
)

func finiteContour(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }

// ValidateContour checks a simple closed polygon or open chain at a geometry
// revision boundary. It rejects holes, crossings, nonfinite points and zero
// length edges, but accepts concave polygons and either winding.
func ValidateContour(points []mathx.Vec2, closed bool) error {
	if len(points) > MaxContourPoints {
		return ErrBudget
	}
	if len(points) < 2 || closed && len(points) < 3 {
		return errors.New("a chain needs two points and a closed contour needs three")
	}
	for _, p := range points {
		if !finiteContour(p.X) || !finiteContour(p.Y) {
			return errors.New("shape coordinates must be finite")
		}
	}
	edges := len(points) - 1
	if closed {
		edges++
	}
	for i := 0; i < edges; i++ {
		a, b := points[i], points[(i+1)%len(points)]
		if a == b {
			return errors.New("shape has a zero-length edge")
		}
		for j := i + 2; j < edges; j++ {
			if closed && i == 0 && j == edges-1 {
				continue
			}
			if shapeSegmentsMeet(a, b, points[j], points[(j+1)%len(points)]) {
				return errors.New("shape contour intersects itself")
			}
		}
	}
	for i := 0; i < len(points); i++ {
		if !closed && (i == 0 || i == len(points)-1) {
			continue
		}
		a, b, c := points[(i+len(points)-1)%len(points)], points[i], points[(i+1)%len(points)]
		if shapeOrient(a, b, c) == 0 &&
			(float64(a.X)-float64(b.X))*(float64(c.X)-float64(b.X))+(float64(a.Y)-float64(b.Y))*(float64(c.Y)-float64(b.Y)) > 0 {
			return errors.New("adjacent shape edges overlap")
		}
	}
	if closed {
		// Relative coordinates and float64 accumulation remain reliable for
		// small shapes far from the local origin.
		o := points[0]
		area := 0.0
		for i := 1; i+1 < len(points); i++ {
			area += shapeOrient(o, points[i], points[i+1])
		}
		if area == 0 {
			return errors.New("closed shape has zero area")
		}
	}
	return nil
}

func shapeOrient(a, b, c mathx.Vec2) float64 {
	return (float64(b.X)-float64(a.X))*(float64(c.Y)-float64(a.Y)) - (float64(b.Y)-float64(a.Y))*(float64(c.X)-float64(a.X))
}

func shapeSegmentsMeet(a, b, c, d mathx.Vec2) bool {
	abc, abd, cda, cdb := shapeOrient(a, b, c), shapeOrient(a, b, d), shapeOrient(c, d, a), shapeOrient(c, d, b)
	between := func(a, b, p mathx.Vec2) bool {
		return p.X >= min(a.X, b.X) && p.X <= max(a.X, b.X) && p.Y >= min(a.Y, b.Y) && p.Y <= max(a.Y, b.Y)
	}
	if abc == 0 && between(a, b, c) || abd == 0 && between(a, b, d) || cda == 0 && between(c, d, a) || cdb == 0 && between(c, d, b) {
		return true
	}
	return (abc < 0 && abd > 0 || abc > 0 && abd < 0) && (cda < 0 && cdb > 0 || cda > 0 && cdb < 0)
}
