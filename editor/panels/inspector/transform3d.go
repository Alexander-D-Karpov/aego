package inspector

import (
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history"
	"aego/editor/core/props"
	"aego/scene"
	"aego/ui/core"
	"aego/ui/widgets"
)

var spaceNames = []string{"2D", "3D"}

func (p *Panel) transform(d *document.SceneDocument) {
	s := d.Scene()
	sp := s.Space(s.Handle(p.nodes[0]))
	for _, id := range p.nodes[1:] {
		if s.Space(s.Handle(id)) != sp {
			ui := p.f.UI
			ui.PushID("transform")
			c := p.beginCard("transform", "Transform", "", "cat.transform", "", false)
			if c.open {
				widgets.LabelDim(ui, "Selected nodes live in different spaces")
			}
			p.endCard(ui, c)
			ui.PopID()
			return
		}
	}
	if sp == scene.Space3D {
		p.transform3D(d)
		return
	}
	p.transform2D(d)
}

func (p *Panel) transform3D(d *document.SceneDocument) {
	ui := p.f.UI
	ui.PushID("transform3d")
	defer ui.PopID()
	card := p.beginCard("transform", "Transform", "", "cat.transform", "Reset", false)
	defer p.endCard(ui, card)
	if card.action {
		err := d.Props.EditTransform3D(nil, p.nodes, func(t *scene.Transform3D) {
			*t = scene.IdentityTransform3D()
		})
		if err != nil {
			p.d.Notices.Notify(context.LevelError, err.Error())
		}
	}
	if !card.open {
		return
	}
	t, _, mixed := d.Props.CommonTransform3D(p.nodes)
	euler := p.euler.Get(p.nodes[0], t.Rotation)
	rows := [3]struct {
		label string
		id    core.ID
		v     mathx.Vec3
		base  int
		o     widgets.NumberOpts
	}{
		{"Position", ui.ID("pos"), t.Position, props.T3PosX, widgets.NumberOpts{Speed: 0.05}},
		{"Rotation", ui.ID("rot"), euler, props.T3RotX, widgets.NumberOpts{Speed: 0.5, Prec: 2, Suffix: "°"}},
		{"Scale", ui.ID("scale"), t.Scale, props.T3ScaleX, widgets.NumberOpts{Speed: 0.01}},
	}
	for _, row := range rows {
		v := row.v
		m := [3]bool{mixed[row.base], mixed[row.base+1], mixed[row.base+2]}
		e := widgets.Vec3Field(ui, row.id, widgets.Field(ui, row.label), &v, row.o, m)
		if !e.Any() {
			continue
		}
		vals := [3]float32{v.X, v.Y, v.Z}
		olds := [3]float32{row.v.X, row.v.Y, row.v.Z}
		base := row.base
		p.apply(e, row.label, func(tx *history.Transaction) error {
			for i := 0; i < 3; i++ {
				if vals[i] == olds[i] && !m[i] {
					continue
				}
				if err := d.Props.SetTransform3DComponent(tx, p.nodes, p.euler, base+i, vals[i]); err != nil {
					return err
				}
			}
			return nil
		})
	}
}

func (p *Panel) spaceRow(d *document.SceneDocument) {
	ui := p.f.UI
	th := &ui.Theme
	s := d.Scene()
	sp := s.Space(s.Handle(p.nodes[0]))
	mixed, locked, reason := false, false, ""
	for _, id := range p.nodes {
		if s.Space(s.Handle(id)) != sp {
			mixed = true
		}
		if l, why := d.Props.SpaceLock(id); l && !locked {
			locked, reason = true, why
		}
	}
	r := widgets.Field(ui, "Space")
	if locked {
		label := spaceNames[sp]
		if mixed {
			label = th.Mixed
		}
		ui.TextIn(r, label, th.TextDim, core.AlignLeft)
		lid := ui.ID("spacelock")
		ui.MouseOver(lid, r)
		widgets.Tooltip(ui, lid, reason)
		return
	}
	cur := int(sp)
	if widgets.Combo(ui, ui.ID("space"), r, &cur, spaceNames, mixed) {
		if err := d.Props.SetSpace(nil, p.nodes, scene.Space(cur)); err != nil {
			p.d.Notices.Notify(context.LevelError, err.Error())
		}
	}
}
