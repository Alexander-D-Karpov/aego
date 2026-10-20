package editor

import (
	"math"
	"strconv"

	"aego/core/mathx"
	"aego/modules/physics/world"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	previewH  = 190
	pxPerM    = 22
	fixedStep = 1.0 / 60
)

func dot(ui *core.Context, p mathx.Vec2, s float32, c mathx.Color) {
	ui.Rect(mathx.Rect{X: float32(int(p.X - s/2)), Y: float32(int(p.Y - s/2)), W: s, H: s}, c)
}

func line(ui *core.Context, a, b mathx.Vec2, w float32, c mathx.Color) {
	d := b.Sub(a)
	n := int(max(mathx.Abs(d.X), mathx.Abs(d.Y)))
	if n == 0 {
		dot(ui, a, w, c)
		return
	}
	step := d.Scale(1 / float32(n))
	for i := 0; i <= n; i++ {
		dot(ui, a.Add(step.Scale(float32(i))), w, c)
	}
}

func arrow(ui *core.Context, a, b mathx.Vec2, w float32, c mathx.Color) {
	line(ui, a, b, w, c)
	d := b.Sub(a)
	l := d.Len()
	if l < 1 {
		return
	}
	u := d.Scale(1 / l)
	n := mathx.Vec2{X: -u.Y, Y: u.X}
	h := min(float32(8), l*0.4)
	line(ui, b, b.Sub(u.Scale(h)).Add(n.Scale(h*0.6)), w, c)
	line(ui, b, b.Sub(u.Scale(h)).Sub(n.Scale(h*0.6)), w, c)
}

func fmtF(v float32, prec int) string {
	return strconv.FormatFloat(float64(v), 'f', prec, 32)
}

func (p *plugin) preview(ui *core.Context, cfg world.Config) {
	th := &ui.Theme
	r := ui.Cursor().Next(previewH)
	ui.RoundRect(r, th.Radius.Card, th.Background)
	ui.PushClip(r)
	defer ui.PopClip()
	pad := float32(10)
	split := r.W * 0.58
	scene := mathx.Rect{X: r.X + pad, Y: r.Y + pad, W: split - 2*pad, H: r.H - 2*pad}
	spring := mathx.Rect{X: r.X + split + pad, Y: r.Y + pad, W: r.W - split - 2*pad, H: r.H - 2*pad}
	ui.Rect(mathx.Rect{X: r.X + split, Y: r.Y + 8, W: 1, H: r.H - 16}, th.Separator)
	p.trajectory(ui, scene, cfg)
	p.contact(ui, spring, cfg)
}

func (p *plugin) trajectory(ui *core.Context, r mathx.Rect, cfg world.Config) {
	th := &ui.Theme
	g := cfg.Gravity
	hot := p.focus == "gravity"
	ground := r.Y + r.H - 36
	ui.Rect(mathx.Rect{X: r.X, Y: ground, W: r.W, H: 1}, th.Border)
	start := mathx.Vec2{X: r.X + 20, Y: ground - 2}
	vel := mathx.Vec2{X: 5, Y: -7}
	pos := mathx.Vec2{}
	col := th.TextDim
	if hot {
		col = th.Accent
	}
	steps := 0
	for i := 0; i < 240; i++ {
		vel = vel.Add(g.Scale(fixedStep))
		pos = pos.Add(vel.Scale(fixedStep))
		sp := start.Add(pos.Scale(pxPerM))
		if sp.Y > ground-2 || sp.X > r.X+r.W || sp.Y < r.Y-200 {
			break
		}
		if sp.Y >= r.Y {
			dot(ui, sp, 2, col)
		}
		steps++
	}
	ui.RoundRect(mathx.Rect{X: start.X - 6, Y: start.Y - 12, W: 12, H: 12}, 6, th.Accent)
	arrow(ui, mathx.Vec2{X: start.X, Y: start.Y - 6}, mathx.Vec2{X: start.X + 5*4, Y: start.Y - 6 - 7*4}, 1, th.TextDim)
	gl := g.Len()
	c := mathx.Vec2{X: r.X + r.W - 46, Y: r.Y + 34}
	if gl > 0 {
		k := min(float32(40), 3*gl)
		gc := th.AxisY
		if hot {
			gc = th.Accent
		}
		arrow(ui, c, c.Add(g.Scale(k/gl)), 2, gc)
	}
	lc := th.TextDim
	if hot {
		lc = th.Accent
	}
	ui.Text("Gravity "+fmtF(gl, 2)+" m/s²", mathx.Vec2{X: r.X, Y: r.Y}, lc)
	info := "airtime " + fmtF(float32(steps)*fixedStep, 2) + " s · " + strconv.Itoa(steps) + " fixed steps · " + strconv.Itoa(cfg.Substeps) + " substeps"
	sc := th.TextDim
	if p.focus == "solver" {
		sc = th.Accent
	}
	ui.Text(info, mathx.Vec2{X: r.X, Y: ground + 3}, sc)
	sleep := "sleeps below " + fmtF(cfg.SleepLinear, 3) + " m/s for " + fmtF(cfg.SleepTime, 2) + " s"
	slc := th.TextDisabled
	if p.focus == "sleep" {
		slc = th.Accent
	}
	ui.Text(sleep, mathx.Vec2{X: r.X, Y: ground + 3 + ui.LineHeight()}, slc)
}

func (p *plugin) contact(ui *core.Context, r mathx.Rect, cfg world.Config) {
	th := &ui.Theme
	hot := p.focus == "contact" || p.focus == "bounce"
	title := "Contact " + fmtF(cfg.ContactHertz, 0) + " Hz · damping " + fmtF(cfg.ContactDampingRatio, 1)
	ui.Text(title, mathx.Vec2{X: r.X, Y: r.Y}, th.TextDim)
	plot := mathx.Rect{X: r.X, Y: r.Y + 22, W: r.W, H: r.H - 44}
	mid := plot.Y + plot.H*0.5
	ui.Rect(mathx.Rect{X: plot.X, Y: mid, W: plot.W, H: 1}, th.Border)
	ui.Rect(mathx.Rect{X: plot.X, Y: plot.Y, W: 1, H: plot.H}, th.Border)
	w := 2 * math.Pi * float64(max(cfg.ContactHertz, 0.1))
	z := float64(max(cfg.ContactDampingRatio, 0))
	window := 4 / float64(max(cfg.ContactHertz, 0.5))
	if z > 1 {
		window = max(window, 5/(w*(z-math.Sqrt(z*z-1))))
	}
	col := th.Text
	if hot {
		col = th.Accent
	}
	prev := mathx.Vec2{}
	settle := -1.0
	for i := 0; i <= int(plot.W); i++ {
		t := window * float64(i) / float64(plot.W)
		x := response(t, w, z)
		if settle < 0 && math.Abs(x) < 0.02 {
			settle = t
		}
		pt := mathx.Vec2{X: plot.X + float32(i), Y: mid - float32(x)*plot.H*0.45}
		if i > 0 {
			line(ui, prev, pt, 1, col)
		}
		prev = pt
	}
	foot := "penetration fades"
	if settle >= 0 {
		foot += " in " + fmtF(float32(settle*1000), 0) + " ms"
	}
	foot += " · push " + fmtF(cfg.ContactPushVelocity, 1) + " m/s"
	ui.Text(widgets.Elide(ui, foot, r.W), mathx.Vec2{X: r.X, Y: plot.Y + plot.H + 4}, th.TextDim)
}

func response(t, w, z float64) float64 {
	switch {
	case z < 1:
		wd := w * math.Sqrt(1-z*z)
		return math.Exp(-z*w*t) * (math.Cos(wd*t) + z/math.Sqrt(1-z*z)*math.Sin(wd*t))
	case z == 1:
		return math.Exp(-w*t) * (1 + w*t)
	}
	s := math.Sqrt(z*z - 1)
	r1, r2 := -w*(z-s), -w*(z+s)
	return (r2*math.Exp(r1*t) - r1*math.Exp(r2*t)) / (r2 - r1)
}
