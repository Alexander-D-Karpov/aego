package inspector

import (
	"path"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/prefabs"
	"aego/scene/prefab"
	"aego/schema"
	"aego/ui/widgets"
)

func ownerOf(td *schema.TypeDescriptor) string {
	if td.Kind == schema.TypeScript {
		return prefab.ScriptOwner(td.FullName)
	}
	return td.FullName
}

func (p *Panel) report(err error) {
	if err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
	}
}

func (p *Panel) overridden(td *schema.TypeDescriptor, fd *schema.FieldDescriptor) bool {
	return p.info != nil && len(p.nodes) == 1 && p.info.Overridden(p.nodes[0], ownerOf(td), fd.Name)
}

func (p *Panel) prefabCard(d *document.SceneDocument) {
	info := p.info
	if info == nil {
		return
	}
	pf := p.d.Prefabs
	ui := p.f.UI
	th := &ui.Theme
	ui.PushID("prefab")
	defer ui.PopID()
	title := "Prefab"
	if info.Path != "" {
		title += " " + strings.TrimSuffix(path.Base(info.Path), path.Ext(info.Path))
	}
	c := p.beginCard("prefab", title, "", "file.prefab", "Open", false)
	defer p.endCard(ui, c)
	if c.action {
		p.report(pf.Open(info.Prefab))
	}
	if !c.open {
		return
	}
	if info.Missing {
		widgets.LabelDim(ui, "The prefab is missing, the instance is kept as it is in the file")
		return
	}
	node := p.nodes[0]
	row := ui.Cursor().Next(0)
	w := (row.W - 4) / 2
	if widgets.ButtonRect(ui, ui.ID("apply"), mathx.Rect{X: row.X, Y: row.Y, W: w, H: row.H}, "Apply All") {
		p.report(pf.Apply(d, prefabs.Change{Node: node, Level: prefabs.LevelAll}))
	}
	if widgets.ButtonRect(ui, ui.ID("revert"), mathx.Rect{X: row.X + w + 4, Y: row.Y, W: w, H: row.H}, "Revert All") {
		p.report(pf.Revert(d, prefabs.Change{Node: node, Level: prefabs.LevelAll}))
	}
	for i, owner := range info.Removed(node) {
		ui.PushIDInt(i)
		p.entryRow("Removed "+owner, "Revert", func() {
			p.report(pf.Revert(d, prefabs.Change{Node: node, Level: prefabs.LevelComponent, Prop: prefab.PropKey{Owner: owner}}))
		})
		ui.PopID()
	}
	orph := info.Orphans()
	if orph.Empty() {
		return
	}
	r := ui.Cursor().Next(0)
	widgets.IconAt(ui, mathx.Rect{X: r.X, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, "status.warning", th.Warn)
	ui.Text("Unused overrides", mathx.Vec2{X: r.X + 20, Y: ui.TextY(r)}, th.Warn)
	remove := func(c prefabs.Change) func() {
		return func() { p.report(pf.Revert(d, c)) }
	}
	for i, o := range orph.Props {
		ui.PushIDInt(1000 + i)
		p.entryRow(o.Prop.String(), "Remove", remove(prefabs.Change{Node: info.Root, Key: o.Node, Level: prefabs.LevelOrphan, Prop: o.Prop}))
		ui.PopID()
	}
	for i, a := range orph.AddComps {
		ui.PushIDInt(2000 + i)
		p.entryRow("added "+a.Comp.Owner, "Remove", remove(prefabs.Change{Node: info.Root, Key: a.Node, Level: prefabs.LevelOrphan, Prop: prefab.PropKey{Owner: a.Comp.Owner}}))
		ui.PopID()
	}
	for i, a := range orph.AddNodes {
		ui.PushIDInt(3000 + i)
		p.entryRow("added node "+a.Node.Name(), "Remove", remove(prefabs.Change{Node: info.Root, Key: prefab.LocalKey(a.Node.Local), Level: prefabs.LevelOrphan}))
		ui.PopID()
	}
}

func (p *Panel) entryRow(text, button string, fn func()) {
	ui := p.f.UI
	r := ui.Cursor().Next(0)
	bw := ui.TextSize(button).X + 14
	ui.PushClip(mathx.Rect{X: r.X, Y: r.Y, W: max(0, r.W-bw-4), H: r.H})
	ui.Text(text, mathx.Vec2{X: r.X, Y: ui.TextY(r)}, ui.Theme.TextDim)
	ui.PopClip()
	if widgets.ButtonRect(ui, ui.ID("entry"), mathx.Rect{X: r.X + r.W - bw, Y: r.Y, W: bw, H: r.H}, button) {
		fn()
	}
}

func (p *Panel) prefabItems(d *document.SceneDocument, c prefabs.Change, enabled bool) {
	if p.info == nil || p.info.Missing {
		return
	}
	ui := p.f.UI
	widgets.Separator(ui)
	for _, t := range p.d.Prefabs.Targets(d, c.Node) {
		c.Nearest = t.Nearest
		if widgets.MenuItemIcon(ui, "Apply to "+t.Name, "", "prefab.apply", enabled, false) {
			p.report(p.d.Prefabs.Apply(d, c))
		}
	}
	label := "Reset to Prefab Value"
	if c.Level != prefabs.LevelProperty {
		label = "Revert to Prefab"
	}
	if widgets.MenuItemIcon(ui, label, "", "prefab.revert", enabled, false) {
		p.report(p.d.Prefabs.Revert(d, c))
	}
}
