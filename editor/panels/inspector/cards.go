package inspector

import (
	"aego/core/mathx"
	"aego/editor/core/clipboard"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/prefabs"
	"aego/editor/core/presets"
	"aego/engine/i18n"
	"aego/platform/clip"
	"aego/scene"
	"aego/scene/prefab"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/widgets"
)

type card struct {
	open   bool
	more   bool
	action bool
	hid    core.ID
	hdr    mathx.Rect
	bg     int
	top    float32
	typ    schema.TypeID
}

func (p *Panel) beginCard(key, title, badge, icon, action string, menu bool) card {
	ui := p.f.UI
	th := &ui.Theme
	cur := ui.Cursor()
	c := card{bg: ui.List().ReserveBox(), top: cur.Y}
	hdr := cur.Next(th.RowHeight + 4)
	c.hdr = hdr
	c.hid = ui.ID("card")
	corners := draw.CornersAll
	if p.d.Collapsed == nil || !p.d.Collapsed[key] {
		corners = draw.CornersTop
	}
	ui.List().RoundRect(hdr, th.Radius.Card, th.CardHeader, corners)
	right := hdr.X + hdr.W - 4
	bh := hdr.H - 6
	if menu {
		mr := mathx.Rect{X: right - bh, Y: hdr.Y + 3, W: bh, H: bh}
		c.more = widgets.IconButtonRect(ui, ui.ID("more"), mr, "ui.more")
		right = mr.X - 4
	}
	if action != "" {
		w := ui.TextSize(action).X + 14
		ar := mathx.Rect{X: right - w, Y: hdr.Y + 3, W: w, H: bh}
		c.action = widgets.ButtonRect(ui, ui.ID("action"), ar, action)
		right = ar.X - 6
	}
	area := mathx.Rect{X: hdr.X, Y: hdr.Y, W: max(0, right-hdr.X), H: hdr.H}
	if ui.Pressed(c.hid, area) && p.d.Collapsed != nil {
		p.d.Collapsed[key] = !p.d.Collapsed[key]
	}
	c.open = p.d.Collapsed == nil || !p.d.Collapsed[key]
	if ui.Hot() == c.hid {
		hl := th.WidgetHot
		hl.A = 0.5
		ui.List().RoundRect(area, th.Radius.Card, hl, corners)
	}
	x := hdr.X + 2
	widgets.Chevron(ui, mathx.Rect{X: x, Y: hdr.Y, W: th.Indent, H: hdr.H}, c.open, th.TextDim)
	x += th.Indent + 2
	widgets.IconAt(ui, mathx.Rect{X: x, Y: hdr.Y + (hdr.H-16)/2, W: 16, H: 16}, icon, th.TextDim)
	x += 22
	if badge != "" {
		bw := ui.TextSize(badge).X + 10
		if x+ui.TextSize(title).X+bw+8 <= right {
			lh := ui.LineHeight() + 2
			br := mathx.Rect{X: right - bw, Y: hdr.Y + (hdr.H-lh)/2, W: bw, H: lh}
			ui.Rect(br, th.Badge)
			ui.TextIn(br, badge, th.Unit, core.AlignCenter)
			right = br.X - 6
		}
	}
	ui.PushClip(mathx.Rect{X: x, Y: hdr.Y, W: max(0, right-x), H: hdr.H})
	ui.Text(title, mathx.Vec2{X: x, Y: ui.TextY(hdr)}, th.Text)
	ui.PopClip()
	if c.open {
		cur.X += 6
		cur.W -= 12
		cur.Y += 2
	}
	return c
}

func (p *Panel) endCard(ui *core.Context, c card) {
	cur := ui.Cursor()
	if c.open {
		cur.X -= 6
		cur.W += 12
		cur.Y += 2
	}
	rect := mathx.Rect{X: cur.X, Y: c.top, W: cur.W, H: cur.Y - c.top}
	ui.List().PatchBox(c.bg, rect, ui.Theme.Radius.Card, ui.Theme.CardBg, mathx.Color{})
	if c.typ != 0 && ui.Hit(rect) && !ui.MouseInOverlay() {
		p.hoverType = c.typ
	}
	cur.Y += 4
}

func (p *Panel) componentMenu(d *document.SceneDocument, td *schema.TypeDescriptor) {
	ui := p.f.UI
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.reset", "Reset"), "", "ui.reset", true, false) {
		p.reset(d, td)
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.remove", "Remove"), "", "edit.delete", true, false) {
		p.remove(d, td)
	}
	widgets.Separator(ui)
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.copy_component", "Copy Component"), p.d.Commands.Shortcut(CommandCopy), "edit.copy", true, false) {
		if err := p.d.Clip.CopyComponent(d.Scene(), p.nodes[0], td.ID); err != nil {
			p.d.Notices.Notify(context.LevelError, err.Error())
		} else {
			p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.inspector.copied_name", "Copied {name}", i18n.Str("name", p.reg.Meta(td).DisplayName)))
		}
	}
	same := false
	if p.d.Clip.Peek() == clipboard.KindComponent {
		if src, _, ok := p.d.Clip.ReadComponent(); ok && src.ID == td.ID {
			same = true
		}
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.paste_component_values", "Paste Component Values"), p.d.Commands.Shortcut(CommandPaste), "edit.paste", same, false) {
		p.confirm(d, i18n.Tr("editor.inspector.paste_name", "Paste {name}", i18n.Str("name", p.reg.Meta(td).DisplayName)), func() error { return p.d.Clip.PasteComponent(d, p.nodes) })
	}
	p.presetItems(d, td)
	if p.info != nil {
		owner := ownerOf(td)
		p.prefabItems(d, prefabs.Change{Node: p.nodes[0], Level: prefabs.LevelComponent, Prop: prefab.PropKey{Owner: owner}},
			p.info.ComponentTouched(p.nodes[0], owner))
	}
	if p.d.Docs.Documents().InPlay() && p.d.Play != nil {
		widgets.Separator(ui)
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.copy_values_to_edit_scene", "Copy values to edit scene"), "", "play.copy_back", p.d.Play.CanCopyToEdit(p.nodes, td.ID), false) {
			if err := p.d.Play.CopyToEdit(p.nodes, td.ID); err != nil {
				p.d.Notices.Notify(context.LevelError, err.Error())
			}
		}
	}
}

func (p *Panel) lookup(d *document.SceneDocument, ref fieldRef) (*schema.TypeDescriptor, *schema.FieldDescriptor, bool) {
	td, ok := d.Scene().Registry().ByID(ref.t)
	if !ok {
		return nil, nil, false
	}
	fd, ok := td.FieldByID(ref.f)
	if !ok {
		return nil, nil, false
	}
	return td, fd, true
}

func (p *Panel) copyValue(d *document.SceneDocument, td *schema.TypeDescriptor, fd *schema.FieldDescriptor) {
	v, mixed, err := d.Props.Common(p.nodes, td.ID, fd.ID)
	if err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
		return
	}
	text := p.summary(d, fd, v, mixed)
	if err := p.d.Clip.CopyValue(d.Scene(), p.nodes[0], td.ID, fd.ID, text); err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
	}
}

func (p *Panel) pasteValue(d *document.SceneDocument, td *schema.TypeDescriptor, fd *schema.FieldDescriptor) {
	v, ok := p.d.Clip.ReadValue(fd)
	if !ok {
		p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.inspector.clipboard_does_not_fit", "The clipboard does not fit {field}", i18n.Str("field", p.labelOf(td, fd))))
		return
	}
	p.confirm(d, i18n.Tr("editor.inspector.paste_name", "Paste {name}", i18n.Str("name", p.labelOf(td, fd))), func() error { return d.Props.Set(nil, p.nodes, td.ID, fd.ID, v) })
}

func (p *Panel) defaultValue(reg *schema.Registry, td *schema.TypeDescriptor, fd *schema.FieldDescriptor) schema.Value {
	if fd.HasDefault {
		return fd.Default
	}
	for _, fv := range p.reg.Defaults(td.ID) {
		if fv.Field == fd.ID {
			return fv.Value
		}
	}
	return zeroValue(reg, fd)
}

func (p *Panel) fieldMenu(d *document.SceneDocument) {
	ui := p.f.UI
	td, fd, ok := p.lookup(d, p.menuField)
	if !ok {
		return
	}
	if td.Kind == schema.TypeScript && p.code() != nil {
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.go_to_declaration", "Go to Declaration"), "Ctrl+Click", "code.implementation", true, false) {
			p.gotoScript(td.FullName, fd.Name)
		}
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.find_usages", "Find Usages"), "", "ui.search", true, false) {
			p.code().FindUsages(td.FullName, fd.Name)
		}
		widgets.Separator(ui)
	}
	ro := fd.Flags.Has(schema.FlagReadOnly)
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.copy_value", "Copy Value"), p.d.Commands.Shortcut(CommandCopy), "edit.copy", true, false) {
		p.copyValue(d, td, fd)
	}
	_, canPaste := p.d.Clip.ReadValue(fd)
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.paste_value", "Paste Value"), p.d.Commands.Shortcut(CommandPaste), "edit.paste", canPaste && !ro, false) {
		p.pasteValue(d, td, fd)
	}
	if textureField(fd) {
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.paste_image", "Paste Image"), "", "file.texture", !ro && clip.HasImage(p.d.Clip.Clip), false) {
			if err := p.pasteImage(d, td, fd); err != nil {
				p.d.Notices.Notify(context.LevelError, err.Error())
			}
		}
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.reset_to_default", "Reset to Default"), "", "ui.reset", !ro, false) {
		def := p.defaultValue(d.Scene().Registry(), td, fd)
		p.confirm(d, i18n.Tr("editor.inspector.reset_name", "Reset {name}", i18n.Str("name", p.labelOf(td, fd))), func() error { return d.Props.Set(nil, p.nodes, td.ID, fd.ID, def) })
	}
	if p.overridden(td, fd) {
		p.prefabItems(d, prefabs.Change{Node: p.nodes[0], Level: prefabs.LevelProperty, Prop: prefab.PropKey{Owner: ownerOf(td), Field: fd.Name}}, true)
	}
	if td.Kind == schema.TypeScript {
		return
	}
	widgets.Separator(ui)
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.copy_component", "Copy Component"), "", "edit.copy", true, false) {
		if err := p.d.Clip.CopyComponent(d.Scene(), p.nodes[0], td.ID); err != nil {
			p.d.Notices.Notify(context.LevelError, err.Error())
		}
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.paste_component_values", "Paste Component Values"), "", "edit.paste", p.d.Clip.Peek() == clipboard.KindComponent, false) {
		p.confirm(d, i18n.Tr("editor.inspector.paste_name", "Paste {name}", i18n.Str("name", p.reg.Meta(td).DisplayName)), func() error { return p.d.Clip.PasteComponent(d, p.nodes) })
	}
}

func (p *Panel) resetTransform(d *document.SceneDocument) {
	err := d.Props.EditTransform(nil, p.nodes, func(tr *scene.Transform2D) {
		tr.Position = mathx.Vec2{}
		tr.Rotation = 0
		tr.Scale = mathx.Vec2{X: 1, Y: 1}
	})
	if err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
	}
}

func (p *Panel) presetItems(d *document.SceneDocument, td *schema.TypeDescriptor) {
	if p.d.Presets == nil || d.IsPlay() {
		return
	}
	ui := p.f.UI
	widgets.Separator(ui)
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.save_as_preset", "Save as Preset…"), "", "file.preset", len(p.nodes) == 1, false) {
		node := p.nodes[0]
		p.d.Notices.Ask(context.Question{
			Title:   i18n.Tr("editor.inspector.save_preset", "Save preset"),
			Message: i18n.Tr("editor.inspector.saved_to", "Saved to {dir}", i18n.Str("dir", presets.Dir)),
			Fields:  []string{i18n.Tr("editor.inspector.name", "Name")},
			Values:  []string{p.reg.Meta(td).DisplayName},
			Buttons: []string{i18n.Tr("editor.common.save", "Save"), i18n.Tr("editor.common.cancel", "Cancel")},
			Submit: func(v []string) {
				rel, err := p.d.Presets.Save(d, node, td, v[0])
				if err != nil {
					p.report(err)
					return
				}
				p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.inspector.saved", "Saved {path}", i18n.Str("path", rel)))
			},
		})
	}
	list := p.d.Presets.For(td.FullName)
	if len(list) == 0 {
		widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.apply_preset", "Apply Preset"), "", "file.preset", false, false)
		return
	}
	if widgets.SubMenuIcon(ui, i18n.Tr("editor.inspector.apply_preset", "Apply Preset"), "file.preset", 220) {
		for i, pr := range list {
			ui.PushIDInt(i)
			if widgets.MenuItemIcon(ui, pr.Name, "", "file.preset", true, false) {
				id, name := pr.ID, pr.Name
				p.confirm(d, i18n.Tr("editor.inspector.apply_preset_name", "Apply Preset {name}", i18n.Str("name", name)), func() error { return p.d.Presets.Apply(d, p.nodes, id) })
			}
			ui.PopID()
		}
		widgets.EndSubMenu(ui)
	}
}
