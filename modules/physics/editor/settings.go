package editor

import (
	"strconv"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/modules/physics/settings"
	"aego/modules/physics/world"
	"aego/ui/core"
	"aego/ui/widgets"
)

func intRow(ui *core.Context, label string, v *int, def int) bool {
	f := float64(*v)
	if *v == 0 {
		f = float64(def)
		label += " (default)"
	}
	e := widgets.DragNumber(ui, ui.ID(label), widgets.Field(ui, label), &f, widgets.NumberOpts{Int: true, Speed: 0.1, Min: 0, Max: 64, HasMin: true, HasMax: true})
	if e.Changed {
		*v = int(f)
	}
	return e.Changed
}

func floatRow(ui *core.Context, label string, v *float32, def float32) bool {
	f := float64(*v)
	if *v == 0 {
		f = float64(def)
		label += " (default)"
	}
	e := widgets.DragNumber(ui, ui.ID(label), widgets.Field(ui, label), &f, widgets.NumberOpts{Speed: 0.01, Min: 0, HasMin: true, Prec: 4})
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
	widgets.Label(ui, "Simulation")
	g := ps.Gravity
	if !ps.HasGravity {
		g = def.Gravity
	}
	if e := widgets.Vec2Field(ui, ui.ID("gravity"), widgets.Field(ui, "Gravity"), &g, widgets.NumberOpts{Speed: 0.05}, false, false); e.Changed {
		ps.Gravity, ps.HasGravity = g, true
		changed = true
	}
	changed = intRow(ui, "Substeps", &ps.Substeps, def.Substeps) || changed
	changed = intRow(ui, "Velocity Iterations", &ps.VelocityIterations, def.VelocityIterations) || changed
	changed = intRow(ui, "Relax Iterations", &ps.RelaxIterations, def.RelaxIterations) || changed
	for _, x := range []struct {
		label string
		v     *float32
		def   float32
	}{
		{"Contact Hertz", &ps.ContactHertz, def.ContactHertz},
		{"Contact Damping Ratio", &ps.ContactDampingRatio, def.ContactDampingRatio},
		{"Contact Push Velocity", &ps.ContactPushVelocity, def.ContactPushVelocity},
		{"Restitution Threshold", &ps.RestitutionThreshold, def.RestitutionThreshold},
		{"Linear Slop", &ps.LinearSlop, def.LinearSlop},
		{"Sleep Linear", &ps.SleepLinear, def.SleepLinear},
		{"Sleep Angular", &ps.SleepAngular, def.SleepAngular},
		{"Sleep Time", &ps.SleepTime, def.SleepTime},
	} {
		changed = floatRow(ui, x.label, x.v, x.def) || changed
	}
	if err := ps.Validate(); err != nil {
		widgets.LabelWrapped(ui, err.Error(), false)
	}
	widgets.LabelWrapped(ui, "Tall stacks and fast motors: Substeps 8 and Contact Hertz 60 (stiff profile) at about twice the solver cost. Zero keeps the engine default.", true)
	widgets.Separator(ui)
	widgets.Label(ui, "Layers")
	changed = layers(ctx, ps) || changed
	widgets.Separator(ui)
	widgets.Label(ui, "Collision Matrix")
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
		ui.Text(strconv.Itoa(i), mathx.Vec2{X: row.X, Y: ui.TextY(row)}, th.TextDim)
		fr := mathx.Rect{X: row.X + 28, Y: row.Y, W: row.W - 28 - row.H - 4, H: row.H}
		name := names[i]
		if _, ok := widgets.TextFieldRect(ui, ui.ID("name"), fr, &name, false); ok && name != names[i] {
			switch {
			case name != "" && !settings.ValidLayerName(name):
				ctx.Notify(context.LevelWarn, name+" is not a valid layer name")
			case name != "" && dup(names, name):
				ctx.Notify(context.LevelWarn, "layer "+name+" already exists")
			default:
				ensureLayers(ps, i+1)
				ps.LayerNames[i] = name
				renameRules(ps, names[i], name)
				changed = true
			}
		}
		if i > 0 && widgets.ButtonRect(ui, ui.ID("clear"), mathx.Rect{X: row.X + row.W - row.H, Y: row.Y, W: row.H, H: row.H}, "−") {
			ensureLayers(ps, i+1)
			ps.LayerNames[i] = ""
			renameRules(ps, names[i], "")
			changed = true
		}
		ui.PopID()
	}
	if widgets.Button(ui, "Add Layer") {
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
	widgets.LabelDim(ui, "Bodies store the layer index; clearing a name keeps the index.")
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
	labelW := float32(110)
	for row, i := range idx {
		r := ui.Cursor().Next(0)
		ui.PushClip(mathx.Rect{X: r.X, Y: r.Y, W: labelW - 4, H: r.H})
		ui.Text(names[i], mathx.Vec2{X: r.X, Y: ui.TextY(r)}, th.TextDim)
		ui.PopClip()
		for col := 0; col <= row; col++ {
			j := idx[col]
			cell := mathx.Rect{X: r.X + labelW + float32(col)*(r.H+2), Y: r.Y, W: r.H, H: r.H}
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
			ui.Rect(cell, ui.WidgetColor(id))
			ui.Outline(cell, th.Border)
			if on {
				widgets.CheckMark(ui, cell, th.Accent)
			}
			widgets.Tooltip(ui, id, names[i]+" × "+names[j])
		}
	}
	if changed {
		ps.SetMatrix(m)
	}
	return changed
}
