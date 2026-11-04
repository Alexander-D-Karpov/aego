// Package animator is the Animator panel: the state graph of an animation controller (D338, D340).
package animator

import (
	"bytes"
	"strconv"
	"strings"

	"aego/anim"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/animedit"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/scene"
	"aego/scene/mutate"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	ID           = "animator"
	animatorName = "animation.Animator"
	sideW        = float32(250)
	nodeW        = float32(150)
	nodeH        = float32(40)
	pillW        = float32(96)
	pillH        = float32(28)
)

// Live reads and drives the Animator of a node in the running game.
type Live interface {
	State(node ids.NodeID) (state string, progress float32, ok bool)
	Param(node ids.NodeID, name string) (float64, bool)
	SetParam(node ids.NodeID, name string, v float64)
}

// Clip is a clip the state inspector offers.
type Clip struct {
	ID   ids.AssetID
	Name string
}

type Deps struct {
	Docs      services.Documents
	Selection services.Selection
	Notices   services.Notices
	Files     animedit.Files
	PathOf    func(ids.AssetID) (string, bool)
	Clips     func() []Clip
	Live      func() Live
	Revision  func() uint64
}

type selKind uint8

const (
	selNone selKind = iota
	selState
	selTransition
	selEntry
	selAny
)

type selection struct {
	kind  selKind
	index int
}

type edge struct {
	label   string
	a, c, b mathx.Vec2
}

type Panel struct {
	d          Deps
	f          *services.Frame
	node       ids.NodeID
	rel        string
	data       []byte
	rev        uint64
	stale      bool
	ver        uint64
	doc        *mutate.CtrlDocument
	g          *anim.Graph
	failed     string
	conds      []string
	params     []string
	names      []string
	heads      []string
	stateHeads []string
	clips      []Clip
	clipNames  []string
	sel        selection
	cam        mathx.Vec2
	zoom       float32
	viewOf     string
	drag       dragState
	menuAt     mathx.Vec2
	menu       selection
	scroll     float32
	content    float32
	ctrlStamp  [2]uint64
	ctrlNode   ids.NodeID
	ctrlScene  *scene.Scene
	ctrlRel    string
	ctrlOK     bool
	edges      []edge
	chips      []mathx.Rect
	area       mathx.Rect
	menuID     core.ID
	clipID     core.ID
	clipRect   mathx.Rect
}

func New(d Deps) *Panel {
	return &Panel{d: d, zoom: 1}
}

func (p *Panel) ID() string    { return ID }
func (p *Panel) Title() string { return i18n.Tr("editor.animator.title", "Animator") }

// ShowsAsset reports whether the existing scene-bound graph can present this
// controller without changing a scene component or creating another document.
func (p *Panel) ShowsAsset(rel string) bool {
	d := p.d.Docs.Doc()
	if d == nil || p.d.Selection == nil {
		return false
	}
	for _, id := range p.d.Selection.Selected() {
		if current, ok := p.controller(d.Scene(), id); ok && current == rel {
			return true
		}
	}
	return false
}

func (p *Panel) notify(err error) {
	if err != nil && p.d.Notices != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
	}
}

// controller finds the controller of the selected node's Animator.
func (p *Panel) controller(s *scene.Scene, id ids.NodeID) (string, bool) {
	h := s.Handle(id)
	td, ok := s.Registry().ByName(animatorName)
	if h.IsZero() || !ok {
		return "", false
	}
	if _, has := s.GetByType(h, td.ID); !has {
		return "", false
	}
	fd, ok := td.Field("controller")
	if !ok {
		return "", false
	}
	v, err := s.GetProperty(h, td.ID, fd.ID)
	if err != nil || v.Asset.IsZero() {
		return "", false
	}
	return p.d.PathOf(v.Asset)
}

// load rereads the controller when History moved or the asset pipeline reimported something, and parses it only when the bytes changed.
func (p *Panel) load(d *document.SceneDocument, rel string) {
	ver := d.History.Version()
	var rev uint64
	if p.d.Revision != nil {
		rev = p.d.Revision()
	}
	if rel == p.rel && ver == p.ver && rev == p.rev && !p.stale {
		return
	}
	p.ver, p.rev, p.stale = ver, rev, false
	data, err := p.d.Files.Read(rel)
	if err != nil {
		p.failed = err.Error()
		return
	}
	if rel == p.rel && bytes.Equal(data, p.data) && p.g != nil {
		return
	}
	if rel != p.rel {
		p.sel, p.cam, p.zoom = selection{}, mathx.Vec2{}, 1
	}
	p.rel, p.data = rel, data
	doc, err := mutate.LoadCtrl(data, rel)
	if err != nil {
		p.failed, p.g = err.Error(), nil
		return
	}
	g, err := doc.Graph()
	if err != nil {
		p.failed, p.g = err.Error(), nil
		return
	}
	g.Layout()
	p.doc, p.g, p.failed = doc, g, ""
	p.labels()
	if p.sel.kind == selState && p.sel.index >= len(g.States) || p.sel.kind == selTransition && p.sel.index >= len(g.Transitions) {
		p.sel = selection{}
	}
	if p.viewOf != rel {
		p.viewOf = rel
		p.frameAll()
	}
}

var opText = map[anim.Op]string{anim.OpGt: ">", anim.OpGe: ">=", anim.OpLt: "<", anim.OpLe: "<=", anim.OpEq: "==", anim.OpNe: "!="}

// labels precomputes the condition text of every transition, so drawing allocates nothing.
func (p *Panel) labels() {
	g := p.g
	p.conds = p.conds[:0]
	for i := range g.Transitions {
		t := &g.Transitions[i]
		var parts []string
		for _, c := range t.Conds {
			name := "?"
			if c.Param >= 0 && int(c.Param) < len(g.Params) {
				name = g.Params[c.Param].Name
			}
			switch {
			case c.Op == anim.OpSet:
				parts = append(parts, name)
			case g.Params[max(c.Param, 0)].Kind == anim.ParamBool:
				if c.Value == 0 {
					parts = append(parts, "!"+name)
				} else {
					parts = append(parts, name)
				}
			default:
				parts = append(parts, name+" "+opText[c.Op]+" "+strconv.FormatFloat(c.Value, 'g', 4, 64))
			}
		}
		if t.HasExit {
			parts = append(parts, "exit "+strconv.FormatFloat(float64(t.ExitTime), 'g', 3, 32))
		}
		p.conds = append(p.conds, strings.Join(parts, ", "))
	}
	p.params, p.names = p.params[:0], p.names[:0]
	for _, pr := range g.Params {
		p.params = append(p.params, pr.Name+"  "+pr.Kind.String())
		p.names = append(p.names, pr.Name)
	}
	p.heads = p.heads[:0]
	for _, t := range g.Transitions {
		from, to := i18n.Tr("editor.animator.any_state", "Any State"), "?"
		if t.From >= 0 && int(t.From) < len(g.States) {
			from = g.States[t.From].Name
		}
		if t.To >= 0 && int(t.To) < len(g.States) {
			to = g.States[t.To].Name
		}
		p.heads = append(p.heads, from+" → "+to)
	}
	p.stateHeads = p.stateHeads[:0]
	for _, st := range g.States {
		p.stateHeads = append(p.stateHeads, i18n.Tr("editor.animator.state_head", "State {name}", i18n.Str("name", st.Name)))
	}
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	p.f = f
	ui := f.UI
	d := p.d.Docs.Doc()
	if d == nil {
		widgets.LabelDim(ui, i18n.Tr("editor.animator.open_a_scene_and_select", "Open a scene and select a node with an Animator"))
		return
	}
	s := d.Scene()
	if sel := p.d.Selection.Selected(); len(sel) > 0 {
		if sel[0] != p.node {
			p.node = sel[0]
		}
	}
	if stamp := [2]uint64{s.Revision(), s.StructureRevision()}; stamp != p.ctrlStamp || p.node != p.ctrlNode || s != p.ctrlScene {
		p.ctrlStamp, p.ctrlNode, p.ctrlScene = stamp, p.node, s
		p.ctrlRel, p.ctrlOK = p.controller(s, p.node)
	}
	rel, ok := p.ctrlRel, p.ctrlOK
	if !ok {
		widgets.LabelDim(ui, i18n.Tr("editor.animator.select_a_node_with_an", "Select a node with an Animator that has a controller, or Create → Animation → Controller"))
		return
	}
	p.load(d, rel)
	if p.g == nil {
		widgets.LabelWrapped(ui, p.failed, true)
		return
	}
	side := mathx.Rect{X: r.X, Y: r.Y, W: min(sideW, r.W*0.4), H: r.H}
	canvas := mathx.Rect{X: side.X + side.W + 1, Y: r.Y, W: max(0, r.W-side.W-1), H: r.H}
	ui.Rect(mathx.Rect{X: side.X + side.W, Y: r.Y, W: 1, H: r.H}, ui.Theme.Separator)
	p.canvas(d, canvas)
	p.sidebar(d, side)
}

// Edit applies controller ops as one History step on the clip file.
func (p *Panel) Edit(label string, ops ...mutate.Op) error {
	d := p.d.Docs.Doc()
	if d == nil || p.rel == "" || len(ops) == 0 {
		return nil
	}
	err := d.History.Exec(&animedit.FileEdit{Name: label, Rel: p.rel, Ctrl: true, Tx: mutate.Tx{Label: label, Ops: ops}, Files: p.d.Files})
	p.stale = true
	return err
}

func pos(v [2]float32) aetext.Value {
	return aetext.List(aetext.Float32(float32(int(v[0]))), aetext.Float32(float32(int(v[1]))))
}

// MoveState places a state, the Entry node (-1) or the Any node (-2) at a graph point.
func (p *Panel) MoveState(i int, at [2]float32) error {
	switch {
	case i == -1:
		return p.Edit(i18n.Tr("editor.history.move_entry", "Move Entry"), mutate.CtrlRoot("entryPos", pos(at)))
	case i == -2:
		return p.Edit(i18n.Tr("editor.history.move_any", "Move Any"), mutate.CtrlRoot("anyPos", pos(at)))
	case i >= 0 && i < len(p.g.States):
		return p.Edit(i18n.Tr("editor.history.move_state", "Move State"), mutate.CtrlAttr("state", p.g.States[i].Name, "pos", pos(at)))
	}
	return nil
}

// AddTransition adds a transition from a state (-1 is Any) to another.
func (p *Panel) AddTransition(from, to int) error {
	g := p.g
	if to < 0 || to >= len(g.States) {
		return nil
	}
	label := "any"
	if from >= 0 {
		label = g.States[from].Name
	}
	body := &aetext.Node{Attrs: []aetext.Field{{Name: "to", Value: aetext.String(g.States[to].Name)}}}
	return p.Edit(i18n.Tr("editor.history.add_transition", "Add Transition"), mutate.CtrlPut("transition", label, body))
}

// AddState adds a state at a graph point.
func (p *Panel) AddState(at [2]float32) error {
	name := "State"
	for n := 1; p.g.StateIndex(name) >= 0; n++ {
		name = "State " + strconv.Itoa(n+1)
	}
	body := &aetext.Node{Attrs: []aetext.Field{{Name: "pos", Value: pos(at)}}}
	ops := []mutate.Op{mutate.CtrlPut("state", name, body)}
	if len(p.g.States) == 0 {
		ops = append(ops, mutate.CtrlRoot("entry", aetext.String(name)))
	}
	return p.Edit(i18n.Tr("editor.history.add_state", "Add State"), ops...)
}

// RemoveState removes a state with the transitions into and out of it.
func (p *Panel) RemoveState(i int) error {
	g := p.g
	if i < 0 || i >= len(g.States) {
		return nil
	}
	var ops []mutate.Op
	for k := len(g.Transitions) - 1; k >= 0; k-- {
		if t := &g.Transitions[k]; int(t.From) == i || int(t.To) == i {
			ops = append(ops, mutate.CtrlRemove("transition", "").At(k))
		}
	}
	ops = append(ops, mutate.CtrlRemove("state", g.States[i].Name))
	p.sel = selection{}
	return p.Edit(i18n.Tr("editor.history.delete_state", "Delete State"), ops...)
}

// AutoLayout writes a fresh layout of every node.
func (p *Panel) AutoLayout() error {
	g := *p.g
	g.States = append([]anim.State(nil), p.g.States...)
	for i := range g.States {
		g.States[i].HasPos = false
	}
	g.HasEntryPos, g.HasAnyPos = false, false
	g.Layout()
	var ops []mutate.Op
	for _, s := range g.States {
		ops = append(ops, mutate.CtrlAttr("state", s.Name, "pos", pos(s.Pos)))
	}
	ops = append(ops, mutate.CtrlRoot("entryPos", pos(g.EntryPos)), mutate.CtrlRoot("anyPos", pos(g.AnyPos)))
	err := p.Edit(i18n.Tr("editor.history.auto_layout", "Auto Layout"), ops...)
	p.viewOf = ""
	return err
}

// Graph is the controller the panel shows, for tests.
func (p *Panel) Graph() *anim.Graph { return p.g }

// SelectTransition selects a transition, for tests and screenshots.
func (p *Panel) SelectTransition(i int) { p.sel = selection{kind: selTransition, index: i} }

// SelectState selects a state, for tests and screenshots.
func (p *Panel) SelectState(i int) { p.sel = selection{kind: selState, index: i} }

// NodeRect is the screen rect of a state, of Entry (-1) or of Any (-2) on the last frame.
func (p *Panel) NodeRect(i int) (mathx.Rect, bool) {
	if p.g == nil || i >= len(p.g.States) || i < -2 {
		return mathx.Rect{}, false
	}
	return p.box(p.area, i), true
}

// EdgePoint is a screen point on transition k on the last frame.
func (p *Panel) EdgePoint(k int) (mathx.Vec2, bool) {
	if k < 0 || k >= len(p.edges) || p.edges[k].a == p.edges[k].b {
		return mathx.Vec2{}, false
	}
	e := &p.edges[k]
	return bezier(e.a, e.c, e.b, 0.3), true
}

// MenuOpen reports whether the graph context menu is open.
func (p *Panel) MenuOpen() bool { return p.f != nil && p.menuID != 0 && p.f.UI.PopupOpen(p.menuID) }

// ClipPickerRect is where the clip picker of the state inspector was drawn on the last frame.
func (p *Panel) ClipPickerRect() mathx.Rect { return p.clipRect }

// ClipPickerOpen reports whether the clip list of the state inspector is open.
func (p *Panel) ClipPickerOpen() bool {
	return p.f != nil && p.clipID != 0 && p.f.UI.PopupOpen(p.clipID)
}

// Selected is the selected state or transition, for tests: kind is "state", "transition" or "".
func (p *Panel) Selected() (string, int) {
	switch p.sel.kind {
	case selState:
		return "state", p.sel.index
	case selTransition:
		return "transition", p.sel.index
	}
	return "", 0
}
