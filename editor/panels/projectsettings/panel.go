package projectsettings

import (
	"slices"
	"sort"

	"aego/asset/pipeline"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/project"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	ID          = "project_settings"
	CommandOpen = "file.project_settings"
)

type Deps struct {
	Project  *project.Project
	Sections *module.Sections
	Pages    func() []context.SettingsPage
	Pipeline *pipeline.Pipeline
	Notices  services.Notices
}

type Panel struct {
	d       Deps
	work    *project.Project
	drafts  map[string]any
	page    string
	dirty   bool
	err     string
	scroll  float32
	content float32
}

func New(d Deps) *Panel {
	return &Panel{d: d, drafts: map[string]any{}}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	if p.dirty {
		return "Project Settings *"
	}
	return "Project Settings"
}

func (p *Panel) Show(page string) {
	if page != "" {
		p.page = page
		p.scroll = 0
	}
}

func clone(src *project.Project) *project.Project {
	c := *src
	c.Input.Axes = slices.Clone(src.Input.Axes)
	c.Extra = make([]*aetext.Node, len(src.Extra))
	for i, b := range src.Extra {
		c.Extra[i] = b.Clone()
	}
	return &c
}

func (p *Panel) pages() []page {
	out := p.builtin()
	var custom []context.SettingsPage
	if p.d.Pages != nil {
		custom = p.d.Pages()
	}
	for _, sec := range p.d.Sections.List() {
		pg := page{id: sec.ID, title: sec.Title, order: sec.Order, section: sec.ID}
		for _, sp := range custom {
			if sp.Section == sec.ID {
				pg.draw = sp.Draw
			}
		}
		if pg.draw == nil && sec.Type != nil {
			s := sec
			pg.drawUI = func(ui *core.Context, d any) bool { return auto(ui, s, d) }
		}
		if pg.draw != nil || pg.drawUI != nil {
			out = append(out, pg)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].order < out[j].order })
	return out
}

func (p *Panel) draft(id string) any {
	if d, ok := p.drafts[id]; ok {
		return d
	}
	d, err := p.d.Sections.Draft(id)
	if err != nil {
		p.err = err.Error()
		return nil
	}
	p.drafts[id] = d
	return d
}

func (p *Panel) reset() {
	p.work = clone(p.d.Project)
	p.drafts = map[string]any{}
	p.dirty, p.err = false, ""
}

func (p *Panel) apply() {
	next := clone(p.work)
	for id, d := range p.drafts {
		n, err := p.d.Sections.Node(id, d)
		if err != nil {
			p.err = err.Error()
			return
		}
		sec, _ := p.d.Sections.Desc(id)
		next.ReplaceBlock(n, sec.Heads()...)
	}
	if err := next.Save(); err != nil {
		p.err = err.Error()
		return
	}
	for id, d := range p.drafts {
		if err := p.d.Sections.Set(id, d); err != nil {
			p.err = err.Error()
			return
		}
	}
	*p.d.Project = *next
	p.reset()
	p.d.Notices.Notify(context.LevelInfo, "Saved project.aego, settings apply on the next Play")
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	if p.d.Project == nil {
		widgets.LabelDim(ui, "No project")
		return
	}
	if p.work == nil {
		p.reset()
	}
	pages := p.pages()
	if p.page == "" {
		p.page = pages[0].id
	}
	bar := ui.Cursor().Next(0)
	if widgets.ButtonRect(ui, ui.ID("apply"), mathx.Rect{X: bar.X, Y: bar.Y, W: 90, H: bar.H}, "Apply") && p.dirty {
		p.apply()
	}
	if widgets.ButtonRect(ui, ui.ID("revert"), mathx.Rect{X: bar.X + 96, Y: bar.Y, W: 90, H: bar.H}, "Revert") {
		p.reset()
	}
	status := mathx.Rect{X: bar.X + 196, Y: bar.Y, W: max(0, bar.W-196), H: bar.H}
	ui.PushClip(status)
	switch {
	case p.err != "":
		ui.Text(p.err, mathx.Vec2{X: status.X, Y: ui.TextY(status)}, th.Error)
	case p.dirty:
		ui.Text("Unsaved changes", mathx.Vec2{X: status.X, Y: ui.TextY(status)}, th.Warn)
	}
	ui.PopClip()
	top := bar.Y + bar.H + th.Spacing
	left := mathx.Rect{X: r.X, Y: top, W: min(170, r.W*0.3), H: max(0, r.Y+r.H-top)}
	ui.Rect(left, th.PanelAlt)
	ui.PushCursor(core.Cursor{X: left.X + th.Padding, Y: left.Y + th.Padding, W: left.W - 2*th.Padding, RowH: th.RowHeight, Pad: th.Padding, Space: th.Spacing})
	for i, pg := range pages {
		if widgets.Selectable(ui, ui.IDInt(1000+i), pg.title, pg.id == p.page) {
			p.page, p.scroll = pg.id, 0
		}
	}
	ui.PopCursor()
	body := mathx.Rect{X: left.X + left.W + 8, Y: top, W: max(0, r.X+r.W-left.X-left.W-8), H: left.H}
	if ui.MouseOver(ui.ID("scroll"), body) {
		if w := ui.WheelY(); w != 0 {
			p.scroll -= w * th.RowHeight * 3
		}
	}
	p.scroll = mathx.Clamp(p.scroll, 0, max(0, p.content-body.H+th.Padding*2))
	ui.PushClip(body)
	ui.PushCursor(core.Cursor{X: body.X + th.Padding, Y: body.Y + th.Padding - p.scroll, W: body.W - 2*th.Padding, RowH: th.RowHeight, Pad: th.Padding, Space: th.Spacing})
	start := ui.Cursor().Y
	ui.PushID(p.page)
	for _, pg := range pages {
		if pg.id != p.page {
			continue
		}
		switch {
		case pg.project != nil:
			if pg.project(ui, p.work) {
				p.dirty = true
			}
		case pg.draw != nil:
			if d := p.draft(pg.section); d != nil && pg.draw(d) {
				p.dirty = true
			}
		case pg.drawUI != nil:
			if d := p.draft(pg.section); d != nil && pg.drawUI(ui, d) {
				p.dirty = true
			}
		}
	}
	ui.PopID()
	p.content = ui.Cursor().Y - start
	ui.PopCursor()
	ui.PopClip()
}
