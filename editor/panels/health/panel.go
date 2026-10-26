package health

import (
	"image/color"
	"strconv"
	"time"

	"aego/asset/pipeline"
	"aego/core/mathx"
	core "aego/editor/core/health"
	"aego/editor/core/services"
	"aego/editor/core/tasks"
	"aego/engine/i18n"
	"aego/engine/problems"
	"aego/project"
	uicore "aego/ui/core"
	"aego/ui/widgets"
)

const ID = "project_health"

type Deps struct {
	Pipeline *pipeline.Pipeline
	Project  *project.Project
	Docs     services.Documents
	Tasks    *tasks.Queue
	Problems func() []problems.Diagnostic
	Missing  func() int
	Cameras  func() []string
	// Palette is the art palette of project.aego; sprites outside it are reported.
	Palette      func() []color.NRGBA
	Exclude      func() []string
	TextureLimit int
	ShowMissing  func()
	ShowProblems func()
	Reveal       func(rel string)
	Export       func() func() []core.Item
}

type Panel struct {
	d       Deps
	items   []core.Item
	extra   []core.Item
	at      time.Time
	open    map[string]bool
	running *tasks.Task
	scroll  float32
}

func New(d Deps) *Panel {
	return &Panel{d: d, open: map[string]bool{}}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	return i18n.Tr("editor.health.title", "Project Health")
}

func (p *Panel) Icon() string {
	return "panel.health"
}

func (p *Panel) Run() {
	env := core.Env{Pipeline: p.d.Pipeline, Project: p.d.Project}
	if p.d.Problems != nil {
		env.Problems = p.d.Problems()
	}
	if p.d.Missing != nil {
		env.Missing = p.d.Missing()
	}
	if p.d.Exclude != nil {
		env.Exclude = p.d.Exclude()
	}
	p.items, p.at = core.Run(env), time.Now()
	if p.running != nil || p.d.Tasks == nil {
		return
	}
	var cams []string
	if p.d.Cameras != nil {
		cams = p.d.Cameras()
	}
	var exp func() []core.Item
	if p.d.Export != nil {
		exp = p.d.Export()
	}
	var pal []color.NRGBA
	if p.d.Palette != nil {
		pal = p.d.Palette()
	}
	var found []core.Item
	pl, limit := p.d.Pipeline, p.d.TextureLimit
	p.running = p.d.Tasks.Work(tasks.Spec{Title: i18n.Tr("editor.health.check_textures_scenes_and_export", "Check textures, scenes and export"), Kind: "health"}, func(*tasks.Task) error {
		found = core.Scan(pl, cams, limit, pal)
		if exp != nil {
			found = append(found, exp()...)
		}
		return nil
	}).Then(func(t *tasks.Task) {
		p.running = nil
		if t.State() == tasks.Done {
			p.extra = found
		}
	})
}

func (p *Panel) all() []core.Item {
	return append(append([]core.Item(nil), p.items...), p.extra...)
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	if p.at.IsZero() {
		p.Run()
	}
	head := ui.Cursor().Next(0)
	status := i18n.Tr("editor.health.checked_at_v", "Checked at {v}", i18n.Str("v", p.at.Format("15:04:05")))
	if p.running != nil {
		status += ", scanning textures…"
	}
	ui.Text(status, mathx.Vec2{X: head.X, Y: ui.TextY(head)}, th.TextDim)
	bw := ui.TextSize(i18n.Tr("editor.health.run_checks", "Run Checks")).X + 16
	if widgets.ButtonRect(ui, ui.ID("run"), mathx.Rect{X: head.X + head.W - bw, Y: head.Y, W: bw, H: head.H}, i18n.Tr("editor.health.run_checks", "Run Checks")) {
		p.Run()
	}
	p.loadProfile(ui)
	items := p.all()
	if len(items) == 0 {
		widgets.LabelDim(ui, i18n.Tr("editor.health.no_problems_found", "No problems found"))
		return
	}
	type line struct {
		item int
		path int
	}
	var lines []line
	for i, it := range items {
		lines = append(lines, line{item: i, path: -1})
		if p.open[it.Code+it.Title] {
			for j := range it.Paths[:min(len(it.Paths), 200)] {
				lines = append(lines, line{item: i, path: j})
			}
		}
	}
	top := ui.Cursor().Y
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	widgets.VirtualList(ui, ui.ID("rows"), list, len(lines), th.RowHeight+2, &p.scroll, func(i int, rr mathx.Rect) {
		ln := lines[i]
		it := items[ln.item]
		if ln.path >= 0 {
			rel := core.PathOf(it.Paths[ln.path])
			id := uicore.IDSeq(ui.ID("path"), i)
			if ui.Clicked(id, rr) && p.d.Reveal != nil {
				p.d.Reveal(rel)
			}
			if ui.Hot() == id {
				ui.Rect(rr, th.WidgetHot)
			}
			ui.PushClip(rr)
			ui.Text(it.Paths[ln.path], mathx.Vec2{X: rr.X + 24 + th.Indent, Y: ui.TextY(rr)}, th.TextDim)
			ui.PopClip()
			return
		}
		p.itemRow(ui, it, i, rr)
	})
}

func (p *Panel) itemRow(ui *uicore.Context, it core.Item, i int, rr mathx.Rect) {
	th := &ui.Theme
	key := it.Code + it.Title
	icon, col := "log.info", th.TextDim
	switch it.Severity {
	case core.Error:
		icon, col = "log.error", th.Error
	case core.Warning:
		icon, col = "status.warning", th.Warn
	}
	id := uicore.IDSeq(ui.ID("item"), i)
	btn := ""
	switch it.Action {
	case core.ActionMissing:
		btn = i18n.Tr("editor.health.resolve", "Resolve…")
	case core.ActionProblems:
		btn = i18n.Tr("editor.health.show", "Show")
	}
	bw := float32(0)
	if btn != "" {
		bw = ui.TextSize(btn).X + 16
		if widgets.ButtonRect(ui, uicore.IDSeq(id, 1), mathx.Rect{X: rr.X + rr.W - bw, Y: rr.Y + 1, W: bw, H: rr.H - 2}, btn) {
			switch it.Action {
			case core.ActionMissing:
				if p.d.ShowMissing != nil {
					p.d.ShowMissing()
				}
			case core.ActionProblems:
				if p.d.ShowProblems != nil {
					p.d.ShowProblems()
				}
			}
		}
	}
	body := mathx.Rect{X: rr.X, Y: rr.Y, W: rr.W - bw - 4, H: rr.H}
	if len(it.Paths) > 0 && ui.Clicked(id, body) {
		p.open[key] = !p.open[key]
	}
	if len(it.Paths) > 0 {
		widgets.Chevron(ui, mathx.Rect{X: rr.X, Y: rr.Y, W: th.Indent, H: rr.H}, p.open[key], th.TextDim)
	}
	widgets.IconAt(ui, mathx.Rect{X: rr.X + th.Indent, Y: rr.Y + (rr.H-16)/2, W: 16, H: 16}, icon, col)
	ui.PushClip(body)
	ui.Text(it.Title, mathx.Vec2{X: rr.X + th.Indent + 20, Y: ui.TextY(rr)}, th.Text)
	ui.TextIn(body, it.Code, th.TextDisabled, uicore.AlignRight)
	ui.PopClip()
}

func (p *Panel) loadProfile(ui *uicore.Context) {
	d := p.d.Docs.Doc()
	if d == nil || d.Load.Took == 0 {
		return
	}
	deps, missing := 0, 0
	if id, ok := p.d.Pipeline.Lookup(d.Path); ok {
		if e, ok := p.d.Pipeline.Entry(id); ok {
			deps = len(e.Deps)
			for _, dep := range e.Deps {
				if _, ok := p.d.Pipeline.PathOf(dep); !ok {
					missing++
				}
			}
		}
	}
	text := i18n.Tr("editor.health.path_opened_in_formatint_ms", "{path}: opened in {formatint} ms, {nodes} node(s), {deps} asset(s)", i18n.Str("path", d.Path), i18n.Str("formatint", strconv.FormatInt(d.Load.Took.Milliseconds(), 10)), i18n.Str("nodes", strconv.Itoa(d.Load.Nodes)), i18n.Str("deps", strconv.Itoa(deps)))
	if missing > 0 {
		text += i18n.Tr("editor.health.missing_missing", ", {missing} missing", i18n.Str("missing", strconv.Itoa(missing)))
	}
	widgets.LabelDim(ui, text)
}
