package inspector

import (
	"aego/editor/ext/inspect"
	"strconv"
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/platform/input"
	"aego/scene"
	"aego/scene/scriptcomp"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

func shortScript(full string) string {
	return full[strings.LastIndexByte(full, '.')+1:]
}

func scriptPackage(full string) string {
	i := strings.LastIndexByte(full, '.')
	if i < 0 {
		return ""
	}
	return strings.ReplaceAll(full[:i], ".", "/")
}

func attachment(s *scene.Scene, id ids.NodeID, name string) *scriptcomp.Attachment {
	sc, ok := scriptcomp.ScriptsOf(s, s.Handle(id))
	if !ok {
		return nil
	}
	if i := sc.Index(name); i >= 0 {
		return &sc.Attachments[i]
	}
	return nil
}

func (p *Panel) commonScripts(s *scene.Scene) []string {
	p.scriptNames = p.scriptNames[:0]
	first, ok := scriptcomp.ScriptsOf(s, s.Handle(p.nodes[0]))
	if !ok {
		return nil
	}
	for i := range first.Attachments {
		name := first.Attachments[i].TypeName
		all := true
		for _, id := range p.nodes[1:] {
			if attachment(s, id, name) == nil {
				all = false
				break
			}
		}
		if all {
			p.scriptNames = append(p.scriptNames, name)
		}
	}
	return p.scriptNames
}

func (p *Panel) scripts(d *document.SceneDocument) {
	names := p.commonScripts(d.Scene())
	if len(names) == 0 {
		return
	}
	ui := p.f.UI
	r := ui.Cursor().Next(ui.LineHeight() + 4)
	ui.Text("Scripts", mathx.Vec2{X: r.X + 2, Y: r.Y + 3}, ui.Theme.TextDisabled)
	list := append([]string(nil), names...)
	for i, name := range list {
		p.scriptCard(d, i, len(list), name)
	}
}

func (p *Panel) scriptEnabled(d *document.SceneDocument, name string) {
	ui := p.f.UI
	s := d.Scene()
	state := -1
	for _, id := range p.nodes {
		a := attachment(s, id, name)
		if a == nil {
			continue
		}
		v := 0
		if a.Enabled {
			v = 1
		}
		switch {
		case state < 0:
			state = v
		case state != v:
			state = 2
		}
	}
	r := widgets.Field(ui, "Enabled")
	if !triCheck(ui, ui.ID("enabled"), mathx.Rect{X: r.X, Y: r.Y, W: r.H, H: r.H}, state) {
		return
	}
	after := state != 1
	tx := d.History.Begin("Toggle " + shortScript(name))
	for _, id := range p.nodes {
		a := attachment(s, id, name)
		if a == nil || a.Enabled == after {
			continue
		}
		if err := tx.Exec(&commands.SetScriptEnabled{Node: id, TypeName: name, Before: a.Enabled, After: after}); err != nil {
			tx.Abort()
			p.d.Notices.Notify(context.LevelError, err.Error())
			return
		}
	}
	tx.Commit()
}

func (p *Panel) unknownProps(d *document.SceneDocument, name string, td *schema.TypeDescriptor, known bool) {
	a := attachment(d.Scene(), p.nodes[0], name)
	if a == nil {
		return
	}
	ui := p.f.UI
	th := &ui.Theme
	for _, f := range a.Unknown {
		if known {
			if _, ok := td.Field(f.Name); ok {
				continue
			}
		}
		r := ui.Cursor().Next(0)
		widgets.IconAt(ui, mathx.Rect{X: r.X, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, "status.warning", th.Warn)
		ui.PushClip(r)
		ui.Text(f.Name+" is not in the script, kept on save", mathx.Vec2{X: r.X + 20, Y: ui.TextY(r)}, th.TextDim)
		ui.PopClip()
	}
}

func (p *Panel) addScript(d *document.SceneDocument) {
	ui := p.f.UI
	th := &ui.Theme
	s := d.Scene()
	types := s.Registry().Scripts()
	if len(types) == 0 {
		return
	}
	row := ui.Cursor().Next(th.RowHeight + 4)
	id := ui.ID("addscript")
	if ui.Pressed(id, row) {
		if ui.PopupOpen(id) {
			ui.ClosePopup(id)
		} else {
			ui.CloseAllPopups()
			ui.OpenPopup(id, mathx.Vec2{X: row.X, Y: row.Y + row.H})
			p.scriptSearch = ""
			p.focusScript = true
		}
	}
	ui.Rect(row, ui.WidgetColor(id))
	ui.Outline(row, th.AccentDim)
	ui.TextIn(row, "Add Script", th.Text, core.AlignCenter)
	if !ui.BeginPopup(id, row.W) {
		return
	}
	sid := ui.ID("scriptsearch")
	if p.focusScript {
		p.focusScript = false
		ui.SetFocus(sid)
		ui.EditBuffer(sid, p.scriptSearch).SelectAll()
	}
	widgets.SearchField(ui, sid, ui.Cursor().Next(0), &p.scriptSearch)
	q := strings.ToLower(strings.TrimSpace(p.scriptSearch))
	first := ""
	for i, td := range types {
		if td.Hidden {
			continue
		}
		missing := false
		for _, n := range p.nodes {
			if attachment(s, n, td.FullName) == nil {
				missing = true
				break
			}
		}
		if !missing {
			continue
		}
		label := inspect.Humanize(shortScript(td.FullName))
		if q != "" && !strings.Contains(strings.ToLower(label), q) && !strings.Contains(strings.ToLower(td.FullName), q) {
			continue
		}
		if first == "" {
			first = td.FullName
		}
		if widgets.Selectable(ui, core.IDSeq(id, i+1), label+"   "+scriptPackage(td.FullName), td.FullName == first && q != "") {
			p.attach(td.FullName)
			ui.ClosePopup(id)
		}
	}
	if first == "" {
		widgets.LabelDim(ui, "No scripts match")
	}
	if first != "" && q != "" && (ui.KeyPressed(input.KeyEnter) || ui.KeyPressed(input.KeyKPEnter)) {
		p.attach(first)
		ui.ClosePopup(id)
	}
	ui.EndPopup()
}

func (p *Panel) attach(name string) {
	if err := p.d.Assets.AttachScriptType(p.nodes, name); err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
	}
}

func (p *Panel) dropScripts(r mathx.Rect) {
	ui := p.f.UI
	if len(p.nodes) == 0 || !p.f.DnD.Hovering(ui, r) {
		return
	}
	pl, _ := p.f.DnD.Payload()
	ap, ok := pl.Data.(context.AssetPayload)
	if pl.Kind != context.KindAsset || !ok || ap.Kind != asset.ArtScript {
		return
	}
	if got, ok := p.f.DnD.Target(ui, r, context.KindAsset); ok {
		p.d.Scenes.AttachScript(p.nodes, got.Data.(context.AssetPayload).ID)
	}
}

func (p *Panel) scriptCard(d *document.SceneDocument, index, count int, name string) {
	ui := p.f.UI
	s := d.Scene()
	td, known := s.Registry().ByName(name)
	known = known && td.Kind == schema.TypeScript
	ui.PushID("script/" + name)
	defer ui.PopID()
	action := ""
	if p.code() != nil {
		action = "Edit"
	}
	c := p.beginCard("script:"+name, inspect.Humanize(shortScript(name)), scriptPackage(name), "cat.scripts", action, true)
	defer func() { p.endCard(ui, c) }()
	if c.action {
		p.gotoScript(name, "")
	}
	menuID := ui.ID("menu")
	if ui.RightClicked(c.hid, c.hdr) {
		p.rightUsed = true
		widgets.OpenContextMenu(ui, menuID)
	} else if c.more {
		widgets.OpenContextMenu(ui, menuID)
	}
	if widgets.ContextMenu(ui, menuID) {
		p.scriptMenu(d, name, td, known, index, count)
		widgets.EndContextMenu(ui)
	}
	if !c.open {
		return
	}
	p.scriptEnabled(d, name)
	p.scriptProblems(name)
	if known {
		p.fields(d, td)
	} else {
		widgets.LabelDim(ui, "Not compiled, properties are kept as they are")
	}
	p.unknownProps(d, name, td, known)
}

func (p *Panel) scriptProblems(name string) {
	pkg := scriptPackage(name)
	first, n, ok := p.d.Problems.FirstError(pkg)
	if !ok {
		return
	}
	ui := p.f.UI
	th := &ui.Theme
	r := ui.Cursor().Next(0)
	id := ui.ID("problems")
	if ui.Clicked(id, r) {
		p.d.Nav.Open(first.Location())
	}
	bg := th.Error
	bg.A = 0.18
	ui.Rect(r, bg)
	widgets.IconAt(ui, mathx.Rect{X: r.X + 2, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, "log.error", th.Error)
	ui.PushClip(r)
	ui.Text(strconv.Itoa(n)+" error(s) in "+pkg+", the last good build runs", mathx.Vec2{X: r.X + 22, Y: ui.TextY(r)}, th.Error)
	ui.PopClip()
	tip := first.Path + ":" + strconv.Itoa(first.Line) + ": " + first.Message
	if first.Help != "" {
		tip += "\n" + first.Help
	}
	widgets.Tooltip(ui, id, tip+"\nClick to open the line")
}

func (p *Panel) scriptMenu(d *document.SceneDocument, name string, td *schema.TypeDescriptor, known bool, index, count int) {
	ui := p.f.UI
	short := shortScript(name)
	if p.code() != nil {
		if widgets.MenuItemIcon(ui, "Edit Script", "", "file.script", true, false) {
			p.gotoScript(name, "")
		}
		if widgets.MenuItemIcon(ui, "Find Usages", "", "ui.search", known, false) {
			p.code().FindUsages(name, "")
		}
		widgets.Separator(ui)
	}
	if widgets.MenuItemIcon(ui, "Reset", "", "ui.reset", known, false) {
		reg := d.Scene().Registry()
		tx := d.History.Begin("Reset " + short)
		for i := range td.Fields {
			fd := &td.Fields[i]
			if err := d.Props.Set(tx, p.nodes, td.ID, fd.ID, p.defaultValue(reg, td, fd)); err != nil {
				tx.Abort()
				p.d.Notices.Notify(context.LevelError, err.Error())
				return
			}
		}
		tx.Commit()
	}
	if known {
		p.presetItems(d, td)
		widgets.Separator(ui)
	}
	if widgets.MenuItemIcon(ui, "Remove", "", "edit.delete", true, false) {
		tx := d.History.Begin("Remove " + short)
		for _, n := range p.nodes {
			if err := tx.Exec(&commands.RemoveScript{Node: n, TypeName: name}); err != nil {
				tx.Abort()
				p.d.Notices.Notify(context.LevelError, err.Error())
				return
			}
		}
		tx.Commit()
	}
	widgets.Separator(ui)
	single := len(p.nodes) == 1
	if widgets.MenuItemIcon(ui, "Move Up", "", "ui.move_up", single && index > 0, false) {
		if err := d.History.Exec(&commands.ReorderScript{Node: p.nodes[0], From: index, To: index - 1}); err != nil {
			p.d.Notices.Notify(context.LevelError, err.Error())
		}
	}
	if widgets.MenuItemIcon(ui, "Move Down", "", "ui.move_down", single && index < count-1, false) {
		if err := d.History.Exec(&commands.ReorderScript{Node: p.nodes[0], From: index, To: index + 1}); err != nil {
			p.d.Notices.Notify(context.LevelError, err.Error())
		}
	}
	widgets.Separator(ui)
	if widgets.MenuItemIcon(ui, "Show Script Files", "", "file.reveal", true, false) {
		p.d.Nav.Open(context.AssetAt(p.d.Assets.ScriptDir(name)))
	}
}
