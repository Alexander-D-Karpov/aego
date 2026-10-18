package inspector

import (
	"strconv"

	"aego/core/curve"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/editor/core/history"
	"aego/editor/core/props"
	"aego/platform/input"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

var all = widgets.Edit{Changed: true, Begin: true, End: true}

type fieldRow struct {
	a, b int
}

func visibleField(fd *schema.FieldDescriptor, play bool) bool {
	return fd.Flags.Has(schema.FlagEditor) && !fd.Hints.Hidden && (!fd.Hints.Runtime || play)
}

func pairGroup(fd *schema.FieldDescriptor) int {
	if len(fd.Hints.Flags) > 0 {
		return 0
	}
	switch fd.Kind {
	case schema.KindBool:
		return 1
	case schema.KindInt32, schema.KindInt64, schema.KindFloat32, schema.KindFloat64:
		return 2
	}
	return 0
}

func plan(td *schema.TypeDescriptor, play bool) []fieldRow {
	var rows []fieldRow
	pending := -1
	for i := range td.Fields {
		fd := &td.Fields[i]
		if !visibleField(fd, play) {
			continue
		}
		if pending >= 0 {
			prev := &td.Fields[pending]
			if g := pairGroup(fd); g != 0 && g == pairGroup(prev) && prev.Hints.Category == fd.Hints.Category {
				rows = append(rows, fieldRow{a: pending, b: i})
				pending = -1
				continue
			}
			rows = append(rows, fieldRow{a: pending, b: -1})
			pending = -1
		}
		if pairGroup(fd) != 0 {
			pending = i
			continue
		}
		rows = append(rows, fieldRow{a: i, b: -1})
	}
	if pending >= 0 {
		rows = append(rows, fieldRow{a: pending, b: -1})
	}
	return rows
}

func pairRects(ui *core.Context, r mathx.Rect, secondLabel string) (*[2]mathx.Rect, *[2]mathx.Rect) {
	lw := widgets.LabelWidth(r.W)
	vx, vw := r.X+lw, r.W-lw
	const gap = float32(8)
	half := (vw - gap) / 2
	first := &[2]mathx.Rect{
		{X: r.X, Y: r.Y, W: lw - 4, H: r.H},
		{X: vx, Y: r.Y, W: half, H: r.H},
	}
	x2 := vx + half + gap
	l2 := min(ui.TextSize(secondLabel).X+10, half*0.5)
	second := &[2]mathx.Rect{
		{X: x2, Y: r.Y, W: l2 - 4, H: r.H},
		{X: x2 + l2, Y: r.Y, W: half - l2, H: r.H},
	}
	return first, second
}

func (p *Panel) fields(d *document.SceneDocument, td *schema.TypeDescriptor) {
	ui := p.f.UI
	wide := ui.Cursor().W >= 300
	category := ""
	for _, row := range plan(td, p.d.Docs.Documents().InPlay()) {
		fd := &td.Fields[row.a]
		if c := fd.Hints.Category; c != "" && c != category {
			category = c
			widgets.LabelDim(ui, c)
		}
		if row.b < 0 || !wide {
			p.field(d, td, fd)
			if row.b >= 0 {
				p.field(d, td, &td.Fields[row.b])
			}
			continue
		}
		second := &td.Fields[row.b]
		r := ui.Cursor().Next(0)
		a, b := pairRects(ui, r, p.labelOf(second))
		p.fieldIn(d, td, fd, a)
		p.fieldIn(d, td, second, b)
	}
}

func (p *Panel) field(d *document.SceneDocument, td *schema.TypeDescriptor, fd *schema.FieldDescriptor) {
	p.fieldIn(d, td, fd, nil)
}

func (p *Panel) fieldIn(d *document.SceneDocument, td *schema.TypeDescriptor, fd *schema.FieldDescriptor, at *[2]mathx.Rect) {
	ui := p.f.UI
	ui.PushRawID(core.ID(fd.ID))
	defer ui.PopID()
	key := ui.ID("value")
	v, mixed, err := d.Props.Common(p.nodes, td.ID, fd.ID)
	if err != nil {
		return
	}
	if cached, ok := p.live[key]; ok {
		if p.editing(ui, key) {
			v, mixed = cached, false
		} else {
			delete(p.live, key)
		}
	}
	cur := ui.Cursor()
	rowTop := cur.Y
	pre := mathx.Rect{X: cur.X, Y: cur.Y, W: cur.W, H: ui.Theme.RowHeight}
	if at != nil {
		pre = mathx.Rect{X: at[0].X, Y: at[0].Y, W: at[1].X + at[1].W - at[0].X, H: at[0].H}
	}
	jump := td.Kind == schema.TypeScript && ui.Primary() && ui.MousePressed(input.MouseLeft) && ui.Hit(pre) && !ui.MouseInOverlay()
	ro := fd.Flags.Has(schema.FlagReadOnly)
	var e widgets.Edit
	var row mathx.Rect
	if at != nil {
		e = p.scalarIn(d, fd, &v, mixed, p.labelOf(fd), fd.Hints.Tooltip, ro, at[0], at[1])
		row = pre
	} else {
		e = p.value(d, fd, &v, mixed, p.labelOf(fd), fd.Hints.Tooltip, ro)
		row = mathx.Rect{X: cur.X, Y: rowTop, W: cur.W, H: max(0, cur.Y-rowTop)}
	}
	if p.overridden(td, fd) {
		ui.Rect(mathx.Rect{X: row.X - 5, Y: row.Y + 1, W: 2, H: max(row.H-2, 2)}, ui.Theme.Accent)
	}
	if jump {
		ui.ReleaseMouse()
		ui.CancelFocus()
		ui.CloseAllPopups()
		delete(p.live, key)
		p.gotoScript(td.FullName, fd.Name)
		return
	}
	if ui.Hit(row) && !ui.MouseInOverlay() {
		p.hoverField = fieldRef{t: td.ID, f: fd.ID, ok: true}
		if ui.MousePressed(input.MouseRight) {
			p.menuField = p.hoverField
			p.rightUsed = true
			widgets.OpenContextMenu(ui, p.fieldMenuID)
		}
	}
	if e.Begin || e.Changed {
		p.live[key] = v
	}
	if e.End {
		delete(p.live, key)
	}
	if !e.Any() {
		return
	}
	t, f, val := td.ID, fd.ID, v
	p.apply(e, "Set "+props.Label(fd), func(tx *history.Transaction) error {
		return d.Props.Set(tx, p.nodes, t, f, val)
	})
}

func (p *Panel) scalarIn(d *document.SceneDocument, fd *schema.FieldDescriptor, v *schema.Value, mixed bool, label, tip string, ro bool, lr, r mathx.Rect) widgets.Edit {
	ui := p.f.UI
	th := &ui.Theme
	ui.PushClip(lr)
	ui.Text(label, mathx.Vec2{X: lr.X, Y: ui.TextY(lr)}, th.TextDim)
	ui.PopClip()
	lid := ui.ID("label")
	ui.MouseOver(lid, lr)
	widgets.Tooltip(ui, lid, tip)
	if ro {
		ui.PushClip(r)
		ui.TextIn(r, p.summary(d, fd, *v, mixed), th.TextDim, core.AlignLeft)
		ui.PopClip()
		return widgets.Edit{}
	}
	return p.scalar(d, ui.ID("value"), r, fd, v, mixed)
}

func (p *Panel) value(d *document.SceneDocument, fd *schema.FieldDescriptor, v *schema.Value, mixed bool, label, tip string, ro bool) widgets.Edit {
	switch fd.Kind {
	case schema.KindList:
		return p.listValue(d, fd, v, mixed, label, ro)
	case schema.KindStruct:
		return p.structValue(d, fd, v, mixed, label, ro)
	}
	ui := p.f.UI
	lr, r := widgets.FieldRects(ui, label)
	lid := ui.ID("label")
	ui.MouseOver(lid, lr)
	widgets.Tooltip(ui, lid, tip)
	if ro {
		ui.PushClip(r)
		ui.TextIn(r, p.summary(d, fd, *v, mixed), ui.Theme.TextDim, core.AlignLeft)
		ui.PopClip()
		return widgets.Edit{}
	}
	return p.scalar(d, ui.ID("value"), r, fd, v, mixed)
}

func numOpts(fd *schema.FieldDescriptor, mixed, isInt bool) widgets.NumberOpts {
	o := widgets.NumberOpts{
		Mixed:  mixed,
		Int:    isInt,
		Min:    fd.Min,
		Max:    fd.Max,
		HasMin: fd.HasMin,
		HasMax: fd.HasMax,
		Speed:  fd.Hints.Step,
	}
	o.Unit = fd.Hints.Unit
	if o.Speed == 0 {
		switch {
		case fd.HasMin && fd.HasMax && fd.Max > fd.Min:
			o.Speed = (fd.Max - fd.Min) / 200
		case isInt:
			o.Speed = 0.1
		default:
			o.Speed = 0.05
		}
	}
	return o
}

func intValue(k schema.Kind, n int64) schema.Value {
	if k == schema.KindInt32 {
		return schema.Int32(int32(n))
	}
	return schema.Int64(n)
}

func floatValue(k schema.Kind, f float64) schema.Value {
	if k == schema.KindFloat32 {
		return schema.Float32(float32(f))
	}
	return schema.Float64(f)
}

func (p *Panel) scalar(d *document.SceneDocument, id core.ID, r mathx.Rect, fd *schema.FieldDescriptor, v *schema.Value, mixed bool) widgets.Edit {
	ui := p.f.UI
	th := &ui.Theme
	switch fd.Kind {
	case schema.KindBool:
		b := v.I != 0
		state := 0
		if b {
			state = 1
		}
		if mixed {
			state = 2
		}
		if triCheck(ui, id, mathx.Rect{X: r.X, Y: r.Y, W: r.H, H: r.H}, state) {
			*v = schema.Bool(!b || mixed)
			return all
		}
	case schema.KindInt32, schema.KindInt64:
		if fd.Hints.Choices.Section != "" {
			return p.choices(id, r, fd, v, mixed)
		}
		if fd.Hints.Bitset.Section != "" {
			return p.bitset(id, r, fd, v, mixed)
		}
		if len(fd.Hints.Flags) > 0 {
			m := uint64(v.AsInt())
			if widgets.MaskField(ui, id, r, &m, fd.Hints.Flags, mixed) {
				*v = intValue(fd.Kind, int64(m))
				return all
			}
			return widgets.Edit{}
		}
		f := float64(v.AsInt())
		e := widgets.DragNumber(ui, id, r, &f, numOpts(fd, mixed, true))
		if e.Changed {
			*v = intValue(fd.Kind, int64(f))
		}
		return e
	case schema.KindFloat32, schema.KindFloat64:
		f := v.AsFloat()
		e := widgets.DragNumber(ui, id, r, &f, numOpts(fd, mixed, false))
		if e.Changed {
			*v = floatValue(fd.Kind, f)
		}
		return e
	case schema.KindVec2:
		vec := v.V
		e := widgets.Vec2Field(ui, id, r, &vec, numOpts(fd, false, false), mixed, mixed)
		if e.Changed {
			*v = schema.Vec2(vec)
		}
		return e
	case schema.KindVec3:
		vec := v.V3
		e := widgets.Vec3Field(ui, id, r, &vec, numOpts(fd, false, false), [3]bool{mixed, mixed, mixed})
		if e.Changed {
			*v = schema.Vec3(vec)
		}
		return e
	case schema.KindQuat:
		eul := props.EulerDeg(v.Q)
		o := numOpts(fd, false, false)
		o.Speed, o.Prec, o.Suffix = 0.5, 2, "°"
		e := widgets.Vec3Field(ui, id, r, &eul, o, [3]bool{mixed, mixed, mixed})
		if e.Changed {
			*v = schema.Quat(props.QuatFromEulerDeg(eul))
		}
		return e
	case schema.KindVec4:
		vec := v.V4
		parts := [4]*float32{&vec.X, &vec.Y, &vec.Z, &vec.W}
		w := (r.W - 6) / 4
		var e widgets.Edit
		for i, ptr := range parts {
			cell := mathx.Rect{X: r.X + float32(i)*(w+2), Y: r.Y, W: w, H: r.H}
			e = e.Or(widgets.DragFloat32(ui, core.IDSeq(id, i+1), cell, ptr, numOpts(fd, mixed, false)))
		}
		if e.Changed {
			*v = schema.Vec4(vec)
		}
		return e
	case schema.KindColor:
		col := v.C
		e := widgets.ColorField(ui, id, r, &col, mixed)
		if e.Changed {
			*v = schema.Color(col)
		}
		return e
	case schema.KindRect:
		rect := v.R
		parts := [4]*float32{&rect.X, &rect.Y, &rect.W, &rect.H}
		w := (r.W - 6) / 4
		var e widgets.Edit
		for i, ptr := range parts {
			cell := mathx.Rect{X: r.X + float32(i)*(w+2), Y: r.Y, W: w, H: r.H}
			e = e.Or(widgets.DragFloat32(ui, core.IDSeq(id, i+1), cell, ptr, numOpts(fd, mixed, false)))
		}
		if e.Changed {
			*v = schema.Rect(rect)
		}
		return e
	case schema.KindString:
		s := v.S
		if mixed {
			s = th.Mixed
		}
		if _, committed := widgets.TextFieldRect(ui, id, r, &s, false); committed && !(mixed && s == th.Mixed) {
			*v = schema.String(s)
			return all
		}
	case schema.KindEnum:
		if len(fd.Enum) == 0 {
			f := float64(v.AsInt())
			e := widgets.DragNumber(ui, id, r, &f, numOpts(fd, mixed, true))
			if e.Changed {
				*v = schema.Enum(int64(f))
			}
			return e
		}
		names := make([]string, len(fd.Enum))
		cur := -1
		for i, ev := range fd.Enum {
			names[i] = ev.Name
			if ev.Value == v.I {
				cur = i
			}
		}
		if widgets.Combo(ui, id, r, &cur, names, mixed) && cur >= 0 {
			*v = schema.Enum(fd.Enum[cur].Value)
			return all
		}
	case schema.KindCurve:
		st := p.curves[id]
		if st == nil {
			st = widgets.NewCurveState()
			p.curves[id] = st
		}
		cv := v.Cv
		e := widgets.CurveField(ui, id, r, &cv, mixed, st)
		if e.Changed {
			*v = schema.Curve(cv)
		}
		return e
	case schema.KindAssetRef:
		return p.assetField(id, r, fd, v, mixed)
	case schema.KindNodeRef:
		return p.nodeField(d, id, r, v, mixed, fd.Component)
	}
	return widgets.Edit{}
}

func zeroValue(reg *schema.Registry, fd *schema.FieldDescriptor) schema.Value {
	switch fd.Kind {
	case schema.KindBool:
		return schema.Bool(false)
	case schema.KindInt32:
		return schema.Int32(0)
	case schema.KindInt64:
		return schema.Int64(0)
	case schema.KindFloat32:
		return schema.Float32(0)
	case schema.KindFloat64:
		return schema.Float64(0)
	case schema.KindVec2:
		return schema.Vec2(mathx.Vec2{})
	case schema.KindVec3:
		return schema.Vec3(mathx.Vec3{})
	case schema.KindVec4:
		return schema.Vec4(mathx.Vec4{})
	case schema.KindQuat:
		return schema.Quat(mathx.QuatIdentity())
	case schema.KindColor:
		return schema.Color(mathx.Color{R: 1, G: 1, B: 1, A: 1})
	case schema.KindRect:
		return schema.Rect(mathx.Rect{})
	case schema.KindCurve:
		return schema.Curve(curve.Line(0, 0, 1, 1))
	case schema.KindString:
		return schema.String("")
	case schema.KindEnum:
		if len(fd.Enum) > 0 {
			return schema.Enum(fd.Enum[0].Value)
		}
		return schema.Enum(0)
	case schema.KindAssetRef:
		return schema.Asset(ids.AssetID{})
	case schema.KindNodeRef:
		return schema.Node(ids.NodeID{})
	case schema.KindList:
		return schema.Value{Kind: schema.KindList}
	case schema.KindStruct:
		td, ok := reg.ByID(fd.Struct)
		if !ok {
			return schema.Value{Kind: schema.KindStruct}
		}
		obj := &schema.Object{Type: td.ID, Fields: make([]schema.Value, len(td.Fields))}
		for i := range td.Fields {
			obj.Fields[i] = zeroValue(reg, &td.Fields[i])
		}
		return schema.Value{Kind: schema.KindStruct, Obj: obj}
	}
	return schema.Value{}
}

func (p *Panel) summary(d *document.SceneDocument, fd *schema.FieldDescriptor, v schema.Value, mixed bool) string {
	if mixed {
		return p.f.UI.Theme.Mixed
	}
	num := func(f float32) string {
		return widgets.FormatNumber(float64(f), widgets.NumberOpts{})
	}
	switch v.Kind {
	case schema.KindBool:
		return strconv.FormatBool(v.I != 0)
	case schema.KindInt32, schema.KindInt64:
		return strconv.FormatInt(v.I, 10)
	case schema.KindFloat32, schema.KindFloat64:
		return widgets.FormatNumber(v.F, widgets.NumberOpts{})
	case schema.KindString:
		return v.S
	case schema.KindVec2:
		return num(v.V.X) + ", " + num(v.V.Y)
	case schema.KindVec3:
		return num(v.V3.X) + ", " + num(v.V3.Y) + ", " + num(v.V3.Z)
	case schema.KindVec4:
		return num(v.V4.X) + ", " + num(v.V4.Y) + ", " + num(v.V4.Z) + ", " + num(v.V4.W)
	case schema.KindQuat:
		e := props.EulerDeg(v.Q)
		return num(e.X) + "°, " + num(e.Y) + "°, " + num(e.Z) + "°"
	case schema.KindColor:
		return widgets.HexColor(v.C)
	case schema.KindCurve:
		if v.Cv.Ease != curve.None {
			return v.Cv.Ease.String()
		}
		return strconv.Itoa(len(v.Cv.Keys)) + " keys"
	case schema.KindEnum:
		if name, ok := fd.EnumNameOf(v.I); ok {
			return name
		}
		return strconv.FormatInt(v.I, 10)
	case schema.KindAssetRef:
		return assetState(p.d.Pipeline, v.Asset).label
	case schema.KindNodeRef:
		if d == nil {
			return "—"
		}
		return nodeState(d.Scene(), v.Node, fd.Component).label
	case schema.KindList:
		return "[" + strconv.Itoa(len(v.List)) + "]"
	}
	return "…"
}

func elemOf(fd *schema.FieldDescriptor) schema.FieldDescriptor {
	return schema.FieldDescriptor{
		Name:      fd.Name,
		Kind:      fd.Elem,
		Struct:    fd.Struct,
		Enum:      fd.Enum,
		EnumName:  fd.EnumName,
		AssetKind: fd.AssetKind,
		Component: fd.Component,
		Flags:     fd.Flags,
		Min:       fd.Min,
		Max:       fd.Max,
		HasMin:    fd.HasMin,
		HasMax:    fd.HasMax,
		Hints:     fd.Hints,
	}
}

func (p *Panel) foldHeader(ui *core.Context, id core.ID, r mathx.Rect, text string) bool {
	th := &ui.Theme
	if ui.Pressed(id, r) {
		p.collapsed[id] = !p.collapsed[id]
	}
	open := !p.collapsed[id]
	widgets.Chevron(ui, mathx.Rect{X: r.X, Y: r.Y, W: th.Indent, H: r.H}, open, th.TextDim)
	ui.Text(text, mathx.Vec2{X: r.X + th.Indent, Y: ui.TextY(r)}, th.TextDim)
	return open
}

func (p *Panel) listValue(d *document.SceneDocument, fd *schema.FieldDescriptor, v *schema.Value, mixed bool, label string, ro bool) widgets.Edit {
	ui := p.f.UI
	th := &ui.Theme
	row := ui.Cursor().Next(0)
	id := ui.ID("list")
	n := len(v.List)
	bw := row.H
	hdr := row
	editable := !ro && !mixed
	if editable {
		hdr.W -= 2*bw + 4
	}
	text := label + " [" + strconv.Itoa(n) + "]"
	if mixed {
		text = label + " " + th.Mixed
	}
	open := p.foldHeader(ui, id, hdr, text)
	var e widgets.Edit
	if editable {
		elem := elemOf(fd)
		if widgets.ButtonRect(ui, core.IDSeq(id, 1), mathx.Rect{X: row.X + row.W - 2*bw - 2, Y: row.Y, W: bw, H: row.H}, "+") {
			v.List = append(v.List, zeroValue(p.schemaOf(d), &elem))
			e = all
		}
		if widgets.ButtonRect(ui, core.IDSeq(id, 2), mathx.Rect{X: row.X + row.W - bw, Y: row.Y, W: bw, H: row.H}, "-") && n > 0 {
			v.List = v.List[:n-1]
			e = all
		}
	}
	if !open || mixed {
		return e
	}
	cur := ui.Cursor()
	cur.X += th.Indent
	cur.W -= th.Indent
	elem := elemOf(fd)
	for i := range v.List {
		ui.PushIDInt(i)
		e = e.Or(p.value(d, &elem, &v.List[i], false, strconv.Itoa(i), "", ro))
		ui.PopID()
	}
	cur.X -= th.Indent
	cur.W += th.Indent
	return e
}

func (p *Panel) structValue(d *document.SceneDocument, fd *schema.FieldDescriptor, v *schema.Value, mixed bool, label string, ro bool) widgets.Edit {
	ui := p.f.UI
	th := &ui.Theme
	reg := p.schemaOf(d)
	if reg == nil {
		return widgets.Edit{}
	}
	td, ok := reg.ByID(fd.Struct)
	if !ok {
		return widgets.Edit{}
	}
	row := ui.Cursor().Next(0)
	text := label
	if mixed {
		text += " " + th.Mixed
	}
	if !p.foldHeader(ui, ui.ID("struct"), row, text) || mixed {
		return widgets.Edit{}
	}
	if v.Obj == nil || len(v.Obj.Fields) != len(td.Fields) {
		*v = zeroValue(reg, fd)
	}
	cur := ui.Cursor()
	cur.X += th.Indent
	cur.W -= th.Indent
	var e widgets.Edit
	for i := range td.Fields {
		sub := &td.Fields[i]
		if !sub.Flags.Has(schema.FlagEditor) || sub.Hints.Hidden {
			continue
		}
		ui.PushIDInt(i)
		e = e.Or(p.value(d, sub, &v.Obj.Fields[i], false, p.labelOf(sub), sub.Hints.Tooltip, ro || sub.Flags.Has(schema.FlagReadOnly)))
		ui.PopID()
	}
	cur.X -= th.Indent
	cur.W += th.Indent
	return e
}
