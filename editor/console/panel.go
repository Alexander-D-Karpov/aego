package console

import (
	"strconv"
	"strings"

	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/context"
	"aego/editor/workspace"
	"aego/ui/core"
	"aego/ui/widgets"
)

type Panel struct {
	sink     *Sink
	entries  []Entry
	view     []int
	rev      uint64
	showInfo bool
	showWarn bool
	showErr  bool
	search   string
	scroll   float32
	follow   bool
	selected int
	OnOpen   func(path string)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " …"
	}
	return s
}

func New(s *Sink) *Panel {
	return &Panel{sink: s, showInfo: true, showWarn: true, showErr: true, follow: true, selected: -1, rev: ^uint64(0)}
}

func (p *Panel) Focus(search string) {
	p.search = search
	p.follow = false
	p.selected = -1
	p.showErr = true
}

func (p *Panel) ID() string {
	return workspace.Console
}

func (p *Panel) Title() string {
	return "Console"
}

func (p *Panel) visible(e *Entry, q string) bool {
	switch {
	case e.Level >= log.Error:
		if !p.showErr {
			return false
		}
	case e.Level == log.Warn:
		if !p.showWarn {
			return false
		}
	default:
		if !p.showInfo {
			return false
		}
	}
	return q == "" || strings.Contains(strings.ToLower(e.Text), q) || strings.Contains(string(e.Channel), q)
}

func (p *Panel) refresh() {
	if rev := p.sink.Revision(); rev != p.rev {
		p.rev = rev
		p.entries = p.sink.Snapshot(p.entries)
	}
	q := strings.ToLower(strings.TrimSpace(p.search))
	p.view = p.view[:0]
	for i := range p.entries {
		if p.visible(&p.entries[i], q) {
			p.view = append(p.view, i)
		}
	}
	if p.selected >= len(p.entries) {
		p.selected = -1
	}
}

func levelColor(th *core.Theme, l log.Level) mathx.Color {
	switch {
	case l >= log.Error:
		return th.Error
	case l == log.Warn:
		return th.Warn
	case l <= log.Debug:
		return th.TextDisabled
	}
	return th.Text
}

func (p *Panel) Draw(ctx *context.Editor, r mathx.Rect) {
	ui := ctx.UI
	th := &ui.Theme
	p.refresh()
	info, warn, errs := p.sink.Counts()
	bar := ui.Cursor().Next(0)
	compact := bar.W < 480
	x, right := bar.X, bar.X+bar.W
	place := func(w float32) (mathx.Rect, bool) {
		if x+w > right {
			return mathx.Rect{}, false
		}
		rc := mathx.Rect{X: x, Y: bar.Y, W: w, H: bar.H}
		x += w + 4
		return rc, true
	}
	if rc, ok := place(56); ok && widgets.ButtonRect(ui, ui.ID("clear"), rc, "Clear") {
		p.sink.Clear()
		p.selected = -1
	}
	infoL, warnL, errL, followL, tw := "Info ", "Warn ", "Error ", "Follow", float32(84)
	if compact {
		infoL, warnL, errL, followL, tw = "I ", "W ", "E ", "Tail", 52
	}
	if rc, ok := place(tw); ok {
		widgets.ToggleButton(ui, ui.ID("info"), rc, infoL+strconv.Itoa(info), &p.showInfo)
	}
	if rc, ok := place(tw); ok {
		widgets.ToggleButton(ui, ui.ID("warn"), rc, warnL+strconv.Itoa(warn), &p.showWarn)
	}
	if rc, ok := place(tw); ok {
		widgets.ToggleButton(ui, ui.ID("err"), rc, errL+strconv.Itoa(errs), &p.showErr)
	}
	if rc, ok := place(min(tw, 64)); ok {
		widgets.ToggleButton(ui, ui.ID("follow"), rc, followL, &p.follow)
	}
	if right-x >= 60 {
		widgets.SearchField(ui, ui.ID("search"), mathx.Rect{X: x, Y: bar.Y, W: right - x, H: bar.H}, &p.search)
	}

	top := bar.Y + bar.H + th.Spacing
	detailH := float32(0)
	if p.selected >= 0 {
		detailH = min(th.RowHeight*8, (r.Y+r.H-top)/2)
	}
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top-detailH)}
	rowH := th.RowHeight
	maxScroll := max(0, float32(len(p.view))*rowH-list.H)
	if p.follow {
		p.scroll = maxScroll
	}
	before := p.scroll
	timeW, chanW := float32(64), float32(72)
	if compact {
		timeW, chanW = 0, 0
	}
	widgets.VirtualList(ui, ui.ID("list"), list, len(p.view), rowH, &p.scroll, func(i int, row mathx.Rect) {
		idx := p.view[i]
		e := &p.entries[idx]
		id := core.IDSeq(ui.ID("row"), idx)
		if ui.Pressed(id, row) {
			p.selected = idx
			p.follow = false
			if ui.DoubleClicked(id) && e.Path != "" && p.OnOpen != nil {
				p.OnOpen(e.Path)
			}
		}
		switch {
		case p.selected == idx:
			ui.Rect(row, th.SelectionDim)
		case ui.Hot() == id:
			ui.Rect(row, th.WidgetHot)
		}
		tx := row.X + 4
		if timeW > 0 {
			ui.Text(strconv.FormatFloat(e.Time, 'f', 2, 64), mathx.Vec2{X: tx, Y: ui.TextY(row)}, th.TextDim)
			tx += timeW
			ui.Text(string(e.Channel), mathx.Vec2{X: tx, Y: ui.TextY(row)}, th.TextDim)
			tx += chanW
		}
		text := firstLine(e.Text)
		if e.Count > 1 {
			text = "(" + strconv.Itoa(e.Count) + ") " + text
		}
		ui.PushClip(mathx.Rect{X: tx, Y: row.Y, W: max(0, row.X+row.W-tx), H: row.H})
		ui.Text(text, mathx.Vec2{X: tx, Y: ui.TextY(row)}, levelColor(th, e.Level))
		ui.PopClip()
	})
	if p.scroll < before && p.scroll < maxScroll {
		p.follow = false
	}
	if p.selected >= 0 && p.selected < len(p.entries) && detailH > 0 {
		e := &p.entries[p.selected]
		dr := mathx.Rect{X: r.X, Y: r.Y + r.H - detailH, W: r.W, H: detailH}
		ui.Rect(dr, th.PanelAlt)
		ui.PushClip(dr)
		ui.PushCursor(core.Cursor{X: dr.X + th.Padding, Y: dr.Y + th.Padding, W: dr.W - 2*th.Padding, RowH: th.RowHeight, Pad: th.Padding, Space: 0})
		widgets.LabelWrapped(ui, "["+e.Level.String()+"] "+string(e.Channel), true)
		for _, line := range strings.Split(e.Text, "\n") {
			widgets.LabelWrapped(ui, line, false)
		}
		if e.Path != "" {
			widgets.LabelDim(ui, "double-click to show "+e.Path)
		}
		ui.PopCursor()
		ui.PopClip()
	}
}
