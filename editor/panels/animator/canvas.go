package animator

import (
	"math"

	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

type dragKind uint8

const (
	dragNone dragKind = iota
	dragNode
	dragLink
	dragPan
)

type dragState struct {
	kind  dragKind
	node  int
	start mathx.Vec2
	off   mathx.Vec2
	moved bool
}

func (p *Panel) toScreen(r mathx.Rect, g [2]float32) mathx.Vec2 {
	return mathx.Vec2{X: r.X + (g[0]-p.cam.X)*p.zoom, Y: r.Y + (g[1]-p.cam.Y)*p.zoom}
}

func (p *Panel) toGraph(r mathx.Rect, s mathx.Vec2) [2]float32 {
	return [2]float32{(s.X-r.X)/p.zoom + p.cam.X, (s.Y-r.Y)/p.zoom + p.cam.Y}
}

// box is the screen rect of a state, of Entry (-1) or of Any (-2), with the drag offset applied.
func (p *Panel) box(r mathx.Rect, i int) mathx.Rect {
	var at [2]float32
	w, h := nodeW, nodeH
	switch i {
	case -1:
		at, w, h = p.g.EntryPos, pillW, pillH
	case -2:
		at, w, h = p.g.AnyPos, pillW, pillH
	default:
		at = p.g.States[i].Pos
	}
	s := p.toScreen(r, at)
	if p.drag.kind == dragNode && p.drag.node == i {
		s = s.Add(p.drag.off)
	}
	return mathx.Rect{X: s.X, Y: s.Y, W: w * p.zoom, H: h * p.zoom}
}

func center(b mathx.Rect) mathx.Vec2 { return mathx.Vec2{X: b.X + b.W/2, Y: b.Y + b.H/2} }

// rim is where a ray from the centre of b towards to leaves b.
func rim(b mathx.Rect, to mathx.Vec2) mathx.Vec2 {
	c := center(b)
	d := to.Sub(c)
	if d.X == 0 && d.Y == 0 {
		return c
	}
	t := float32(math.Inf(1))
	if d.X != 0 {
		t = min(t, mathx.Abs(b.W/2/d.X))
	}
	if d.Y != 0 {
		t = min(t, mathx.Abs(b.H/2/d.Y))
	}
	return c.Add(d.Scale(t))
}

func (p *Panel) frameAll() { p.zoom = 0 }

func (p *Panel) fit(r mathx.Rect) {
	lo := mathx.Vec2{X: p.g.EntryPos[0], Y: p.g.EntryPos[1]}
	hi := lo.Add(mathx.Vec2{X: pillW, Y: pillH})
	grow := func(at [2]float32, w, h float32) {
		lo = mathx.Vec2{X: min(lo.X, at[0]), Y: min(lo.Y, at[1])}
		hi = mathx.Vec2{X: max(hi.X, at[0]+w), Y: max(hi.Y, at[1]+h)}
	}
	grow(p.g.AnyPos, pillW, pillH)
	for _, s := range p.g.States {
		grow(s.Pos, nodeW, nodeH)
	}
	size := hi.Sub(lo)
	p.zoom = mathx.Clamp(min((r.W-60)/max(size.X, 1), (r.H-60)/max(size.Y, 1)), 0.5, 1.2)
	p.cam = mathx.Vec2{X: lo.X - (r.W/p.zoom-size.X)/2, Y: lo.Y - (r.H/p.zoom-size.Y)/2}
}

func (p *Panel) canvas(d *document.SceneDocument, r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	p.area = r
	if p.zoom == 0 {
		p.fit(r)
	}
	ui.Rect(r, th.Background)
	ui.PushClip(r)
	p.grid(r)
	live, state, progress := p.liveState()
	p.drawEdges(r)
	if p.drag.kind == dragLink {
		a := center(p.box(r, p.drag.node))
		ui.Line(a, ui.Mouse(), 2, th.Accent)
	}
	p.pill(r, -1, "Entry", p.f.UI.Theme.StateEntry)
	p.pill(r, -2, "Any State", p.f.UI.Theme.StateAny)
	for i := range p.g.States {
		p.stateNode(r, i, live && i == state, progress)
	}
	p.edgeLabels(r)
	ui.PopClip()
	p.input(d, r)
	p.menus(r)
}

func (p *Panel) grid(r mathx.Rect) {
	ui := p.f.UI
	col := ui.Theme.Border
	col.A *= 0.35
	step := 40 * p.zoom
	if step < 12 {
		return
	}
	ox := float32(math.Mod(float64(-p.cam.X*p.zoom), float64(step)))
	oy := float32(math.Mod(float64(-p.cam.Y*p.zoom), float64(step)))
	for x := r.X + ox; x < r.X+r.W; x += step {
		ui.Rect(mathx.Rect{X: x, Y: r.Y, W: 1, H: r.H}, col)
	}
	for y := r.Y + oy; y < r.Y+r.H; y += step {
		ui.Rect(mathx.Rect{X: r.X, Y: y, W: r.W, H: 1}, col)
	}
}

func (p *Panel) liveState() (bool, int, float32) {
	if p.d.Live == nil {
		return false, -1, 0
	}
	l := p.d.Live()
	if l == nil {
		return false, -1, 0
	}
	name, progress, ok := l.State(p.node)
	if !ok {
		return false, -1, 0
	}
	return true, int(p.g.StateIndex(name)), progress
}

func (p *Panel) pill(r mathx.Rect, i int, label string, col mathx.Color) {
	ui := p.f.UI
	th := &ui.Theme
	b := p.box(r, i)
	sel := (i == -1 && p.sel.kind == selEntry) || (i == -2 && p.sel.kind == selAny)
	border := col
	if sel {
		border = th.Accent
	}
	ui.Box(b, b.H/2, col.Lerp(th.Background, 0.65), border)
	ui.TextIn(b, label, th.Text, core.AlignCenter)
}

func (p *Panel) stateNode(r mathx.Rect, i int, active bool, progress float32) {
	ui := p.f.UI
	th := &ui.Theme
	b := p.box(r, i)
	s := &p.g.States[i]
	bg, border := th.CardBg, th.Border
	if int32(i) == p.g.Entry {
		border = p.f.UI.Theme.StateEntry
	}
	if p.sel.kind == selState && p.sel.index == i {
		border = th.Accent
	}
	if active {
		bg = th.Selection
	}
	ui.Box(b, 6, bg, border)
	if active {
		ui.List().RectOutline(mathx.Rect{X: b.X - 2, Y: b.Y - 2, W: b.W + 4, H: b.H + 4}, th.Accent, 2)
		ring(ui, mathx.Vec2{X: b.X + 12*p.zoom, Y: b.Y + b.H/2}, 6*p.zoom, progress, th.Accent)
	}
	ui.PushClip(b)
	ui.TextIn(b, s.Name, th.Text, core.AlignCenter)
	ui.PopClip()
	h := p.handle(b)
	ui.RoundRect(h, h.W/2, th.TextDim)
}

// ring draws the share f of a circle, clockwise from the top.
func ring(ui *core.Context, c mathx.Vec2, rad, f float32, col mathx.Color) {
	const n = 24
	faint := col
	faint.A *= 0.3
	prev := c.Add(mathx.Vec2{Y: -rad})
	for k := 1; k <= n; k++ {
		a := float64(k) / n * 2 * math.Pi
		pt := c.Add(mathx.Vec2{X: rad * float32(math.Sin(a)), Y: -rad * float32(math.Cos(a))})
		cc := faint
		if float32(k)/n <= f {
			cc = col
		}
		ui.Line(prev, pt, 2, cc)
		prev = pt
	}
}

func (p *Panel) handle(b mathx.Rect) mathx.Rect {
	s := max(8*p.zoom, 6)
	return mathx.Rect{X: b.X + b.W - s/2, Y: b.Y + b.H/2 - s/2, W: s, H: s}
}

func (p *Panel) drawEdges(r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	g := p.g
	p.edges = p.edges[:0]
	if g.Entry >= 0 && int(g.Entry) < len(g.States) {
		a, b := p.box(r, -1), p.box(r, int(g.Entry))
		arrow(ui, rim(a, center(b)), rim(b, center(a)), p.f.UI.Theme.StateEntry)
	}
	for k := range g.Transitions {
		t := &g.Transitions[k]
		from := int(t.From)
		if from < 0 {
			from = -2
		}
		if t.To < 0 || int(t.To) >= len(g.States) || from >= len(g.States) {
			p.edges = append(p.edges, edge{})
			continue
		}
		fa, fb := p.box(r, from), p.box(r, int(t.To))
		ca, cb := center(fa), center(fb)
		var bend float32
		if p.reverse(k) {
			bend = 34 * p.zoom
		}
		d := cb.Sub(ca)
		var n mathx.Vec2
		if l := d.Len(); l > 0 {
			n = mathx.Vec2{X: d.Y / l, Y: -d.X / l}
		}
		mid := ca.Add(cb).Scale(0.5).Add(n.Scale(bend))
		a, b := rim(fa, mid), rim(fb, mid)
		ctl := a.Add(b).Scale(0.5).Add(n.Scale(bend * 1.4))
		col := th.TextDim
		if t.From < 0 {
			col = p.f.UI.Theme.StateAny
		}
		if p.sel.kind == selTransition && p.sel.index == k {
			col = th.Accent
		}
		curveArrow(ui, a, ctl, b, col)
		p.edges = append(p.edges, edge{a: a, c: ctl, b: b, label: p.conds[k]})
	}
}

// edgeLabels draws the conditions on chips; a chip that would cover a node or an earlier chip is left out, except the selected one.
func (p *Panel) edgeLabels(r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	if p.zoom < 0.6 {
		return
	}
	p.chips = p.chips[:0]
	for i := range p.g.States {
		p.chips = append(p.chips, p.box(r, i))
	}
	p.chips = append(p.chips, p.box(r, -1), p.box(r, -2))
	order := func(k int) {
		e := &p.edges[k]
		if e.label == "" || e.a == e.b {
			return
		}
		sel := p.sel.kind == selTransition && p.sel.index == k
		top := bezier(e.a, e.c, e.b, 0.5)
		sz := ui.TextSize(e.label)
		chip := mathx.Rect{X: top.X - sz.X/2 - 4, Y: top.Y - sz.Y/2 - 1, W: sz.X + 8, H: sz.Y + 2}
		if !sel {
			for _, o := range p.chips {
				if overlaps(o, chip) {
					return
				}
			}
		}
		p.chips = append(p.chips, chip)
		col := th.TextDim
		if sel {
			col = th.Text
		}
		ui.Box(chip, 3, th.Background, th.Border)
		ui.Text(e.label, mathx.Vec2{X: chip.X + 4, Y: chip.Y + 1}, col)
	}
	sel := -1
	if p.sel.kind == selTransition && p.sel.index < len(p.edges) {
		sel = p.sel.index
		order(sel)
	}
	for k := range p.edges {
		if k != sel {
			order(k)
		}
	}
}

func bezier(a, c, b mathx.Vec2, t float32) mathx.Vec2 {
	u := 1 - t
	return a.Scale(u * u).Add(c.Scale(2 * u * t)).Add(b.Scale(t * t))
}

// curveArrow draws a quadratic curve from a to b bending towards c, with a head at b.
func curveArrow(ui *core.Context, a, c, b mathx.Vec2, col mathx.Color) {
	const n = 12
	prev := a
	for k := 1; k <= n; k++ {
		pt := bezier(a, c, b, float32(k)/n)
		ui.Line(prev, pt, 2, col)
		prev = pt
	}
	head(ui, bezier(a, c, b, 0.9), b, col)
}

func (p *Panel) reverse(k int) bool {
	t := &p.g.Transitions[k]
	for j := range p.g.Transitions {
		if o := &p.g.Transitions[j]; j != k && o.From == t.To && o.To == t.From {
			return true
		}
	}
	return false
}

func arrow(ui *core.Context, a, b mathx.Vec2, col mathx.Color) {
	ui.Line(a, b, 2, col)
	head(ui, a, b, col)
}

func head(ui *core.Context, a, b mathx.Vec2, col mathx.Color) {
	d := b.Sub(a)
	l := d.Len()
	if l < 1 {
		return
	}
	u := d.Scale(1 / l)
	n := mathx.Vec2{X: -u.Y, Y: u.X}
	back := b.Sub(u.Scale(10))
	ui.Line(b, back.Add(n.Scale(5)), 2, col)
	ui.Line(b, back.Sub(n.Scale(5)), 2, col)
}

func segDist(pt, a, b mathx.Vec2) float32 {
	d := b.Sub(a)
	l := d.Dot(d)
	if l == 0 {
		return pt.Sub(a).Len()
	}
	t := mathx.Clamp(pt.Sub(a).Dot(d)/l, 0, 1)
	return pt.Sub(a.Add(d.Scale(t))).Len()
}

// hit finds what is under a screen point: a node (state index, -1 Entry, -2 Any), its link handle, or a transition.
func (p *Panel) hit(r mathx.Rect, m mathx.Vec2) (sel selection, node int, handle bool) {
	for i := len(p.g.States) - 1; i >= 0; i-- {
		b := p.box(r, i)
		if p.handle(b).Contains(m) {
			return selection{kind: selState, index: i}, i, true
		}
		if b.Contains(m) {
			return selection{kind: selState, index: i}, i, false
		}
	}
	if p.box(r, -1).Contains(m) {
		return selection{kind: selEntry}, -1, false
	}
	if b := p.box(r, -2); b.Contains(m) {
		return selection{kind: selAny}, -2, b.X+b.W-12 < m.X
	}
	for k, e := range p.edges {
		if e.a == e.b {
			continue
		}
		prev := e.a
		for s := 1; s <= 8; s++ {
			pt := bezier(e.a, e.c, e.b, float32(s)/8)
			if segDist(m, prev, pt) < 5 {
				return selection{kind: selTransition, index: k}, 0, false
			}
			prev = pt
		}
	}
	return selection{}, 0, false
}

func (p *Panel) input(d *document.SceneDocument, r mathx.Rect) {
	ui := p.f.UI
	id := ui.ID("canvas")
	p.menuID = ui.ID("graphmenu")
	over := ui.MouseOver(id, r)
	m := ui.Mouse()
	if over {
		if z := ui.ZoomY(); z != 0 {
			at := p.toGraph(r, m)
			p.zoom = mathx.Clamp(p.zoom*float32(math.Pow(1.15, float64(z))), 0.3, 2.5)
			p.cam = mathx.Vec2{X: at[0] - (m.X-r.X)/p.zoom, Y: at[1] - (m.Y-r.Y)/p.zoom}
		}
		if ui.MousePressed(input.MouseMiddle) {
			p.drag = dragState{kind: dragPan, start: m}
			ui.CaptureMouse(id)
		}
	}
	if ui.Pressed(id, r) {
		sel, node, handle := p.hit(r, m)
		p.sel = sel
		switch {
		case handle:
			p.drag = dragState{kind: dragLink, node: node, start: m}
		case sel.kind == selState || sel.kind == selEntry || sel.kind == selAny:
			p.drag = dragState{kind: dragNode, node: node, start: m}
		case sel.kind == selNone:
			p.drag = dragState{kind: dragPan, start: m}
		}
	}
	if ui.RightClicked(id, r) {
		sel, _, _ := p.hit(r, m)
		p.sel, p.menu, p.menuAt = sel, sel, m
		widgets.OpenContextMenu(ui, p.menuID)
	}
	p.dragging(r)
	if ui.Focus() == 0 && p.f.Focused && ui.KeyPressed(input.KeyDelete) {
		p.deleteSelection()
	}
}

func (p *Panel) dragging(r mathx.Rect) {
	ui := p.f.UI
	if p.drag.kind == dragNone {
		return
	}
	m := ui.Mouse()
	held := ui.MouseDown(input.MouseLeft) || (p.drag.kind == dragPan && ui.MouseDown(input.MouseMiddle))
	if held {
		delta := m.Sub(p.drag.start)
		if delta.Len() > 3 {
			p.drag.moved = true
		}
		switch p.drag.kind {
		case dragNode:
			p.drag.off = delta
		case dragPan:
			p.cam = p.cam.Sub(ui.MouseDelta().Scale(1 / p.zoom))
		}
		return
	}
	d := p.drag
	p.drag = dragState{}
	ui.ReleaseMouse()
	switch {
	case d.kind == dragNode && d.moved:
		var at [2]float32
		switch d.node {
		case -1:
			at = p.g.EntryPos
		case -2:
			at = p.g.AnyPos
		default:
			at = p.g.States[d.node].Pos
		}
		at = [2]float32{at[0] + d.off.X/p.zoom, at[1] + d.off.Y/p.zoom}
		p.notify(p.MoveState(d.node, at))
	case d.kind == dragLink:
		if _, to, _ := p.hit(r, m); to >= 0 {
			from := d.node
			if from == -2 {
				from = -1
			}
			p.notify(p.AddTransition(from, to))
		}
	}
}

func (p *Panel) deleteSelection() {
	switch p.sel.kind {
	case selState:
		p.notify(p.RemoveState(p.sel.index))
	case selTransition:
		i := p.sel.index
		p.sel = selection{}
		p.notify(p.RemoveTransition(i))
	}
}

func (p *Panel) menus(r mathx.Rect) {
	ui := p.f.UI
	if !widgets.ContextMenu(ui, p.menuID) {
		return
	}
	g := p.g
	switch p.menu.kind {
	case selState:
		i := p.menu.index
		if widgets.MenuItem(ui, "Set as Entry", "", int32(i) != g.Entry, false) {
			p.notify(p.SetEntry(i))
		}
		if widgets.MenuItem(ui, "Delete State", "Del", true, false) {
			p.notify(p.RemoveState(i))
		}
	case selTransition:
		i := p.menu.index
		if widgets.MenuItemIcon(ui, "Move Up", "", "ui.move_up", i > 0, false) {
			p.notify(p.MoveTransition(i, -1))
		}
		if widgets.MenuItemIcon(ui, "Move Down", "", "ui.move_down", i+1 < len(g.Transitions), false) {
			p.notify(p.MoveTransition(i, 1))
		}
		if widgets.MenuItem(ui, "Delete Transition", "Del", true, false) {
			p.notify(p.RemoveTransition(i))
		}
	default:
		if widgets.MenuItemIcon(ui, "Add State", "", "ui.plus", true, false) {
			p.notify(p.AddState(p.toGraph(r, p.menuAt)))
		}
		if widgets.MenuItemIcon(ui, "Auto Layout", "", "anim.graph", true, false) {
			p.notify(p.AutoLayout())
		}
		if widgets.MenuItem(ui, "Frame All", "", true, false) {
			p.frameAll()
		}
	}
	widgets.EndContextMenu(ui)
}

func overlaps(a, b mathx.Rect) bool {
	return a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H
}
