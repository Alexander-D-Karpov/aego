package problems

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/editor/context"
	"aego/editor/workspace"
	"aego/ui/core"
	"aego/ui/widgets"
)

type row struct {
	file  bool
	path  string
	errs  int
	warns int
	index int
}

type Panel struct {
	showErr   bool
	showWarn  bool
	filter    string
	list      []context.ScriptProblem
	view      []context.ScriptProblem
	rows      []row
	collapsed map[string]bool
	scroll    float32
	selKey    string
	Opened    int
}

func New() *Panel {
	return &Panel{showErr: true, showWarn: true, collapsed: map[string]bool{}}
}

func (p *Panel) ID() string {
	return workspace.Problems
}

func (p *Panel) Title() string {
	return "Problems"
}

func (p *Panel) Focus(filter string) {
	p.filter = filter
	p.scroll = 0
}

func key(pr *context.ScriptProblem) string {
	return pr.Path + ":" + strconv.Itoa(pr.Line) + ":" + strconv.Itoa(pr.Col) + ":" + pr.Code
}

func (p *Panel) refresh(ctx *context.Editor) (int, int) {
	p.list, _ = ctx.ScriptProblems(p.list)
	q := strings.ToLower(strings.TrimSpace(p.filter))
	errs, warns := 0, 0
	p.view = p.view[:0]
	for _, pr := range p.list {
		if pr.Error {
			errs++
		} else {
			warns++
		}
		if (pr.Error && !p.showErr) || (!pr.Error && !p.showWarn) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(pr.Path+" "+pr.Package+" "+pr.Code+" "+pr.Message), q) {
			continue
		}
		p.view = append(p.view, pr)
	}
	sort.SliceStable(p.view, func(i, j int) bool {
		a, b := p.view[i], p.view[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Col < b.Col
	})
	p.rows = p.rows[:0]
	for i := 0; i < len(p.view); {
		j := i
		fr := row{file: true, path: p.view[i].Path}
		for j < len(p.view) && p.view[j].Path == fr.path {
			if p.view[j].Error {
				fr.errs++
			} else {
				fr.warns++
			}
			j++
		}
		p.rows = append(p.rows, fr)
		if !p.collapsed[fr.path] {
			for k := i; k < j; k++ {
				p.rows = append(p.rows, row{path: fr.path, index: k})
			}
		}
		i = j
	}
	return errs, warns
}

func (p *Panel) selected() *context.ScriptProblem {
	if p.selKey == "" {
		return nil
	}
	for i := range p.view {
		if key(&p.view[i]) == p.selKey {
			return &p.view[i]
		}
	}
	return nil
}

func (p *Panel) Draw(ctx *context.Editor, r mathx.Rect) {
	ui := ctx.UI
	th := &ui.Theme
	errs, warns := p.refresh(ctx)
	bar := ui.Cursor().Next(0)
	x, right := bar.X, bar.X+bar.W
	place := func(w float32) mathx.Rect {
		rc := mathx.Rect{X: x, Y: bar.Y, W: w, H: bar.H}
		x += w + 4
		return rc
	}
	widgets.ToggleButton(ui, ui.ID("err"), place(96), "Errors "+strconv.Itoa(errs), &p.showErr)
	widgets.ToggleButton(ui, ui.ID("warn"), place(116), "Warnings "+strconv.Itoa(warns), &p.showWarn)
	if right-x >= 80 {
		widgets.SearchField(ui, ui.ID("search"), mathx.Rect{X: x, Y: bar.Y, W: right - x, H: bar.H}, &p.filter)
	}
	top := bar.Y + bar.H + th.Spacing
	sel := p.selected()
	detailH := float32(0)
	if sel != nil {
		detailH = min(th.RowHeight*4.5, (r.Y+r.H-top)/2)
	}
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top-detailH)}
	if len(p.rows) == 0 {
		msg := "No problems"
		if len(p.list) > 0 {
			msg = "No problems match the filter"
		}
		ui.TextIn(list, msg, th.TextDisabled, core.AlignCenter)
	}
	widgets.VirtualList(ui, ui.ID("list"), list, len(p.rows), th.RowHeight, &p.scroll, func(i int, rr mathx.Rect) {
		p.drawRow(ctx, i, rr)
	})
	if sel == nil || detailH <= 0 {
		return
	}
	dr := mathx.Rect{X: r.X, Y: r.Y + r.H - detailH, W: r.W, H: detailH}
	ui.Rect(dr, th.PanelAlt)
	ui.PushClip(dr)
	ui.PushCursor(core.Cursor{X: dr.X + th.Padding, Y: dr.Y + th.Padding, W: dr.W - 2*th.Padding, RowH: th.RowHeight, Pad: th.Padding})
	widgets.LabelWrapped(ui, sel.Code+"  "+sel.Message, false)
	if sel.Help != "" {
		widgets.LabelWrapped(ui, sel.Help, true)
	}
	widgets.LabelDim(ui, sel.Path+":"+strconv.Itoa(sel.Line)+":"+strconv.Itoa(sel.Col)+"   package "+sel.Package)
	ui.PopCursor()
	ui.PopClip()
}

func (p *Panel) drawRow(ctx *context.Editor, i int, rr mathx.Rect) {
	ui := ctx.UI
	th := &ui.Theme
	rw := p.rows[i]
	if rw.file {
		id := core.IDOf("problems.file/" + rw.path)
		if ui.Pressed(id, rr) {
			p.collapsed[rw.path] = !p.collapsed[rw.path]
		}
		if ui.Hot() == id {
			ui.Rect(rr, th.WidgetHot)
		}
		widgets.Chevron(ui, mathx.Rect{X: rr.X, Y: rr.Y, W: th.Indent, H: rr.H}, !p.collapsed[rw.path], th.TextDim)
		x := rr.X + th.Indent + 4
		name := path.Base(rw.path)
		ui.Text(name, mathx.Vec2{X: x, Y: ui.TextY(rr)}, th.Text)
		x += ui.TextSize(name).X + 10
		ui.Text(path.Dir(rw.path), mathx.Vec2{X: x, Y: ui.TextY(rr)}, th.TextDim)
		counts := ""
		if rw.errs > 0 {
			counts = strconv.Itoa(rw.errs) + " errors"
		}
		if rw.warns > 0 {
			if counts != "" {
				counts += ", "
			}
			counts += strconv.Itoa(rw.warns) + " warnings"
		}
		col := th.Warn
		if rw.errs > 0 {
			col = th.Error
		}
		ui.TextIn(rr, counts, col, core.AlignRight)
		return
	}
	pr := &p.view[rw.index]
	k := key(pr)
	id := core.IDSeq(ui.ID("row"), i)
	if ui.Pressed(id, rr) {
		p.selKey = k
		p.Opened++
		ctx.Open(pr.Location())
	}
	switch {
	case p.selKey == k:
		ui.Rect(rr, th.SelectionDim)
	case ui.Hot() == id:
		ui.Rect(rr, th.WidgetHot)
	}
	icon, col := "log.warn", th.Warn
	if pr.Error {
		icon, col = "log.error", th.Error
	}
	x := rr.X + th.Indent*1.5
	widgets.IconAt(ui, mathx.Rect{X: x, Y: rr.Y + (rr.H-14)/2, W: 14, H: 14}, icon, col)
	x += 20
	loc := strconv.Itoa(pr.Line) + ":" + strconv.Itoa(pr.Col)
	lw := ui.TextSize(loc).X + 12
	ui.PushClip(mathx.Rect{X: x, Y: rr.Y, W: max(0, rr.X+rr.W-x-lw), H: rr.H})
	ui.Text(pr.Code, mathx.Vec2{X: x, Y: ui.TextY(rr)}, th.TextDim)
	ui.Text(pr.Message, mathx.Vec2{X: x + ui.TextSize(pr.Code).X + 10, Y: ui.TextY(rr)}, th.Text)
	ui.PopClip()
	ui.TextIn(rr, loc, th.TextDim, core.AlignRight)
	widgets.Tooltip(ui, id, pr.Help)
}
