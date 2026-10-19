package templates

import (
	"slices"

	"aego/core/ids"
	"aego/editor/core/context"
	"aego/scene"
	"aego/ui/widgets"
)

type Section struct {
	Title  string
	Icon   string
	Module bool
	Items  []Template
	Breaks []int
}

func Layout(r *Registry, pl Place) []Section {
	root := Section{}
	nested := Section{Title: Category3D, Icon: "cat.3d"}
	var menus []Section
	index := map[string]int{}
	for _, t := range r.Root() {
		if !pl.Fits(t) {
			continue
		}
		switch {
		case t.Menu != "":
			i, ok := index[t.Menu]
			if !ok {
				i = len(menus)
				index[t.Menu] = i
				menus = append(menus, Section{Title: t.Menu, Icon: "cat." + t.Module})
			}
			menus[i].Items = append(menus[i].Items, t)
		case t.Space == scene.Space3D && pl.At3 == nil:
			nested.Items = append(nested.Items, t)
		default:
			root.Items = append(root.Items, t)
		}
	}
	out := []Section{root}
	if len(nested.Items) > 0 {
		out = append(out, nested)
	}
	out = append(out, menus...)
	for _, m := range r.Modules() {
		sec := Section{Title: m, Icon: "cat." + m, Module: true}
		last := ""
		for _, t := range r.InModule(m) {
			if !pl.Fits(t) {
				continue
			}
			if len(sec.Items) > 0 && t.Category != last {
				sec.Breaks = append(sec.Breaks, len(sec.Items))
			}
			last = t.Category
			sec.Items = append(sec.Items, t)
		}
		if len(sec.Items) > 0 {
			out = append(out, sec)
		}
	}
	return out
}

func Menu(ctx *context.Editor, r *Registry, parent ids.NodeID, pl Place, onCreate func([]ids.NodeID)) {
	ui := ctx.UI
	enabled := ctx.Doc() != nil
	item := func(t Template) {
		if !widgets.Entry(ui, widgets.Item{Label: t.Name, Icon: t.Icon, Enabled: enabled}) {
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
	separated := false
	for _, sec := range Layout(r, pl) {
		switch {
		case sec.Title == "":
			for _, t := range sec.Items {
				item(t)
			}
		case !sec.Module:
			ui.PushID("menu/" + sec.Title)
			if widgets.SubMenuIcon(ui, sec.Title, sec.Icon, 200) {
				for _, t := range sec.Items {
					item(t)
				}
				widgets.EndSubMenu(ui)
			}
			ui.PopID()
		default:
			if !separated {
				widgets.Separator(ui)
				separated = true
			}
			ui.PushID("mod/" + sec.Title)
			if widgets.SubMenuIcon(ui, sec.Title, sec.Icon, 220) {
				for i, t := range sec.Items {
					if slices.Contains(sec.Breaks, i) {
						widgets.Separator(ui)
					}
					item(t)
				}
				widgets.EndSubMenu(ui)
			}
			ui.PopID()
		}
	}
}
