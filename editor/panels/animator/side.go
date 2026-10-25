package animator

import (
	"strconv"

	"aego/anim"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/format/aetext"
	"aego/scene/mutate"
	"aego/ui/core"
	"aego/ui/widgets"
)

var (
	kindNames = []string{"Bool", "Int", "Float", "Trigger"}
	opNames   = []string{"set", "==", "!=", ">", ">=", "<", "<="}
)

// SetEntry makes a state the one the Animator starts in.
func (p *Panel) SetEntry(i int) error {
	return p.Edit("Set Entry State", mutate.CtrlRoot("entry", aetext.String(p.g.States[i].Name)))
}

// MoveTransition moves a transition up or down; earlier transitions are tried first.
func (p *Panel) MoveTransition(i, delta int) error {
	err := p.Edit("Move Transition", mutate.CtrlMove(i, delta))
	if err == nil && p.sel.kind == selTransition && p.sel.index == i {
		p.sel.index = i + delta
	}
	return err
}

func (p *Panel) RemoveTransition(i int) error {
	p.sel = selection{}
	return p.Edit("Delete Transition", mutate.CtrlRemove("transition", "").At(i))
}

// AddParam adds a parameter of a kind with a free name.
func (p *Panel) AddParam(kind anim.ParamKind) error {
	name := kind.String()
	for n := 1; p.g.ParamIndex(name) >= 0; n++ {
		name = kind.String() + strconv.Itoa(n+1)
	}
	body := &aetext.Node{Attrs: []aetext.Field{{Name: "kind", Value: aetext.Ident(kind.String())}}}
	return p.Edit("Add Parameter", mutate.CtrlPut("param", name, body))
}

// transitionBody is the text block of transition i, to edit its conditions.
func (p *Panel) transitionBody(i int) *aetext.Node {
	n := 0
	for _, b := range p.doc.Root().Blocks {
		if b.Head != "transition" {
			continue
		}
		if n == i {
			return b.Clone()
		}
		n++
	}
	return nil
}

func (p *Panel) putTransition(i int, label string, body *aetext.Node) error {
	return p.Edit(label, mutate.CtrlPut("transition", "", body).At(i))
}

func condValue(kind anim.ParamKind, v float64) aetext.Value {
	switch kind {
	case anim.ParamBool:
		return aetext.Bool(v != 0)
	case anim.ParamInt:
		return aetext.Int(int64(v))
	}
	return aetext.Float64(v)
}

// SetCond replaces condition c of transition i; c equal to the count adds one.
func (p *Panel) SetCond(i, c int, cond anim.Cond) error {
	body := p.transitionBody(i)
	if body == nil || cond.Param < 0 || int(cond.Param) >= len(p.g.Params) {
		return nil
	}
	pr := p.g.Params[cond.Param]
	n := &aetext.Node{Head: "cond", Label: pr.Name}
	if pr.Kind != anim.ParamTrigger {
		op := cond.Op
		if op == anim.OpSet {
			op = defaultOp(pr.Kind)
		}
		n.Attrs = append(n.Attrs,
			aetext.Field{Name: "op", Value: aetext.Ident(op.String())},
			aetext.Field{Name: "value", Value: condValue(pr.Kind, cond.Value)})
	}
	k := 0
	for j, b := range body.Blocks {
		if b.Head != "cond" {
			continue
		}
		if k == c {
			body.Blocks[j] = n
			return p.putTransition(i, "Edit Condition", body)
		}
		k++
	}
	body.Blocks = append(body.Blocks, n)
	return p.putTransition(i, "Add Condition", body)
}

func (p *Panel) RemoveCond(i, c int) error {
	body := p.transitionBody(i)
	if body == nil {
		return nil
	}
	k := 0
	for j, b := range body.Blocks {
		if b.Head != "cond" {
			continue
		}
		if k == c {
			body.Blocks = append(body.Blocks[:j], body.Blocks[j+1:]...)
			return p.putTransition(i, "Remove Condition", body)
		}
		k++
	}
	return nil
}

func (p *Panel) sidebar(d *document.SceneDocument, r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	ui.Rect(r, th.Panel)
	if ui.MouseOver(ui.ID("side"), r) {
		p.scroll -= ui.ScrollY(th.RowHeight * 3)
	}
	p.scroll = mathx.Clamp(p.scroll, 0, max(0, p.content-r.H))
	top := r.Y + th.Padding - p.scroll
	ui.PushCursor(core.Cursor{X: r.X + th.Padding, Y: top, W: r.W - 2*th.Padding, RowH: th.RowHeight, Pad: th.Padding, Space: 3})
	ui.PushClip(r)
	switch p.sel.kind {
	case selState:
		p.stateSection(p.sel.index)
	case selTransition:
		p.transitionSection(p.sel.index)
	default:
		widgets.LabelDim(ui, "Select a state or a transition")
		widgets.LabelWrapped(ui, "Drag from a state's dot to another state to add a transition", true)
	}
	ui.Cursor().Next(6)
	p.paramsSection()
	p.content = ui.Cursor().Y - top + th.Padding
	ui.PopClip()
	ui.PopCursor()
}

func (p *Panel) header(text string) {
	ui := p.f.UI
	r := ui.Cursor().Next(0)
	ui.Text(text, mathx.Vec2{X: r.X, Y: ui.TextY(r)}, ui.Theme.TextDim)
}

func (p *Panel) paramsSection() {
	ui := p.f.UI
	th := &ui.Theme
	head := ui.Cursor().Next(0)
	ui.Text("Parameters", mathx.Vec2{X: head.X, Y: ui.TextY(head)}, th.TextDim)
	add := mathx.Rect{X: head.X + head.W - head.H, Y: head.Y, W: head.H, H: head.H}
	aid := ui.ID("addparam")
	if widgets.IconButton(ui, aid, add, "ui.plus", "Add a parameter") {
		ui.CloseAllPopups()
		ui.OpenPopup(aid, mathx.Vec2{X: add.X, Y: add.Y + add.H})
	}
	if ui.BeginPopup(aid, 140) {
		for k, n := range kindNames {
			if widgets.MenuItem(ui, n, "", true, false) {
				p.notify(p.AddParam(anim.ParamKind(k)))
			}
		}
		ui.EndPopup()
	}
	var live Live
	if p.d.Live != nil {
		live = p.d.Live()
	}
	for i, pr := range p.g.Params {
		row := ui.Cursor().Next(0)
		lw := row.W * 0.55
		ui.PushClip(mathx.Rect{X: row.X, Y: row.Y, W: lw - 4, H: row.H})
		ui.Text(p.params[i], mathx.Vec2{X: row.X, Y: ui.TextY(row)}, th.Text)
		ui.PopClip()
		vr := mathx.Rect{X: row.X + lw, Y: row.Y, W: row.W - lw, H: row.H}
		id := core.IDSeq(ui.ID("param"), i)
		if live == nil {
			if widgets.IconButton(ui, id, mathx.Rect{X: vr.X + vr.W - vr.H, Y: vr.Y, W: vr.H, H: vr.H}, "ui.minus", "Remove the parameter") {
				p.notify(p.Edit("Remove Parameter", mutate.CtrlRemove("param", pr.Name)))
			}
			continue
		}
		v, _ := live.Param(p.node, pr.Name)
		switch pr.Kind {
		case anim.ParamTrigger:
			if widgets.ButtonRect(ui, id, vr, "Fire") {
				live.SetParam(p.node, pr.Name, 1)
			}
		case anim.ParamBool:
			on := v != 0
			box := mathx.Rect{X: vr.X, Y: vr.Y + 3, W: vr.H - 6, H: vr.H - 6}
			if ui.Clicked(id, box) {
				live.SetParam(p.node, pr.Name, flag(!on))
			}
			if on {
				ui.RoundRect(box, th.Radius.Field, th.Accent)
				widgets.CheckMark(ui, box, th.Background)
			} else {
				ui.Box(box, th.Radius.Field, th.Widget, th.Border)
			}
		default:
			f := float32(v)
			if e := widgets.DragFloat32(ui, id, vr, &f, widgets.NumberOpts{Speed: 0.05}); e.Changed {
				live.SetParam(p.node, pr.Name, float64(f))
			}
		}
	}
	if len(p.g.Params) == 0 {
		widgets.LabelDim(ui, "No parameters")
	}
}

func flag(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func (p *Panel) stateSection(i int) {
	ui := p.f.UI
	s := &p.g.States[i]
	p.header(p.stateHeads[i])
	if p.d.Clips != nil {
		if p.clips == nil {
			p.clips = p.d.Clips()
			p.clipNames = p.clipNames[:0]
			for _, c := range p.clips {
				p.clipNames = append(p.clipNames, c.Name)
			}
		}
		cur := -1
		for k, c := range p.clips {
			if c.ID == s.Clip {
				cur = k
			}
		}
		p.clipID = ui.ID("clip")
		p.clipRect = widgets.Field(ui, "Clip")
		if widgets.Combo(ui, p.clipID, p.clipRect, &cur, p.clipNames, cur < 0) && cur >= 0 {
			p.notify(p.Edit("Set Clip", mutate.CtrlAttr("state", s.Name, "clip", aetext.AssetRef(p.clips[cur].ID.String()))))
		}
	}
	speed := s.Speed
	if e := widgets.DragFloat32(ui, ui.ID("speed"), widgets.Field(ui, "Speed"), &speed, widgets.NumberOpts{Speed: 0.01}); e.End && speed != s.Speed {
		p.notify(p.Edit("Set Speed", mutate.CtrlAttr("state", s.Name, "speed", aetext.Float32(speed))))
	}
	if int32(i) != p.g.Entry && widgets.Button(ui, "Set as Entry") {
		p.notify(p.SetEntry(i))
	}
	if widgets.Button(ui, "Delete State") {
		p.notify(p.RemoveState(i))
	}
}

func (p *Panel) transitionSection(i int) {
	ui := p.f.UI
	th := &ui.Theme
	g := p.g
	t := &g.Transitions[i]
	row := ui.Cursor().Next(0)
	ui.Text(p.heads[i], mathx.Vec2{X: row.X, Y: ui.TextY(row)}, th.TextDim)
	up := mathx.Rect{X: row.X + row.W - 2*row.H - 2, Y: row.Y, W: row.H, H: row.H}
	down := mathx.Rect{X: row.X + row.W - row.H, Y: row.Y, W: row.H, H: row.H}
	if i > 0 && widgets.IconButton(ui, ui.ID("up"), up, "ui.move_up", "Try earlier") {
		p.notify(p.MoveTransition(i, -1))
		return
	}
	if i+1 < len(g.Transitions) && widgets.IconButton(ui, ui.ID("down"), down, "ui.move_down", "Try later") {
		p.notify(p.MoveTransition(i, 1))
		return
	}
	attr := func(name string, v aetext.Value) {
		p.notify(p.Edit("Edit Transition", mutate.CtrlAttr("transition", "", name, v).At(i)))
	}
	dur := t.Duration
	if e := widgets.DragFloat32(ui, ui.ID("dur"), widgets.Field(ui, "Duration"), &dur, widgets.NumberOpts{Speed: 0.01, Min: 0, HasMin: true}); e.End && dur != t.Duration {
		attr("duration", aetext.Float32(dur))
	}
	exit := t.HasExit
	if widgets.Checkbox(ui, "Exit Time", &exit) {
		if exit {
			attr("exitTime", aetext.Float32(1))
		} else {
			p.notify(p.Edit("Edit Transition", mutate.CtrlAttrClear("transition", "", "exitTime").At(i)))
		}
	}
	if t.HasExit {
		et := t.ExitTime
		if e := widgets.DragFloat32(ui, ui.ID("exit"), widgets.Field(ui, "At"), &et, widgets.NumberOpts{Speed: 0.01, Min: 0, HasMin: true}); e.End && et != t.ExitTime {
			attr("exitTime", aetext.Float32(et))
		}
	}
	intr := t.Interrupt
	if widgets.Checkbox(ui, "Interrupts", &intr) {
		attr("interrupt", aetext.Bool(intr))
	}
	p.header("Conditions")
	for c, cond := range t.Conds {
		p.condRow(i, c, cond, p.names)
	}
	if len(g.Params) > 0 && widgets.Button(ui, "Add Condition") {
		p.notify(p.SetCond(i, len(t.Conds), anim.Cond{Param: 0, Op: defaultOp(g.Params[0].Kind)}))
	}
	if widgets.Button(ui, "Delete Transition") {
		p.notify(p.RemoveTransition(i))
	}
}

func defaultOp(k anim.ParamKind) anim.Op {
	switch k {
	case anim.ParamTrigger:
		return anim.OpSet
	case anim.ParamBool:
		return anim.OpEq
	}
	return anim.OpGt
}

func (p *Panel) condRow(i, c int, cond anim.Cond, names []string) {
	ui := p.f.UI
	row := ui.Cursor().Next(0)
	ui.PushIDInt(c)
	defer ui.PopID()
	w := (row.W - row.H - 8) / 3
	pi := int(cond.Param)
	if widgets.Combo(ui, ui.ID("param"), mathx.Rect{X: row.X, Y: row.Y, W: w, H: row.H}, &pi, names, false) {
		p.notify(p.SetCond(i, c, anim.Cond{Param: int32(pi), Op: defaultOp(p.g.Params[pi].Kind), Value: cond.Value}))
		return
	}
	kind := anim.ParamTrigger
	if cond.Param >= 0 && int(cond.Param) < len(p.g.Params) {
		kind = p.g.Params[cond.Param].Kind
	}
	switch kind {
	case anim.ParamFloat, anim.ParamInt:
		op := int(cond.Op)
		if widgets.Combo(ui, ui.ID("op"), mathx.Rect{X: row.X + w + 4, Y: row.Y, W: w * 0.6, H: row.H}, &op, opNames, false) && op > 0 {
			p.notify(p.SetCond(i, c, anim.Cond{Param: cond.Param, Op: anim.Op(op), Value: cond.Value}))
			return
		}
		v := float32(cond.Value)
		if e := widgets.DragFloat32(ui, ui.ID("v"), mathx.Rect{X: row.X + w*1.6 + 8, Y: row.Y, W: w * 1.4, H: row.H}, &v, widgets.NumberOpts{Speed: 0.05}); e.End && float64(v) != cond.Value {
			p.notify(p.SetCond(i, c, anim.Cond{Param: cond.Param, Op: cond.Op, Value: float64(v)}))
			return
		}
	case anim.ParamBool:
		on := cond.Value != 0
		label := "is false"
		if on {
			label = "is true"
		}
		if widgets.ButtonRect(ui, ui.ID("b"), mathx.Rect{X: row.X + w + 4, Y: row.Y, W: w * 2, H: row.H}, label) {
			p.notify(p.SetCond(i, c, anim.Cond{Param: cond.Param, Op: anim.OpEq, Value: flag(!on)}))
			return
		}
	}
	if widgets.IconButton(ui, ui.ID("rm"), mathx.Rect{X: row.X + row.W - row.H, Y: row.Y, W: row.H, H: row.H}, "ui.minus", "Remove the condition") {
		p.notify(p.RemoveCond(i, c))
	}
}
