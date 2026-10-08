package inspector

import (
	"strconv"
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/assets"
	"aego/editor/context"
	"aego/editor/document"
	"aego/editor/history/commands"
	"aego/platform/input"
	"aego/scene"
	"aego/scene/components"
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

func attachment(s *scene.Scene, id ids.NodeID, name string) *components.Attachment {
	sc, ok := components.ScriptsOf(s, s.Handle(id))
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
	first, ok := components.ScriptsOf(s, s.Handle(p.nodes[0]))
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

func (p *Panel) scripts(ctx *context.Editor, d *document.SceneDocument) {
	names := p.commonScripts(d.Scene())
	if len(names) == 0 {
		return
	}
	ui := ctx.UI
	r := ui.Cursor().Next(ui.LineHeight() + 4)
	ui.Text("Scripts", mathx.Vec2{X: r.X + 2, Y: r.Y + 3}, ui.Theme.TextDisabled)
	list := append([]string(nil), names...)
	for i, name := range list {
		p.scriptCard(ctx, d, i, len(list), name)
	}
}

func (p *Panel) scriptCard(ctx *context.Editor, d *document.SceneDocument, index, count int, name string) {
	ui := ctx.UI
	s := d.Scene()
	td, known := s.Registry().ByName(name)
	known = known && td.Kind == schema.TypeScript
	ui.PushID("script/" + name)
	defer ui.PopID()
	c := p.beginCard(ctx, "script:"+name, humanize(shortScript(name)), scriptPackage(name), "cat.scripts", "", true)
	defer func() { p.endCard(ui, c) }()
	menuID := ui.ID("menu")
	if ui.RightClicked(c.hid, c.hdr) {
		p.rightUsed = true
		widgets.OpenContextMenu(ui, menuID)
	} else if c.more {
		widgets.OpenContextMenu(ui, menuID)
	}
	if widgets.ContextMenu(ui, menuID) {
		p.scriptMenu(ctx, d, name, td, known, index, count)
		widgets.EndContextMenu(ui)
	}
	if !c.open {
		return
	}
	p.scriptEnabled(ctx, d, name)
	p.scriptProblems(ctx, name)
	if known {
		p.fields(ctx, d, td)
	} else {
		widgets.LabelDim(ui, "Not compiled, properties are kept as they are")
	}
	p.unknownProps(ctx, d, name, td, known)
}

func (p *Panel) scriptEnabled(ctx *context.Editor, d *document.SceneDocument, name string) {
	ui := ctx.UI
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
			ctx.Notify(context.LevelError, err.Error())
			return
		}
	}
	tx.Commit()
}

func (p *Panel) scriptProblems(ctx *context.Editor, name string) {
	if ctx.Scripts == nil {
		return
	}
	ps, _ := ctx.Scripts.Problems()
	pkg := scriptPackage(name)
	n := 0
	var first context.ScriptProblem
	for _, pr := range ps {
		if pr.Error && pr.Package == pkg {
			if n == 0 {
				first = pr
			}
			n++
		}
	}
	if n == 0 {
		return
	}
	ui := ctx.UI
	th := &ui.Theme
	r := ui.Cursor().Next(0)
	id := ui.ID("problems")
	if ui.Clicked(id, r) && ctx.ShowProblems != nil {
		ctx.ShowProblems(pkg)
	}
	bg := th.Error
	bg.A = 0.18
	ui.Rect(r, bg)
	ui.PushClip(r)
	ui.TextIn(r, strconv.Itoa(n)+" error(s) in "+pkg+", the last good build runs", th.Error, core.AlignLeft)
	ui.PopClip()
	widgets.Tooltip(ui, id, first.Path+":"+strconv.Itoa(first.Line)+": "+first.Message+" (click to open Console)")
}

func (p *Panel) unknownProps(ctx *context.Editor, d *document.SceneDocument, name string, td *schema.TypeDescriptor, known bool) {
	a := attachment(d.Scene(), p.nodes[0], name)
	if a == nil {
		return
	}
	ui := ctx.UI
	th := &ui.Theme
	for _, f := range a.Unknown {
		if known {
			if _, ok := td.Field(f.Name); ok {
				continue
			}
		}
		r := ui.Cursor().Next(0)
		widgets.IconAt(ui, mathx.Rect{X: r.X, Y: r.Y + (r.H-14)/2, W: 14, H: 14}, "status.warning", th.Warn)
		ui.PushClip(r)
		ui.Text(f.Name+" is not in the script, kept on save", mathx.Vec2{X: r.X + 20, Y: ui.TextY(r)}, th.TextDim)
		ui.PopClip()
	}
}

func (p *Panel) scriptMenu(ctx *context.Editor, d *document.SceneDocument, name string, td *schema.TypeDescriptor, known bool, index, count int) {
	ui := ctx.UI
	short := shortScript(name)
	if widgets.MenuItem(ui, "Reset", "", known, false) {
		reg := d.Scene().Registry()
		tx := d.History.Begin("Reset " + short)
		for i := range td.Fields {
			fd := &td.Fields[i]
			if err := d.Props.Set(tx, p.nodes, td.ID, fd.ID, p.defaultValue(reg, td, fd)); err != nil {
				tx.Abort()
				ctx.Notify(context.LevelError, err.Error())
				return
			}
		}
		tx.Commit()
	}
	if widgets.MenuItem(ui, "Remove", "", true, false) {
		tx := d.History.Begin("Remove " + short)
		for _, n := range p.nodes {
			if err := tx.Exec(&commands.RemoveScript{Node: n, TypeName: name}); err != nil {
				tx.Abort()
				ctx.Notify(context.LevelError, err.Error())
				return
			}
		}
		tx.Commit()
	}
	widgets.Separator(ui)
	single := len(p.nodes) == 1
	if widgets.MenuItem(ui, "Move Up", "", single && index > 0, false) {
		if err := d.History.Exec(&commands.ReorderScript{Node: p.nodes[0], From: index, To: index - 1}); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		}
	}
	if widgets.MenuItem(ui, "Move Down", "", single && index < count-1, false) {
		if err := d.History.Exec(&commands.ReorderScript{Node: p.nodes[0], From: index, To: index + 1}); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		}
	}
	widgets.Separator(ui)
	if widgets.MenuItem(ui, "Show Script Files", "", ctx.RevealPath != nil, false) {
		ctx.RevealPath(assets.ScriptDir(ctx, name))
	}
}

func (p *Panel) addScript(ctx *context.Editor, d *document.SceneDocument) {
	ui := ctx.UI
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
		label := humanize(shortScript(td.FullName))
		if q != "" && !strings.Contains(strings.ToLower(label), q) && !strings.Contains(strings.ToLower(td.FullName), q) {
			continue
		}
		if first == "" {
			first = td.FullName
		}
		if widgets.Selectable(ui, core.IDSeq(id, i+1), label+"   "+scriptPackage(td.FullName), td.FullName == first && q != "") {
			p.attach(ctx, td.FullName)
			ui.ClosePopup(id)
		}
	}
	if first == "" {
		widgets.LabelDim(ui, "No scripts match")
	}
	if first != "" && q != "" && (ui.KeyPressed(input.KeyEnter) || ui.KeyPressed(input.KeyKPEnter)) {
		p.attach(ctx, first)
		ui.ClosePopup(id)
	}
	ui.EndPopup()
}

func (p *Panel) attach(ctx *context.Editor, name string) {
	if err := assets.AttachScript(ctx, p.nodes, name); err != nil {
		ctx.Notify(context.LevelError, err.Error())
	}
}

func (p *Panel) dropScripts(ctx *context.Editor, r mathx.Rect) {
	ui := ctx.UI
	if len(p.nodes) == 0 || !ctx.DnD.Hovering(ui, r) {
		return
	}
	pl, _ := ctx.DnD.Payload()
	ap, ok := pl.Data.(context.AssetPayload)
	if pl.Kind != context.KindAsset || !ok || ap.Kind != asset.ArtScript {
		return
	}
	if got, ok := ctx.DnD.Target(ui, r, context.KindAsset); ok {
		assets.AttachScriptAsset(ctx, p.nodes, got.Data.(context.AssetPayload).ID)
	}
}
