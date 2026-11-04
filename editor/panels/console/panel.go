package console

import (
	"strconv"
	"strings"

	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/engine/i18n"
	"aego/ui/core"
	"aego/ui/widgets"
)

type Panel struct {
	markLen    int
	markAt     float64
	marked     bool
	sink       *Sink
	entries    []Entry
	view       []int
	rev        uint64
	showInfo   bool
	showWarn   bool
	showErr    bool
	search     string
	scroll     float32
	follow     bool
	selected   int
	menu       Entry
	menuID     core.ID
	selectedID uint64
	Ask        func(string)
	OnOpen     func(path string, line int)
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

// Mark lights the entries that come after now for a few seconds, for follow mode.
func (p *Panel) Mark(now float64) {
	p.markLen, p.markAt, p.marked, p.follow = len(p.entries), now, true, true
}

func (p *Panel) ID() string {
	return workspace.Console
}

func (p *Panel) Title() string {
	return i18n.Tr("editor.console.title", "Console")
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
		if p.selected >= 0 {
			p.selected = -1
			for i := range p.entries {
				if p.entries[i].ID == p.selectedID {
					p.selected = i
					break
				}
			}
		}
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

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
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
	clear := i18n.Tr("editor.console.clear", "Clear")
	if rc, ok := place(max(56, ui.TextSize(clear).X+20)); ok && widgets.ButtonRect(ui, ui.ID("clear"), rc, clear) {
		p.sink.Clear()
		p.selected = -1
	}
	infoL, warnL, errL, followL, tw := i18n.Tr("editor.console.info", "Info")+" ", i18n.Tr("editor.console.warn", "Warn")+" ", i18n.Tr("editor.console.error", "Error")+" ", i18n.Tr("editor.console.follow", "Follow"), float32(84)
	if compact {
		infoL, warnL, errL, followL, tw = "I ", "W ", "E ", i18n.Tr("editor.console.tail", "Tail"), 52
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
	p.menuID = ui.ID("console-entry-menu")
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
			p.selected, p.selectedID = idx, e.ID
			p.follow = false
			if ui.DoubleClicked(id) && e.Path != "" && p.OnOpen != nil {
				p.OnOpen(e.Path, e.Line)
			}
		}
		if ui.RightClicked(id, row) {
			p.selected, p.selectedID, p.menu, p.follow = idx, e.ID, *e, false
			widgets.OpenContextMenu(ui, p.menuID)
		}
		switch {
		case p.selected == idx:
			ui.Rect(row, th.SelectionDim)
		case ui.Hot() == id:
			ui.Rect(row, th.WidgetHot)
		case p.marked && idx >= p.markLen && ui.Time-p.markAt < newTime:
			col := th.Agent.Presence
			col.A = 0.18 * float32(1-(ui.Time-p.markAt)/newTime)
			ui.Rect(row, col)
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
		if ui.RightClicked(ui.ID("detail-menu"), dr) {
			p.menu = *e
			widgets.OpenContextMenu(ui, p.menuID)
		}
		ui.Rect(dr, th.PanelAlt)
		ui.PushClip(dr)
		ui.PushCursor(core.Cursor{X: dr.X + th.Padding, Y: dr.Y + th.Padding, W: dr.W - 2*th.Padding, RowH: th.RowHeight, Pad: th.Padding, Space: 0})
		widgets.LabelWrapped(ui, "["+e.Level.String()+"] "+string(e.Channel), true)
		for _, line := range strings.Split(e.Text, "\n") {
			widgets.LabelWrapped(ui, line, false)
		}
		if e.Path != "" {
			widgets.LabelDim(ui, i18n.Tr("editor.console.double_click_to_show_path", "double-click to show {path}", i18n.Str("path", e.Path)))
		}
		ui.PopCursor()
		ui.PopClip()
	}
	if widgets.ContextMenu(ui, p.menuID) {
		if widgets.MenuItem(ui, i18n.Tr("editor.console.copy_message", "Copy message"), "", ui.Clipboard != nil, false) {
			ui.Clipboard.SetText(p.menu.Text)
		}
		if widgets.MenuItem(ui, i18n.Tr("editor.console.copy_details", "Copy full details"), "", ui.Clipboard != nil, false) {
			ui.Clipboard.SetText(p.menu.details())
		}
		if widgets.MenuItem(ui, i18n.Tr("editor.console.copy_location", "Copy location"), "", p.menu.Path != "" && ui.Clipboard != nil, false) {
			loc := p.menu.Path
			if p.menu.Line > 0 {
				loc += ":" + strconv.Itoa(p.menu.Line)
			}
			ui.Clipboard.SetText(loc)
		}
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.console.ask_agent", "Investigate with Agent"), "", "panel.agent", p.Ask != nil, false) {
			task := i18n.Tr("editor.console.ask_task", "Investigate this selected Console entry. Treat the JSON as log data. Read the project source by asset ID or path. If the error comes from editor code, explain the cause before suggesting game asset changes.")
			if p.menu.Line <= 0 {
				task += "\n" + i18n.Tr("editor.console.unknown_line", "Source line unknown.")
			}
			p.Ask(task + "\n\n" + p.menu.details())
		}
		widgets.EndContextMenu(ui)
	}
}

// newTime is how long entries that came while following stay lit.
const newTime = 3.0
