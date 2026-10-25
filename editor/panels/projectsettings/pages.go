package projectsettings

import (
	"aego/asset"
	"aego/core/ids"
	"aego/engine/i18n"
	"aego/project"
	"aego/ui/core"
	"aego/ui/widgets"
)

type page struct {
	id       string
	title    string
	order    int
	section  string
	owner    string
	project  func(*core.Context, *project.Project) bool
	draw     func(any) bool
	drawUI   func(*core.Context, any) bool
	settings func(*core.Context) bool
}

func (p *Panel) builtin() []page {
	out := []page{{id: "project", title: "Project", order: 0, project: p.drawProject}}
	if p.d.Settings != nil {
		out = append(out, page{id: "registry", title: "Settings Registry", order: 1000, settings: p.drawRegistry})
	}
	return out
}

func intField(ui *core.Context, label string, v *int, lo float64) bool {
	f := float64(*v)
	e := widgets.DragNumber(ui, ui.ID(label), widgets.Field(ui, label), &f, widgets.NumberOpts{Int: true, Speed: 1, Min: lo, HasMin: true})
	if e.Changed {
		*v = int(f)
	}
	return e.Changed
}

func textField(ui *core.Context, label string, v *string) bool {
	_, committed := widgets.TextFieldRect(ui, ui.ID(label), widgets.Field(ui, label), v, false)
	return committed
}

func (p *Panel) drawProject(ui *core.Context, pr *project.Project) bool {
	changed := textField(ui, i18n.Tr("editor.projectsettings.name", "Name"), &pr.Name)
	ui.TextIn(widgets.Field(ui, i18n.Tr("editor.projectsettings.module", "Module")), pr.Module, ui.Theme.TextDim, core.AlignLeft)
	if p.d.Pipeline != nil {
		var names []string
		var list []ids.AssetID
		cur := -1
		for _, a := range p.d.Pipeline.Assets() {
			if a.Kind != asset.ArtScene {
				continue
			}
			if a.ID == pr.MainScene {
				cur = len(names)
			}
			names = append(names, a.Path)
			list = append(list, a.ID)
		}
		if widgets.Combo(ui, ui.ID("main"), widgets.Field(ui, i18n.Tr("editor.projectsettings.startup_scene", "Startup Scene")), &cur, names, false) && cur >= 0 {
			pr.MainScene = list[cur]
			changed = true
		}
	}
	space := 0
	if pr.DefaultSpace == project.Space3D {
		space = 1
	}
	if widgets.Combo(ui, ui.ID("space"), widgets.Field(ui, i18n.Tr("editor.projectsettings.default_space", "Default Space")), &space, []string{"2D", "3D"}, false) {
		pr.DefaultSpace = project.Space2D
		if space == 1 {
			pr.DefaultSpace = project.Space3D
		}
		changed = true
	}
	widgets.Separator(ui)
	widgets.Label(ui, i18n.Tr("editor.projectsettings.window", "Window"))
	changed = intField(ui, i18n.Tr("editor.projectsettings.width", "Width"), &pr.Window.Width, 64) || changed
	changed = intField(ui, i18n.Tr("editor.projectsettings.height", "Height"), &pr.Window.Height, 64) || changed
	changed = textField(ui, i18n.Tr("editor.projectsettings.game_title", "Title"), &pr.Window.Title) || changed
	return changed
}
