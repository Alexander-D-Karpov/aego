package editor

import (
	"strconv"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/engine/i18n"
	"aego/modules/physics/settings"
	"aego/modules/physics/world"
	"aego/ui/core"
	"aego/ui/widgets"
)

func intRow(ui *core.Context, label string, v *int, def int) bool {
	f := float64(*v)
	o := widgets.NumberOpts{Int: true, Speed: 0.1, Min: 0, Max: 64, HasMin: true, HasMax: true}
	if *v == 0 {
		f = float64(def)
		o.Suffix = "  default"
	}
	e := widgets.DragNumber(ui, ui.ID(label), widgets.Field(ui, label), &f, o)
	if e.Changed {
		*v = int(f)
	}
	return e.Changed
}

func floatRow(ui *core.Context, label string, v *float32, def float32) bool {
	f := float64(*v)
	o := widgets.NumberOpts{Speed: 0.01, Min: 0, HasMin: true, Prec: 4}
	if *v == 0 {
		f = float64(def)
		o.Suffix = "  default"
	}
	e := widgets.DragNumber(ui, ui.ID(label), widgets.Field(ui, label), &f, o)
	if e.Changed {
		*v = float32(f)
	}
	return e.Changed
}

func (p *plugin) settingsPage(ctx *context.Editor, draft any) bool {
	ps, ok := draft.(*settings.Project)
	if !ok {
		return false
	}
	ui := ctx.UI
	def := world.DefaultConfig()
	changed := false
	cfg := ps.Config()
	p.preview(ui, cfg)
	focus := ""
	hot := func(label, part string) {
		if ui.Hot() == ui.ID(label) || ui.Active() == ui.ID(label) {
			focus = part
		}
	}
	widgets.Label(ui, i18n.Tr("i18n.physics.simulation", "Simulation"))
	g := ps.Gravity
	if !ps.HasGravity {
		g = def.Gravity
	}
	gid := ui.ID("gravity")
	if e := widgets.Vec2Field(ui, gid, widgets.Field(ui, i18n.Tr("i18n.physics.gravity", "Gravity")), &g, widgets.NumberOpts{Speed: 0.05}, false, false); e.Changed {
		ps.Gravity, ps.HasGravity = g, true
		changed = true
	}
	if h := ui.Hot(); h == core.IDSeq(gid, 0) || h == core.IDSeq(gid, 1) || ui.Active() == core.IDSeq(gid, 0) || ui.Active() == core.IDSeq(gid, 1) {
		focus = "gravity"
	}
	for _, x := range []struct {
		label string
		v     *int
		def   int
	}{
		{i18n.Tr("i18n.physics.substeps", "Substeps"), &ps.Substeps, def.Substeps},
		{i18n.Tr("i18n.physics.velocity_iterations", "Velocity Iterations"), &ps.VelocityIterations, def.VelocityIterations},
		{i18n.Tr("i18n.physics.relax_iterations", "Relax Iterations"), &ps.RelaxIterations, def.RelaxIterations},
	} {
		changed = intRow(ui, x.label, x.v, x.def) || changed
		hot(x.label, "solver")
	}
	for _, x := range []struct {
		label string
		part  string
		v     *float32
		def   float32
	}{
		{i18n.Tr("i18n.physics.contact_hertz", "Contact Hertz"), "contact", &ps.ContactHertz, def.ContactHertz},
		{i18n.Tr("i18n.physics.contact_damping_ratio", "Contact Damping Ratio"), "contact", &ps.ContactDampingRatio, def.ContactDampingRatio},
		{i18n.Tr("i18n.physics.contact_push_velocity", "Contact Push Velocity"), "contact", &ps.ContactPushVelocity, def.ContactPushVelocity},
		{i18n.Tr("i18n.physics.restitution_threshold", "Restitution Threshold"), "bounce", &ps.RestitutionThreshold, def.RestitutionThreshold},
		{i18n.Tr("i18n.physics.linear_slop", "Linear Slop"), "contact", &ps.LinearSlop, def.LinearSlop},
		{i18n.Tr("i18n.physics.sleep_linear", "Sleep Linear"), "sleep", &ps.SleepLinear, def.SleepLinear},
		{i18n.Tr("i18n.physics.sleep_angular", "Sleep Angular"), "sleep", &ps.SleepAngular, def.SleepAngular},
		{i18n.Tr("i18n.physics.sleep_time", "Sleep Time"), "sleep", &ps.SleepTime, def.SleepTime},
	} {
		changed = floatRow(ui, x.label, x.v, x.def) || changed
		hot(x.label, x.part)
	}
	p.focus = focus
	if err := ps.Validate(); err != nil {
		widgets.LabelWrapped(ui, err.Error(), false)
	}
	widgets.LabelWrapped(ui, i18n.Tr("i18n.physics.tall_stacks_and_fast_motors", "Tall stacks and fast motors: Substeps 8 and Contact Hertz 60 (stiff profile) at about twice the solver cost. Zero keeps the engine default."), true)
	widgets.Separator(ui)
	widgets.Label(ui, i18n.Tr("i18n.physics.layers", "Layers"))
	changed = layers(ctx, ps) || changed
	widgets.Separator(ui)
	widgets.Label(ui, i18n.Tr("i18n.physics.collision_matrix", "Collision Matrix"))
	widgets.LabelDim(ui, i18n.Tr("i18n.physics.a_checked_cell_means_bodies", "A checked cell means bodies on those two layers collide."))
	changed = matrix(ui, ps) || changed
	return changed
}

func ensureLayers(ps *settings.Project, n int) {
	if len(ps.LayerNames) == 0 {
		ps.LayerNames = []string{settings.DefaultLayer}
	}
	for len(ps.LayerNames) < n {
		ps.LayerNames = append(ps.LayerNames, "")
	}
}

func renameRules(ps *settings.Project, old, next string) {
	out := ps.Collide[:0]
	for _, r := range ps.Collide {
		if r.Layer == old {
			if next == "" {
				continue
			}
			r.Layer = next
		}
		with := r.With[:0]
		for _, w := range r.With {
			switch {
			case w != old:
				with = append(with, w)
			case next != "":
				with = append(with, next)
			}
		}
		r.With = with
		out = append(out, r)
	}
	ps.Collide = out
}

func layers(ctx *context.Editor, ps *settings.Project) bool {
	ui := ctx.UI
	th := &ui.Theme
	names := ps.Layers()
	changed := false
	last := 0
	for i, n := range names {
		if n != "" {
			last = i
		}
	}
	for i := 0; i <= last; i++ {
		ui.PushIDInt(i)
		row := ui.Cursor().Next(0)
		chip := mathx.Rect{X: row.X, Y: row.Y + 2, W: 24, H: row.H - 4}
		ui.RoundRect(chip, th.Radius.Button, th.Badge)
		ui.TextIn(chip, strconv.Itoa(i), th.TextDim, core.AlignCenter)
		fr := mathx.Rect{X: row.X + 30, Y: row.Y, W: row.W - 30 - row.H - 4, H: row.H}
		name := names[i]
		if _, ok := widgets.TextFieldRect(ui, ui.ID("name"), fr, &name, false); ok && name != names[i] {
			switch {
			case name != "" && !settings.ValidLayerName(name):
				ctx.Notify(context.LevelWarn, i18n.Note("i18n.physics.name_is_not_a_valid", "{name} is not a valid layer name", i18n.Str("name", name)))
			case name != "" && dup(names, name):
				ctx.Notify(context.LevelWarn, i18n.Note("i18n.physics.layer_name_already_exists", "layer {name} already exists", i18n.Str("name", name)))
			default:
				ensureLayers(ps, i+1)
				ps.LayerNames[i] = name
				renameRules(ps, names[i], name)
				changed = true
			}
		}
		if i > 0 && widgets.IconButton(ui, ui.ID("clear"), mathx.Rect{X: row.X + row.W - row.H, Y: row.Y, W: row.H, H: row.H}, "ui.minus", i18n.Tr("i18n.physics.clear_the_name_bodies_keep", "Clear the name; bodies keep index {i}", i18n.Str("i", strconv.Itoa(i)))) {
			ensureLayers(ps, i+1)
			ps.LayerNames[i] = ""
			renameRules(ps, names[i], "")
			changed = true
		}
		ui.PopID()
	}
	if last < settings.MaxLayers-1 && widgets.Button(ui, i18n.Tr("i18n.physics.add_layer", "Add Layer")) {
		for i := 1; i < settings.MaxLayers; i++ {
			if names[i] != "" {
				continue
			}
			ensureLayers(ps, i+1)
			ps.LayerNames[i] = "Layer" + strconv.Itoa(i)
			changed = true
			break
		}
	}
	widgets.LabelDim(ui, i18n.Tr("i18n.physics.bodies_store_the_layer_index", "Bodies store the layer index; clearing a name keeps the index."))
	return changed
}

func dup(names [settings.MaxLayers]string, n string) bool {
	for _, x := range names {
		if x == n {
			return true
		}
	}
	return false
}

func matrix(ui *core.Context, ps *settings.Project) bool {
	th := &ui.Theme
	names := ps.Layers()
	var idx []int
	for i, n := range names {
		if n != "" {
			idx = append(idx, i)
		}
	}
	m := ps.Matrix()
	changed := false
	cs := ui.Theme.RowHeight
	labelW := float32(30)
	for _, i := range idx {
		labelW = max(labelW, ui.TextSize(strconv.Itoa(i)+"  "+names[i]).X+16)
	}
	labelW = min(labelW, 160)
	head := ui.Cursor().Next(0)
	hotRow, hotCol := -1, -1
	for row, i := range idx {
		for col := 0; col <= row; col++ {
			j := idx[col]
			if ui.Hot() == ui.IDInt(i*64+j) {
				hotRow, hotCol = row, col
			}
		}
	}
	for col, j := range idx {
		cr := mathx.Rect{X: head.X + labelW + float32(col)*(cs+2), Y: head.Y, W: cs, H: head.H}
		c := th.TextDim
		if col == hotCol || col == hotRow {
			c = th.Text
		}
		ui.TextIn(cr, strconv.Itoa(j), c, core.AlignCenter)
	}
	for row, i := range idx {
		r := ui.Cursor().Next(0)
		c := th.TextDim
		if row == hotRow || row == hotCol {
			c = th.Text
		}
		ui.Text(widgets.Elide(ui, strconv.Itoa(i)+"  "+names[i], labelW-8), mathx.Vec2{X: r.X, Y: ui.TextY(r)}, c)
		for col := 0; col <= row; col++ {
			j := idx[col]
			cell := mathx.Rect{X: r.X + labelW + float32(col)*(cs+2), Y: r.Y, W: cs, H: r.H}
			id := ui.IDInt(i*64 + j)
			on := (m[i]>>j)&1 == 1
			if ui.Clicked(id, cell) {
				on = !on
				if on {
					m[i] |= 1 << j
					m[j] |= 1 << i
				} else {
					m[i] &^= 1 << j
					m[j] &^= 1 << i
				}
				changed = true
			}
			bg := th.Widget
			if ui.Hot() == id {
				bg = th.WidgetHot
			}
			if on {
				bg = th.Accent
			}
			ui.RoundRect(mathx.Rect{X: cell.X + 1, Y: cell.Y + 1, W: cell.W - 2, H: cell.H - 2}, th.Radius.Button, bg)
			if on {
				widgets.CheckMark(ui, cell, th.Background)
			}
			verb := " do not collide"
			if on {
				verb = " collide"
			}
			widgets.Tooltip(ui, id, names[i]+" and "+names[j]+verb)
		}
	}
	if changed {
		ps.SetMatrix(m)
	}
	return changed
}
