package profiler

import (
	"bytes"
	"strconv"

	"aego/core/mathx"
	"aego/core/trace"
	"aego/editor/core/context"
	"aego/editor/core/state"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	TimelineHeight float32 = 240
	stripHeight    float32 = 48
	flameRow       float32 = 18
	budgetNS               = 16_666_667
	stripScaleNS           = 2 * budgetNS
)

type Timeline struct {
	back   int
	paused bool
}

func nsMS(ns int64) string {
	return strconv.FormatFloat(float64(ns)/1e6, 'f', 2, 64) + " ms"
}

func (tl *Timeline) Draw(ui *core.Context, d Deps, r mathx.Rect) {
	th := &ui.Theme
	tr := d.Tracer
	label := "Pause"
	if tl.paused {
		label = "Resume"
	}
	if widgets.ButtonRect(ui, ui.ID("tl.pause"), mathx.Rect{X: r.X, Y: r.Y, W: 80, H: th.RowHeight}, label) {
		tl.paused = !tl.paused
		tl.back = 0
		tr.SetEnabled(!tl.paused)
	}
	if widgets.ButtonRect(ui, ui.ID("tl.export"), mathx.Rect{X: r.X + 86, Y: r.Y, W: 120, H: th.RowHeight}, "Export Trace") {
		tl.export(d, tr)
	}
	strip := mathx.Rect{X: r.X, Y: r.Y + th.RowHeight + th.Spacing, W: r.W, H: stripHeight}
	tl.strip(ui, d, tr, strip)
	top := strip.Y + strip.H + th.Spacing
	head := mathx.Rect{X: r.X, Y: top, W: r.W, H: th.RowHeight}
	flame := mathx.Rect{X: r.X, Y: top + head.H, W: r.W, H: max(0, r.Y+r.H-top-head.H)}
	f := tr.Frame(tl.back)
	if f == nil {
		ui.TextIn(flame, "No frames recorded yet", th.TextDim, core.AlignCenter)
		return
	}
	info := "Frame " + strconv.FormatUint(f.Index, 10) + "   " + nsMS(f.Dur)
	if f.GC > 0 {
		info += "   GC " + strconv.Itoa(int(f.GC))
	}
	if f.Dropped > 0 {
		info += "   dropped " + strconv.Itoa(int(f.Dropped))
	}
	if f.LastTick > 0 {
		info += "   ticks " + strconv.FormatUint(f.FirstTick, 10) + "–" + strconv.FormatUint(f.LastTick, 10)
	}
	if f.FrameID > 0 {
		info += "   world frame " + strconv.FormatUint(f.FrameID, 10)
	}
	ui.TextIn(head, info, th.TextDim, core.AlignLeft)
	tl.flame(ui, tr, f, flame)
}

func (tl *Timeline) strip(ui *core.Context, d Deps, tr *trace.Tracer, r mathx.Rect) {
	th := &ui.Theme
	ui.Rect(r, th.PanelAlt)
	n := min(tr.Frames(), int(r.W/3))
	if n == 0 {
		return
	}
	tl.back = min(tl.back, n-1)
	w := r.W / float32(n)
	for k := 0; k < n; k++ {
		back := n - 1 - k
		f := tr.Frame(back)
		h := min(float32(f.Dur)/stripScaleNS, 1) * r.H
		col := th.Ok
		switch {
		case f.Dur > 2*budgetNS:
			col = th.Error
		case f.Dur > budgetNS:
			col = th.Warn
		}
		if back == tl.back && tl.paused {
			col = th.Accent
		}
		ui.Rect(mathx.Rect{X: r.X + float32(k)*w, Y: r.Y + r.H - h, W: max(w-1, 1), H: h}, col)
	}
	line := r.Y + r.H - r.H*budgetNS/stripScaleNS
	ui.Rect(mathx.Rect{X: r.X, Y: line, W: r.W, H: 1}, th.TextDim)
	if !ui.MousePressed(input.MouseLeft) || !ui.Hit(r) {
		return
	}
	k := min(int((ui.Mouse().X-r.X)/w), n-1)
	tl.back = n - 1 - k
	if !tl.paused {
		tl.paused = true
		tr.SetEnabled(false)
	}
	if f := tr.Frame(tl.back); f != nil && f.FrameID > 0 && d.Nav != nil {
		d.Nav.Open(context.EventsAt(f.FrameID))
	}
}

func (tl *Timeline) flame(ui *core.Context, tr *trace.Tracer, f *trace.Frame, r mathx.Rect) {
	th := &ui.Theme
	dur := float32(f.Dur)
	if dur <= 0 {
		return
	}
	id := ui.ID("tl.flame")
	ui.MouseOver(id, r)
	m := ui.Mouse()
	hover := -1
	ui.PushClip(r)
	for i, e := range f.Events {
		y := r.Y + float32(e.Depth)*flameRow
		if y > r.Y+r.H {
			continue
		}
		er := mathx.Rect{
			X: r.X + float32(e.Start)/dur*r.W,
			Y: y,
			W: max(float32(e.Dur)/dur*r.W, 1),
			H: flameRow - 1,
		}
		ui.Rect(er, th.Series[int(e.Zone)%len(th.Series)])
		if er.W > 40 {
			ui.PushClip(er)
			ui.Text(tr.Name(e.Zone), mathx.Vec2{X: er.X + 3, Y: ui.TextY(er)}, th.Text)
			ui.PopClip()
		}
		if er.Contains(m) {
			hover = i
		}
	}
	ui.PopClip()
	if hover >= 0 && ui.Hit(r) {
		e := f.Events[hover]
		pct := strconv.FormatFloat(float64(e.Dur)/float64(f.Dur)*100, 'f', 1, 64)
		widgets.Tooltip(ui, id, tr.Name(e.Zone)+"\n"+nsMS(int64(e.Dur))+"   "+pct+"% of the frame")
	}
}

func (tl *Timeline) export(d Deps, tr *trace.Tracer) {
	if d.Root == "" || d.Notices == nil {
		return
	}
	var buf bytes.Buffer
	if err := trace.WriteChrome(&buf, tr, tr.Snapshot(tr.Frames())); err != nil {
		d.Notices.Notify(context.LevelError, "cannot write the trace: "+err.Error())
		return
	}
	path, err := state.WriteTrace(d.Root, buf.Bytes())
	if err != nil {
		d.Notices.Notify(context.LevelError, "cannot save the trace: "+err.Error())
		return
	}
	d.Notices.Notify(context.LevelInfo, "Trace saved to "+path+", open it in ui.perfetto.dev")
}
