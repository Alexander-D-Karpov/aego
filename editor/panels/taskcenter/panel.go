package taskcenter

import (
	"strconv"

	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/editor/core/tasks"
	"aego/ui/core"
	"aego/ui/widgets"
)

const ID = "task_center"

var laneNames = [...]string{"worker", "main", "render"}

type Panel struct {
	q      *tasks.Queue
	scroll float32
}

func New(q *tasks.Queue) *Panel {
	return &Panel{q: q}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	return "Tasks"
}

func (p *Panel) Icon() string {
	return "panel.tasks"
}

func stateIcon(t *tasks.Task) (string, func(th *core.Theme) mathx.Color) {
	switch t.State() {
	case tasks.Failed:
		return "log.error", func(th *core.Theme) mathx.Color { return th.Error }
	case tasks.Canceled:
		return "status.warning", func(th *core.Theme) mathx.Color { return th.Warn }
	case tasks.Done:
		return "status.ok", func(th *core.Theme) mathx.Color { return th.TextDim }
	}
	return "status.busy", func(th *core.Theme) mathx.Color { return th.Accent }
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	live, recent := p.q.Tasks(), p.q.Recent()
	head := ui.Cursor().Next(0)
	ui.Text(strconv.Itoa(len(live))+" running, "+strconv.Itoa(len(recent))+" finished", mathx.Vec2{X: head.X, Y: ui.TextY(head)}, th.TextDim)
	x := head.X + head.W
	for _, b := range []struct {
		id, label string
		on        bool
		fn        func()
	}{
		{"cancel", "Cancel All", len(live) > 0, p.q.CancelAll},
		{"clear", "Clear Finished", len(recent) > 0, p.q.ClearRecent},
	} {
		w := ui.TextSize(b.label).X + 16
		x -= w + 4
		if b.on && widgets.ButtonRect(ui, ui.ID(b.id), mathx.Rect{X: x, Y: head.Y, W: w, H: head.H}, b.label) {
			b.fn()
		}
	}
	top := ui.Cursor().Y
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	n := len(live) + len(recent)
	if n == 0 {
		ui.TextIn(list, "No background tasks", th.TextDisabled, core.AlignCenter)
		return
	}
	widgets.VirtualList(ui, ui.ID("rows"), list, n, th.RowHeight+4, &p.scroll, func(i int, rr mathx.Rect) {
		if i < len(live) {
			p.row(ui, live[i], rr, true)
			return
		}
		p.row(ui, recent[len(recent)-1-(i-len(live))], rr, false)
	})
}

func (p *Panel) row(ui *core.Context, t *tasks.Task, rr mathx.Rect, active bool) {
	th := &ui.Theme
	id := core.IDSeq(ui.ID("task"), int(t.ID))
	icon, col := stateIcon(t)
	widgets.IconAt(ui, mathx.Rect{X: rr.X + 2, Y: rr.Y + (rr.H-16)/2, W: 16, H: 16}, icon, col(th))
	right := rr.X + rr.W
	if active {
		bw := rr.H
		if widgets.IconButtonRect(ui, core.IDSeq(id, 1), mathx.Rect{X: right - bw, Y: rr.Y + 2, W: bw, H: rr.H - 4}, "ui.close") {
			t.Cancel()
		}
		right -= bw + 6
		pw := min(120, rr.W*0.25)
		pr := mathx.Rect{X: right - pw, Y: rr.Y + rr.H/2 - 3, W: pw, H: 6}
		ui.Rect(pr, th.Widget)
		if f := t.Progress(); f > 0 {
			ui.Rect(mathx.Rect{X: pr.X, Y: pr.Y, W: pr.W * f, H: pr.H}, th.Accent)
		}
		right = pr.X - 8
	}
	meta := t.Kind + " · " + laneNames[t.Lane()]
	mw := ui.TextSize(meta).X
	ui.PushClip(mathx.Rect{X: rr.X, Y: rr.Y, W: max(0, right-rr.X-mw-8), H: rr.H})
	ui.Text(t.Title, mathx.Vec2{X: rr.X + 22, Y: ui.TextY(rr)}, th.Text)
	ui.PopClip()
	ui.Text(meta, mathx.Vec2{X: right - mw, Y: ui.TextY(rr)}, th.TextDisabled)
	tip := t.Detail
	if err := t.Err(); err != nil {
		tip = err.Error() + "\n" + tip
	}
	ui.MouseOver(id, rr)
	widgets.Tooltip(ui, id, tip)
}
