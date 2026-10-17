package crashes

import (
	"strconv"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/editor/core/state"
	"aego/ui/core"
	"aego/ui/widgets"
)

const ID = "crashes"

type Panel struct {
	root   string
	nav    services.Navigation
	list   []state.CrashReport
	at     float64
	scroll float32
}

func New(root string, nav services.Navigation) *Panel {
	return &Panel{root: root, nav: nav, at: -10}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	return "Crash Reports"
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	if f.Now-p.at > 2 {
		p.list, p.at = state.CrashReports(p.root), f.Now
	}
	if len(p.list) == 0 {
		widgets.LabelDim(ui, "No crash reports in .aego/crash")
		return
	}
	widgets.LabelDim(ui, strconv.Itoa(len(p.list))+" report(s); click opens the script line, or copies the folder path")
	top := ui.Cursor().Y
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	widgets.VirtualList(ui, ui.ID("list"), list, len(p.list), th.RowHeight, &p.scroll, func(i int, row mathx.Rect) {
		c := &p.list[i]
		id := core.IDSeq(ui.ID("row"), i)
		if ui.Pressed(id, row) {
			if c.Path != "" {
				p.nav.Open(context.At(c.Path, c.Line, 1))
			} else if ui.Clipboard != nil {
				ui.Clipboard.SetText(c.Dir)
			}
		}
		if ui.Hot() == id {
			ui.Rect(row, th.WidgetHot)
		}
		col := th.Error
		if c.Kind == "spike" {
			col = th.Warn
		}
		ui.PushClip(row)
		ui.Text(c.Time+"  "+c.Kind+"  "+c.Where+"  "+c.Message, mathx.Vec2{X: row.X + 4, Y: ui.TextY(row)}, col)
		ui.PopClip()
	})
}
