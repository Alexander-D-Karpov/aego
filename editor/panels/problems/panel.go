package problems

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/props"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/engine/i18n"
	"aego/scene"
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
	src       services.Problems
	nav       services.Navigation
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
	Scene     func(path string) *scene.Scene
	// Ask prefills the Agent panel; nil hides Ask Agent to Fix.
	Ask    func(prompt string)
	menu   context.ScriptProblem
	menuID core.ID
}

func New(src services.Problems, nav services.Navigation) *Panel {
	return &Panel{src: src, nav: nav, showErr: true, showWarn: true, collapsed: map[string]bool{}}
}

func (p *Panel) ID() string {
	return workspace.Problems
}

func (p *Panel) Title() string {
	return i18n.Tr("editor.problems.title", "Problems")
}

func (p *Panel) Focus(filter string) {
	p.filter = filter
	p.scroll = 0
}

func key(pr *context.ScriptProblem) string {
	return pr.Category + ":" + pr.Path + ":" + strconv.Itoa(pr.Line) + ":" + strconv.Itoa(pr.Col) + ":" + pr.Node.String() + ":" + pr.Code
}

func where(pr *context.ScriptProblem) string {
	if !pr.Node.IsZero() {
		return i18n.Tr("editor.problems.node_v", "node {v}", i18n.Str("v", pr.Node.String()[:8]))
	}
	return strconv.Itoa(pr.Line) + ":" + strconv.Itoa(pr.Col)
}

func (p *Panel) node(pr *context.ScriptProblem) (icon, text, tip string) {
	if pr.Node.IsZero() || p.Scene == nil {
		return "", "", ""
	}
	if s := p.Scene(pr.Path); s != nil {
		return props.NodeLabel(s, s.Handle(pr.Node), false)
	}
	return "", "", ""
}

func (p *Panel) refresh() (int, int) {
	p.list, _ = p.src.ScriptProblems(p.list)
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
		if q != "" && !strings.Contains(strings.ToLower(pr.Category+" "+pr.Path+" "+pr.Package+" "+pr.Code+" "+pr.Message), q) {
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
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		return a.Node.String() < b.Node.String()
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

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	errs, warns := p.refresh()
	p.menuID = ui.ID("menu")
	bar := ui.Cursor().Next(0)
	x, right := bar.X, bar.X+bar.W
	place := func(w float32) mathx.Rect {
		rc := mathx.Rect{X: x, Y: bar.Y, W: w, H: bar.H}
		x += w + 4
		return rc
	}
	errLabel := i18n.Tr("editor.problems.errors_n", "Errors {n}", i18n.Int("n", int64(errs)))
	warnLabel := i18n.Tr("editor.problems.warnings_n", "Warnings {n}", i18n.Int("n", int64(warns)))
	widgets.ToggleButton(ui, ui.ID("err"), place(max(96, ui.TextSize(errLabel).X+20)), errLabel, &p.showErr)
	widgets.ToggleButton(ui, ui.ID("warn"), place(max(116, ui.TextSize(warnLabel).X+20)), warnLabel, &p.showWarn)
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
		msg := i18n.Tr("editor.problems.no_problems", "No problems")
		if len(p.list) > 0 {
			msg = i18n.Tr("editor.problems.no_match", "No problems match the filter")
		}
		ui.TextIn(list, msg, th.TextDisabled, core.AlignCenter)
	}
	widgets.VirtualList(ui, ui.ID("list"), list, len(p.rows), th.RowHeight, &p.scroll, func(i int, rr mathx.Rect) {
		p.drawRow(ui, i, rr)
	})
	p.problemMenu(ui)
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
	loc := where(sel)
	if _, name, tip := p.node(sel); name != "" {
		loc = tip
	}
	info := sel.Path + "   " + loc
	if sel.Script() {
		info = sel.Path + ":" + loc + "   " + i18n.Tr("editor.problems.package", "package {name}", i18n.Str("name", sel.Package))
	} else {
		info += "   " + categoryTitle(sel.Category)
	}
	widgets.LabelDim(ui, info)
	ui.PopCursor()
	ui.PopClip()
}

func (p *Panel) drawRow(ui *core.Context, i int, rr mathx.Rect) {
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
			counts = i18n.TrN("editor.problems.errors", int64(rw.errs), "{n} error", "{n} errors")
		}
		if rw.warns > 0 {
			if counts != "" {
				counts += ", "
			}
			counts += i18n.TrN("editor.problems.warnings", int64(rw.warns), "{n} warning", "{n} warnings")
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
		p.nav.Open(pr.Location())
	}
	if p.Ask != nil && ui.RightClicked(id, rr) {
		p.selKey, p.menu = k, *pr
		widgets.OpenContextMenu(ui, p.menuID)
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
	widgets.IconAt(ui, mathx.Rect{X: x, Y: rr.Y + (rr.H-16)/2, W: 16, H: 16}, icon, col)
	x += 20
	code := pr.Code
	if !pr.Script() {
		code = categoryTitle(pr.Category) + " " + pr.Code
	}
	loc := where(pr)
	nicon, name, tip := p.node(pr)
	if name != "" {
		loc = name
	}
	lw := ui.TextSize(loc).X + 12
	if nicon != "" {
		lw += 20
	}
	ui.PushClip(mathx.Rect{X: x, Y: rr.Y, W: max(0, rr.X+rr.W-x-lw), H: rr.H})
	ui.Text(code, mathx.Vec2{X: x, Y: ui.TextY(rr)}, th.TextDim)
	ui.Text(pr.Message, mathx.Vec2{X: x + ui.TextSize(code).X + 10, Y: ui.TextY(rr)}, th.Text)
	ui.PopClip()
	ui.TextIn(rr, loc, th.TextDim, core.AlignRight)
	if nicon != "" {
		widgets.IconAt(ui, mathx.Rect{X: rr.X + rr.W - lw + 6, Y: rr.Y + (rr.H-16)/2, W: 16, H: 16}, nicon, th.TextDim)
	}
	help := pr.Help
	if tip != "" {
		help = strings.TrimSpace(tip + "\n" + help)
	}
	widgets.Tooltip(ui, id, help)
}

func askPrompt(pr *context.ScriptProblem) string {
	code := pr.Code
	if !pr.Script() {
		code = pr.Category + " " + pr.Code
	}
	return "Fix this problem: " + code + " " + pr.Message + " (" + pr.Path + " " + where(pr) + "). "
}

func (p *Panel) problemMenu(ui *core.Context) {
	if !widgets.ContextMenu(ui, p.menuID) {
		return
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.problems.ask_agent_to_fix", "Ask Agent to Fix"), "", "panel.agent", true, false) {
		p.Ask(askPrompt(&p.menu))
	}
	widgets.EndContextMenu(ui)
}

func categoryTitle(c string) string {
	switch c {
	case context.ProblemScene:
		return i18n.Tr("editor.problems.category.scene", "scene")
	case context.ProblemScript:
		return i18n.Tr("editor.problems.category.script", "script")
	}
	return c
}
