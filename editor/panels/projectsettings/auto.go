package projectsettings

import (
	"reflect"

	"aego/editor/ext/inspect"
	"aego/engine/i18n"
	"aego/engine/module"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

func auto(ui *core.Context, sec *module.Section, draft any) bool {
	rv := reflect.ValueOf(draft)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return false
	}
	ptr := rv.UnsafePointer()
	changed := false
	for i := range sec.Type.Fields {
		fd := &sec.Type.Fields[i]
		if !fd.Flags.Has(schema.FlagEditor) || fd.Hints.Hidden {
			continue
		}
		var v schema.Value
		fd.Get(ptr, &v)
		label := fd.Hints.DisplayName
		if label == "" {
			label = inspect.Humanize(fd.Name)
		}
		ui.PushID(fd.Name)
		nv, ok := field(ui, label, fd, v)
		ui.PopID()
		if ok && fd.Set(ptr, nv) == nil {
			changed = true
		}
		if fd.Doc != "" {
			widgets.LabelWrapped(ui, fd.Doc, true)
		}
	}
	return changed
}

func numOpts(fd *schema.FieldDescriptor, isInt bool, speed float64) widgets.NumberOpts {
	return widgets.NumberOpts{Int: isInt, Speed: speed, Min: fd.Min, Max: fd.Max, HasMin: fd.HasMin, HasMax: fd.HasMax}
}

func field(ui *core.Context, label string, fd *schema.FieldDescriptor, v schema.Value) (schema.Value, bool) {
	r := widgets.Field(ui, label)
	id := ui.ID("value")
	switch fd.Kind {
	case schema.KindBool:
		b := v.I != 0
		before := b
		text := i18n.Tr("editor.projectsettings.off", "Off")
		if b {
			text = i18n.Tr("editor.projectsettings.on", "On")
		}
		widgets.ToggleButton(ui, id, r, text, &b)
		return schema.Bool(b), b != before
	case schema.KindInt32, schema.KindInt64:
		f := float64(v.I)
		if e := widgets.DragNumber(ui, id, r, &f, numOpts(fd, true, 0.1)); !e.Changed {
			return v, false
		}
		if fd.Kind == schema.KindInt32 {
			return schema.Int32(int32(f)), true
		}
		return schema.Int64(int64(f)), true
	case schema.KindFloat32, schema.KindFloat64:
		f := v.F
		if e := widgets.DragNumber(ui, id, r, &f, numOpts(fd, false, 0.01)); !e.Changed {
			return v, false
		}
		if fd.Kind == schema.KindFloat32 {
			return schema.Float32(float32(f)), true
		}
		return schema.Float64(f), true
	case schema.KindVec2:
		vec := v.V
		e := widgets.Vec2Field(ui, id, r, &vec, widgets.NumberOpts{Speed: 0.05}, false, false)
		return schema.Vec2(vec), e.Changed
	case schema.KindColor:
		c := v.C
		e := widgets.ColorField(ui, id, r, &c, false)
		return schema.Color(c), e.Changed
	case schema.KindString:
		s := v.S
		if _, committed := widgets.TextFieldRect(ui, id, r, &s, false); committed {
			return schema.String(s), true
		}
	case schema.KindEnum:
		names := make([]string, len(fd.Enum))
		cur := -1
		for i, ev := range fd.Enum {
			names[i] = ev.Name
			if ev.Value == v.I {
				cur = i
			}
		}
		if widgets.Combo(ui, id, r, &cur, names, false) && cur >= 0 {
			return schema.Enum(fd.Enum[cur].Value), true
		}
	}
	return v, false
}
