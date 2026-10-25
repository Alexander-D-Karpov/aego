package shell

import (
	"hash/fnv"

	"aego/platform"
)

const appMenuTitle = "Aego Editor"

type nativeMenu struct {
	bar    platform.MenuBar
	acts   []func()
	clicks []int
	sum    uint64
	next   float64
}

func (a *App) initNativeMenu() {
	if a.eng.Config().Headless {
		return
	}
	if mb, ok := a.eng.Window().(platform.MenuBar); ok {
		a.native = &nativeMenu{bar: mb}
	}
}

func flag(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func hashMenus(menus []platform.Menu) uint64 {
	h := fnv.New64a()
	var walk func([]platform.MenuItem)
	walk = func(items []platform.MenuItem) {
		for _, it := range items {
			h.Write([]byte(it.Label))
			h.Write([]byte{0})
			h.Write([]byte(it.Shortcut))
			h.Write([]byte{flag(it.Enabled), flag(it.Checked), flag(it.Separator), byte(it.Role), 1})
			walk(it.Items)
			h.Write([]byte{2})
		}
	}
	for _, m := range menus {
		h.Write([]byte(m.Title))
		h.Write([]byte{3})
		walk(m.Items)
	}
	return h.Sum64()
}

func (a *App) buildNativeMenu() ([]platform.Menu, []func()) {
	reg := a.ctx.Commands
	var acts []func()
	act := func(fn func()) int {
		acts = append(acts, fn)
		return len(acts) - 1
	}
	menus := []platform.Menu{{Title: appMenuTitle, Items: []platform.MenuItem{
		{Role: platform.RoleHide, Label: "Hide " + appMenuTitle},
		{Role: platform.RoleHideOthers, Label: "Hide Others"},
		{Role: platform.RoleShowAll, Label: "Show All"},
		{Separator: true},
		{ID: act(func() { reg.Run(CommandQuit) }), Label: "Quit " + appMenuTitle, Shortcut: reg.Shortcut(CommandQuit), Enabled: true},
	}}}
	for _, cat := range a.menuCategories() {
		if cat == "Create" {
			continue
		}
		m := platform.Menu{Title: cat}
		last := -1
		for _, d := range reg.Category(cat) {
			if last >= 0 && d.Order/100 != last/100 {
				m.Items = append(m.Items, platform.MenuItem{Separator: true})
			}
			last = d.Order
			label := d.Name
			if d.Label != nil {
				label = d.Label()
			}
			id := d.ID
			m.Items = append(m.Items, platform.MenuItem{
				ID: act(func() { reg.Run(id) }), Label: label, Shortcut: d.Shortcut.String(),
				Enabled: reg.Enabled(d.ID), Checked: d.Checked != nil && d.Checked(),
			})
		}
		if cat == "Window" && reg.HasCategory(themeCategory) {
			sub := platform.MenuItem{Label: themeCategory, Enabled: true}
			for _, d := range reg.Category(themeCategory) {
				id := d.ID
				sub.Items = append(sub.Items, platform.MenuItem{
					ID: act(func() { reg.Run(id) }), Label: themeLabel(d.Name), Enabled: true, Checked: d.Checked(),
				})
			}
			m.Items = append(m.Items, platform.MenuItem{Separator: true}, sub)
		}
		if cat == "Scene" {
			docs := a.ctx.Docs.Docs()
			if len(docs) > 0 {
				m.Items = append(m.Items, platform.MenuItem{Separator: true})
			}
			active := a.ctx.Docs.Edit()
			for _, d := range docs {
				name := d.Path
				if d.Dirty() {
					name += " *"
				}
				doc := d.ID
				m.Items = append(m.Items, platform.MenuItem{
					ID: act(func() { a.ctx.Docs.SetActive(doc) }), Label: name, Enabled: true,
					Checked: active != nil && d.ID == active.ID,
				})
			}
		}
		if len(m.Items) > 0 {
			menus = append(menus, m)
		}
	}
	return menus, acts
}

func (a *App) syncNativeMenu(now float64) {
	n := a.native
	if n == nil {
		return
	}
	n.clicks = n.bar.MenuClicks(n.clicks[:0])
	for _, c := range n.clicks {
		if c >= 0 && c < len(n.acts) {
			n.acts[c]()
		}
	}
	if n.sum != 0 && now < n.next {
		return
	}
	n.next = now + 0.25
	menus, acts := a.buildNativeMenu()
	if s := hashMenus(menus); s != n.sum {
		n.sum, n.acts = s, acts
		n.bar.SetMenuBar(appMenuTitle, menus)
	}
}
