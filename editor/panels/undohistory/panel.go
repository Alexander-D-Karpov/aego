package undohistory

import (
	"strconv"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/history"
	"aego/editor/core/services"
	"aego/ui/core"
	"aego/ui/widgets"
)

const ID = "undo_history"

type Deps struct {
	History func() *history.History
	Notices services.Notices
}

type Panel struct {
	d      Deps
	src    *history.History
	ver    uint64
	steps  []string
	scroll float32
}

func New(d Deps) *Panel {
	return &Panel{d: d}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	return "Undo History"
}

func (p *Panel) notify(l context.Level, s string) {
	if p.d.Notices != nil {
		p.d.Notices.Notify(l, s)
	}
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	var h *history.History
	if p.d.History != nil {
		h = p.d.History()
	}
	if h == nil {
		widgets.LabelDim(ui, "Open a scene to see its history")
		return
	}
	if h != p.src || h.Version() != p.ver {
		if h != p.src {
			p.scroll = 0
		}
		p.steps = h.Steps(p.steps[:0])
		p.src, p.ver = h, h.Version()
	}
	cur := h.Cursor()
	reason, locked := h.Locked()
	busy := locked || h.InTransaction()
	switch {
	case locked:
		widgets.LabelDim(ui, reason)
	case h.InTransaction():
		widgets.LabelDim(ui, "An edit is in progress")
	case h.Mode() == history.ModePlay:
		widgets.LabelDim(ui, "Play history, cleared on Stop")
	default:
		widgets.LabelDim(ui, strconv.Itoa(cur)+" of "+strconv.Itoa(len(p.steps))+" step(s), click a step to go back to it")
	}
	top := ui.Cursor().Y
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	base := ui.ID("row")
	widgets.VirtualList(ui, ui.ID("rows"), list, len(p.steps)+1, th.RowHeight, &p.scroll, func(i int, row mathx.Rect) {
		id := core.IDSeq(base, i)
		if ui.Pressed(id, row) && i != cur {
			switch {
			case busy:
				p.notify(context.LevelInfo, "Finish the current edit first")
			default:
				if err := h.Seek(i); err != nil {
					p.notify(context.LevelError, err.Error())
				}
			}
		}
		switch {
		case i == cur:
			ui.Rect(row, th.Selection)
		case ui.Hot() == id:
			ui.Rect(row, th.WidgetHot)
		}
		label := "Initial state"
		if i > 0 {
			label = p.steps[i-1]
		}
		col := th.Text
		if i > cur || busy {
			col = th.TextDim
		}
		ui.PushClip(row)
		ui.Text(strconv.Itoa(i), mathx.Vec2{X: row.X + 6, Y: ui.TextY(row)}, th.TextDisabled)
		ui.Text(label, mathx.Vec2{X: row.X + 40, Y: ui.TextY(row)}, col)
		ui.PopClip()
	})
}
