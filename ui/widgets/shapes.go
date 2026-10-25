package widgets

import (
	"aego/core/mathx"
	"aego/ui/core"
)

func Chevron(c *core.Context, r mathx.Rect, open bool, col mathx.Color) {
	id := "ui.chevron_right"
	if open {
		id = "ui.chevron_down"
	}
	if c.Icons.Has(id) {
		is := min(float32(16), max(r.W, r.H))
		IconAt(c, mathx.Rect{X: float32(int(r.X + (r.W-is)/2)), Y: float32(int(r.Y + (r.H-is)/2)), W: is, H: is}, id, col)
		return
	}
	s := float32(int(min(r.W, r.H) * 0.42))
	if s < 6 {
		s = 6
	}
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	l := c.List()
	half := float32(int(s/2 + 0.5))
	if open {
		for i := float32(0); i < half; i++ {
			w := s - 2*i
			l.Rect(mathx.Rect{X: float32(int(cx - w/2)), Y: float32(int(cy - half/2 + i)), W: w, H: 1}, col)
		}
		return
	}
	for i := float32(0); i < half; i++ {
		h := s - 2*i
		l.Rect(mathx.Rect{X: float32(int(cx - half/2 + i)), Y: float32(int(cy - h/2)), W: 1, H: h}, col)
	}
}

func CheckMark(c *core.Context, r mathx.Rect, col mathx.Color) {
	l := c.List()
	t := max(float32(1.5), r.H/7)
	seg := func(a, b mathx.Vec2) {
		d := b.Sub(a)
		n := int(d.Len()/(t*0.5)) + 1
		for i := 0; i <= n; i++ {
			p := a.Add(d.Scale(float32(i) / float32(n)))
			l.Rect(mathx.Rect{X: p.X - t/2, Y: p.Y - t/2, W: t, H: t}, col)
		}
	}
	a := mathx.Vec2{X: r.X + r.W*0.22, Y: r.Y + r.H*0.52}
	m := mathx.Vec2{X: r.X + r.W*0.42, Y: r.Y + r.H*0.72}
	e := mathx.Vec2{X: r.X + r.W*0.78, Y: r.Y + r.H*0.30}
	seg(a, m)
	seg(m, e)
}
