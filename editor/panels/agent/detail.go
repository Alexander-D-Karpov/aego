package agent

import (
	"strings"

	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/engine/i18n"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	tileW    = 76
	tileH    = 58
	maxTiles = 24
	maxLines = 16
)

// shownRows is how many rows of a result are shown before "+N more": a tree shows its first two levels.
func shownRows(d *agentui.Detail, all bool) (n, hidden int) {
	for i := range d.Rows {
		if d.Kind == agentui.DetailTree && !all && d.Rows[i].Depth > 1 {
			hidden++
			continue
		}
		if !all && n == maxRows {
			hidden++
			continue
		}
		n++
	}
	return n, hidden
}

// diffLines counts the rows of a change list: a header for every node, a row per change, the lines of script edits.
func diffLines(rows []agentui.Row, all bool) (n, hidden int) {
	group := "\x00"
	for i := range rows {
		r := &rows[i]
		if !all && n >= maxRows {
			hidden++
			continue
		}
		if r.Group != group {
			group = r.Group
			n++
		}
		n++
		if isCode(r) {
			n += codeLines(r.Before, all) + codeLines(r.After, all)
		}
	}
	return n, hidden
}

func isCode(r *agentui.Row) bool {
	return strings.Contains(r.Before, "\n") || strings.Contains(r.After, "\n") || r.Icon == "file.script"
}

func codeLines(s string, all bool) int {
	if s == "" {
		return 0
	}
	n := strings.Count(strings.TrimRight(s, "\n"), "\n") + 1
	if !all {
		n = min(n, 6)
	}
	return n
}

func chipLayout(ui *core.Context, rows []agentui.Row, w float32, draw func(r *agentui.Row, at mathx.Rect, header bool)) float32 {
	th := &ui.Theme
	row := th.RowHeight
	y, x := float32(0), w
	group := "\x00"
	for i := range rows {
		r := &rows[i]
		if r.Group != group {
			group = r.Group
			if i > 0 {
				y += row + 4
			}
			if draw != nil {
				draw(r, mathx.Rect{X: 0, Y: y, W: w, H: row}, true)
			}
			y += row
			x = 0
		}
		cw := ui.TextSize(r.Title).X + 14
		if x > 0 && x+cw > w {
			x, y = 0, y+row+4
		}
		if draw != nil {
			draw(r, mathx.Rect{X: x, Y: y, W: cw, H: row}, false)
		}
		x += cw + 4
	}
	if len(rows) > 0 {
		y += row
	}
	return y
}

// detailHeight is the height of a laid-out result for a width.
func detailHeight(ui *core.Context, d *agentui.Detail, w float32, all bool) float32 {
	return float32(len(d.Images))*min(w, 160) + textDetailHeight(ui, d, w, all)
}
func textDetailHeight(ui *core.Context, d *agentui.Detail, w float32, all bool) float32 {
	th := &ui.Theme
	row := th.RowHeight
	more := func(hidden int) float32 {
		if hidden > 0 {
			return row
		}
		return 0
	}
	switch d.Kind {
	case agentui.DetailList, agentui.DetailTree, agentui.DetailTable, agentui.DetailLog:
		n, hidden := shownRows(d, all)
		return float32(n)*row + more(hidden) + th.Spacing
	case agentui.DetailDiff:
		n, hidden := diffLines(d.Rows, all)
		return float32(n)*row + more(hidden) + th.Spacing
	case agentui.DetailChips:
		return chipLayout(ui, d.Rows, w, nil) + th.Spacing
	case agentui.DetailGrid:
		n := len(d.Rows)
		if !all {
			n = min(n, maxTiles)
		}
		cols := max(1, int(w/tileW))
		return float32((n+cols-1)/cols)*tileH + more(len(d.Rows)-n) + th.Spacing
	case agentui.DetailText:
		n, hidden := codeLines(d.Text, true), 0
		if !all && n > maxLines {
			n, hidden = maxLines, 1
		}
		return float32(n)*ui.MonoFont().LineHeight + more(hidden) + th.Spacing*2
	}
	return 0
}

// drawDetail draws a step's result; it reports true when "+N more" changed its height.
func (p *Panel) drawDetail(ui *core.Context, s *step, r mathx.Rect) bool {
	th := &ui.Theme
	row := th.RowHeight
	d := &s.look.Detail
	for _, key := range d.Images {
		h := min(r.W, 160)
		p.drawImage(ui, key, mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: h})
		r.Y += h
	}
	y := r.Y
	moreRow := func(hidden int) bool {
		if hidden == 0 {
			return false
		}
		mr := mathx.Rect{X: r.X, Y: y, W: r.W, H: row}
		id := ui.ID("more")
		label := i18n.Tr("editor.agent.v_more", "+{v} more", i18n.Str("v", itoa(hidden)))
		col := th.TextDim
		if ui.Hot() == id {
			col = th.Accent
		}
		ui.Text(label, mathx.Vec2{X: mr.X + 2, Y: ui.TextY(mr)}, col)
		if ui.Clicked(id, mathx.Rect{X: mr.X, Y: mr.Y, W: ui.TextSize(label).X + 8, H: row}) {
			s.all = true
			return true
		}
		return false
	}
	link := func(rr mathx.Rect, key int, target string) bool {
		if target == "" {
			return false
		}
		id := ui.IDInt(2000 + key)
		if ui.Clicked(id, rr) {
			p.openLink(target)
		}
		if ui.Hot() == id {
			ui.RoundRect(rr, th.Radius.Button, th.WidgetHot)
			return true
		}
		return false
	}
	switch d.Kind {
	case agentui.DetailList, agentui.DetailTree, agentui.DetailTable, agentui.DetailLog:
		_, hidden := shownRows(d, s.all)
		shown := 0
		for i := range d.Rows {
			rw := &d.Rows[i]
			if d.Kind == agentui.DetailTree && !s.all && rw.Depth > 1 {
				continue
			}
			if !s.all && shown == maxRows {
				break
			}
			shown++
			rr := mathx.Rect{X: r.X, Y: y, W: r.W, H: row}
			hot := link(rr, i, rw.Link)
			x := rr.X + 2 + float32(rw.Depth)*14
			icon := rw.Icon
			col := th.TextDim
			if d.Kind == agentui.DetailLog {
				switch rw.Level {
				case 2:
					icon, col = "log.error", th.Error
				case 1:
					icon, col = "log.warn", th.Warn
				}
			}
			if icon != "" {
				widgets.IconAt(ui, mathx.Rect{X: x, Y: rr.Y + (row-14)/2, W: 14, H: 14}, icon, col)
				x += 20
			}
			tc := th.Text
			if hot {
				tc = th.Accent
			}
			if d.Kind == agentui.DetailTable {
				nw := rr.W * 0.42
				ui.Text(widgets.Elide(ui, rw.Title, nw-8), mathx.Vec2{X: x, Y: ui.TextY(rr)}, th.TextDim)
				ui.Text(widgets.Elide(ui, firstLine(rw.Text), rr.X+rr.W-x-nw), mathx.Vec2{X: x + nw, Y: ui.TextY(rr)}, th.Text)
			} else {
				title := rw.Title
				if title == "" {
					title, tc = rw.Text, th.TextDim
				}
				tw := min(ui.TextSize(title).X, rr.X+rr.W-x)
				ui.Text(widgets.Elide(ui, title, rr.X+rr.W-x), mathx.Vec2{X: x, Y: ui.TextY(rr)}, tc)
				if rw.Title != "" && rw.Text != "" {
					tx := x + tw + 8
					ui.Text(widgets.Elide(ui, firstLine(rw.Text), max(0, rr.X+rr.W-tx)), mathx.Vec2{X: tx, Y: ui.TextY(rr)}, th.TextDisabled)
				}
			}
			y += row
		}
		return moreRow(hidden)
	case agentui.DetailDiff:
		return p.drawDiff(ui, d.Rows, mathx.Rect{X: r.X, Y: y, W: r.W}, &s.all)
	case agentui.DetailChips:
		chipLayout(ui, d.Rows, r.W, func(rw *agentui.Row, at mathx.Rect, header bool) {
			at.X += r.X
			at.Y += r.Y
			if header {
				ui.Text(rw.Group, mathx.Vec2{X: at.X, Y: ui.TextY(at)}, th.TextDim)
				return
			}
			id := ui.ID(rw.Title)
			fill := th.Widget
			if rw.Link != "" && ui.Clicked(id, at) {
				p.openLink(rw.Link)
			}
			if ui.Hot() == id {
				fill = th.WidgetHot
			}
			ui.RoundRect(at, at.H/2, fill)
			ui.TextIn(at, rw.Title, th.Text, core.AlignCenter)
			widgets.Tooltip(ui, id, rw.Text)
		})
	case agentui.DetailGrid:
		cols := max(1, int(r.W/tileW))
		n := len(d.Rows)
		if !s.all {
			n = min(n, maxTiles)
		}
		for i := 0; i < n; i++ {
			rw := &d.Rows[i]
			tr := mathx.Rect{X: r.X + float32(i%cols)*tileW, Y: r.Y + float32(i/cols)*tileH, W: tileW - 4, H: tileH - 4}
			link(tr, i, rw.Link)
			widgets.IconAt(ui, mathx.Rect{X: tr.X + (tr.W-24)/2, Y: tr.Y + 6, W: 24, H: 24}, rw.Icon, th.TextDim)
			ui.TextIn(mathx.Rect{X: tr.X, Y: tr.Y + 32, W: tr.W, H: row}, widgets.Elide(ui, rw.Title, tr.W-4), th.Text, core.AlignCenter)
		}
		y = r.Y + float32((n+cols-1)/cols)*tileH
		return moreRow(len(d.Rows) - n)
	case agentui.DetailText:
		mono := ui.MonoFont()
		ui.PushFont(mono)
		lines := strings.Split(strings.TrimRight(d.Text, "\n"), "\n")
		n := len(lines)
		if !s.all {
			n = min(n, maxLines)
		}
		bg := mathx.Rect{X: r.X, Y: y, W: r.W, H: float32(n)*mono.LineHeight + th.Spacing}
		ui.RoundRect(bg, th.Radius.Button, th.Code.Background)
		y += th.Spacing / 2
		for _, l := range lines[:n] {
			ui.Text(widgets.Elide(ui, l, r.W-8), mathx.Vec2{X: r.X + 4, Y: y}, th.Code.Plain)
			y += mono.LineHeight
		}
		ui.PopFont()
		y += th.Spacing * 1.5
		return moreRow(len(lines) - n)
	}
	return false
}

// drawDiff draws changes grouped by node: a header per node, then each field with its old and new value.
func (p *Panel) drawDiff(ui *core.Context, rows []agentui.Row, r mathx.Rect, all *bool) bool {
	th := &ui.Theme
	row := th.RowHeight
	y := r.Y
	group := "\x00"
	n, hidden := 0, 0
	for i := range rows {
		rw := &rows[i]
		if !*all && n >= maxRows {
			hidden++
			continue
		}
		if rw.Group != group {
			group = rw.Group
			hr := mathx.Rect{X: r.X, Y: y, W: r.W, H: row}
			icon := "cat.node"
			if rw.Icon == "file.script" {
				icon = "file.script"
			}
			widgets.IconAt(ui, mathx.Rect{X: hr.X, Y: hr.Y + (row-14)/2, W: 14, H: 14}, icon, th.TextDim)
			ui.Text(widgets.Elide(ui, rw.Title, hr.W-20), mathx.Vec2{X: hr.X + 20, Y: ui.TextY(hr)}, th.Text)
			y += row
			n++
		}
		fr := mathx.Rect{X: r.X + 20, Y: y, W: r.W - 20, H: row}
		col := th.TextDim
		switch rw.Icon {
		case "edit.add":
			col = th.Agent.DiffAdd
		case "edit.remove":
			col = th.Agent.DiffRemove
		}
		widgets.IconAt(ui, mathx.Rect{X: fr.X, Y: fr.Y + (row-14)/2, W: 14, H: 14}, rw.Icon, col)
		if isCode(rw) {
			ui.Text(widgets.Elide(ui, rw.Text, fr.W-20), mathx.Vec2{X: fr.X + 20, Y: ui.TextY(fr)}, th.TextDim)
			y += row
			n++
			y = codeDiff(ui, rw.Before, "−", th.Agent.DiffRemove, mathx.Rect{X: fr.X + 20, Y: y, W: fr.W - 20}, *all)
			y = codeDiff(ui, rw.After, "+", th.Agent.DiffAdd, mathx.Rect{X: fr.X + 20, Y: y, W: fr.W - 20}, *all)
			continue
		}
		lw := min(ui.TextSize(rw.Text).X+32, fr.W*0.62)
		if rw.Before == "" && rw.After == "" {
			lw = fr.W - 20
		}
		ui.Text(widgets.Elide(ui, rw.Text, lw-24), mathx.Vec2{X: fr.X + 20, Y: ui.TextY(fr)}, th.Text)
		if rw.Before != "" || rw.After != "" {
			vx := fr.X + lw
			vw := fr.X + fr.W - vx
			before := widgets.Elide(ui, rw.Before, vw*0.45)
			ui.Text(before, mathx.Vec2{X: vx, Y: ui.TextY(fr)}, th.Agent.DiffRemove)
			ax := vx + ui.TextSize(before).X + 6
			ui.Text("→", mathx.Vec2{X: ax, Y: ui.TextY(fr)}, th.TextDisabled)
			ax += ui.TextSize("→").X + 6
			ui.Text(widgets.Elide(ui, rw.After, max(0, fr.X+fr.W-ax)), mathx.Vec2{X: ax, Y: ui.TextY(fr)}, th.Agent.DiffAdd)
		}
		y += row
		n++
	}
	if hidden == 0 {
		return false
	}
	mr := mathx.Rect{X: r.X, Y: y, W: r.W, H: row}
	label := i18n.Tr("editor.agent.v_more", "+{v} more", i18n.Str("v", itoa(hidden)))
	id := ui.ID("diffmore")
	if ui.Clicked(id, mathx.Rect{X: mr.X, Y: mr.Y, W: ui.TextSize(label).X + 8, H: row}) {
		*all = true
		return true
	}
	col := th.TextDim
	if ui.Hot() == id {
		col = th.Accent
	}
	ui.Text(label, mathx.Vec2{X: mr.X + 2, Y: ui.TextY(mr)}, col)
	return false
}

// codeDiff draws the lines of one side of a script edit, each marked − or +; one row per line, as diffLines counts them.
func codeDiff(ui *core.Context, s, mark string, col mathx.Color, r mathx.Rect, all bool) float32 {
	th := &ui.Theme
	row := th.RowHeight
	y := r.Y
	if s == "" {
		return y
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if !all {
		lines = lines[:min(len(lines), 6)]
	}
	mono := ui.MonoFont()
	ui.PushFont(mono)
	bg := col
	bg.A = 0.12
	for _, l := range lines {
		lr := mathx.Rect{X: r.X, Y: y, W: r.W, H: row}
		ui.Rect(lr, bg)
		ui.Text(mark, mathx.Vec2{X: lr.X + 4, Y: y + (row-mono.LineHeight)/2}, col)
		ui.Text(widgets.Elide(ui, l, lr.W-24), mathx.Vec2{X: lr.X + 18, Y: y + (row-mono.LineHeight)/2}, th.Code.Plain)
		y += row
	}
	ui.PopFont()
	return y
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
