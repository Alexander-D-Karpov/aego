package dock

import (
	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/format/aetext"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/draw"
)

const LayoutVersion = 2

type NodeID uint32

type Kind uint8

const (
	KindTabs Kind = iota
	KindSplit
)

type Axis uint8

const (
	AxisX Axis = iota
	AxisY
)

type Side uint8

const (
	SideCenter Side = iota
	SideLeft
	SideRight
	SideTop
	SideBottom
)

type Node struct {
	ID     NodeID
	Kind   Kind
	Axis   Axis
	Ratio  float32
	First  NodeID
	Second NodeID
	Tabs   []string
	Active int
	Parent NodeID
	Rect   mathx.Rect
}

func (n *Node) ActivePanel() (string, bool) {
	if n.Kind != KindTabs || n.Active < 0 || n.Active >= len(n.Tabs) {
		return "", false
	}
	return n.Tabs[n.Active], true
}

type dragState struct {
	panel   string
	from    NodeID
	start   mathx.Vec2
	id      core.ID
	pending bool
	active  bool
}

type Tree struct {
	Title   func(panel string) string
	Icon    func(panel string) string
	Dirty   func(panel string) bool
	OnClose func(panel string) bool
	MinSize float32
	nodes   map[NodeID]*Node
	floats  []Float
	root    NodeID
	next    NodeID
	gap     float32
	tabH    float32
	bounds  mathx.Rect
	drag    dragState
	menu    menuState
	changed bool
	max     string
	maxRect mathx.Rect
}

func newTree() *Tree {
	return &Tree{MinSize: 80, nodes: map[NodeID]*Node{}, gap: 4, tabH: 24}
}

func New() *Tree {
	t := newTree()
	t.root = t.newTabs(nil)
	return t
}

func Standard(left, center, bottom, right []string) *Tree {
	t := newTree()
	l := t.newTabs(left)
	c := t.newTabs(center)
	b := t.newTabs(bottom)
	r := t.newTabs(right)
	t.root = t.newSplit(AxisX, 0.2, l, t.newSplit(AxisX, 0.75, t.newSplit(AxisY, 0.68, c, b), r))
	t.normalize()
	return t
}

func Columns(left, leftBottom, center, bottom, right []string) *Tree {
	t := newTree()
	l := t.newSplit(AxisY, 0.6, t.newTabs(left), t.newTabs(leftBottom))
	c := t.newSplit(AxisY, 0.78, t.newTabs(center), t.newTabs(bottom))
	t.root = t.newSplit(AxisX, 0.22, l, t.newSplit(AxisX, 0.78, c, t.newTabs(right)))
	t.normalize()
	return t
}

func (t *Tree) alloc(n *Node) NodeID {
	t.next++
	n.ID = t.next
	t.nodes[n.ID] = n
	return n.ID
}

func (t *Tree) newTabs(panels []string) NodeID {
	return t.alloc(&Node{Kind: KindTabs, Tabs: append([]string(nil), panels...)})
}

func (t *Tree) newSplit(axis Axis, ratio float32, a, b NodeID) NodeID {
	id := t.alloc(&Node{Kind: KindSplit, Axis: axis, Ratio: ratio, First: a, Second: b})
	t.nodes[a].Parent = id
	t.nodes[b].Parent = id
	return id
}

func (t *Tree) Root() NodeID {
	return t.root
}

func (t *Tree) Node(id NodeID) (*Node, bool) {
	n, ok := t.nodes[id]
	return n, ok
}

func (t *Tree) Changed() bool {
	c := t.changed
	t.changed = false
	return c
}

func (t *Tree) Dragging() bool {
	return t.drag.active
}

func (t *Tree) title(p string) string {
	if t.Title != nil {
		if s := t.Title(p); s != "" {
			return s
		}
	}
	return p
}

func (t *Tree) replace(parent, old, repl NodeID) {
	if parent == 0 {
		t.root = repl
		t.nodes[repl].Parent = 0
		return
	}
	p := t.nodes[parent]
	if p.First == old {
		p.First = repl
	} else {
		p.Second = repl
	}
	t.nodes[repl].Parent = parent
}

func (t *Tree) removeNode(id NodeID) {
	n := t.nodes[id]
	if n == nil || id == t.root {
		return
	}
	p := t.nodes[n.Parent]
	sibling := p.First
	if sibling == id {
		sibling = p.Second
	}
	delete(t.nodes, id)
	delete(t.nodes, p.ID)
	t.replace(p.Parent, p.ID, sibling)
	t.changed = true
}

func (t *Tree) normalize() {
	for {
		removed := false
		for id, n := range t.nodes {
			if n.Kind == KindTabs && len(n.Tabs) == 0 && id != t.root {
				t.removeNode(id)
				removed = true
				break
			}
		}
		if !removed {
			return
		}
	}
}

func (t *Tree) firstTabs(id NodeID) NodeID {
	for {
		n := t.nodes[id]
		if n == nil {
			return 0
		}
		if n.Kind == KindTabs {
			return id
		}
		id = n.First
	}
}

func (t *Tree) FindPanel(panel string) NodeID {
	for id, n := range t.nodes {
		if n.Kind != KindTabs {
			continue
		}
		for _, p := range n.Tabs {
			if p == panel {
				return id
			}
		}
	}
	return 0
}

// Largest is the tab group with the biggest area at the last layout, for documents with no better place.
func (t *Tree) Largest() NodeID {
	best, area := NodeID(0), float32(-1)
	t.walk(t.root, func(n *Node) {
		if a := n.Rect.W * n.Rect.H; n.Kind == KindTabs && a > area {
			best, area = n.ID, a
		}
	})
	return best
}

func (t *Tree) Has(panel string) bool {
	return t.FindPanel(panel) != 0 || t.floatIndex(panel) >= 0
}

func (t *Tree) Panels() []string {
	var out []string
	t.walk(t.root, func(n *Node) {
		out = append(out, n.Tabs...)
	})
	for _, f := range t.floats {
		out = append(out, f.Panel)
	}
	return out
}

func (t *Tree) walk(id NodeID, fn func(*Node)) {
	n := t.nodes[id]
	if n == nil {
		return
	}
	if n.Kind == KindSplit {
		t.walk(n.First, fn)
		t.walk(n.Second, fn)
		return
	}
	fn(n)
}

func (t *Tree) Activate(panel string) bool {
	if i := t.floatIndex(panel); i >= 0 {
		t.raise(i)
		return true
	}
	id := t.FindPanel(panel)
	if id == 0 {
		return false
	}
	n := t.nodes[id]
	for i, p := range n.Tabs {
		if p == panel && n.Active != i {
			n.Active = i
			t.changed = true
		}
	}
	return true
}

func (t *Tree) DockPanel(panel string, target NodeID, side Side) {
	if t.Has(panel) {
		t.Undock(panel)
	}
	n := t.nodes[target]
	if n == nil || n.Kind != KindTabs {
		n = t.nodes[t.firstTabs(t.root)]
	}
	t.changed = true
	if side == SideCenter || len(n.Tabs) == 0 {
		n.Tabs = append(n.Tabs, panel)
		n.Active = len(n.Tabs) - 1
		return
	}
	parent := n.Parent
	nt := t.newTabs([]string{panel})
	axis := AxisX
	if side == SideTop || side == SideBottom {
		axis = AxisY
	}
	var sp NodeID
	if side == SideLeft || side == SideTop {
		sp = t.newSplit(axis, 0.3, nt, n.ID)
	} else {
		sp = t.newSplit(axis, 0.7, n.ID, nt)
	}
	t.replace(parent, n.ID, sp)
}

func (t *Tree) EnsurePanel(panel string) {
	if !t.Has(panel) {
		t.DockPanel(panel, t.firstTabs(t.root), SideCenter)
	}
}

func (t *Tree) Undock(panel string) bool {
	if i := t.floatIndex(panel); i >= 0 {
		t.floats = append(t.floats[:i], t.floats[i+1:]...)
		t.changed = true
		return true
	}
	id := t.FindPanel(panel)
	if id == 0 {
		return false
	}
	n := t.nodes[id]
	for i, p := range n.Tabs {
		if p != panel {
			continue
		}
		n.Tabs = append(n.Tabs[:i], n.Tabs[i+1:]...)
		if i < n.Active {
			n.Active--
		}
		if n.Active >= len(n.Tabs) {
			n.Active = len(n.Tabs) - 1
		}
		if n.Active < 0 {
			n.Active = 0
		}
		break
	}
	if len(n.Tabs) == 0 {
		t.removeNode(id)
	}
	t.changed = true
	return true
}

func (t *Tree) Retain(keep func(panel string) bool) {
	for _, p := range t.Panels() {
		if !keep(p) {
			t.Undock(p)
		}
	}
}

func (t *Tree) Layout(r mathx.Rect) {
	t.layout(t.root, r)
}

func (t *Tree) layout(id NodeID, r mathx.Rect) {
	n := t.nodes[id]
	if n == nil {
		return
	}
	n.Rect = r
	if n.Kind != KindSplit {
		return
	}
	a, b := t.childRects(n)
	t.layout(n.First, a)
	t.layout(n.Second, b)
}

func (t *Tree) childRects(n *Node) (mathx.Rect, mathx.Rect) {
	r, g := n.Rect, t.gap
	if n.Axis == AxisX {
		w := (r.W - g) * n.Ratio
		return mathx.Rect{X: r.X, Y: r.Y, W: w, H: r.H},
			mathx.Rect{X: r.X + w + g, Y: r.Y, W: r.W - w - g, H: r.H}
	}
	h := (r.H - g) * n.Ratio
	return mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: h},
		mathx.Rect{X: r.X, Y: r.Y + h + g, W: r.W, H: r.H - h - g}
}

func (t *Tree) handleRect(n *Node) mathx.Rect {
	a, _ := t.childRects(n)
	if n.Axis == AxisX {
		return mathx.Rect{X: a.X + a.W, Y: n.Rect.Y, W: t.gap, H: n.Rect.H}
	}
	return mathx.Rect{X: n.Rect.X, Y: a.Y + a.H, W: n.Rect.W, H: t.gap}
}

func (t *Tree) content(n *Node) mathx.Rect {
	r := n.Rect
	return mathx.Rect{X: r.X, Y: r.Y + t.tabH, W: r.W, H: max(0, r.H-t.tabH)}
}

func (t *Tree) PanelRect(panel string) (mathx.Rect, bool) {
	if m := t.Maximized(); m != "" {
		if panel == m {
			return t.maxRect, true
		}
		return mathx.Rect{}, false
	}
	if i := t.floatIndex(panel); i >= 0 {
		return t.floatContent(i), true
	}
	n := t.nodes[t.FindPanel(panel)]
	if n == nil {
		return mathx.Rect{}, false
	}
	if p, ok := n.ActivePanel(); !ok || p != panel {
		return mathx.Rect{}, false
	}
	return t.content(n), true
}

func (t *Tree) PanelAt(p mathx.Vec2) (string, bool) {
	if m := t.Maximized(); m != "" {
		if t.maxRect.Contains(p) {
			return m, true
		}
		return "", false
	}
	for i := len(t.floats) - 1; i >= 0; i-- {
		if t.floats[i].Rect.Contains(p) {
			if t.floatContent(i).Contains(p) {
				return t.floats[i].Panel, true
			}
			return "", false
		}
	}
	id := t.leafAt(t.root, p)
	n := t.nodes[id]
	if n == nil || !t.content(n).Contains(p) {
		return "", false
	}
	return n.ActivePanel()
}

func (t *Tree) leafAt(id NodeID, p mathx.Vec2) NodeID {
	n := t.nodes[id]
	if n == nil || !n.Rect.Contains(p) {
		return 0
	}
	if n.Kind == KindTabs {
		return id
	}
	if hit := t.leafAt(n.First, p); hit != 0 {
		return hit
	}
	return t.leafAt(n.Second, p)
}

func (t *Tree) Draw(ui *core.Context, r mathx.Rect, fn func(panel string, r mathx.Rect)) {
	th := &ui.Theme
	if th.TabHeight > 0 {
		t.tabH = th.TabHeight
	}
	if th.SplitterW > 0 {
		t.gap = th.SplitterW
	}
	t.bounds = r
	if p := t.Maximized(); p != "" {
		t.drawMax(ui, r, p, fn)
		t.drawMenu(ui)
		return
	}
	t.Layout(r)
	t.drawNode(ui, t.root, fn)
	t.drawFloats(ui, fn)
	t.updateDrag(ui)
	t.drawMenu(ui)
}

func (t *Tree) drawNode(ui *core.Context, id NodeID, fn func(string, mathx.Rect)) {
	n := t.nodes[id]
	if n == nil {
		return
	}
	if n.Kind == KindSplit {
		t.drawNode(ui, n.First, fn)
		t.drawNode(ui, n.Second, fn)
		t.drawSplitter(ui, n)
		return
	}
	t.drawTabs(ui, n, fn)
}

func (t *Tree) drawSplitter(ui *core.Context, n *Node) {
	th := &ui.Theme
	h := t.handleRect(n)
	grab := mathx.Rect{X: h.X - 2, Y: h.Y - 2, W: h.W + 4, H: h.H + 4}
	id := core.IDSeq(core.IDOf("dock.split"), int(n.ID))
	if delta, ok := ui.Drag(id, grab); ok {
		size, d := n.Rect.W-t.gap, delta.X
		if n.Axis == AxisY {
			size, d = n.Rect.H-t.gap, delta.Y
		}
		if size > 0 && d != 0 {
			lim := min(float32(0.45), t.MinSize/size)
			n.Ratio = mathx.Clamp(n.Ratio+d/size, lim, 1-lim)
			t.changed = true
			t.layout(n.ID, n.Rect)
		}
	}
	col := th.Background
	if ui.Hot() == id || ui.Active() == id {
		col = th.AccentDim
		if n.Axis == AxisX {
			ui.SetCursorShape(core.CursorResizeEW)
		} else {
			ui.SetCursorShape(core.CursorResizeNS)
		}
	}
	ui.Rect(h, col)
}

func tabID(panel string) core.ID {
	return core.IDOf("dock.tab/" + panel)
}

func (t *Tree) drawTabs(ui *core.Context, n *Node, fn func(string, mathx.Rect)) {
	th := &ui.Theme
	r := n.Rect
	bar := mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: t.tabH}
	ui.Rect(bar, th.Header)
	ui.PushClip(bar)
	x := bar.X
	for i, p := range n.Tabs {
		tr := mathx.Rect{X: x, Y: bar.Y, W: t.tabWidth(ui, p), H: bar.H}
		id := tabID(p)
		if ui.Pressed(id, tr) {
			if n.Active != i {
				n.Active = i
				t.changed = true
			}
			if ui.DoubleClicked(id) {
				t.ToggleMaximize(p)
			} else {
				t.drag = dragState{panel: p, from: n.ID, start: ui.Mouse(), id: id, pending: true}
			}
		}
		if ui.RightClicked(id, tr) {
			t.openMenu(ui, p, false)
		}
		col := th.Tab
		switch {
		case i == n.Active:
			col = th.TabActive
		case ui.Hot() == id:
			col = th.TabHot
		}
		ui.List().RoundRect(mathx.Rect{X: tr.X, Y: tr.Y + 3, W: tr.W, H: tr.H - 3}, th.Radius.Tab, col, draw.CornersTop)
		textCol := th.TextDim
		if i == n.Active {
			ui.Rect(mathx.Rect{X: tr.X, Y: tr.Y + tr.H - 2, W: tr.W, H: 2}, th.Accent)
			textCol = th.Text
		}
		t.tabLabel(ui, p, tr, textCol)
		x += tr.W + 1
	}
	ui.PopClip()
	content := t.content(n)
	ui.Rect(content, th.Panel)
	if p, ok := n.ActivePanel(); ok && content.W > 0 && content.H > 0 {
		ui.PushClip(content)
		ui.PushID(p)
		fn(p, content)
		ui.PopID()
		ui.PopClip()
	}
}

func (t *Tree) updateDrag(ui *core.Context) {
	d := &t.drag
	if !d.pending && !d.active {
		return
	}
	m := ui.Mouse()
	floating := ui.Mods().Has(input.ModShift) || !t.bounds.Contains(m)
	if !ui.MouseDown(input.MouseLeft) {
		if d.active {
			target, side, ok := t.dropTarget(m)
			if floating || !ok {
				t.Float(d.panel, m)
			} else {
				t.move(d.panel, d.from, target, side)
			}
			ui.ReleaseMouse()
		}
		t.drag = dragState{}
		return
	}
	if d.pending && m.Sub(d.start).Len() > 6 {
		d.pending = false
		d.active = true
		ui.CaptureMouse(d.id)
	}
	if !d.active {
		return
	}
	th := &ui.Theme
	ui.SetCursorShape(core.CursorMove)
	l := ui.List()
	l.PushLayer(draw.LayerTop)
	if floating {
		fr := t.floatAt(m)
		l.Rect(fr, th.DropTarget)
		l.RectOutline(fr, th.Accent, 1)
	} else if target, side, ok := t.dropTarget(m); ok {
		l.Rect(t.zoneRect(t.nodes[target], side), th.DropTarget)
	}
	title := t.title(d.panel)
	size := ui.TextSize(title)
	ghost := mathx.Rect{X: m.X + 12, Y: m.Y + 12, W: size.X + 2*th.Padding, H: t.tabH}
	l.Rect(ghost, th.TabActive)
	l.RectOutline(ghost, th.Accent, 1)
	l.Text(title, mathx.Vec2{X: ghost.X + th.Padding, Y: ghost.Y + (ghost.H-ui.LineHeight())/2}, th.Text)
	l.PopLayer()
}

func (t *Tree) move(panel string, from, target NodeID, side Side) {
	src := t.nodes[from]
	if target == from && (side == SideCenter || src == nil || len(src.Tabs) == 1) {
		return
	}
	t.DockPanel(panel, target, side)
}

func (t *Tree) dropTarget(m mathx.Vec2) (NodeID, Side, bool) {
	id := t.leafAt(t.root, m)
	n := t.nodes[id]
	if n == nil || n.Rect.W <= 0 || n.Rect.H <= 0 {
		return 0, SideCenter, false
	}
	if m.Y < n.Rect.Y+t.tabH {
		return id, SideCenter, true
	}
	body := t.content(n)
	ex := min(body.W*0.25, 80)
	ey := min(body.H*0.25, 80)
	side, best := SideCenter, float32(1e9)
	if d := m.X - body.X; d < ex && d < best {
		side, best = SideLeft, d
	}
	if d := body.X + body.W - m.X; d < ex && d < best {
		side, best = SideRight, d
	}
	if d := m.Y - body.Y; d < ey && d < best {
		side, best = SideTop, d
	}
	if d := body.Y + body.H - m.Y; d < ey && d < best {
		side = SideBottom
	}
	return id, side, true
}

func (t *Tree) zoneRect(n *Node, side Side) mathx.Rect {
	r := n.Rect
	switch side {
	case SideLeft:
		return mathx.Rect{X: r.X, Y: r.Y, W: r.W / 2, H: r.H}
	case SideRight:
		return mathx.Rect{X: r.X + r.W/2, Y: r.Y, W: r.W / 2, H: r.H}
	case SideTop:
		return mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H / 2}
	case SideBottom:
		return mathx.Rect{X: r.X, Y: r.Y + r.H/2, W: r.W, H: r.H / 2}
	}
	return r
}

func (t *Tree) Save() []byte {
	doc := &aetext.Node{Head: "dock"}
	doc.Attrs = append(doc.Attrs, aetext.Field{Name: "version", Value: aetext.Int(LayoutVersion)})
	if root := t.saveNode(t.root); root != nil {
		doc.Blocks = append(doc.Blocks, root)
	}
	for _, f := range t.floats {
		n := &aetext.Node{Head: "float", Label: f.Panel}
		n.Fields = append(n.Fields, aetext.Field{Name: "rect", Value: aetext.List(
			aetext.Float32(f.Rect.X), aetext.Float32(f.Rect.Y), aetext.Float32(f.Rect.W), aetext.Float32(f.Rect.H),
		)})
		doc.Blocks = append(doc.Blocks, n)
	}
	return aetext.Write(doc)
}

func (t *Tree) saveNode(id NodeID) *aetext.Node {
	n := t.nodes[id]
	if n == nil {
		return nil
	}
	if n.Kind == KindSplit {
		axis := "x"
		if n.Axis == AxisY {
			axis = "y"
		}
		out := &aetext.Node{Head: "split"}
		out.Attrs = append(out.Attrs,
			aetext.Field{Name: "axis", Value: aetext.Ident(axis)},
			aetext.Field{Name: "ratio", Value: aetext.Float32(n.Ratio)},
		)
		out.Blocks = append(out.Blocks, t.saveNode(n.First), t.saveNode(n.Second))
		return out
	}
	out := &aetext.Node{Head: "tabs"}
	out.Attrs = append(out.Attrs, aetext.Field{Name: "active", Value: aetext.Int(int64(n.Active))})
	items := make([]aetext.Value, len(n.Tabs))
	for i, p := range n.Tabs {
		items[i] = aetext.String(p)
	}
	out.Fields = append(out.Fields, aetext.Field{Name: "panels", Value: aetext.List(items...)})
	return out
}

func Load(data []byte) (*Tree, error) {
	doc, err := aetext.Parse(data, "layout.aetext")
	if err != nil {
		return nil, err
	}
	if doc.Head != "dock" || len(doc.Blocks) == 0 {
		return nil, errs.New(errs.UIBadLayout, "not a dock layout", log.F("head", doc.Head))
	}
	if v, ok := doc.Attr("version"); ok && int(v.AsInt()) > LayoutVersion {
		return nil, errs.New(errs.UIBadLayout, "layout was written by a newer editor", log.F("version", v.AsInt()))
	}
	t := newTree()
	seen := map[string]bool{}
	root, err := t.load(doc.Blocks[0], seen)
	if err != nil {
		return nil, err
	}
	t.root = root
	for _, b := range doc.Blocks[1:] {
		if b.Head != "float" || b.Label == "" || seen[b.Label] {
			continue
		}
		v, ok := b.Field("rect")
		if !ok || len(v.List) != 4 {
			continue
		}
		seen[b.Label] = true
		f := func(i int) float32 { return float32(v.List[i].AsFloat()) }
		t.floats = append(t.floats, Float{Panel: b.Label, Rect: mathx.Rect{X: f(0), Y: f(1), W: f(2), H: f(3)}})
	}
	t.normalize()
	return t, nil
}

func (t *Tree) load(n *aetext.Node, seen map[string]bool) (NodeID, error) {
	switch n.Head {
	case "split":
		if len(n.Blocks) != 2 {
			return 0, errs.New(errs.UIBadLayout, "split needs exactly two children")
		}
		a, err := t.load(n.Blocks[0], seen)
		if err != nil {
			return 0, err
		}
		b, err := t.load(n.Blocks[1], seen)
		if err != nil {
			return 0, err
		}
		axis := AxisX
		if v, ok := n.Attr("axis"); ok && v.Ident == "y" {
			axis = AxisY
		}
		ratio := float32(0.5)
		if v, ok := n.Attr("ratio"); ok {
			ratio = float32(v.AsFloat())
		}
		return t.newSplit(axis, mathx.Clamp(ratio, 0.05, 0.95), a, b), nil
	case "tabs":
		var panels []string
		if v, ok := n.Field("panels"); ok {
			for _, item := range v.List {
				if item.Str == "" || seen[item.Str] {
					return 0, errs.New(errs.UIBadLayout, "panel is empty or listed twice", log.F("panel", item.Str))
				}
				seen[item.Str] = true
				panels = append(panels, item.Str)
			}
		}
		id := t.newTabs(panels)
		if v, ok := n.Attr("active"); ok {
			if a := int(v.AsInt()); a >= 0 && a < len(panels) {
				t.nodes[id].Active = a
			}
		}
		return id, nil
	}
	return 0, errs.New(errs.UIBadLayout, "unknown layout block", log.F("head", n.Head))
}
