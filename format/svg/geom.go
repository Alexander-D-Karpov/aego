package svg

import (
	"encoding/xml"
	"fmt"
	"math"
	"strings"
)

type pt struct{ x, y float64 }

// matrix is an affine transform a b c d e f, as SVG writes it.
type matrix [6]float64

func identity() matrix { return matrix{1, 0, 0, 1, 0, 0} }

func (m matrix) mul(n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[2]*n[1], m[1]*n[0] + m[3]*n[1],
		m[0]*n[2] + m[2]*n[3], m[1]*n[2] + m[3]*n[3],
		m[0]*n[4] + m[2]*n[5] + m[4], m[1]*n[4] + m[3]*n[5] + m[5],
	}
}

func (m matrix) apply(p pt) pt {
	return pt{m[0]*p.x + m[2]*p.y + m[4], m[1]*p.x + m[3]*p.y + m[5]}
}

// scale is how much the transform stretches lengths on average, for stroke widths.
func (m matrix) scale() float64 {
	return math.Sqrt(math.Abs(m[0]*m[3] - m[1]*m[2]))
}

func parseTransform(v string) (matrix, error) {
	m := identity()
	for rest := strings.TrimSpace(v); rest != ""; rest = strings.TrimLeft(rest, " ,\t\n") {
		open := strings.IndexByte(rest, '(')
		end := strings.IndexByte(rest, ')')
		if open < 0 || end < open {
			return m, fmt.Errorf("svg: bad transform %q", v)
		}
		name := strings.TrimSpace(rest[:open])
		f := numbers(rest[open+1 : end])
		rest = rest[end+1:]
		var t matrix
		switch {
		case name == "matrix" && len(f) == 6:
			t = matrix{f[0], f[1], f[2], f[3], f[4], f[5]}
		case name == "translate" && len(f) >= 1:
			t = matrix{1, 0, 0, 1, f[0], 0}
			if len(f) > 1 {
				t[5] = f[1]
			}
		case name == "scale" && len(f) >= 1:
			t = matrix{f[0], 0, 0, f[0], 0, 0}
			if len(f) > 1 {
				t[3] = f[1]
			}
		case name == "rotate" && len(f) >= 1:
			a := f[0] * math.Pi / 180
			s, c := math.Sincos(a)
			t = matrix{c, s, -s, c, 0, 0}
			if len(f) == 3 {
				t = matrix{1, 0, 0, 1, f[1], f[2]}.mul(t).mul(matrix{1, 0, 0, 1, -f[1], -f[2]})
			}
		case name == "skewX" && len(f) == 1:
			t = matrix{1, 0, math.Tan(f[0] * math.Pi / 180), 1, 0, 0}
		case name == "skewY" && len(f) == 1:
			t = matrix{1, math.Tan(f[0] * math.Pi / 180), 0, 1, 0, 0}
		default:
			return m, fmt.Errorf("svg: bad transform %q", v)
		}
		m = m.mul(t)
	}
	return m, nil
}

// subpath is a flattened run of points; closed ones join their ends.
type subpath struct {
	pts    []pt
	closed bool
}

// shape turns a basic shape into subpaths in its user space.
func shape(name string, as []xml.Attr) ([]subpath, error) {
	switch name {
	case "path":
		d, _ := attr(as, "d")
		return parsePath(d)
	case "rect":
		x, y, w, h := num(as, "x"), num(as, "y"), num(as, "width"), num(as, "height")
		rx, ry := num(as, "rx"), num(as, "ry")
		if rx == 0 {
			rx = ry
		}
		if ry == 0 {
			ry = rx
		}
		rx, ry = math.Min(rx, w/2), math.Min(ry, h/2)
		if w <= 0 || h <= 0 {
			return nil, nil
		}
		if rx <= 0 {
			return []subpath{{pts: []pt{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}}, closed: true}}, nil
		}
		var pts []pt
		corner := func(cx, cy, from float64) {
			for i := 0; i <= 8; i++ {
				a := from + float64(i)*math.Pi/16
				pts = append(pts, pt{cx + rx*math.Cos(a), cy + ry*math.Sin(a)})
			}
		}
		corner(x+w-rx, y+ry, -math.Pi/2)
		corner(x+w-rx, y+h-ry, 0)
		corner(x+rx, y+h-ry, math.Pi/2)
		corner(x+rx, y+ry, math.Pi)
		return []subpath{{pts: pts, closed: true}}, nil
	case "circle":
		r := num(as, "r")
		return []subpath{ellipse(num(as, "cx"), num(as, "cy"), r, r)}, nil
	case "ellipse":
		return []subpath{ellipse(num(as, "cx"), num(as, "cy"), num(as, "rx"), num(as, "ry"))}, nil
	case "line":
		return []subpath{{pts: []pt{{num(as, "x1"), num(as, "y1")}, {num(as, "x2"), num(as, "y2")}}}}, nil
	case "polyline", "polygon":
		v, _ := attr(as, "points")
		f := numbers(v)
		var pts []pt
		for i := 0; i+1 < len(f); i += 2 {
			pts = append(pts, pt{f[i], f[i+1]})
		}
		return []subpath{{pts: pts, closed: name == "polygon"}}, nil
	}
	return nil, nil
}

func ellipse(cx, cy, rx, ry float64) subpath {
	n := 48
	pts := make([]pt, n)
	for i := range pts {
		s, c := math.Sincos(2 * math.Pi * float64(i) / float64(n))
		pts[i] = pt{cx + rx*c, cy + ry*s}
	}
	return subpath{pts: pts, closed: true}
}

type scanner struct {
	s string
	i int
}

func (p *scanner) done() bool { return p.i >= len(p.s) }

func (p *scanner) space() {
	for p.i < len(p.s) && strings.IndexByte(" \t\r\n,", p.s[p.i]) >= 0 {
		p.i++
	}
}

func (p *scanner) number() (float64, bool) {
	p.space()
	start := p.i
	if p.i < len(p.s) && (p.s[p.i] == '-' || p.s[p.i] == '+') {
		p.i++
	}
	dot, exp, digits := false, false, false
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c >= '0' && c <= '9':
			digits = true
		case c == '.' && !dot && !exp:
			dot = true
		case (c == 'e' || c == 'E') && digits && !exp:
			exp = true
			if p.i+1 < len(p.s) && (p.s[p.i+1] == '-' || p.s[p.i+1] == '+') {
				p.i++
			}
		default:
			goto end
		}
		p.i++
	}
end:
	if !digits {
		p.i = start
		return 0, false
	}
	var f float64
	_, err := fmt.Sscan(p.s[start:p.i], &f)
	return f, err == nil
}

// flag reads an arc flag, which may be written without a separator.
func (p *scanner) flag() (bool, bool) {
	p.space()
	if p.i < len(p.s) && (p.s[p.i] == '0' || p.s[p.i] == '1') {
		p.i++
		return p.s[p.i-1] == '1', true
	}
	return false, false
}

// parsePath flattens path data into subpaths: M L H V C S Q T A Z, absolute and relative.
func parsePath(d string) ([]subpath, error) {
	var out []subpath
	var cur subpath
	var pos, start, ctrl pt
	var last byte
	flush := func() {
		if len(cur.pts) > 1 {
			out = append(out, cur)
		}
		cur = subpath{}
	}
	p := &scanner{s: d}
	cmd := byte(0)
	for {
		p.space()
		if p.done() {
			break
		}
		if c := p.s[p.i]; strings.IndexByte("MmLlHhVvCcSsQqTtAaZz", c) >= 0 {
			cmd = c
			p.i++
		} else if cmd == 0 {
			return nil, fmt.Errorf("svg: path data must start with a command: %q", d)
		}
		rel := cmd >= 'a'
		get := func(n int) ([]float64, error) {
			f := make([]float64, n)
			for i := range f {
				v, ok := p.number()
				if !ok {
					return nil, fmt.Errorf("svg: path %q: %c needs %d numbers", d, cmd, n)
				}
				f[i] = v
			}
			return f, nil
		}
		at := func(x, y float64) pt {
			if rel {
				return pt{pos.x + x, pos.y + y}
			}
			return pt{x, y}
		}
		switch cmd | 0x20 {
		case 'm':
			f, err := get(2)
			if err != nil {
				return nil, err
			}
			flush()
			pos = at(f[0], f[1])
			start = pos
			cur.pts = append(cur.pts, pos)
			if rel {
				cmd = 'l'
			} else {
				cmd = 'L'
			}
		case 'l':
			f, err := get(2)
			if err != nil {
				return nil, err
			}
			pos = at(f[0], f[1])
			cur.pts = append(cur.pts, pos)
		case 'h':
			f, err := get(1)
			if err != nil {
				return nil, err
			}
			if rel {
				pos.x += f[0]
			} else {
				pos.x = f[0]
			}
			cur.pts = append(cur.pts, pos)
		case 'v':
			f, err := get(1)
			if err != nil {
				return nil, err
			}
			if rel {
				pos.y += f[0]
			} else {
				pos.y = f[0]
			}
			cur.pts = append(cur.pts, pos)
		case 'c', 's':
			var c1 pt
			var f []float64
			var err error
			if cmd|0x20 == 'c' {
				if f, err = get(6); err != nil {
					return nil, err
				}
				c1 = at(f[0], f[1])
				f = f[2:]
			} else {
				if f, err = get(4); err != nil {
					return nil, err
				}
				c1 = pos
				if last|0x20 == 'c' || last|0x20 == 's' {
					c1 = pt{2*pos.x - ctrl.x, 2*pos.y - ctrl.y}
				}
			}
			c2, end := at(f[0], f[1]), at(f[2], f[3])
			cur.pts = append(cur.pts, cubic(pos, c1, c2, end)...)
			ctrl, pos = c2, end
		case 'q', 't':
			var c1, end pt
			if cmd|0x20 == 'q' {
				f, err := get(4)
				if err != nil {
					return nil, err
				}
				c1, end = at(f[0], f[1]), at(f[2], f[3])
			} else {
				f, err := get(2)
				if err != nil {
					return nil, err
				}
				c1 = pos
				if last|0x20 == 'q' || last|0x20 == 't' {
					c1 = pt{2*pos.x - ctrl.x, 2*pos.y - ctrl.y}
				}
				end = at(f[0], f[1])
			}
			cur.pts = append(cur.pts, cubic(pos, pt{pos.x + 2.0/3*(c1.x-pos.x), pos.y + 2.0/3*(c1.y-pos.y)},
				pt{end.x + 2.0/3*(c1.x-end.x), end.y + 2.0/3*(c1.y-end.y)}, end)...)
			ctrl, pos = c1, end
		case 'a':
			f, err := get(3)
			if err != nil {
				return nil, err
			}
			large, ok1 := p.flag()
			sweep, ok2 := p.flag()
			e, err := get(2)
			if err != nil || !ok1 || !ok2 {
				return nil, fmt.Errorf("svg: path %q: bad arc", d)
			}
			end := at(e[0], e[1])
			cur.pts = append(cur.pts, arc(pos, end, f[0], f[1], f[2], large, sweep)...)
			pos = end
		case 'z':
			cur.closed = true
			flush()
			pos = start
			cur.pts = append(cur.pts, pos)
		}
		last = cmd
	}
	flush()
	return out, nil
}

func cubic(p0, p1, p2, p3 pt) []pt {
	n := 16
	out := make([]pt, 0, n)
	for i := 1; i <= n; i++ {
		t := float64(i) / float64(n)
		u := 1 - t
		a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
		out = append(out, pt{a*p0.x + b*p1.x + c*p2.x + d*p3.x, a*p0.y + b*p1.y + c*p2.y + d*p3.y})
	}
	return out
}

// arc flattens an elliptical arc with the endpoint parametrisation of the SVG spec (F.6.5).
func arc(p0, p1 pt, rx, ry, rot float64, large, sweep bool) []pt {
	if p0 == p1 {
		return nil
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	if rx == 0 || ry == 0 {
		return []pt{p1}
	}
	sin, cos := math.Sincos(rot * math.Pi / 180)
	dx, dy := (p0.x-p1.x)/2, (p0.y-p1.y)/2
	x1 := cos*dx + sin*dy
	y1 := -sin*dx + cos*dy
	if l := x1*x1/(rx*rx) + y1*y1/(ry*ry); l > 1 {
		rx, ry = rx*math.Sqrt(l), ry*math.Sqrt(l)
	}
	num := rx*rx*ry*ry - rx*rx*y1*y1 - ry*ry*x1*x1
	den := rx*rx*y1*y1 + ry*ry*x1*x1
	k := math.Sqrt(math.Max(0, num/den))
	if large == sweep {
		k = -k
	}
	cx1, cy1 := k*rx*y1/ry, -k*ry*x1/rx
	cx := cos*cx1 - sin*cy1 + (p0.x+p1.x)/2
	cy := sin*cx1 + cos*cy1 + (p0.y+p1.y)/2
	angle := func(ux, uy, vx, vy float64) float64 {
		return math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
	}
	t0 := angle(1, 0, (x1-cx1)/rx, (y1-cy1)/ry)
	dt := angle((x1-cx1)/rx, (y1-cy1)/ry, (-x1-cx1)/rx, (-y1-cy1)/ry)
	if !sweep && dt > 0 {
		dt -= 2 * math.Pi
	} else if sweep && dt < 0 {
		dt += 2 * math.Pi
	}
	n := int(math.Ceil(math.Abs(dt) / (math.Pi / 16)))
	out := make([]pt, 0, n)
	for i := 1; i <= n; i++ {
		t := t0 + dt*float64(i)/float64(n)
		s, c := math.Sincos(t)
		x, y := rx*c, ry*s
		out = append(out, pt{cos*x - sin*y + cx, sin*x + cos*y + cy})
	}
	out[len(out)-1] = p1
	return out
}
