package settingsui

import (
	"strings"

	"aego/core/mathx"
	"aego/engine/settings"
	raw "aego/platform/input"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

type Editor struct {
	key   settings.Key
	index int
	on    bool
}

func (e *Editor) Capturing() bool {
	return e.on
}

func (e *Editor) Cancel() {
	e.on = false
}

const resetW = 24

func Partner(st *settings.Store, k settings.Key) settings.Key {
	d := st.Desc(k)
	if d == nil || d.Kind != schema.KindFloat64 || !strings.HasSuffix(d.ID, ".volume") {
		return 0
	}
	m := st.Key(strings.TrimSuffix(d.ID, ".volume") + ".mute")
	if md := st.Desc(m); md == nil || md.Kind != schema.KindBool {
		return 0
	}
	return m
}

func Paired(st *settings.Store, k settings.Key) bool {
	d := st.Desc(k)
	if d == nil || d.Kind != schema.KindBool || !strings.HasSuffix(d.ID, ".mute") {
		return false
	}
	return Partner(st, st.Key(strings.TrimSuffix(d.ID, ".mute")+".volume")) == k
}

func (e *Editor) Pair(ui *core.Context, st *settings.Store, k settings.Key, v *schema.Value, m settings.Key, mv *schema.Value) (bool, bool) {
	d, md := st.Desc(k), st.Desc(m)
	label := strings.TrimSuffix(d.Label(), " Volume")
	lr, r := widgets.FieldRects(ui, label)
	id := ui.ID(d.ID)
	if r.W > 160 {
		r.W -= resetW
		if !settings.Equal(*v, d.Default) || !settings.Equal(*mv, md.Default) {
			rr := mathx.Rect{X: r.X + r.W + 2, Y: r.Y, W: resetW - 2, H: r.H}
			if widgets.IconButton(ui, core.IDSeq(id, 9000), rr, "ui.reset", "Reset to "+st.FormatValue(k, d.Default)) {
				*v, *mv = d.Default, md.Default
				return true, true
			}
		}
	}
	sw := min(float32(84), r.W*0.35)
	slider := mathx.Rect{X: r.X, Y: r.Y, W: max(0, r.W-sw-6), H: r.H}
	mute := mathx.Rect{X: slider.X + slider.W + 6, Y: r.Y, W: sw, H: r.H}
	if !settings.Equal(*v, d.Default) || !settings.Equal(*mv, md.Default) {
		ui.Rect(mathx.Rect{X: lr.X - 4, Y: lr.Y + 4, W: 2, H: lr.H - 8}, ui.Theme.Accent)
	}
	on := mv.I != 0
	f := v.F
	o := widgets.NumberOpts{Speed: speed(d), Min: d.Min, Max: d.Max, HasMin: d.HasMin, HasMax: d.HasMax, Percent: true, Mixed: false}
	vc := false
	if on {
		ui.TextIn(slider, "Muted", ui.Theme.TextDisabled, core.AlignCenter)
		ui.MouseOver(id, slider)
	} else if ed := widgets.DragNumber(ui, id, slider, &f, o); ed.Changed {
		*v = schema.Float64(f)
		vc = true
	}
	mc := false
	if widgets.Switch(ui, ui.ID(md.ID), mute, &on, "Mute") {
		*mv = schema.Bool(on)
		mc = true
	}
	return vc, mc
}

func speed(d *settings.Desc) float64 {
	switch {
	case d.Step > 0:
		return d.Step
	case d.HasMin && d.HasMax:
		return (d.Max - d.Min) / 200
	case d.Kind == schema.KindInt64:
		return 0.1
	}
	return 0.01
}

func (e *Editor) Field(ui *core.Context, st *settings.Store, k settings.Key, v *schema.Value) bool {
	d := st.Desc(k)
	if d == nil {
		return false
	}
	lr, r := widgets.FieldRects(ui, d.Label())
	id := ui.ID(d.ID)
	if r.W > 120 {
		r.W -= resetW
		rr := mathx.Rect{X: r.X + r.W + 2, Y: r.Y, W: resetW - 2, H: r.H}
		if !settings.Equal(*v, d.Default) && st.Enabled(k) {
			ui.Rect(mathx.Rect{X: lr.X - 4, Y: lr.Y + 4, W: 2, H: lr.H - 8}, ui.Theme.Accent)
			rid := core.IDSeq(id, 9000)
			if widgets.IconButton(ui, rid, rr, "ui.reset", "Reset to "+st.FormatValue(k, d.Default)) {
				*v = d.Default
				return true
			}
		}
	}
	if !st.Enabled(k) {
		ui.TextIn(r, st.FormatValue(k, *v), ui.Theme.TextDisabled, core.AlignLeft)
		return false
	}
	if d.Flags.Has(settings.Controls) {
		return e.controls(ui, id, r, k, v)
	}
	if cs := st.Choices(k); len(cs) > 0 && d.Kind != schema.KindList {
		labels := make([]string, len(cs))
		cur := -1
		for i, c := range cs {
			labels[i] = c.Label
			if labels[i] == "" {
				labels[i] = c.Name
			}
			if settings.Equal(c.Value, *v) {
				cur = i
			}
		}
		if widgets.Combo(ui, id, r, &cur, labels, false) && cur >= 0 {
			*v = cs[cur].Value
			return true
		}
		return false
	}
	switch d.Kind {
	case schema.KindBool:
		on := v.I != 0
		label := "Off"
		if on {
			label = "On"
		}
		if widgets.Switch(ui, id, r, &on, label) {
			*v = schema.Bool(on)
			return true
		}
	case schema.KindInt64, schema.KindFloat64:
		f := v.F
		if d.Kind == schema.KindInt64 {
			f = float64(v.I)
		}
		o := widgets.NumberOpts{Speed: speed(d), Min: d.Min, Max: d.Max, HasMin: d.HasMin, HasMax: d.HasMax, Unit: d.Unit, Int: d.Kind == schema.KindInt64}
		o.Percent = d.Kind == schema.KindFloat64 && d.Unit == "" && d.HasMin && d.HasMax && d.Min == 0 && d.Max == 1
		if ed := widgets.DragNumber(ui, id, r, &f, o); ed.Changed {
			if d.Kind == schema.KindInt64 {
				*v = schema.Int64(int64(f))
			} else {
				*v = schema.Float64(f)
			}
			return true
		}
	case schema.KindString:
		s := v.S
		if _, ok := widgets.TextFieldRect(ui, id, r, &s, false); ok {
			*v = schema.String(s)
			return true
		}
	case schema.KindVec2:
		p := v.V
		if ed := widgets.Vec2Field(ui, id, r, &p, widgets.NumberOpts{Speed: speed(d)}, false, false); ed.Changed {
			*v = schema.Vec2(p)
			return true
		}
	case schema.KindColor:
		c := v.C
		if ed := widgets.ColorField(ui, id, r, &c, false); ed.Changed {
			*v = schema.Color(c)
			return true
		}
	default:
		ui.TextIn(r, st.FormatValue(k, *v), ui.Theme.TextDim, core.AlignLeft)
	}
	return false
}

func (e *Editor) controls(ui *core.Context, id core.ID, r mathx.Rect, k settings.Key, v *schema.Value) bool {
	changed := false
	if e.on && e.key == k {
		if c, ok := Pressed(ui.Input()); ok {
			list := append([]schema.Value(nil), v.List...)
			if e.index < len(list) {
				list[e.index] = schema.String(c)
			} else {
				list = append(list, schema.String(c))
			}
			*v = schema.Value{Kind: schema.KindList, List: list}
			e.on = false
			changed = true
		} else if ui.KeyPressed(raw.KeyEscape) {
			e.on = false
		}
	}
	x := r.X
	place := func(label string) (mathx.Rect, bool) {
		w := max(ui.TextSize(label).X+16, 56)
		br := mathx.Rect{X: x, Y: r.Y, W: min(w, r.X+r.W-x), H: r.H}
		x += w + 4
		return br, br.W > 8
	}
	for i, item := range v.List {
		label := item.S
		if e.on && e.key == k && e.index == i {
			label = "Press a key…"
		}
		br, ok := place(label)
		if !ok {
			break
		}
		if widgets.ButtonRect(ui, core.IDSeq(id, i+1), br, label) && !strings.Contains(item.S, "/") {
			e.key, e.index, e.on = k, i, true
		}
	}
	if br, ok := place("+"); ok && widgets.ButtonRect(ui, core.IDSeq(id, 0), br, "+") {
		e.key, e.index, e.on = k, len(v.List), true
	}
	return changed
}

func Pressed(in *raw.State) (string, bool) {
	if in == nil {
		return "", false
	}
	for k := raw.Scancode(1); k < raw.ScancodeCount; k++ {
		if k != raw.KeyEscape && in.KeyPressed(k) {
			if n, ok := raw.KeyName(k); ok {
				return "Key." + n, true
			}
		}
	}
	for _, b := range [...]struct {
		b raw.Button
		n string
	}{{raw.MouseRight, "Right"}, {raw.MouseMiddle, "Middle"}, {raw.MouseX1, "X1"}, {raw.MouseX2, "X2"}} {
		if in.ButtonPressed(b.b) {
			return "Mouse." + b.n, true
		}
	}
	for i := 0; i < raw.MaxPads; i++ {
		if !in.PadConnected(i) {
			continue
		}
		for b := raw.PadButton(0); b < raw.PadButtonCount; b++ {
			if in.PadPressed(i, b) {
				return "Pad." + b.String(), true
			}
		}
		for a := raw.PadAxis(0); a < raw.PadAxisCount; a++ {
			if mathx.Abs(in.PadAxisValue(i, a)) > 0.6 {
				return "Pad." + a.String(), true
			}
		}
	}
	return "", false
}
