package editor

import (
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/shape"
	"aego/engine/i18n"
	"aego/modules/physics/components"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

func (p *plugin) edit(ctx *context.Editor, d *document.SceneDocument, nodes []ids.NodeID, fd *schema.FieldDescriptor, e widgets.Edit, v schema.Value) {
	if e.Begin && p.tx == nil {
		p.tx = d.History.Begin(i18n.Tr("editor.history.set_mass", "Set Mass"))
	}
	if e.Changed {
		if err := d.Props.Set(p.tx, nodes, p.body.ID, fd.ID, v); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		}
	}
	if e.End && p.tx != nil {
		p.tx.Commit()
		p.tx = nil
	}
}

func (p *plugin) bodyInspector(ctx *context.Editor, nodes []ids.NodeID, def func()) {
	d := ctx.Doc()
	massFD, ok1 := p.body.Field("mass")
	modeFD, ok2 := p.body.Field("massMode")
	if d == nil || !ok1 || !ok2 {
		def()
		return
	}
	ui := ctx.UI
	mode, modeMixed, _ := d.Props.Common(nodes, p.body.ID, modeFD.ID)
	switch {
	case !modeMixed && components.MassMode(mode.I) == components.MassOverride:
		v, mixed, _ := d.Props.Common(nodes, p.body.ID, massFD.ID)
		f := v.AsFloat()
		e := widgets.DragNumber(ui, ui.ID("mass"), widgets.Field(ui, i18n.Tr("i18n.physics.mass", "Mass")), &f, widgets.NumberOpts{Speed: 0.05, Min: 0, HasMin: true, Mixed: mixed})
		p.edit(ctx, d, nodes, massFD, e, schema.Float32(float32(f)))
	case len(nodes) == 1:
		text := "from colliders"
		if sim := p.sim(); sim != nil {
			if bh, ok := sim.BodyOf(d.Scene().Handle(nodes[0])); ok {
				m, _, _, _ := sim.World().BodyMass(bh)
				text = i18n.Tr("i18n.physics.formatnumber_from_colliders", "{formatnumber} (from colliders)", i18n.Str("formatnumber", widgets.FormatNumber(float64(m), widgets.NumberOpts{})))
			}
		}
		ui.TextIn(widgets.Field(ui, i18n.Tr("i18n.physics.mass", "Mass")), text, ui.Theme.TextDim, core.AlignLeft)
	}
	def()
}

func (p *plugin) colliderInspector(k components.Kind) func(*context.Editor, []ids.NodeID, func()) {
	return func(ctx *context.Editor, nodes []ids.NodeID, def func()) {
		d := ctx.Doc()
		ui := ctx.UI
		th := &ui.Theme
		if d != nil && len(nodes) == 1 {
			if msg, bad := p.issue(nodes[0], components.Names[k]); bad {
				for _, line := range widgets.WrapLines(ui, msg, ui.Cursor().W-8) {
					r := ui.Cursor().Next(0)
					bg := th.Error
					bg.A = 0.18
					ui.Rect(r, bg)
					ui.Text(line, mathx.Vec2{X: r.X + 4, Y: ui.TextY(r)}, th.Error)
				}
			}
		}
		def()
		if d == nil || k == components.KindPolygon || k == components.KindEdge {
			return
		}
		if widgets.Button(ui, i18n.Tr("i18n.physics.snap_to_sprite", "Snap to Sprite")) {
			p.snapToSprite(ctx, d, nodes, k)
		}
	}
}

func (p *plugin) snapToSprite(ctx *context.Editor, d *document.SceneDocument, nodes []ids.NodeID, k components.Kind) {
	s := d.Scene()
	set := shape.Set{Reg: ctx.Shapes}
	if ctx.Loader != nil {
		set.Res = ctx.Loader
	}
	td := p.types[k]
	type value struct {
		name string
		v    schema.Value
	}
	tx := d.History.Begin(i18n.Tr("editor.history.snap_to_sprite", "Snap to Sprite"))
	done := 0
	for _, n := range nodes {
		sh, ok := set.Of(s, s.Handle(n))
		if !ok || sh.Kind != shape.Rect || sh.Order < 0 {
			continue
		}
		r := sh.Local
		vals := []value{{"offset", schema.Vec2(mathx.Vec2{X: r.X + r.W/2, Y: r.Y + r.H/2})}}
		switch k {
		case components.KindBox:
			vals = append(vals, value{"size", schema.Vec2(mathx.Vec2{X: r.W, Y: r.H})})
		case components.KindCircle:
			vals = append(vals, value{"radius", schema.Float32(max(r.W, r.H) / 2)})
		case components.KindCapsule:
			vals = append(vals, value{"length", schema.Float32(r.W)}, value{"radius", schema.Float32(r.H / 2)})
		}
		for _, x := range vals {
			fd, ok := td.Field(x.name)
			if !ok {
				continue
			}
			if err := d.Props.Set(tx, []ids.NodeID{n}, td.ID, fd.ID, x.v); err != nil {
				tx.Abort()
				ctx.Notify(context.LevelError, err.Error())
				return
			}
		}
		done++
	}
	tx.Commit()
	if done == 0 {
		ctx.Notify(context.LevelInfo, i18n.Note("i18n.physics.no_sprite_on_the_selected", "No sprite on the selected nodes"))
	}
}
