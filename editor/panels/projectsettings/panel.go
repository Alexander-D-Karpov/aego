package projectsettings

import (
	"sort"

	"aego/asset/pipeline"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/engine/i18n"
	"aego/engine/module"
	es "aego/engine/settings"
	"aego/engine/settings/settingsui"
	"aego/format/aetext"
	"aego/project"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	narrowPanel = 420
	ID          = "project_settings"
	CommandOpen = "file.project_settings"
)

type Deps struct {
	Project  *project.Project
	Sections *module.Sections
	Settings *es.Store
	Pages    func() []context.SettingsPage
	Pipeline *pipeline.Pipeline
	Notices  services.Notices
	OnApply  func() string
}

type Panel struct {
	d         Deps
	work      *project.Project
	drafts    map[string]any
	proj      map[es.Key]schema.Value
	projDirty bool
	tab       map[string]int
	filter    string
	ed        settingsui.Editor
	page      string
	dirty     bool
	err       string
	scroll    float32
	content   float32
	bar       widgets.ScrollState
	base      state
	undo      []state
	redo      []state
	gesture   bool
	touched   bool
	flash     string
	flashAt   float64
	flashGo   bool
}

func New(d Deps) *Panel {
	return &Panel{d: d, drafts: map[string]any{}, tab: map[string]int{}}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	if p.dirty {
		return i18n.Tr("editor.projectsettings.title", "Project Settings") + " *"
	}
	return i18n.Tr("editor.projectsettings.title", "Project Settings")
}

func (p *Panel) Show(page string) {
	if page != "" {
		p.page = page
		p.scroll = 0
	}
}

func clone(src *project.Project) *project.Project {
	c := *src
	c.Extra = make([]*aetext.Node, len(src.Extra))
	for i, b := range src.Extra {
		c.Extra[i] = b.Clone()
	}
	c.Settings = src.Settings.Clone()
	return &c
}

func (p *Panel) pages() []page {
	out := p.builtin()
	var custom []context.SettingsPage
	if p.d.Pages != nil {
		custom = p.d.Pages()
	}
	withSection := map[string]bool{}
	for _, sec := range p.d.Sections.List() {
		pg := page{id: sec.ID, title: sec.Title, order: sec.Order, section: sec.ID, owner: sec.Owner}
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
			withSection[sec.Owner] = true
		}
	}
	for _, o := range p.projectOwners() {
		if withSection[o.owner] {
			continue
		}
		owner := o.owner
		out = append(out, page{id: "defaults/" + owner, title: o.title, order: 500, owner: owner,
			settings: func(ui *core.Context) bool { return p.drawDefaults(ui, owner) }})
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
	p.proj = map[es.Key]schema.Value{}
	if p.d.Settings != nil {
		p.proj = p.d.Settings.ProjectValues()
	}
	p.dirty, p.projDirty, p.err = false, false, ""
	p.undo, p.redo, p.gesture = nil, nil, false
	p.base = p.capture()
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
	st := p.d.Settings
	if p.projDirty && st != nil {
		next.Settings = st.ProjectNode(p.proj)
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
	if p.projDirty && st != nil {
		if err := st.SetProject(p.proj); err != nil {
			p.err = err.Error()
			return
		}
	}
	*p.d.Project = *next
	p.reset()
	msg := i18n.Tr("editor.projectsettings.saved_project_aego_settings_apply", "Saved project.aego, settings apply on the next Play")
	if p.d.OnApply != nil {
		if note := p.d.OnApply(); note != "" {
			msg = i18n.Tr("editor.projectsettings.saved_project_aego_note", "Saved project.aego, {note}", i18n.Str("note", note))
		}
	}
	p.d.Notices.Notify(context.LevelInfo, msg)
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	if p.d.Project == nil {
		widgets.LabelDim(ui, i18n.Tr("editor.projectsettings.no_project", "No project"))
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
	if widgets.ButtonRect(ui, ui.ID("apply"), mathx.Rect{X: bar.X, Y: bar.Y, W: 90, H: bar.H}, i18n.Tr("editor.projectsettings.apply", "Apply")) && p.dirty {
		p.apply()
	}
	if widgets.ButtonRect(ui, ui.ID("revert"), mathx.Rect{X: bar.X + 96, Y: bar.Y, W: 90, H: bar.H}, i18n.Tr("editor.projectsettings.revert", "Revert")) {
		p.reset()
	}
	status := mathx.Rect{X: bar.X + 196, Y: bar.Y, W: max(0, bar.W-196), H: bar.H}
	switch {
	case p.err != "":
		ui.Text(widgets.Elide(ui, p.err, status.W), mathx.Vec2{X: status.X, Y: ui.TextY(status)}, th.Error)
	case p.dirty:
		ui.Text(widgets.Elide(ui, i18n.Tr("editor.projectsettings.unsaved_changes", "Unsaved changes"), status.W), mathx.Vec2{X: status.X, Y: ui.TextY(status)}, th.Warn)
	}
	top := bar.Y + bar.H + th.Spacing
	sw := th.ScrollbarW
	var body mathx.Rect
	if r.W < narrowPanel {
		sel := 0
		names := make([]string, len(pages))
		for i, pg := range pages {
			names[i] = pg.title
			if pg.id == p.page {
				sel = i
			}
		}
		row := mathx.Rect{X: r.X + th.Padding, Y: top, W: r.W - 2*th.Padding, H: th.RowHeight}
		if widgets.Combo(ui, ui.ID("pages"), row, &sel, names, false) && sel < len(pages) {
			p.page, p.scroll = pages[sel].id, 0
		}
		top += th.RowHeight + th.Spacing
		body = mathx.Rect{X: r.X, Y: top, W: max(0, r.W-sw-2), H: max(0, r.Y+r.H-top)}
	} else {
		left := mathx.Rect{X: r.X, Y: top, W: min(180, max(120, r.W*0.24)), H: max(0, r.Y+r.H-top)}
		p.nav(ui, left, pages)
		body = mathx.Rect{X: left.X + left.W + 8, Y: top, W: max(0, r.X+r.W-left.X-left.W-8-sw-2), H: left.H}
	}
	view := body.H - th.Padding*2
	maxOff := max(0, p.content-view)
	if ui.MouseOver(ui.ID("scroll"), body) {
		if w := ui.ScrollY(th.RowHeight * 3); w != 0 {
			p.scroll -= w
		}
	}
	p.scroll = mathx.Clamp(p.scroll, 0, maxOff)
	ui.PushClip(body)
	ui.PushCursor(core.Cursor{X: body.X + th.Padding, Y: body.Y + th.Padding - p.scroll, W: body.W - 2*th.Padding, RowH: th.RowHeight, Pad: th.Padding, Space: th.Spacing})
	start := ui.Cursor().Y
	if p.flash != "" {
		if p.flashAt == 0 {
			p.flashAt = ui.Time
		}
		ui.Flash, ui.FlashAt, ui.FlashSeen = p.flash, p.flashAt, false
	}
	ui.PushID(p.page)
	changed := false
	for _, pg := range pages {
		if pg.id != p.page {
			continue
		}
		changed = p.drawPage(ui, pg) || changed
	}
	ui.PopID()
	if p.flash != "" {
		if ui.FlashSeen && p.flashGo {
			p.scroll = mathx.Clamp(ui.FlashY-start-view/3, 0, max(0, ui.Cursor().Y-start-view))
			p.flashGo = false
		}
		if ui.Time-p.flashAt > 2 {
			p.flash = ""
		}
		ui.Flash = ""
	}
	p.content = ui.Cursor().Y - start
	ui.PopCursor()
	ui.PopClip()
	track := mathx.Rect{X: body.X + body.W + 2, Y: body.Y, W: sw, H: body.H}
	widgets.ScrollBar(ui, ui.ID("vbar"), track, &p.scroll, p.content, view, &p.bar)
	if changed {
		p.touch()
	}
	p.settle(ui)
}

func (p *Panel) nav(ui *core.Context, r mathx.Rect, pages []page) {
	th := &ui.Theme
	ui.Rect(r, th.PanelAlt)
	ui.PushClip(r)
	y := r.Y + th.Padding
	for i, pg := range pages {
		row := mathx.Rect{X: r.X + 4, Y: y, W: r.W - 8, H: th.RowHeight}
		y += th.RowHeight + 2
		id := ui.IDInt(1000 + i)
		if ui.Clicked(id, row) {
			p.page, p.scroll = pg.id, 0
		}
		sel := pg.id == p.page
		col := th.TextDim
		switch {
		case sel:
			ui.RoundRect(row, th.Radius.Menu, th.SelectionDim)
			ui.Rect(mathx.Rect{X: row.X, Y: row.Y + 3, W: 2, H: row.H - 6}, th.Accent)
			col = th.Text
		case ui.Hot() == id:
			ui.RoundRect(row, th.Radius.Menu, th.WidgetHot)
			col = th.Text
		}
		ui.Text(widgets.Elide(ui, pg.title, row.W-14), mathx.Vec2{X: row.X + 10, Y: ui.TextY(row)}, col)
		if ui.TextSize(pg.title).X > row.W-14 {
			widgets.Tooltip(ui, id, pg.title)
		}
	}
	ui.PopClip()
}

func (p *Panel) drawPage(ui *core.Context, pg page) bool {
	if pg.settings != nil {
		return pg.settings(ui)
	}
	if pg.owner != "" && p.hasDefaults(pg.owner) && p.tabs(ui, pg.id) == 1 {
		return p.drawDefaults(ui, pg.owner)
	}
	switch {
	case pg.project != nil:
		return pg.project(ui, p.work)
	case pg.draw != nil:
		if d := p.draft(pg.section); d != nil {
			return pg.draw(d)
		}
	case pg.drawUI != nil:
		if d := p.draft(pg.section); d != nil {
			return pg.drawUI(ui, d)
		}
	}
	return false
}

func (p *Panel) Focus(page, field string) {
	p.Show(page)
	p.flash, p.flashAt, p.flashGo = field, 0, true
}

// Refresh reloads the panel after project.aego changed outside it, unless the user has unapplied changes.
func (p *Panel) Refresh() {
	if p.work == nil {
		return
	}
	if p.dirty {
		p.d.Notices.Notify(context.LevelWarn, i18n.Note("editor.projectsettings.project_aego_changed_while_project", "project.aego changed while Project Settings has unapplied changes: Apply overwrites it, Revert loads it"))
		return
	}
	p.reset()
}
