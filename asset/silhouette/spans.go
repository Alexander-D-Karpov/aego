// Package silhouette converts alpha coverage to bounded, interior-disjoint rectangles.
// Coordinates are on the original pixel canvas, not the atlas page.
package silhouette

import "errors"

const MaxParts = 256

type Rect struct{ X, Y, W, H int }
type Shape struct {
	W, H  int
	Parts []Rect
}

// Trace merges identical horizontal runs vertically. Unlike tracing one outer hull,
// it preserves holes, disconnected islands and the runner's gap between its legs.
func Trace(pixels []byte, pw, ph int, cut Rect, canvasW, canvasH, dx, dy int) (Shape, error) {
	if pw <= 0 || ph <= 0 || pw > len(pixels)/4/ph || cut.W <= 0 || cut.H <= 0 || cut.W > 512 || cut.H > 512 || cut.X < 0 || cut.Y < 0 || cut.X > pw-cut.W || cut.Y > ph-cut.H || dx < 0 || dy < 0 || dx > canvasW-cut.W || dy > canvasH-cut.H {
		return Shape{}, errors.New("invalid or oversize sprite shadow frame (maximum 512x512 stored pixels)")
	}
	s := Shape{W: canvasW, H: canvasH}
	last, next := map[[2]int]int{}, map[[2]int]int{}
	for y := 0; y < cut.H; y++ {
		clear(next)
		row := (cut.Y+y)*pw + cut.X
		for x := 0; x < cut.W; {
			if pixels[(row+x)*4+3] < 128 {
				x++
				continue
			}
			x0 := x
			for x < cut.W && pixels[(row+x)*4+3] >= 128 {
				x++
			}
			key := [2]int{x0, x}
			idx, ok := last[key]
			if ok {
				s.Parts[idx].H++
			} else {
				if len(s.Parts) >= MaxParts {
					return Shape{}, errors.New("sprite silhouette exceeds shadow geometry budget")
				}
				idx = len(s.Parts)
				s.Parts = append(s.Parts, Rect{dx + x0, dy + y, x - x0, 1})
			}
			next[key] = idx
		}
		last, next = next, last
	}
	return s, nil
}
