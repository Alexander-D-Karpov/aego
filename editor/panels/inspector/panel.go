package inspector

import (
	"aego/editor/core/presence"
	"aego/editor/core/selection"
	"aego/editor/ext/inspect"
	"aego/engine/i18n"
	wpresence "aego/ui/widgets/presence"
	"fmt"
	"strconv"
	"strings"

	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/clipboard"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/dataedit"
	"aego/editor/core/document"
	"aego/editor/core/history"
	"aego/editor/core/history/commands"
	"aego/editor/core/prefabs"
	"aego/editor/core/presets"
	"aego/editor/core/props"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/engine/module"
	"aego/platform/input"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/scene/scriptcomp"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

type Deps struct {
	Docs      services.Documents
	Selection services.Selection
	Commands  *command.Registry
	Notices   services.Notices
	Nav       services.Navigation
	Problems  services.Problems
	Prefabs   services.Prefabs
	Sections  *module.Sections
	Pipeline  *pipeline.Pipeline
	Thumbs    context.ThumbSource
	Play      context.PlayBridge
	Code      func() context.ScriptNav
	Clip      clipboard.Env
	Assets    services.AssetOps
	Scenes    services.SceneOps
	Collapsed map[string]bool
	Watch     func(expr string)
	Data      *dataedit.Service
	Schema    *schema.Registry
	Presets   *presets.Service
	AssetView func(ids.AssetID) bool
	Audition  func(id ids.AssetID, toggle bool) (playing, ok bool)
	Driven    func(d *document.SceneDocument, node ids.NodeID, t schema.TypeID, f schema.FieldID) bool
	// Presence and AgentSel show the agent's node while following, with its changes lit (11b5b).
	Presence *presence.Model
	AgentSel *selection.Service
	// Defer makes the user's edit of a field a waiting card also changes again after the card.
	Defer func(sp presence.Spot, run func())
	// StopFollowing pauses follow mode when the user asks for their own selection back.
	StopFollowing func()
	// AddComponent opens the palette for the nodes under at, with a query for the screenshots.
	AddComponent func(nodes []ids.NodeID, at *mathx.Vec2, query string)
}

type Panel struct {
	catLang      string
	catNames     map[string]string
	assetBoxes   []assetBox
	scrollTo     string
	top          float32
	openAdd      *string
	reg          *inspect.Registry
	d            Deps
	f            *services.Frame
	tx           *history.Transaction
	txDoc        document.ID
	doc          *document.SceneDocument
	locked       bool
	lockDoc      document.ID
	lockNodes    []ids.NodeID
	agentView    bool
	agentNodes   []ids.NodeID
	collapsed    map[core.ID]bool
	scroll       float32
	content      float32
	pick         string
	nodes        []ids.NodeID
	types        []schema.TypeID
	labels       map[string]string
	typeNames    map[schema.TypeID][2]string
	live         map[core.ID]schema.Value
	curves       map[core.ID]*widgets.CurveState
	liveT        *scene.Transform2D
	euler        *props.EulerCache
	info         *prefabs.Info
	hoverField   fieldRef
	hoverType    schema.TypeID
	menuField    fieldRef
	rightUsed    bool
	fieldMenuID  core.ID
	bgMenuID     core.ID
	scriptNames  []string
	scriptSearch string
	focusScript  bool
	dataMenuID   core.ID
	dataMenu     string
	dataStep     bool
}

const (
	Scope        = "inspector"
	CommandCopy  = "inspector.copy"
	CommandPaste = "inspector.paste"
	CommandWatch = "inspector.watch"
)

type fieldRef struct {
	t  schema.TypeID
	f  schema.FieldID
	ok bool
}

func New(reg *inspect.Registry, d Deps) *Panel {
	return &Panel{
		reg:       reg,
		d:         d,
		collapsed: map[core.ID]bool{},
		labels:    map[string]string{},
		typeNames: map[schema.TypeID][2]string{},
		live:      map[core.ID]schema.Value{},
		curves:    map[core.ID]*widgets.CurveState{},
		euler:     props.NewEulerCache(),
	}
}

func (p *Panel) code() context.ScriptNav {
	if p.d.Code == nil {
		return nil
	}
	return p.d.Code()
}

func (p *Panel) gotoScript(typeName, field string) {
	c := p.code()
	if c == nil {
		return
	}
	if l, ok := c.Declaration(typeName, field); ok {
		p.d.Nav.Open(l)
		return
	}
	p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.inspector.declaration_unavailable", "The declaration of {name} is not available until the scripts compile", i18n.Str("name", shortScript(typeName))))
}

func (p *Panel) ID() string {
	return workspace.Inspector
}

func (p *Panel) Title() string {
	return i18n.Tr("editor.inspector.title", "Inspector")
}

func (p *Panel) Scope() string {
	return Scope
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	p.f = f
	p.assetBoxes = p.assetBoxes[:0]
	ui := f.UI
	p.doc = nil
	d := p.target()
	p.settle(d)
	p.hoverField, p.hoverType, p.rightUsed = fieldRef{}, 0, false
	p.fieldMenuID, p.bgMenuID = ui.ID("fieldmenu"), ui.ID("bgmenu")
	p.info = nil
	if p.d.Data != nil && !p.locked {
		if id := p.d.Data.Inspected(); !id.IsZero() {
			if p.d.AssetView == nil || !p.d.AssetView(id) {
				p.drawData(r, id)
			}
			return
		}
	}
	if d == nil {
		widgets.LabelDim(ui, i18n.Tr("editor.inspector.no_scene_open", "No scene open"))
		return
	}
	p.doc = d
	s := d.Scene()
	p.nodes = p.nodes[:0]
	src := p.d.Selection.Selected()
	if p.locked {
		src = p.lockNodes
	}
	p.agentView = false
	if m := p.d.Presence; m != nil && m.Following && p.d.AgentSel != nil && !p.locked {
		if nodes := p.d.AgentSel.Nodes(d.ID, p.agentNodes[:0]); len(nodes) > 0 {
			src, p.agentView, p.agentNodes = nodes, true, nodes
		}
	}
	for _, id := range src {
		if h := s.Handle(id); !h.IsZero() && s.IsAlive(h) {
			p.nodes = append(p.nodes, id)
		}
	}
	if p.locked && len(p.nodes) == 0 {
		p.unlock()
		for _, id := range p.d.Selection.Selected() {
			if h := s.Handle(id); !h.IsZero() && s.IsAlive(h) {
				p.nodes = append(p.nodes, id)
			}
		}
	}
	r = p.lockBar(d, r)
	if p.agentView {
		r = p.agentBar(r)
	}
	if len(p.nodes) == 0 {
		widgets.LabelDim(ui, i18n.Tr("editor.inspector.nothing_selected", "Nothing selected"))
		return
	}
	if p.d.Prefabs != nil && len(p.nodes) == 1 {
		if info, ok := p.d.Prefabs.Info(d, p.nodes[0]); ok {
			p.info = info
		}
	}
	th := &ui.Theme
	if ui.MouseOver(ui.ID("scroll"), r) {
		if w := ui.ScrollY(th.RowHeight * 3); w != 0 {
			p.scroll -= w
		}
	}
	p.scroll = mathx.Clamp(p.scroll, 0, max(0, p.content-r.H+th.Padding*2))
	cur := ui.Cursor()
	top := cur.Y
	p.top = top
	cur.Y -= p.scroll
	ui.PushClip(r)
	p.header(d)
	p.prefabCard(d)
	p.transform(d)
	p.components(d)
	p.scripts(d)
	p.addComponent(d)
	p.addScript(d)
	ui.PopClip()
	p.content = cur.Y + p.scroll - top
	p.menus(d, r)
	p.dropScripts(r)
}

func (p *Panel) menus(d *document.SceneDocument, r mathx.Rect) {
	ui := p.f.UI
	if !p.rightUsed && ui.MousePressed(input.MouseRight) && ui.Hit(r) && !ui.MouseInOverlay() {
		widgets.OpenContextMenu(ui, p.bgMenuID)
	}
	if widgets.ContextMenu(ui, p.bgMenuID) {
		kind := p.d.Clip.Peek()
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.paste_component", "Paste Component"), p.d.Commands.Shortcut(CommandPaste), "edit.paste", kind == clipboard.KindComponent, false) {
			p.confirm(d, i18n.Tr("editor.inspector.paste_component", "Paste Component"), func() error { return p.d.Clip.PasteComponent(d, p.nodes) })
		}
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.paste_components", "Paste Components"), "", "edit.paste", kind == clipboard.KindComponents, false) {
			p.confirm(d, i18n.Tr("editor.inspector.paste_components", "Paste Components"), func() error { return p.d.Clip.PasteComponents(d, p.nodes) })
		}
		widgets.EndContextMenu(ui)
	}
	if widgets.ContextMenu(ui, p.fieldMenuID) {
		p.fieldMenu(d)
		widgets.EndContextMenu(ui)
	}
}

func (p *Panel) CanCopy() bool {
	return p.hoverField.ok || p.hoverType != 0
}

func (p *Panel) CopyHovered() error {
	d := p.cur()
	if d == nil || len(p.nodes) == 0 {
		return nil
	}
	if p.hoverField.ok {
		if td, fd, ok := p.lookup(d, p.hoverField); ok && !p.copyTexture(d, td, fd) {
			p.copyValue(d, td, fd)
		}
		return nil
	}
	if p.hoverType != 0 {
		return p.d.Clip.CopyComponent(d.Scene(), p.nodes[0], p.hoverType)
	}
	return nil
}

func (p *Panel) CanWatch() bool {
	return p.hoverField.ok && p.d.Watch != nil
}

func (p *Panel) WatchHovered() error {
	d := p.cur()
	if d == nil || len(p.nodes) == 0 || !p.CanWatch() {
		return nil
	}
	td, fd, ok := p.lookup(d, p.hoverField)
	if !ok {
		return nil
	}
	s := d.Scene()
	comp := td.FullName[strings.LastIndexByte(td.FullName, '.')+1:]
	p.d.Watch(s.Path(s.Handle(p.nodes[0])) + "." + comp + "." + fd.Name)
	return nil
}

func (p *Panel) PasteHovered() error {
	d := p.cur()
	if d == nil || len(p.nodes) == 0 {
		return nil
	}
	if p.hoverField.ok {
		if td, fd, ok := p.lookup(d, p.hoverField); ok {
			if v, ok := p.d.Clip.ReadValue(fd); ok {
				p.confirm(d, i18n.Tr("editor.inspector.paste_name", "Paste {name}", i18n.Str("name", p.labelOf(td, fd))), func() error { return d.Props.Set(nil, p.nodes, td.ID, fd.ID, v) })
				return nil
			}
			if done, err := p.pasteAsset(d, td, fd); done {
				return err
			}
		}
	}
	switch p.d.Clip.Peek() {
	case clipboard.KindComponent:
		p.confirm(d, i18n.Tr("editor.inspector.paste_component", "Paste Component"), func() error { return p.d.Clip.PasteComponent(d, p.nodes) })
		return nil
	case clipboard.KindComponents:
		p.confirm(d, i18n.Tr("editor.inspector.paste_components", "Paste Components"), func() error { return p.d.Clip.PasteComponents(d, p.nodes) })
		return nil
	}
	p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.inspector.the_clipboard_does_not_fit", "The clipboard does not fit this field"))
	return nil
}

func (p *Panel) settle(d *document.SceneDocument) {
	if p.tx == nil {
		return
	}
	ui := p.f.UI
	if d == nil || d.ID != p.txDoc || (!ui.MouseDown(input.MouseLeft) && ui.Focus() == 0) {
		p.tx.Commit()
		p.tx = nil
	}
}

func (p *Panel) apply(e widgets.Edit, label string, fn func(*history.Transaction) error) {
	d := p.cur()
	if d == nil || d.History == nil {
		return
	}
	h := d.History
	if e.Begin && e.Changed && e.End && p.tx == nil && len(p.nodes) > confirmOver {
		p.confirm(d, label, func() error {
			tx := h.Begin(label)
			if err := fn(tx); err != nil {
				tx.Abort()
				return err
			}
			tx.Commit()
			return nil
		})
		return
	}
	if e.Begin && p.tx == nil {
		p.tx = h.Begin(label)
		p.txDoc = d.ID
	}
	if e.Changed {
		tx, own := p.tx, false
		if tx == nil {
			tx, own = h.Begin(label), true
		}
		if err := fn(tx); err != nil {
			p.d.Notices.Notify(context.LevelError, err.Error())
		}
		if own {
			tx.Commit()
		}
	}
	if e.End && p.tx != nil {
		p.tx.Commit()
		p.tx = nil
	}
}

func (p *Panel) labelOf(td *schema.TypeDescriptor, fd *schema.FieldDescriptor) string {
	if td != nil {
		return schema.FieldLabel(td, fd)
	}
	if fd.Hints.DisplayName != "" {
		return fd.Hints.DisplayName
	}
	if l, ok := p.labels[fd.Name]; ok {
		return l
	}
	l := inspect.Humanize(fd.Name)
	p.labels[fd.Name] = l
	return l
}

func triCheck(ui *core.Context, id core.ID, r mathx.Rect, state int) bool {
	th := &ui.Theme
	clicked := ui.Clicked(id, r)
	box := mathx.Rect{X: r.X + 3, Y: r.Y + 3, W: r.W - 6, H: r.H - 6}
	switch state {
	case 1:
		ui.Rect(box, th.Accent)
		widgets.CheckMark(ui, box, th.Background)
	case 2:
		ui.Rect(box, ui.WidgetColor(id))
		ui.Outline(box, th.Border)
		ui.Rect(mathx.Rect{X: box.X + 3, Y: box.Y + box.H/2 - 1, W: box.W - 6, H: 2}, th.Accent)
	default:
		ui.Rect(box, ui.WidgetColor(id))
		ui.Outline(box, th.Border)
	}
	return clicked
}

func (p *Panel) header(d *document.SceneDocument) {
	ui := p.f.UI
	th := &ui.Theme
	ui.PushID("node")
	defer ui.PopID()
	c := p.beginCard("node", i18n.Tr("editor.inspector.node", "Node"), "", "cat.node", "", true)
	defer p.endCard(ui, c)
	menuID := ui.ID("menu")
	if ui.RightClicked(c.hid, c.hdr) {
		p.rightUsed = true
		widgets.OpenContextMenu(ui, menuID)
	} else if c.more {
		widgets.OpenContextMenu(ui, menuID)
	}
	if widgets.ContextMenu(ui, menuID) {
		p.nodeMenu(d)
		widgets.EndContextMenu(ui)
	}
	if !c.open {
		return
	}
	s := d.Scene()
	first := s.Handle(p.nodes[0])
	row := ui.Cursor().Next(0)
	state := 0
	if s.Enabled(first) {
		state = 1
	}
	for _, id := range p.nodes[1:] {
		if s.Enabled(s.Handle(id)) != (state == 1) {
			state = 2
			break
		}
	}
	if triCheck(ui, ui.ID("enabled"), mathx.Rect{X: row.X, Y: row.Y, W: row.H, H: row.H}, state) {
		after := state != 1
		p.confirm(d, i18n.Tr("editor.inspector.toggle_enabled", "Toggle Enabled"), func() error { return d.Props.SetEnabled(nil, p.nodes, after) })
	}
	nr := mathx.Rect{X: row.X + row.H + 4, Y: row.Y, W: row.W - row.H - 4, H: row.H}
	if len(p.nodes) == 1 {
		name := s.Name(first)
		if _, committed := widgets.TextFieldRect(ui, ui.ID("name"), nr, &name, false); committed {
			if err := d.Props.Rename(nil, p.nodes[0], name); err != nil {
				p.d.Notices.Notify(context.LevelError, err.Error())
			}
		}
	} else {
		ui.TextIn(nr, i18n.Tr("editor.inspector.nodes_nodes", "{nodes} nodes", i18n.Str("nodes", strconv.Itoa(len(p.nodes)))), th.TextDim, core.AlignLeft)
	}
	mask := s.Layers(first)
	mixed := false
	for _, id := range p.nodes[1:] {
		if s.Layers(s.Handle(id)) != mask {
			mixed = true
			break
		}
	}
	if widgets.MaskField(ui, ui.ID("layers"), widgets.Field(ui, i18n.Tr("editor.inspector.layers", "Layers")), &mask, nil, mixed) {
		m := mask
		p.confirm(d, i18n.Tr("editor.inspector.set_layers", "Set Layers"), func() error { return d.Props.SetLayers(nil, p.nodes, m) })
	}
	p.spaceRow(d)
}

func (p *Panel) editing(ui *core.Context, key core.ID) bool {
	a, f := ui.Active(), ui.Focus()
	if a == key || f == key || ui.PopupOpen(key) {
		return true
	}
	for i := 1; i <= 4; i++ {
		s := core.IDSeq(key, i)
		if a == s || f == s {
			return true
		}
	}
	return false
}

func (p *Panel) transform2D(d *document.SceneDocument) {
	ui := p.f.UI
	ui.PushID("transform")
	defer ui.PopID()
	card := p.beginCard("transform", i18n.Tr("editor.inspector.transform", "Transform"), "", "cat.transform", i18n.Tr("editor.inspector.reset", "Reset"), false)
	defer p.endCard(ui, card)
	if card.action {
		p.resetTransform(d)
	}
	if !card.open {
		return
	}
	posID, rotID, scaleID := ui.ID("pos"), ui.ID("rot"), ui.ID("scale")
	t, mixed := d.Props.CommonTransform(p.nodes)
	if p.liveT != nil && (p.editing(ui, posID) || p.editing(ui, rotID) || p.editing(ui, scaleID)) {
		t, mixed = *p.liveT, [5]bool{}
	} else {
		p.liveT = nil
	}
	keep := func(e widgets.Edit, nt scene.Transform2D) {
		if e.Begin || e.Changed {
			p.liveT = &nt
		}
		if e.End {
			p.liveT = nil
		}
	}

	pos := t.Position
	e := widgets.Vec2Field(ui, posID, widgets.Field(ui, i18n.Tr("editor.inspector.position", "Position")), &pos, widgets.NumberOpts{Speed: 0.05}, mixed[0], mixed[1])
	if e.Any() {
		x, y := pos.X != t.Position.X, pos.Y != t.Position.Y
		nt := t
		nt.Position = pos
		keep(e, nt)
		p.apply(e, i18n.Tr("editor.history.move", "Move"), func(tx *history.Transaction) error {
			return d.Props.EditTransform(tx, p.nodes, func(tr *scene.Transform2D) {
				if x {
					tr.Position.X = pos.X
				}
				if y {
					tr.Position.Y = pos.Y
				}
			})
		})
	}

	deg := float64(mathx.Rad2Deg(t.Rotation))
	e = widgets.DragNumber(ui, rotID, widgets.Field(ui, i18n.Tr("editor.inspector.rotation", "Rotation")), &deg, widgets.NumberOpts{Speed: 0.5, Prec: 2, Suffix: "°", Mixed: mixed[2]})
	if e.Any() {
		rad := mathx.Deg2Rad(float32(deg))
		nt := t
		nt.Rotation = rad
		keep(e, nt)
		p.apply(e, i18n.Tr("editor.history.rotate", "Rotate"), func(tx *history.Transaction) error {
			return d.Props.EditTransform(tx, p.nodes, func(tr *scene.Transform2D) {
				tr.Rotation = rad
			})
		})
	}

	sc := t.Scale
	e = widgets.Vec2Field(ui, scaleID, widgets.Field(ui, i18n.Tr("editor.inspector.scale", "Scale")), &sc, widgets.NumberOpts{Speed: 0.01}, mixed[3], mixed[4])
	if e.Any() {
		x, y := sc.X != t.Scale.X, sc.Y != t.Scale.Y
		nt := t
		nt.Scale = sc
		keep(e, nt)
		p.apply(e, i18n.Tr("editor.history.scale", "Scale"), func(tx *history.Transaction) error {
			return d.Props.EditTransform(tx, p.nodes, func(tr *scene.Transform2D) {
				if x {
					tr.Scale.X = sc.X
				}
				if y {
					tr.Scale.Y = sc.Y
				}
			})
		})
	}
}

func (p *Panel) components(d *document.SceneDocument) {
	ui := p.f.UI
	th := &ui.Theme
	s := d.Scene()
	reg := s.Registry()
	unknown, hasUnknown := reg.ByName("engine.Unknown")
	first := s.Handle(p.nodes[0])
	p.types = p.types[:0]
	scriptT := schema.HashName(scriptcomp.ScriptName)
	s.ComponentsOf(first, func(t schema.TypeID) {
		if (hasUnknown && t == unknown.ID) || t == scriptT {
			return
		}
		if td, ok := reg.ByID(t); ok && td.Hidden {
			return
		}
		for _, id := range p.nodes[1:] {
			if _, ok := s.GetByType(s.Handle(id), t); !ok {
				return
			}
		}
		p.types = append(p.types, t)
	})
	for _, sec := range groupComponents(p.reg, reg, p.types) {
		r := ui.Cursor().Next(ui.LineHeight() + 4)
		ui.Text(p.categoryTitle(sec.Category), mathx.Vec2{X: r.X + 2, Y: r.Y + 3}, th.TextDisabled)
		for _, t := range sec.Types {
			p.component(d, t)
		}
	}
	if hasUnknown && len(p.nodes) == 1 {
		if u, ok := scene.Get[sceneio.Unknown](s, first); ok {
			for _, c := range u.Components {
				widgets.LabelDim(ui, i18n.Tr("editor.inspector.typename_not_loaded_kept_as", "{typename} (not loaded, kept as is)", i18n.Str("typename", c.TypeName)))
			}
		}
	}
}

// agentBar says the Inspector shows the agent's node while following, with a way back to the user's own selection.
func (p *Panel) agentBar(r mathx.Rect) mathx.Rect {
	ui := p.f.UI
	th := &ui.Theme
	bar := ui.Cursor().Next(0)
	col := th.Agent.Presence
	col.A = 0.16
	ui.RoundRect(bar, th.Radius.Button, col)
	wpresence.Marker(ui, mathx.Rect{X: bar.X + 4, Y: bar.Y + (bar.H-14)/2, W: 14, H: 14}, 0)
	label := i18n.Tr("editor.inspector.show_mine", "Show My Selection")
	bw := ui.TextSize(label).X + 20
	ui.Text(widgets.Elide(ui, i18n.Tr("editor.inspector.following_agent", "Following the agent"), bar.W-bw-30), mathx.Vec2{X: bar.X + 24, Y: ui.TextY(bar)}, th.Text)
	if widgets.ButtonRect(ui, ui.ID("mine"), mathx.Rect{X: bar.X + bar.W - bw, Y: bar.Y, W: bw, H: bar.H}, label) && p.d.StopFollowing != nil {
		p.d.StopFollowing()
	}
	return r
}

// agentEditing reports whether a waiting card of the agent changes a field of this component on the node.
func (p *Panel) agentEditing(d *document.SceneDocument, td *schema.TypeDescriptor) bool {
	m := p.d.Presence
	if m == nil || len(p.nodes) != 1 {
		return false
	}
	for i := range td.Fields {
		if m.Editing(d.Path, p.nodes[0], td.FullName, td.Fields[i].Name) {
			return true
		}
	}
	return false
}

func (p *Panel) component(d *document.SceneDocument, t schema.TypeID) {
	ui := p.f.UI
	td, ok := d.Scene().Registry().ByID(t)
	if !ok {
		return
	}
	if p.scrollTo == td.FullName {
		p.scroll += ui.Cursor().Y - p.top
		p.scrollTo = ""
	}
	ui.PushRawID(core.ID(t))
	defer ui.PopID()
	meta := p.reg.Meta(td)
	badge := meta.Badge
	if p.info != nil && p.info.ComponentAdded(p.nodes[0], ownerOf(td)) {
		badge = "+ added"
	}
	c := p.beginCard(td.FullName, meta.DisplayName, badge, meta.Icon, "", true)
	c.typ = t
	if m := p.d.Presence; m != nil && len(p.nodes) == 1 {
		wpresence.Light(ui, c.hdr, m.NodeLight(d.Path, p.nodes[0], td.FullName, p.f.Now))
	}
	if p.agentEditing(d, td) {
		wr := ui.Cursor().Next(0)
		wpresence.Marker(ui, mathx.Rect{X: wr.X, Y: wr.Y + (wr.H-14)/2, W: 14, H: 14}, 0)
		ui.Text(widgets.Elide(ui, i18n.Tr("editor.inspector.agent_editing", "Agent is editing this; your change will be applied after"), wr.W-20), mathx.Vec2{X: wr.X + 20, Y: ui.TextY(wr)}, ui.Theme.Warn)
	}
	if ui.Hot() == c.hid && ui.KeyPressed(input.KeyF1) {
		p.d.Nav.Open(context.DocsAt(td.FullName, meta.DisplayName))
	}
	defer func() { p.endCard(ui, c) }()
	menuID := ui.ID("menu")
	if ui.RightClicked(c.hid, c.hdr) {
		p.rightUsed = true
		widgets.OpenContextMenu(ui, menuID)
	} else if c.more {
		widgets.OpenContextMenu(ui, menuID)
	}
	if widgets.ContextMenu(ui, menuID) {
		p.componentMenu(d, td)
		widgets.EndContextMenu(ui)
	}
	if !c.open {
		return
	}
	def := func() { p.fields(d, td) }
	custom, ok := p.reg.Custom(t)
	if !ok {
		def()
		return
	}
	mark := ui.Mark()
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				ui.Restore(mark)
				msg := fmt.Sprint(rec)
				p.reg.Disable(t, msg)
				p.d.Notices.Notify(context.LevelError, i18n.Note("editor.inspector.inspector_for_fullname_crashed_msg", "inspector for {fullname} crashed: {msg}", i18n.Str("fullname", td.FullName), i18n.Str("msg", msg)))
			}
		}()
		p.reg.DrawCustom(custom, p.nodes, def)
	}()
}

func (p *Panel) remove(d *document.SceneDocument, td *schema.TypeDescriptor) {
	p.confirm(d, i18n.Tr("editor.inspector.remove_name", "Remove {name}", i18n.Str("name", p.reg.Meta(td).DisplayName)), func() error {
		env := commands.EnvFor(d.Scene())
		tx := d.History.Begin(i18n.Tr("editor.history.remove", "Remove {name}", i18n.Str("name", schema.TypeLabel(td))))
		for _, id := range p.nodes {
			if err := tx.Exec(commands.NewMutation(i18n.Tr("editor.history.remove", "Remove {name}", i18n.Str("name", schema.TypeLabel(td))), env, commands.RemoveComp(id, td))); err != nil {
				tx.Abort()
				return err
			}
		}
		tx.Commit()
		return nil
	})
}

func (p *Panel) reset(d *document.SceneDocument, td *schema.TypeDescriptor) {
	p.confirm(d, i18n.Tr("editor.inspector.reset_name", "Reset {name}", i18n.Str("name", p.reg.Meta(td).DisplayName)), func() error {
		s := d.Scene()
		env := commands.EnvFor(s)
		defaults := p.reg.Defaults(td.ID)
		tx := d.History.Begin(i18n.Tr("editor.history.reset", "Reset {name}", i18n.Str("name", schema.TypeLabel(td))))
		for _, id := range p.nodes {
			if err := tx.Exec(commands.NewMutation(i18n.Tr("editor.history.remove", "Remove {name}", i18n.Str("name", schema.TypeLabel(td))), env, commands.RemoveComp(id, td))); err != nil {
				tx.Abort()
				return err
			}
			if err := tx.Exec(commands.NewMutation(i18n.Tr("editor.history.add", "Add {name}", i18n.Str("name", schema.TypeLabel(td))), env, commands.AddComp(s, id, td, defaults))); err != nil {
				tx.Abort()
				return err
			}
		}
		tx.Commit()
		return nil
	})
}

// ScrollTo brings the card of a component type to the top on the next draw, for the documentation screenshots.
func (p *Panel) ScrollTo(fullName string) {
	p.scrollTo = fullName
}

// OpenAdd opens Add Component with the search text on the next draw, for the documentation screenshots.
func (p *Panel) OpenAdd(query string) {
	p.openAdd = &query
}

func (p *Panel) addComponent(d *document.SceneDocument) {
	ui := p.f.UI
	th := &ui.Theme
	row := ui.Cursor().Next(th.RowHeight + 4)
	id := ui.ID("add")
	at := mathx.Vec2{X: row.X, Y: row.Y + row.H}
	if ui.Pressed(id, row) && p.d.AddComponent != nil {
		p.d.AddComponent(p.nodes, &at, "")
	}
	if p.openAdd != nil && p.d.AddComponent != nil {
		p.d.AddComponent(p.nodes, &at, *p.openAdd)
		p.openAdd = nil
	}
	ui.Rect(row, ui.WidgetColor(id))
	ui.Outline(row, th.AccentDim)
	ui.TextIn(row, i18n.Tr("editor.inspector.add_component", "Add Component"), th.Text, core.AlignCenter)
}

func (p *Panel) nodeMenu(d *document.SceneDocument) {
	ui := p.f.UI
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.copy_all_components", "Copy All Components"), "", "edit.copy", len(p.types) > 0, false) {
		if err := p.d.Clip.CopyComponents(d.Scene(), p.nodes[0], p.types); err != nil {
			p.report(err)
		} else {
			p.d.Notices.Notify(context.LevelInfo, i18n.TrN("editor.inspector.copied_components", int64(len(p.types)), "Copied {n} component", "Copied {n} components"))
		}
	}
	can := p.d.Clip.Peek() == clipboard.KindComponents
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.paste_components", "Paste Components"), "", "edit.paste", can, false) {
		p.confirm(d, i18n.Tr("editor.inspector.paste_components", "Paste Components"), func() error { return p.d.Clip.PasteComponents(d, p.nodes) })
	}
}

// categoryTitle translates a component category once per language, so the frame does not build keys.
func (p *Panel) categoryTitle(cat string) string {
	if lang := i18n.Default().Language(); lang != p.catLang || p.catNames == nil {
		p.catLang, p.catNames = lang, map[string]string{}
	}
	t, ok := p.catNames[cat]
	if !ok {
		t = i18n.Lookup("editor.category."+strings.ToLower(cat), cat)
		p.catNames[cat] = t
	}
	return t
}
