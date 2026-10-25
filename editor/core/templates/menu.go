package templates

import "aego/scene"

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
