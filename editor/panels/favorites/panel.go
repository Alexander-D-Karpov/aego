package favorites

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/editor/core/context"
	favs "aego/editor/core/favorites"
	"aego/editor/core/services"
	"aego/ui/core"
	"aego/ui/widgets"
)

const ID = "favorites"

type Deps struct {
	Store   *favs.Store
	Env     favs.Env
	Notices services.Notices
}

type Panel struct {
	d      Deps
	scroll float32
}

func New(d Deps) *Panel {
	return &Panel{d: d}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	return "Favorites"
}

func (p *Panel) Icon() string {
	return "panel.favorites"
}

func (p *Panel) icon(it favs.Item, fallback string) string {
	if it.Kind != favs.Asset || p.d.Env.Pipeline == nil {
		return fallback
	}
	e, ok := p.d.Env.Pipeline.Entry(it.Asset)
	if !ok {
		return fallback
	}
	switch e.Kind {
	case asset.ArtScene:
		return "file.scene"
	case asset.ArtPrefab:
		return "file.prefab"
	case asset.ArtScript:
		return "file.script"
	case asset.ArtData:
		return "file.data"
	case asset.ArtTable:
		return "file.table"
	case asset.ArtMesh:
		return "file.mesh"
	}
	return "file.texture"
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	if p.d.Store == nil {
		return
	}
	items := p.d.Store.Items()
	if len(items) == 0 {
		widgets.LabelWrapped(ui, "Nothing here yet. Add assets from their menu, nodes from the Hierarchy menu and actions with Ctrl+D in the command palette.", true)
		return
	}
	top := ui.Cursor().Y
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	var remove, open = -1, -1
	widgets.VirtualList(ui, ui.ID("list"), list, len(items), th.RowHeight+2, &p.scroll, func(i int, row mathx.Rect) {
		it := items[i]
		label, detail, icon, ok := favs.Describe(p.d.Env, it)
		icon = p.icon(it, icon)
		id := core.IDSeq(ui.ID("row"), i)
		xr := mathx.Rect{X: row.X + row.W - row.H, Y: row.Y, W: row.H, H: row.H}
		body := mathx.Rect{X: row.X, Y: row.Y, W: row.W - row.H - 2, H: row.H}
		if ui.Clicked(id, body) {
			open = i
		}
		if ui.Hot() == id {
			ui.Rect(row, th.WidgetHot)
		}
		col, dim := th.Text, th.TextDim
		if !ok {
			col, dim = th.TextDisabled, th.TextDisabled
		}
		widgets.IconAt(ui, mathx.Rect{X: row.X + 4, Y: row.Y + (row.H-16)/2, W: 16, H: 16}, icon, dim)
		ui.PushClip(body)
		ui.Text(label, mathx.Vec2{X: row.X + 26, Y: ui.TextY(row)}, col)
		ui.Text(detail, mathx.Vec2{X: row.X + 38 + ui.TextSize(label).X, Y: ui.TextY(row)}, dim)
		ui.PopClip()
		if widgets.IconButtonRect(ui, core.IDSeq(id, 1), xr, "ui.close") {
			remove = i
		}
	})
	if open >= 0 {
		if err := favs.Open(p.d.Env, items[open]); err != nil && p.d.Notices != nil {
			p.d.Notices.Notify(context.LevelInfo, err.Error())
		}
	}
	if remove >= 0 {
		if err := p.d.Store.Remove(items[remove]); err != nil && p.d.Notices != nil {
			p.d.Notices.Notify(context.LevelError, err.Error())
		}
	}
}
