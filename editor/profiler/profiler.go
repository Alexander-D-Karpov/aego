package profiler

import (
	"strconv"
	"time"

	"aego/core/mathx"
	"aego/core/stats"
	"aego/editor/context"
	"aego/editor/play"
	"aego/editor/workspace"
	"aego/render"
	"aego/ui/core"
	"aego/ui/widgets"
)

const history = 120

const (
	mFrame = iota
	mUpdate
	mRender
	mDraws
	mSprites
	mGarbage
	mGameUpdate
	mGameRender
	mGameSprites
	mGameNodes
	mScriptInstr
	mScriptHost
	mScriptHeap
	mCoroutines
	mEmits
	metricCount
)

type bucket struct {
	avg     [metricCount]float32
	peak    [metricCount]float32
	frames  int
	playing bool
}

type Panel struct {
	ring       [history]bucket
	head       int
	n          int
	acc        bucket
	start      float64
	interval   int
	paused     bool
	scroll     float32
	content    float32
	scripts    context.ScriptStats
	hasScripts bool
}

func intervals() [4]float64 {
	return [4]float64{0.1, 0.25, 0.5, 1}
}

func New() *Panel {
	return &Panel{interval: 1}
}

func (p *Panel) ID() string {
	return workspace.Profiler
}

func (p *Panel) Title() string {
	return "Profiler"
}

func (p *Panel) seconds() float64 {
	return intervals()[p.interval]
}

func (p *Panel) Record(now float64, f *stats.Frame, r render.Stats, g play.Stats, sc context.ScriptStats, hasSc, playing bool) {
	if p.paused || f == nil {
		return
	}
	p.scripts, p.hasScripts = sc, hasSc && playing
	var s [metricCount]float32
	s[mFrame] = float32(f.FrameTime * 1000)
	s[mUpdate] = float32(f.UpdateTime * 1000)
	s[mRender] = float32(f.RenderTime * 1000)
	s[mDraws] = float32(r.DrawCalls)
	s[mSprites] = float32(r.Visible)
	s[mGarbage] = float32(f.AllocBytes) / 1024
	if playing {
		s[mGameUpdate] = float32(g.Update * 1000)
		s[mGameRender] = float32(g.Render * 1000)
		s[mGameSprites] = float32(g.Sprites)
		s[mGameNodes] = float32(g.Nodes)
		if hasSc {
			s[mScriptInstr] = float32(sc.Instructions)
			s[mScriptHost] = float32(sc.HostCalls)
			s[mScriptHeap] = float32(sc.HeapBytes) / 1024
			s[mCoroutines] = float32(sc.Coroutines.Active)
			s[mEmits] = float32(sc.Signals.EmitsPerFrame)
		}
	}
	if p.acc.frames == 0 {
		p.start = now
	}
	for i := range s {
		p.acc.avg[i] += s[i]
		p.acc.peak[i] = max(p.acc.peak[i], s[i])
	}
	p.acc.frames++
	p.acc.playing = p.acc.playing || playing
	if now-p.start < p.seconds() {
		return
	}
	for i := range p.acc.avg {
		p.acc.avg[i] /= float32(p.acc.frames)
	}
	p.ring[p.head] = p.acc
	p.head = (p.head + 1) % history
	if p.n < history {
		p.n++
	}
	p.acc = bucket{}
}

func ms(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds()*1000, 'f', 2, 64)
}

func (p *Panel) scriptTable(ui *core.Context) {
	th := &ui.Theme
	s := &p.scripts
	widgets.LabelDim(ui, "instances "+strconv.Itoa(s.Instances)+
		"   update "+strconv.Itoa(s.Update)+
		"   fixed "+strconv.Itoa(s.Fixed)+
		"   faults "+strconv.Itoa(s.Faults)+
		"   budget aborts "+strconv.FormatUint(s.BudgetAborts, 10)+
		"   heap objects "+strconv.Itoa(s.HeapObjects)+
		"   GC "+strconv.FormatUint(s.GCRuns, 10)+" / "+ms(s.GCTime)+" ms")
	co := &s.Coroutines
	widgets.LabelDim(ui, "coroutines active "+strconv.Itoa(co.Active)+
		"   sleeping "+strconv.Itoa(co.Sleeping)+
		"   resumes/frame "+strconv.FormatUint(co.ResumesPerFrame, 10)+
		"   canceled "+strconv.FormatUint(co.Canceled, 10))
	sg := &s.Signals
	widgets.LabelDim(ui, "signal connections "+strconv.Itoa(sg.Connections)+
		"   emits/frame "+strconv.FormatUint(sg.EmitsPerFrame, 10)+
		"   handlers/frame "+strconv.FormatUint(sg.HandlersPerFrame, 10))
	cols := [5]float32{0, 0.5, 0.64, 0.78, 0.9}
	names := [5]string{"script", "instances", "calls", "ms", "faults"}
	row := func(vals [5]string, col mathx.Color) {
		r := ui.Cursor().Next(0)
		ui.PushClip(r)
		for i, v := range vals {
			ui.Text(v, mathx.Vec2{X: r.X + r.W*cols[i], Y: ui.TextY(r)}, col)
		}
		ui.PopClip()
	}
	row(names, th.TextDim)
	for _, t := range s.Types {
		row([5]string{t.Name, strconv.Itoa(t.Instances), strconv.FormatUint(t.Calls, 10), ms(t.Time), strconv.Itoa(t.Faults)}, th.Text)
	}
}

func (p *Panel) at(i int) *bucket {
	return &p.ring[(p.head-p.n+i+history)%history]
}

func (p *Panel) series(m int) (last, avg, peak float32) {
	if p.n == 0 {
		return 0, 0, 0
	}
	var sum float32
	for i := 0; i < p.n; i++ {
		b := p.at(i)
		sum += b.avg[m]
		peak = max(peak, b.peak[m])
	}
	return p.at(p.n - 1).avg[m], sum / float32(p.n), peak
}

func fmtf(v float32, prec int) string {
	return strconv.FormatFloat(float64(v), 'f', prec, 32)
}

func (p *Panel) anyPlaying() bool {
	for i := 0; i < p.n; i++ {
		if p.at(i).playing {
			return true
		}
	}
	return false
}

func (p *Panel) graph(ui *core.Context, title, unit string, prec int, budget float32, col mathx.Color, m int) {
	th := &ui.Theme
	last, avg, peak := p.series(m)

	hr := ui.Cursor().Next(th.RowHeight)
	ui.Rect(mathx.Rect{X: hr.X, Y: hr.Y + (hr.H-9)/2, W: 3, H: 9}, col)
	tx := hr.X + 10
	ui.Text(title, mathx.Vec2{X: tx, Y: ui.TextY(hr)}, th.Text)
	tx += ui.TextSize(title).X + 12
	valCol := th.Text
	if budget > 0 && last > budget {
		valCol = th.Warn
	}
	ui.Text(fmtf(last, prec)+unit, mathx.Vec2{X: tx, Y: ui.TextY(hr)}, valCol)
	ui.TextIn(hr, "avg "+fmtf(avg, prec)+"    max "+fmtf(peak, prec), th.TextDim, core.AlignRight)

	r := ui.Cursor().Next(th.RowHeight * 2.6)
	ui.Rect(r, th.PanelAlt)
	top := max(peak, budget, 1e-6) * 1.15
	bw := r.W / history
	if p.n > 0 {
		base := r.Y + r.H
		off := float32(history-p.n) * bw
		fill, env, line := col, col, col
		fill.A, env.A = 0.22, 0.09
		yOf := func(v float32) float32 { return r.H * v / top }
		sample := func(u float32) (float32, float32) {
			k := int(u)
			if k >= p.n-1 {
				b := p.at(p.n - 1)
				return b.avg[m], b.peak[m]
			}
			f := u - float32(k)
			a, b := p.at(k), p.at(k+1)
			return a.avg[m] + (b.avg[m]-a.avg[m])*f, a.peak[m] + (b.peak[m]-a.peak[m])*f
		}
		prev := float32(-1)
		for px := int(off); px < int(r.W); px++ {
			x := r.X + float32(px)
			av, pk := sample((float32(px) - off) / bw)
			ah, ph := yOf(av), yOf(pk)
			ui.Rect(mathx.Rect{X: x, Y: base - ph, W: 1, H: ph}, env)
			ui.Rect(mathx.Rect{X: x, Y: base - ah, W: 1, H: ah}, fill)
			lo, hi := ah, ah
			if prev >= 0 {
				lo, hi = min(ah, prev), max(ah, prev)
			}
			ui.Rect(mathx.Rect{X: x, Y: base - hi - 1, W: 1, H: hi - lo + 2}, line)
			prev = ah
		}
	}
	if budget > 0 {
		warn := th.Warn
		warn.A = 0.7
		ui.Rect(mathx.Rect{X: r.X, Y: r.Y + r.H - r.H*budget/top, W: r.W, H: 1}, warn)
	}
	ui.TextIn(mathx.Rect{X: r.X, Y: r.Y + 2, W: r.W - 5, H: ui.LineHeight()}, fmtf(top, prec)+unit, th.TextDisabled, core.AlignRight)
	if !ui.Hit(r) || p.n == 0 {
		return
	}
	i := int((ui.Mouse().X-r.X)/bw) - (history - p.n)
	if i < 0 || i >= p.n {
		return
	}
	b := p.at(i)
	x := r.X + float32(history-p.n+i)*bw
	ui.Rect(mathx.Rect{X: x, Y: r.Y, W: max(1, bw-1), H: r.H}, mathx.Color{R: 1, G: 1, B: 1, A: 0.08})
	ago := float64(p.n-1-i) * p.seconds()
	label := fmtf(b.avg[m], prec) + " / " + fmtf(b.peak[m], prec) + unit + "   " + strconv.FormatFloat(ago, 'f', 1, 64) + " s ago"
	size := ui.TextSize(label)
	box := mathx.Rect{X: r.X + r.W - size.X - 10, Y: r.Y + 2, W: size.X + 8, H: size.Y + 2}
	ui.Rect(box, th.Background)
	ui.Text(label, mathx.Vec2{X: box.X + 4, Y: box.Y + 1}, th.Text)
}

func (p *Panel) Draw(ctx *context.Editor, r mathx.Rect) {
	ui := ctx.UI
	th := &ui.Theme
	if ui.MouseOver(ui.ID("scroll"), r) {
		if w := ui.WheelY(); w != 0 {
			p.scroll -= w * th.RowHeight * 3
		}
	}
	p.scroll = mathx.Clamp(p.scroll, 0, max(0, p.content-r.H+th.Padding*2))
	cur := ui.Cursor()
	top := cur.Y
	cur.Y -= p.scroll

	bar := ui.Cursor().Next(0)
	label := "Pause"
	if p.paused {
		label = "Resume"
	}
	if widgets.ButtonRect(ui, ui.ID("pause"), mathx.Rect{X: bar.X, Y: bar.Y, W: 80, H: bar.H}, label) {
		p.paused = !p.paused
	}
	names := []string{"0.1 s", "0.25 s", "0.5 s", "1 s"}
	if widgets.Combo(ui, ui.ID("interval"), mathx.Rect{X: bar.X + 86, Y: bar.Y, W: 80, H: bar.H}, &p.interval, names, false) {
		p.n, p.head, p.acc = 0, 0, bucket{}
	}
	if p.n > 0 {
		fps := float64(p.at(p.n-1).frames) / p.seconds()
		span := strconv.FormatFloat(p.seconds()*history, 'f', 0, 64)
		ui.Text("fps "+strconv.Itoa(int(fps+0.5))+"   window "+span+" s", mathx.Vec2{X: bar.X + 176, Y: ui.TextY(bar)}, th.Text)
	}

	widgets.Label(ui, "Editor")
	p.graph(ui, "frame", " ms", 2, 1000.0/60, th.Accent, mFrame)
	p.graph(ui, "update", " ms", 2, 0, th.AxisY, mUpdate)
	p.graph(ui, "render", " ms", 2, 0, th.AxisX, mRender)
	p.graph(ui, "draw calls", "", 0, 0, th.Selection, mDraws)
	p.graph(ui, "sprites", "", 0, 0, th.Selection, mSprites)
	p.graph(ui, "garbage", " KB", 1, 64, th.Warn, mGarbage)
	if p.anyPlaying() {
		widgets.Separator(ui)
		widgets.Label(ui, "Game")
		p.graph(ui, "game update", " ms", 2, 0, th.AxisY, mGameUpdate)
		p.graph(ui, "game render", " ms", 2, 0, th.AxisX, mGameRender)
		p.graph(ui, "game sprites", "", 0, 0, th.Selection, mGameSprites)
		p.graph(ui, "game nodes", "", 0, 0, th.Selection, mGameNodes)
		if p.hasScripts {
			p.graph(ui, "script instructions", "", 0, 0, th.Accent, mScriptInstr)
			p.graph(ui, "script host calls", "", 0, 0, th.AxisY, mScriptHost)
			p.graph(ui, "script heap", " KB", 1, 0, th.Warn, mScriptHeap)
			p.graph(ui, "coroutines", "", 0, 0, th.AxisZ, mCoroutines)
			p.graph(ui, "signal emits", "", 0, 0, th.Selection, mEmits)
			p.scriptTable(ui)
		}
	} else {
		widgets.LabelDim(ui, "Press Play to see game timings")
	}
	p.content = cur.Y + p.scroll - top
}
