package scenes

import (
	"path"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/assetfiles"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/engine/i18n"
	"aego/project"
	"aego/render/texture"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	ID    = "scenes"
	tileW = float32(150)
	thumb = float32(96)
)

type Previews interface {
	Get(info pipeline.AssetInfo) (texture.Region, bool)
}

type Deps struct {
	Assets   func() []pipeline.AssetInfo
	Previews Previews
	Files    *assetfiles.Files
	Docs     services.Documents
	Notices  services.Notices
	Project  *project.Project
	Reveal   func(rel string)
}

type Panel struct {
	d      Deps
	f      *services.Frame
	all    []pipeline.AssetInfo
	view   []int
	at     float64
	filter string
	sel    ids.AssetID
	menuOn pipeline.AssetInfo
	menu   core.ID
	scroll float32
}

func New(d Deps) *Panel {
	return &Panel{d: d, at: -10}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	return i18n.Tr("editor.scenes.title", "Scenes")
}

func (p *Panel) Icon() string {
	return "panel.scenes"
}

func (p *Panel) Refresh() {
	p.at = -10
}

func (p *Panel) startup(id ids.AssetID) bool {
	return p.d.Project != nil && p.d.Project.MainScene == id
}

func (p *Panel) refresh(now float64) {
	if now-p.at < 0.5 || p.d.Assets == nil {
		return
	}
	p.at = now
	p.all = p.all[:0]
	for _, a := range p.d.Assets() {
		if a.Kind == asset.ArtScene {
			p.all = append(p.all, a)
		}
	}
}

func (p *Panel) build() {
	q := strings.ToLower(strings.TrimSpace(p.filter))
	p.view = p.view[:0]
	for i := range p.all {
		if q == "" || strings.Contains(strings.ToLower(p.all[i].Path), q) {
			p.view = append(p.view, i)
		}
	}
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	p.f = f
	ui := f.UI
	th := &ui.Theme
	p.menu = ui.ID("menu")
	p.refresh(f.Now)
	widgets.SearchField(ui, ui.ID("filter"), ui.Cursor().Next(0), &p.filter)
	p.build()
	top := ui.Cursor().Y
	body := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	if len(p.view) == 0 {
		msg := i18n.Tr("editor.scenes.no_scenes_yet_file_new", "No scenes yet, File → New Scene creates one")
		if p.filter != "" {
			msg = i18n.Tr("editor.scenes.no_scenes_match", "No scenes match")
		}
		ui.TextIn(body, msg, th.TextDisabled, core.AlignCenter)
		p.contextMenu()
		return
	}
	cols := max(1, int(body.W/tileW))
	tileH := thumb + 2*th.RowHeight + 12
	rows := (len(p.view) + cols - 1) / cols
	widgets.VirtualList(ui, ui.ID("grid"), body, rows, tileH, &p.scroll, func(row int, rr mathx.Rect) {
		for c := 0; c < cols; c++ {
			i := row*cols + c
			if i >= len(p.view) {
				return
			}
			p.tile(&p.all[p.view[i]], mathx.Rect{X: rr.X + float32(c)*tileW + 2, Y: rr.Y + 2, W: tileW - 4, H: tileH - 4})
		}
	})
	p.contextMenu()
}

func (p *Panel) tile(a *pipeline.AssetInfo, r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	id := core.IDOf("scenes.tile/" + a.Path)
	if ui.Pressed(id, r) {
		p.sel = a.ID
		if ui.DoubleClicked(id) {
			p.open(a)
		}
	}
	if ui.RightClicked(id, r) {
		p.sel, p.menuOn = a.ID, *a
		widgets.OpenContextMenu(ui, p.menu)
	}
	switch {
	case p.sel == a.ID:
		ui.Rect(r, th.Selection)
	case ui.Hot() == id:
		ui.Rect(r, th.WidgetHot)
	}
	img := mathx.Rect{X: r.X + (r.W-thumb)/2, Y: r.Y + 4, W: thumb, H: thumb}
	ui.Rect(img, th.PanelAlt)
	var reg texture.Region
	ok := false
	if p.d.Previews != nil {
		reg, ok = p.d.Previews.Get(*a)
	}
	if ok {
		ui.List().Texture(reg, img, mathx.White)
	} else {
		widgets.IconAt(ui, mathx.Rect{X: img.X + thumb/2 - 16, Y: img.Y + thumb/2 - 16, W: 32, H: 32}, "file.scene", th.TextDim)
	}
	start := p.startup(a.ID)
	if start {
		widgets.IconAt(ui, mathx.Rect{X: img.X + img.W - 18, Y: img.Y + 2, W: 16, H: 16}, "play.start", th.Accent)
	}
	name := mathx.Rect{X: r.X + 2, Y: img.Y + img.H + 2, W: r.W - 4, H: th.RowHeight}
	dir := mathx.Rect{X: name.X, Y: name.Y + th.RowHeight, W: name.W, H: th.RowHeight}
	ui.PushClip(r)
	ui.TextIn(name, assetops.Stem(a.Path), th.Text, core.AlignCenter)
	ui.TextIn(dir, path.Dir(a.Path), th.TextDim, core.AlignCenter)
	ui.PopClip()
	tip := a.Path
	if start {
		tip += " · startup scene"
	}
	widgets.Tooltip(ui, id, tip)
}

func (p *Panel) notify(err error) {
	if err != nil && p.d.Notices != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
	}
}

func (p *Panel) open(a *pipeline.AssetInfo) {
	if p.d.Docs == nil {
		return
	}
	_, err := p.d.Docs.Documents().Open(a.Path)
	p.notify(err)
}

func (p *Panel) contextMenu() {
	ui := p.f.UI
	if !widgets.ContextMenu(ui, p.menu) {
		return
	}
	a := p.menuOn
	files := p.d.Files != nil
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.scenes.open", "Open"), "", "file.open", true, false) {
		p.open(&a)
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.scenes.set_as_startup", "Set as Startup"), "", "play.start", files && !p.startup(a.ID), p.startup(a.ID)) {
		p.d.Files.SetStartup(a.ID, a.Path)
	}
	widgets.Separator(ui)
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.scenes.rename", "Rename…"), "", "edit.rename", files, false) {
		p.rename(a)
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.scenes.duplicate", "Duplicate"), "", "edit.duplicate", files, false) {
		_, err := p.d.Files.Duplicate(a.ID)
		p.notify(err)
		p.Refresh()
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.scenes.delete", "Delete…"), "", "edit.delete", files, false) {
		p.remove(a)
	}
	widgets.Separator(ui)
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.scenes.show_in_assets", "Show in Assets"), "", "panel.assets", p.d.Reveal != nil, false) {
		p.d.Reveal(a.Path)
	}
	widgets.EndContextMenu(ui)
}

func (p *Panel) rename(a pipeline.AssetInfo) {
	p.d.Notices.Ask(context.Question{
		Title:   i18n.Tr("editor.scenes.rename_scene", "Rename scene"),
		Fields:  []string{i18n.Tr("editor.scenes.name", "Name")},
		Values:  []string{assetops.Stem(a.Path)},
		Buttons: []string{i18n.Tr("editor.scenes.rename_2", "Rename"), i18n.Tr("editor.scenes.cancel", "Cancel")},
		Submit: func(v []string) {
			name := strings.TrimSpace(v[0])
			if !assetops.ValidName(name) {
				p.notify(errInvalid(name))
				return
			}
			to := path.Join(path.Dir(a.Path), name+path.Ext(a.Path))
			if to == a.Path {
				return
			}
			p.notify(p.d.Files.Apply([]assetops.FileOp{{ID: a.ID, From: a.Path, To: to}}))
			p.Refresh()
		},
	})
}

func (p *Panel) remove(a pipeline.AssetInfo) {
	msg := i18n.Tr("editor.scenes.delete_path", "Delete {path}?", i18n.Str("path", a.Path))
	if p.startup(a.ID) {
		msg += " It is the startup scene; the project will have none until you pick another."
	}
	p.d.Notices.Ask(context.Question{
		Title:   i18n.Tr("editor.scenes.delete_scene", "Delete scene"),
		Message: msg,
		Buttons: []string{i18n.Tr("editor.scenes.delete_2", "Delete"), i18n.Tr("editor.scenes.cancel", "Cancel")},
		Then: func(choice int) {
			if choice != 0 {
				return
			}
			p.notify(p.d.Files.Delete([]ids.AssetID{a.ID}))
			p.sel = ids.AssetID{}
			p.Refresh()
		},
	})
}
