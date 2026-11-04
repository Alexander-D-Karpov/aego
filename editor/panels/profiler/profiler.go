package profiler

import (
	"fmt"
	"runtime/metrics"
	"strconv"
	"strings"
	"time"

	"aego/core/mathx"
	"aego/core/stats"
	"aego/core/trace"
	"aego/editor/core/context"
	"aego/editor/core/play"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/engine"
	"aego/engine/i18n"
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
	avg    [metricCount]float32
	peak   [metricCount]float32
	frames int
}

type mark struct {
	title string
	y     float32
}

type Panel struct {
	ring       [history]bucket
	head       int
	n          int
	valid      [metricCount]int
	acc        bucket
	start      float64
	interval   int
	paused     bool
	playing    bool
	scroll     float32
	content    float32
	top        float32
	scripts    context.ScriptStats
	hasScripts bool
	game       play.Stats
	materials  render.Stats
	mem        []metrics.Sample
	probe      bool
	allocs     []uint64
	slow       int
	alertAt    float64
	titles     []string
	marks      []mark
	tl         Timeline
	deps       Deps
}

type Deps struct {
	Tracer      *trace.Tracer
	Root        string
	Notices     services.Notices
	Sections    func() []context.ProfilerSection
	Nav         services.Navigation
	PhaseAllocs func([]uint64) []uint64
	SetProbe    func(bool)
	Memory      func() MemStats
	Throttled   func() bool
}

func (p *Panel) Sections() []string {
	return p.titles
}

func (p *Panel) section(ui *core.Context, title string) {
	p.titles = append(p.titles, title)
	p.marks = append(p.marks, mark{title: title, y: ui.Cursor().Y - p.top})
	widgets.Label(ui, title)
}

func (p *Panel) forget(from int) {
	for i := from; i < metricCount; i++ {
		p.valid[i] = 0
		p.acc.avg[i], p.acc.peak[i] = 0, 0
	}
}

func (p *Panel) count(m int) int {
	return min(p.n, p.valid[m])
}

func intervals() [4]float64 {
	return [4]float64{0.1, 0.25, 0.5, 1}
}

var memNames = [...]string{
	"/memory/classes/heap/objects:bytes",
	"/gc/heap/goal:bytes",
	"/memory/classes/total:bytes",
	"/sched/goroutines:goroutines",
	"/gc/cycles/total:gc-cycles",
	"/gc/heap/live:bytes",
}

func New(d Deps) *Panel {
	p := &Panel{interval: 1, deps: d}
	p.mem = make([]metrics.Sample, len(memNames))
	for i, n := range memNames {
		p.mem[i].Name = n
	}
	return p
}

func mib(b uint64) string {
	return strconv.FormatFloat(float64(b)/(1<<20), 'f', 1, 64) + " MiB"
}

type MemStats struct {
	// Mapped is Go-managed address space. Process is reserved for callers
	// supplying their own process measurement (for example, process RSS).
	Mapped     uint64
	Live       uint64
	Heap       uint64
	Goal       uint64
	Process    uint64
	Goroutines uint64
	GC         uint64
}

func (p *Panel) readMemory() MemStats {
	if p.deps.Memory != nil {
		return p.deps.Memory()
	}
	metrics.Read(p.mem)
	v := func(i int) uint64 {
		if p.mem[i].Value.Kind() == metrics.KindUint64 {
			return p.mem[i].Value.Uint64()
		}
		return 0
	}
	return MemStats{Heap: v(0), Goal: v(1), Mapped: v(2), Goroutines: v(3), GC: v(4), Live: v(5)}
}

func (p *Panel) memory(ui *core.Context) {
	m := p.readMemory()
	if m.Mapped == 0 && m.Process > 0 && m.Live == 0 {
		// An external sample may provide process memory, but no Go live/mapped measurement.
		widgets.LabelDim(ui, i18n.Tr("editor.profiler.go_heap_heap_goal_goal", "Go heap {heap}   goal {goal}   process {process}   goroutines {formatuint}   GC {formatuint2}", i18n.Str("heap", mib(m.Heap)), i18n.Str("goal", mib(m.Goal)), i18n.Str("process", mib(m.Process)), i18n.Str("formatuint", strconv.FormatUint(m.Goroutines, 10)), i18n.Str("formatuint2", strconv.FormatUint(m.GC, 10))))
	} else {
		live := i18n.Tr("editor.profiler.no_live_sample", "awaiting GC sample")
		if m.Live > 0 || m.GC > 0 {
			live = mib(m.Live)
		}
		widgets.LabelDim(ui, i18n.Tr("editor.profiler.live_heap", "Live at last GC {live}   Allocated now {heap}   Next GC goal {goal}", i18n.Str("live", live), i18n.Str("heap", mib(m.Heap)), i18n.Str("goal", mib(m.Goal))))
		widgets.LabelDim(ui, i18n.Tr("editor.profiler.runtime_mapped", "Go mapped {mapped}   Goroutines {count}   GC cycles {cycles}", i18n.Str("mapped", mib(m.Mapped)), i18n.Str("count", strconv.FormatUint(m.Goroutines, 10)), i18n.Str("cycles", strconv.FormatUint(m.GC, 10))))
	}
	if p.hasScripts {
		widgets.LabelDim(ui, i18n.Tr("editor.profiler.script_heap_heapobjects_objects_heapbytes", "script heap {heapobjects} objects, {heapbytes}", i18n.Str("heapobjects", strconv.Itoa(p.scripts.HeapObjects)), i18n.Str("heapbytes", mib(uint64(p.scripts.HeapBytes)))))
	}
	label := i18n.Tr("editor.profiler.allocations_by_phase_off", "Allocations by phase: off")
	if p.probe {
		label = i18n.Tr("editor.profiler.allocations_by_phase_on", "Allocations by phase: on")
	}
	if widgets.Button(ui, label) && p.deps.SetProbe != nil {
		p.probe = !p.probe
		p.deps.SetProbe(p.probe)
	}
	if !p.probe || p.deps.PhaseAllocs == nil {
		return
	}
	p.allocs = p.deps.PhaseAllocs(p.allocs[:0])
	for i, n := range p.allocs {
		if n > 0 {
			widgets.LabelDim(ui, i18n.Tr("editor.profiler.v_formatuint_objects", "{v}   {formatuint} objects", i18n.Str("v", engine.Phase(i).String()), i18n.Str("formatuint", strconv.FormatUint(n, 10))))
		}
	}
	widgets.LabelDim(ui, i18n.Tr("editor.profiler.approximate_counted_process_wide_from", "Approximate: counted process-wide from phase start to end; a play world ticked inside a phase counts there too."))
}

func (p *Panel) slowest() string {
	f := p.deps.Tracer.Frame(0)
	if f == nil {
		return ""
	}
	best, name := uint32(0), ""
	for _, e := range f.Events {
		if e.Depth == 1 && e.Dur > best {
			best, name = e.Dur, p.deps.Tracer.Name(e.Zone)
		}
	}
	return name
}

func (p *Panel) budget(now float64, f *stats.Frame) {
	if f == nil {
		return
	}
	if p.deps.Throttled != nil && p.deps.Throttled() {
		p.slow = 0
		return
	}
	if f.FrameTime*1000 > 2*1000.0/60 {
		p.slow++
	} else {
		p.slow = 0
	}
	if p.slow != 3 || now-p.alertAt < 5 || p.deps.Notices == nil {
		return
	}
	p.alertAt = now
	msg := i18n.Tr("editor.profiler.frame_took_formatfloat_ms_for", "Frame took {formatfloat} ms for 3 frames in a row", i18n.Str("formatfloat", strconv.FormatFloat(f.FrameTime*1000, 'f', 1, 64)))
	if z := p.slowest(); z != "" {
		msg += i18n.Tr("editor.profiler.slowest_zone_z", ", slowest zone {z}", i18n.Str("z", z))
	}
	p.deps.Notices.Notify(context.LevelWarn, msg)
}

func (p *Panel) ID() string {
	return workspace.Profiler
}

func (p *Panel) Title() string {
	return i18n.Tr("editor.profiler.title", "Profiler")
}

func (p *Panel) seconds() float64 {
	return intervals()[p.interval]
}

func gameLine(g *play.Stats) string {
	return i18n.Tr("editor.profiler.frame_formatuint_tick_formatuint2_alpha", "frame {formatuint}   tick {formatuint2}   alpha {formatfloat}   scale ×{formatfloat2}   dropped ticks {formatuint3}", i18n.Str("formatuint", strconv.FormatUint(g.Frame, 10)), i18n.Str("formatuint2", strconv.FormatUint(g.Tick, 10)), i18n.Str("formatfloat", strconv.FormatFloat(g.Alpha, 'f', 3, 64)), i18n.Str("formatfloat2", strconv.FormatFloat(g.Scale, 'g', 3, 64)), i18n.Str("formatuint3", strconv.FormatUint(g.DroppedFixed, 10)))
}

func (p *Panel) Record(now float64, f *stats.Frame, r render.Stats, g play.Stats, sc context.ScriptStats, hasSc, playing bool) {
	p.game = g
	p.budget(now, f)
	if playing && !p.playing {
		p.forget(mGameUpdate)
	}
	p.playing = playing
	if !playing {
		p.hasScripts = false
	}
	if p.paused || f == nil {
		return
	}
	p.materials = r
	if hasSc && playing && !p.hasScripts {
		p.forget(mScriptInstr)
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
	for i := range p.valid {
		p.valid[i] = min(p.valid[i]+1, history)
	}
	p.acc = bucket{}
}

func ms(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds()*1000, 'f', 2, 64)
}

func (p *Panel) scriptTable(ui *core.Context) {
	th := &ui.Theme
	s := &p.scripts
	widgets.LabelDim(ui, i18n.Tr("editor.profiler.instances_instances_update_update_fixed", "instances {instances}   update {update}   fixed {fixed}   faults {faults}   budget aborts {formatuint}   heap objects {heapobjects}   GC {formatuint2} / {gctime} ms", i18n.Str("instances", strconv.Itoa(s.Instances)), i18n.Str("update", strconv.Itoa(s.Update)), i18n.Str("fixed", strconv.Itoa(s.Fixed)), i18n.Str("faults", strconv.Itoa(s.Faults)), i18n.Str("formatuint", strconv.FormatUint(s.BudgetAborts, 10)), i18n.Str("heapobjects", strconv.Itoa(s.HeapObjects)), i18n.Str("formatuint2", strconv.FormatUint(s.GCRuns, 10)), i18n.Str("gctime", ms(s.GCTime))))
	co := &s.Coroutines
	widgets.LabelDim(ui, i18n.Tr("editor.profiler.coroutines_active_active_sleeping_sleeping", "coroutines active {active}   sleeping {sleeping}   resumes/frame {formatuint}   canceled {formatuint2}", i18n.Str("active", strconv.Itoa(co.Active)), i18n.Str("sleeping", strconv.Itoa(co.Sleeping)), i18n.Str("formatuint", strconv.FormatUint(co.ResumesPerFrame, 10)), i18n.Str("formatuint2", strconv.FormatUint(co.Canceled, 10))))
	sg := &s.Signals
	widgets.LabelDim(ui, i18n.Tr("editor.profiler.signal_connections_connections_emits_frame", "signal connections {connections}   emits/frame {formatuint}   handlers/frame {formatuint2}", i18n.Str("connections", strconv.Itoa(sg.Connections)), i18n.Str("formatuint", strconv.FormatUint(sg.EmitsPerFrame, 10)), i18n.Str("formatuint2", strconv.FormatUint(sg.HandlersPerFrame, 10))))
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
	n := p.count(m)
	if n == 0 {
		return 0, 0, 0
	}
	var sum float32
	for i := p.n - n; i < p.n; i++ {
		b := p.at(i)
		sum += b.avg[m]
		peak = max(peak, b.peak[m])
	}
	return p.at(p.n - 1).avg[m], sum / float32(n), peak
}

func fmtf(v float32, prec int) string {
	return strconv.FormatFloat(float64(v), 'f', prec, 32)
}

func (p *Panel) graph(ui *core.Context, title, unit string, prec int, budget float32, col mathx.Color, m int) {
	th := &ui.Theme
	last, avg, peak := p.series(m)
	n := p.count(m)
	first := p.n - n

	hr := ui.Cursor().Next(th.RowHeight)
	if offscreen(ui, hr.Y, th.RowHeight*3.6) {
		ui.Cursor().Next(th.RowHeight * 2.6)
		return
	}
	nameW, valW := hr.W*0.42, hr.W*0.2
	cell := func(c mathx.Rect, s string, tc mathx.Color, align core.Align) {
		ui.PushClip(c)
		ui.TextIn(c, s, tc, align)
		ui.PopClip()
	}
	ui.Rect(mathx.Rect{X: hr.X, Y: hr.Y + (hr.H-9)/2, W: 3, H: 9}, col)
	valCol := th.Text
	if budget > 0 && last > budget {
		valCol = th.Warn
	}
	cell(mathx.Rect{X: hr.X + 4, Y: hr.Y, W: nameW - 4, H: hr.H}, title, th.Text, core.AlignLeft)
	cell(mathx.Rect{X: hr.X + nameW, Y: hr.Y, W: valW, H: hr.H}, fmtf(last, prec)+unit, valCol, core.AlignLeft)
	cell(mathx.Rect{X: hr.X + nameW + valW, Y: hr.Y, W: hr.W - nameW - valW, H: hr.H}, "avg "+fmtf(avg, prec)+"    max "+fmtf(peak, prec), th.TextDim, core.AlignRight)

	r := ui.Cursor().Next(th.RowHeight * 2.6)
	ui.PushClip(r)
	defer ui.PopClip()
	ui.Rect(r, th.PanelAlt)
	top := max(peak, budget, 1e-6) * 1.15
	bw := r.W / history
	if n > 0 {
		floor := r.Y + r.H
		off := float32(history-n) * bw
		fill, env, line := col, col, col
		fill.A, env.A = 0.22, 0.09
		yOf := func(v float32) float32 { return r.H * v / top }
		sample := func(u float32) (float32, float32) {
			k := int(u)
			if k >= n-1 {
				b := p.at(first + n - 1)
				return b.avg[m], b.peak[m]
			}
			f := u - float32(k)
			a, b := p.at(first+k), p.at(first+k+1)
			return a.avg[m] + (b.avg[m]-a.avg[m])*f, a.peak[m] + (b.peak[m]-a.peak[m])*f
		}
		prev := float32(-1)
		for px := int(off); px < int(r.W); px++ {
			x := r.X + float32(px)
			av, pk := sample((float32(px) - off) / bw)
			ah, ph := yOf(av), yOf(pk)
			ui.Rect(mathx.Rect{X: x, Y: floor - ph, W: 1, H: ph}, env)
			ui.Rect(mathx.Rect{X: x, Y: floor - ah, W: 1, H: ah}, fill)
			lo, hi := ah, ah
			if prev >= 0 {
				lo, hi = min(ah, prev), max(ah, prev)
			}
			ui.Rect(mathx.Rect{X: x, Y: floor - hi - 1, W: 1, H: hi - lo + 2}, line)
			prev = ah
		}
	}
	if budget > 0 {
		warn := th.Warn
		warn.A = 0.7
		ui.Rect(mathx.Rect{X: r.X, Y: r.Y + r.H - r.H*budget/top, W: r.W, H: 1}, warn)
	}
	ui.TextIn(mathx.Rect{X: r.X, Y: r.Y + 2, W: r.W - 5, H: ui.LineHeight()}, fmtf(top, prec)+unit, th.TextDisabled, core.AlignRight)
	if !ui.Hit(r) || n == 0 {
		return
	}
	i := int((ui.Mouse().X-r.X)/bw) - (history - n)
	if i < 0 || i >= n {
		return
	}
	b := p.at(first + i)
	x := r.X + float32(history-n+i)*bw
	ui.Rect(mathx.Rect{X: x, Y: r.Y, W: max(1, bw-1), H: r.H}, ui.Theme.Highlight)
	ago := float64(n-1-i) * p.seconds()
	label := i18n.Tr("editor.profiler.fmtf_fmtf2_unit_formatfloat_s", "{fmtf} / {fmtf2}{unit}   {formatfloat} s ago", i18n.Str("fmtf", fmtf(b.avg[m], prec)), i18n.Str("fmtf2", fmtf(b.peak[m], prec)), i18n.Str("unit", unit), i18n.Str("formatfloat", strconv.FormatFloat(ago, 'f', 1, 64)))
	size := ui.TextSize(label)
	box := mathx.Rect{X: r.X + r.W - size.X - 10, Y: r.Y + 2, W: size.X + 8, H: size.Y + 2}
	ui.Rect(box, th.Background)
	ui.Text(label, mathx.Vec2{X: box.X + 4, Y: box.Y + 1}, th.Text)
}

func (p *Panel) toolbar(ui *core.Context, bar mathx.Rect) {
	th := &ui.Theme
	label := i18n.Tr("editor.profiler.pause", "Pause")
	if p.paused {
		label = i18n.Tr("editor.profiler.resume", "Resume")
	}
	if widgets.ButtonRect(ui, ui.ID("pause"), mathx.Rect{X: bar.X, Y: bar.Y, W: 80, H: bar.H}, label) {
		p.paused = !p.paused
	}
	names := []string{"0.1 s", "0.25 s", "0.5 s", "1 s"}
	if widgets.Combo(ui, ui.ID("interval"), mathx.Rect{X: bar.X + 86, Y: bar.Y, W: 80, H: bar.H}, &p.interval, names, false) {
		p.n, p.head, p.acc = 0, 0, bucket{}
		p.valid = [metricCount]int{}
	}
	if p.n == 0 {
		return
	}
	fps := float64(p.at(p.n-1).frames) / p.seconds()
	span := strconv.FormatFloat(p.seconds()*history, 'f', 0, 64)
	info := mathx.Rect{X: bar.X + 176, Y: bar.Y, W: max(0, bar.W-176), H: bar.H}
	ui.PushClip(info)
	ui.Text(i18n.Tr("editor.profiler.fps_v_window_span_s", "fps {v}   window {span} s", i18n.Str("v", strconv.Itoa(int(fps+0.5))), i18n.Str("span", span)), mathx.Vec2{X: info.X, Y: ui.TextY(bar)}, th.Text)
	ui.PopClip()
}

func (p *Panel) sticky(ui *core.Context, body mathx.Rect) {
	th := &ui.Theme
	cur := -1
	for i, m := range p.marks {
		if m.y < p.scroll {
			cur = i
		}
	}
	if cur < 0 {
		return
	}
	y := body.Y
	if next := cur + 1; next < len(p.marks) {
		if gap := p.marks[next].y - p.scroll; gap < th.RowHeight {
			y -= th.RowHeight - gap
		}
	}
	h := mathx.Rect{X: body.X, Y: y, W: body.W, H: th.RowHeight}
	ui.Rect(h, th.Panel)
	ui.Rect(mathx.Rect{X: h.X, Y: h.Y + h.H - 1, W: h.W, H: 1}, th.Border)
	ui.TextIn(h, p.marks[cur].title, th.Text, core.AlignLeft)
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	body := r
	if ui.MouseOver(ui.ID("scroll"), body) {
		if w := ui.ScrollY(th.RowHeight * 3); w != 0 {
			p.scroll -= w
		}
	}
	p.scroll = mathx.Clamp(p.scroll, 0, max(0, p.content-body.H+th.Padding))
	p.top = body.Y - p.scroll
	ui.PushClip(body)
	y := p.top
	if p.deps.Tracer != nil {
		p.tl.Draw(ui, p.deps, mathx.Rect{X: r.X + th.Padding, Y: y + th.Padding, W: r.W - 2*th.Padding, H: TimelineHeight - th.Padding})
		y += TimelineHeight
	}
	bar := mathx.Rect{X: r.X + th.Padding, Y: y + th.Padding, W: r.W - 2*th.Padding, H: th.RowHeight}
	p.toolbar(ui, bar)
	y = bar.Y + bar.H + th.Spacing
	ui.PushCursor(core.Cursor{X: body.X + th.Padding, Y: y, W: body.W - 2*th.Padding, RowH: th.RowHeight, Pad: th.Padding, Space: th.Spacing})
	p.titles, p.marks = p.titles[:0], p.marks[:0]
	p.section(ui, i18n.Tr("editor.profiler.frame", "Frame"))
	p.graph(ui, i18n.Tr("editor.profiler.graph.frame", "frame"), " ms", 2, 1000.0/60, th.Accent, mFrame)
	p.graph(ui, i18n.Tr("editor.profiler.graph.update", "update"), " ms", 2, 0, th.AxisY, mUpdate)
	p.graph(ui, i18n.Tr("editor.profiler.graph.render", "render"), " ms", 2, 0, th.AxisX, mRender)
	p.graph(ui, i18n.Tr("editor.profiler.graph.draw_calls", "draw calls"), "", 0, 0, th.Selection, mDraws)
	p.graph(ui, i18n.Tr("editor.profiler.graph.sprites", "sprites"), "", 0, 0, th.Selection, mSprites)
	p.graph(ui, i18n.Tr("editor.profiler.graph.garbage", "garbage"), " KB", 1, 64, th.Warn, mGarbage)
	widgets.Separator(ui)
	p.section(ui, i18n.Tr("editor.profiler.materials", "Materials and shaders"))
	p.materialTable(ui)
	p.section(ui, i18n.Tr("editor.profiler.memory", "Memory"))
	p.memory(ui)
	if p.playing {
		widgets.Separator(ui)
		p.section(ui, i18n.Tr("editor.profiler.game", "Game"))
		p.graph(ui, i18n.Tr("editor.profiler.graph.game_update", "game update"), " ms", 2, 0, th.AxisY, mGameUpdate)
		p.graph(ui, i18n.Tr("editor.profiler.graph.game_render", "game render"), " ms", 2, 0, th.AxisX, mGameRender)
		p.graph(ui, i18n.Tr("editor.profiler.graph.game_sprites", "game sprites"), "", 0, 0, th.Selection, mGameSprites)
		p.graph(ui, i18n.Tr("editor.profiler.graph.game_nodes", "game nodes"), "", 0, 0, th.Selection, mGameNodes)
		widgets.LabelDim(ui, gameLine(&p.game))
		if p.hasScripts {
			widgets.Separator(ui)
			p.section(ui, i18n.Tr("editor.profiler.scripts", "Scripts"))
			p.graph(ui, i18n.Tr("editor.profiler.graph.script_instructions", "script instructions"), "", 0, 0, th.Accent, mScriptInstr)
			p.graph(ui, i18n.Tr("editor.profiler.graph.script_host_calls", "script host calls"), "", 0, 0, th.AxisY, mScriptHost)
			p.graph(ui, i18n.Tr("editor.profiler.graph.script_heap", "script heap"), " KB", 1, 0, th.Warn, mScriptHeap)
			p.graph(ui, i18n.Tr("editor.profiler.graph.coroutines", "coroutines"), "", 0, 0, th.AxisZ, mCoroutines)
			p.graph(ui, i18n.Tr("editor.profiler.graph.signal_emits", "signal emits"), "", 0, 0, th.Selection, mEmits)
			p.scriptTable(ui)
		}
	}
	var secs []context.ProfilerSection
	if p.deps.Sections != nil {
		secs = p.deps.Sections()
	}
	for _, sec := range secs {
		if sec.Play && !p.playing {
			continue
		}
		widgets.Separator(ui)
		p.section(ui, sec.Title.String())
		sec.Draw()
	}
	if !p.playing {
		widgets.LabelDim(ui, playHint(secs))
	}
	p.content = ui.Cursor().Y - p.top
	ui.PopCursor()
	p.sticky(ui, body)
	ui.PopClip()
}

func playHint(list []context.ProfilerSection) string {
	var extra []string
	for _, sec := range list {
		if sec.Play {
			extra = append(extra, strings.ToLower(sec.Title.String()))
		}
	}
	if len(extra) == 0 {
		return i18n.Tr("editor.profiler.press_play_to_see_game", "Press Play to see game and script timings")
	}
	return i18n.Tr("editor.profiler.press_play_to_see_game_2", "Press Play to see game, script and {join} timings", i18n.Str("join", strings.Join(extra, ", ")))
}

// offscreen reports whether a graph from y down h is outside the clip, so it is skipped: its columns would be clipped but still sent to the GPU.
func offscreen(ui *core.Context, y, h float32) bool {
	clip, ok := ui.ClipRect()
	return ok && (y > clip.Y+clip.H || y+h < clip.Y)
}

func (p *Panel) materialTable(ui *core.Context) {
	r := &p.materials
	widgets.LabelDim(ui, fmt.Sprintf("Materials %d   programs %d   variants %d   pending %d", r.MaterialCount, r.MaterialPrograms, r.MaterialVariants, r.MaterialPending))
	widgets.LabelDim(ui, fmt.Sprintf("Uploads/frame: frame %d B   view %d B   material %d B", r.FrameUBOBytes, r.ViewUBOBytes, r.MaterialUBOBytes))
	widgets.LabelDim(ui, fmt.Sprintf("Instance bytes: sprites %d B   material lanes %d B", r.InstanceBytes, r.MaterialLaneBytes))
	widgets.LabelDim(ui, fmt.Sprintf("Pipeline switches/frame %d", r.PipelineSwitches))
	widgets.LabelDim(ui, fmt.Sprintf("Batch splits/frame: material %d   texture/sampler %d   pipeline %d   clip %d   mesh %d", r.MaterialBreaks, r.TextureBreaks, r.PipelineBreaks, r.ClipBreaks, r.MeshBreaks))
	widgets.LabelDim(ui, fmt.Sprintf("Since renderer start: compile failures %d   committed reloads %d", r.MaterialCompileFailures, r.MaterialReloadGenerations))
}
