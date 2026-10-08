package inspector

import (
	"aego/core/mathx"
	"aego/editor/clipboard"
	"aego/editor/context"
	"aego/editor/document"
	"aego/scene"
	"aego/schema"
	"aego/ui/core"
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

func (p *Panel) beginCard(ctx *context.Editor, key, title, badge, icon, action string, menu bool) card {
	ui := ctx.UI
	th := &ui.Theme
	cur := ui.Cursor()
	c := card{bg: ui.List().Reserve(), top: cur.Y}
	hdr := cur.Next(th.RowHeight + 4)
	c.hdr = hdr
	c.hid = ui.ID("card")
	ui.Rect(hdr, th.CardHeader)
	right := hdr.X + hdr.W - 4
	bh := hdr.H - 6
	if menu {
		mr := mathx.Rect{X: right - bh, Y: hdr.Y + 3, W: bh, H: bh}
		c.more = widgets.ButtonRect(ui, ui.ID("more"), mr, widgets.Glyph(ui, '…', "..."))
		right = mr.X - 4
	}
	if action != "" {
		w := ui.TextSize(action).X + 14
		ar := mathx.Rect{X: right - w, Y: hdr.Y + 3, W: w, H: bh}
		c.action = widgets.ButtonRect(ui, ui.ID("action"), ar, action)
		right = ar.X - 6
	}
	area := mathx.Rect{X: hdr.X, Y: hdr.Y, W: max(0, right-hdr.X), H: hdr.H}
	if ui.Pressed(c.hid, area) && ctx.Collapsed != nil {
		ctx.Collapsed[key] = !ctx.Collapsed[key]
	}
	c.open = ctx.Collapsed == nil || !ctx.Collapsed[key]
	if ui.Hot() == c.hid {
		hl := th.WidgetHot
		hl.A = 0.5
		ui.Rect(area, hl)
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
	ui.List().Patch(c.bg, rect, ui.Theme.CardBg)
	if c.typ != 0 && ui.Hit(rect) && !ui.MouseInOverlay() {
		p.hoverType = c.typ
	}
	cur.Y += 4
}

func (p *Panel) componentMenu(ctx *context.Editor, d *document.SceneDocument, td *schema.TypeDescriptor) {
	ui := ctx.UI
	if widgets.MenuItem(ui, "Reset", "", true, false) {
		p.reset(ctx, d, td)
	}
	if widgets.MenuItem(ui, "Remove", "", true, false) {
		p.remove(ctx, d, td)
	}
	widgets.Separator(ui)
	if widgets.MenuItem(ui, "Copy Component", ctx.Commands.Shortcut(CommandCopy), true, false) {
		if err := clipboard.CopyComponent(ctx, d.Scene(), p.nodes[0], td.ID); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		} else {
			ctx.Notify(context.LevelInfo, "Copied "+p.reg.Meta(td).DisplayName)
		}
	}
	same := false
	if clipboard.Peek(ctx) == clipboard.KindComponent {
		if src, _, ok := clipboard.ReadComponent(ctx); ok && src.ID == td.ID {
			same = true
		}
	}
	if widgets.MenuItem(ui, "Paste Component Values", ctx.Commands.Shortcut(CommandPaste), same, false) {
		if err := clipboard.PasteComponent(ctx, d, p.nodes); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		}
	}
	if ctx.Docs.InPlay() && ctx.Play != nil {
		widgets.Separator(ui)
		if widgets.MenuItem(ui, "Copy values to edit scene", "", ctx.Play.CanCopyToEdit(p.nodes, td.ID), false) {
			if err := ctx.Play.CopyToEdit(p.nodes, td.ID); err != nil {
				ctx.Notify(context.LevelError, err.Error())
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

func (p *Panel) copyValue(ctx *context.Editor, d *document.SceneDocument, td *schema.TypeDescriptor, fd *schema.FieldDescriptor) {
	v, mixed, err := d.Props.Common(p.nodes, td.ID, fd.ID)
	if err != nil {
		ctx.Notify(context.LevelError, err.Error())
		return
	}
	text := p.summary(ctx, d, fd, v, mixed)
	if err := clipboard.CopyValue(ctx, d.Scene(), p.nodes[0], td.ID, fd.ID, text); err != nil {
		ctx.Notify(context.LevelError, err.Error())
	}
}

func (p *Panel) pasteValue(ctx *context.Editor, d *document.SceneDocument, td *schema.TypeDescriptor, fd *schema.FieldDescriptor) {
	v, ok := clipboard.ReadValue(ctx, fd)
	if !ok {
		ctx.Notify(context.LevelInfo, "The clipboard does not fit "+p.labelOf(fd))
		return
	}
	if err := d.Props.Set(nil, p.nodes, td.ID, fd.ID, v); err != nil {
		ctx.Notify(context.LevelError, err.Error())
	}
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

func (p *Panel) fieldMenu(ctx *context.Editor, d *document.SceneDocument) {
	ui := ctx.UI
	td, fd, ok := p.lookup(d, p.menuField)
	if !ok {
		return
	}
	ro := fd.Flags.Has(schema.FlagReadOnly)
	if widgets.MenuItem(ui, "Copy Value", ctx.Commands.Shortcut(CommandCopy), true, false) {
		p.copyValue(ctx, d, td, fd)
	}
	_, canPaste := clipboard.ReadValue(ctx, fd)
	if widgets.MenuItem(ui, "Paste Value", ctx.Commands.Shortcut(CommandPaste), canPaste && !ro, false) {
		p.pasteValue(ctx, d, td, fd)
	}
	if widgets.MenuItem(ui, "Reset to Default", "", !ro, false) {
		if err := d.Props.Set(nil, p.nodes, td.ID, fd.ID, p.defaultValue(d.Scene().Registry(), td, fd)); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		}
	}
	widgets.Separator(ui)
	if widgets.MenuItem(ui, "Copy Component", "", true, false) {
		if err := clipboard.CopyComponent(ctx, d.Scene(), p.nodes[0], td.ID); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		}
	}
	if widgets.MenuItem(ui, "Paste Component Values", "", clipboard.Peek(ctx) == clipboard.KindComponent, false) {
		if err := clipboard.PasteComponent(ctx, d, p.nodes); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		}
	}
}

func (p *Panel) resetTransform(ctx *context.Editor, d *document.SceneDocument) {
	err := d.Props.EditTransform(nil, p.nodes, func(tr *scene.Transform2D) {
		tr.Position = mathx.Vec2{}
		tr.Rotation = 0
		tr.Scale = mathx.Vec2{X: 1, Y: 1}
	})
	if err != nil {
		ctx.Notify(context.LevelError, err.Error())
	}
}
