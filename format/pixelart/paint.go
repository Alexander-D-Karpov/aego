package pixelart

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"aego/format/svg"
)

const (
	MaxPixels = 1 << 20
	MaxOps    = 4096
	maxWork   = 16 * MaxPixels
)

// Paint starts from a copy of source, or a transparent width×height canvas.
// Coordinates are source pixels, with the origin at the top left.
func Paint(source image.Image, width, height int, palette Palette, ops []Op) (*image.NRGBA, error) {
	if source != nil {
		b := source.Bounds()
		if width == 0 {
			width = b.Dx()
		}
		if height == 0 {
			height = b.Dy()
		}
		if width != b.Dx() || height != b.Dy() {
			return nil, errors.New("source and canvas dimensions differ")
		}
	}
	if width < 1 || height < 1 || width > 4096 || height > 4096 || width*height > MaxPixels {
		return nil, errors.New("invalid canvas size: maximum 4096 per side and 1048576 pixels")
	}
	if len(ops) > MaxOps {
		return nil, errors.New("too many drawing operations")
	}
	im := image.NewNRGBA(image.Rect(0, 0, width, height))
	if source != nil {
		draw.Draw(im, im.Bounds(), source, source.Bounds().Min, draw.Src)
	}
	left := maxWork
	for i, op := range ops {
		if err := op.apply(im, palette, &left); err != nil {
			return nil, fmt.Errorf("operation %d: %w", i+1, err)
		}
	}
	return im, nil
}

type Op struct {
	Op    string `json:"op"`
	X     int    `json:"x,omitempty"`
	Y     int    `json:"y,omitempty"`
	X2    int    `json:"x2,omitempty"`
	Y2    int    `json:"y2,omitempty"`
	W     int    `json:"w,omitempty"`
	H     int    `json:"h,omitempty"`
	ToX   int    `json:"toX,omitempty"`
	ToY   int    `json:"toY,omitempty"`
	FlipX bool   `json:"flipX,omitempty"`
	FlipY bool   `json:"flipY,omitempty"`
	Color string `json:"color,omitempty"`
}

func (op Op) apply(im *image.NRGBA, pal Palette, left *int) error {
	b := im.Bounds()
	p := image.Pt(op.X, op.Y)
	if !p.In(b) {
		return errors.New("origin outside the canvas")
	}
	var c color.NRGBA
	if op.Op != "copy" {
		k := []rune(op.Color)
		if len(k) == 1 {
			var ok bool
			c, ok = pal[k[0]]
			if !ok && k[0] != '.' {
				return fmt.Errorf("unknown palette key %q", op.Color)
			}
		} else if op.Color != "transparent" {
			if op.Color == "" {
				return errors.New("color is required")
			}
			var err error
			c, err = svg.ParseColor(op.Color)
			if err != nil {
				return err
			}
		}
	}
	cost := 1
	switch op.Op {
	case "pixel":
	case "line":
		if !image.Pt(op.X2, op.Y2).In(b) {
			return errors.New("line endpoint outside the canvas")
		}
		cost = max(absDelta(op.X2-op.X), absDelta(op.Y2-op.Y)) + 1
	case "rect", "copy":
		if op.W <= 0 || op.H <= 0 || op.W > b.Dx()-op.X || op.H > b.Dy()-op.Y {
			return errors.New("rectangle exceeds canvas")
		}
		if op.Op == "copy" && (op.ToX < 0 || op.ToY < 0 || op.ToX > b.Dx()-op.W || op.ToY > b.Dy()-op.H) {
			return errors.New("copy destination exceeds canvas")
		}
		cost = op.W * op.H
	case "fill":
		cost = b.Dx() * b.Dy()
	default:
		return fmt.Errorf("unknown drawing operation %q", op.Op)
	}
	if cost > *left {
		return errors.New("drawing batch exceeds its pixel budget")
	}
	*left -= cost
	switch op.Op {
	case "pixel":
		im.SetNRGBA(op.X, op.Y, c)
	case "rect":
		for y := op.Y; y < op.Y+op.H; y++ {
			for x := op.X; x < op.X+op.W; x++ {
				im.SetNRGBA(x, y, c)
			}
		}
	case "line":
		x, y := op.X, op.Y
		dx, dy := absDelta(op.X2-x), -absDelta(op.Y2-y)
		sx, sy := 1, 1
		if x > op.X2 {
			sx = -1
		}
		if y > op.Y2 {
			sy = -1
		}
		e := dx + dy
		for {
			im.SetNRGBA(x, y, c)
			if x == op.X2 && y == op.Y2 {
				break
			}
			twice := 2 * e
			if twice >= dy {
				e += dy
				x += sx
			}
			if twice <= dx {
				e += dx
				y += sy
			}
		}
	case "copy":
		tmp := image.NewNRGBA(image.Rect(0, 0, op.W, op.H))
		draw.Draw(tmp, tmp.Bounds(), im, p, draw.Src)
		for y := 0; y < op.H; y++ {
			for x := 0; x < op.W; x++ {
				sx, sy := x, y
				if op.FlipX {
					sx = op.W - 1 - x
				}
				if op.FlipY {
					sy = op.H - 1 - y
				}
				im.SetNRGBA(op.ToX+x, op.ToY+y, tmp.NRGBAAt(sx, sy))
			}
		}
	case "fill":
		old := im.NRGBAAt(op.X, op.Y)
		if old == c {
			return nil
		}
		q := []image.Point{p}
		im.SetNRGBA(p.X, p.Y, c)
		for len(q) > 0 {
			last := len(q) - 1
			v := q[last]
			q = q[:last]
			for _, d := range [...]image.Point{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}} {
				n := v.Add(d)
				if n.In(b) && im.NRGBAAt(n.X, n.Y) == old {
					im.SetNRGBA(n.X, n.Y, c)
					q = append(q, n)
				}
			}
		}
	}
	return nil
}

func absDelta(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
