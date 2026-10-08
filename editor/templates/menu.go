package templates

import (
	"aego/core/ids"
	"aego/editor/context"
	"aego/ui/widgets"
)

func Menu(ctx *context.Editor, r *Registry, parent ids.NodeID, pl Place, onCreate func([]ids.NodeID)) {
	ui := ctx.UI
	enabled := ctx.Doc() != nil
	item := func(t Template) {
		if !widgets.MenuItem(ui, t.Name, "", enabled, false) {
			return
		}
		created, err := InstantiatePlace(ctx, t, parent, -1, pl)
		if err != nil {
			ctx.Notify(context.LevelError, err.Error())
			return
		}
		if onCreate != nil {
			onCreate(created)
		}
	}
	var nested []Template
	for _, t := range r.Root() {
		if !pl.Fits(t) {
			continue
		}
		if t.Category == Category3D && pl.At3 == nil {
			nested = append(nested, t)
			continue
		}
		item(t)
	}
	if len(nested) > 0 {
		ui.PushID("3d")
		if widgets.SubMenu(ui, Category3D, 200) {
			for _, t := range nested {
				item(t)
			}
			widgets.EndSubMenu(ui)
		}
		ui.PopID()
	}
	var mods []string
	for _, m := range r.Modules() {
		for _, t := range r.InModule(m) {
			if pl.Fits(t) {
				mods = append(mods, m)
				break
			}
		}
	}
	if len(mods) > 0 {
		widgets.Separator(ui)
	}
	for _, m := range mods {
		ui.PushID("mod/" + m)
		if widgets.SubMenu(ui, m, 220) {
			last := ""
			first := true
			for _, t := range r.InModule(m) {
				if !pl.Fits(t) {
					continue
				}
				if !first && t.Category != last {
					widgets.Separator(ui)
				}
				first = false
				last = t.Category
				item(t)
			}
			widgets.EndSubMenu(ui)
		}
		ui.PopID()
	}
}
