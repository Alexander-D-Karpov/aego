package hierarchy

import (
	"aego/editor/core/favorites"
	"aego/engine/i18n"
	"path"
	"strconv"
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/clipboard"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/editor/core/prefabs"
	"aego/editor/core/props"
	"aego/editor/core/services"
	"aego/editor/core/templates"
	"aego/editor/core/workspace"
	"aego/platform/input"
	"aego/scene"
	"aego/ui/core"
	"aego/ui/dnd"
	"aego/ui/widgets"
)

const (
	zoneBefore = iota
	zoneInto
	zoneAfter
)

type row struct {
	h     scene.NodeHandle
	id    ids.NodeID
	depth int
	has   bool
	last  bool
	lines uint64
}

type Deps struct {
	Templates *templates.Registry
	Docs      services.Documents
	Selection services.Selection
	Commands  *command.Registry
	Scenes    services.SceneOps
	Notices   services.Notices
	Prefabs   services.Prefabs
	Tools     *context.SceneSettings
	// Ask prefills the Agent panel; nil hides Ask Agent.
	Ask func(prompt string)
}

type Panel struct {
	ambig         map[string]bool
	d             Deps
	rows          []row
	scroll        float32
	filter        string
	anchor        ids.NodeID
	pendingSingle ids.NodeID
	renaming      ids.NodeID
	renameStart   bool
	renameText    string
	reveal        ids.NodeID
	lastRev       uint64
	menuNode      ids.NodeID
	menuID        core.ID
	sceneMenuID   core.ID
	doc           document.ID
}

func New(d Deps) *Panel {
	return &Panel{d: d}
}

func (p *Panel) ID() string {
	return workspace.Hierarchy
}

func (p *Panel) Title() string {
	return i18n.Tr("editor.hierarchy.title", "Hierarchy")
}

func (p *Panel) Scope() string {
	return command.ScopeHierarchy
}

func (p *Panel) StartRename(id ids.NodeID) {
	p.renaming = id
	p.renameStart = true
	p.reveal = id
	p.filter = ""
}

func (p *Panel) sync() {
	p.lastRev = p.d.Selection.SelectionState().Revision()
}

func (p *Panel) report(err error) {
	if err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
	}
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	d := p.d.Docs.Doc()
	if d == nil {
		widgets.LabelDim(ui, i18n.Tr("editor.hierarchy.no_scene_open", "No scene open"))
		return
	}
	if d.ID != p.doc {
		p.doc = d.ID
		p.scroll = 0
		p.anchor = ids.NodeID{}
		p.renaming = ids.NodeID{}
	}
	p.menuID = ui.ID("menu")
	p.sceneMenuID = ui.ID("scenemenu")
	p.track(d)
	if p.renameStart {
		p.expandTo(d, p.renaming)
	}

	bar := ui.Cursor().Next(0)
	if widgets.SearchField(ui, ui.ID("search"), mathx.Rect{X: bar.X, Y: bar.Y, W: bar.W - bar.H - 4, H: bar.H}, &p.filter) {
		p.scroll = 0
	}
	addR := mathx.Rect{X: bar.X + bar.W - bar.H, Y: bar.Y, W: bar.H, H: bar.H}
	addID := ui.ID("add")
	if ui.Pressed(addID, addR) {
		p.create(d, p.activeNode(d), &mathx.Vec2{X: addR.X, Y: addR.Y + addR.H})
	}
	ui.Rect(addR, ui.WidgetColor(addID))
	ui.TextIn(addR, "+", th.Text, core.AlignCenter)
	hdr := ui.Cursor().Next(0)
	p.drawHeader(f, d, hdr)

	top := hdr.Y + hdr.H + th.Spacing
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	p.build(d)
	if !p.reveal.IsZero() {
		for i := range p.rows {
			if p.rows[i].id == p.reveal {
				widgets.EnsureVisible(&p.scroll, i, th.RowHeight, list.H)
				break
			}
		}
		p.reveal = ids.NodeID{}
	}
	widgets.VirtualList(ui, ui.ID("list"), list, len(p.rows), th.RowHeight, &p.scroll, func(i int, rr mathx.Rect) {
		p.drawRow(f, d, i, rr)
	})
	if f.DnD.Active() {
		p.pendingSingle = ids.NodeID{}
	}
	end := max(list.Y+float32(len(p.rows))*th.RowHeight-p.scroll, list.Y)
	if end < list.Y+list.H {
		p.emptyArea(f, d, mathx.Rect{X: list.X, Y: end, W: list.W, H: list.Y + list.H - end})
	}
	if widgets.ContextMenu(ui, p.menuID) {
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.hierarchy.create", "Create…"), p.d.Commands.Shortcut(command.SceneCreate), "ui.plus", true, false) {
			at := ui.Mouse()
			p.create(d, p.menuNode, &at)
		}
		widgets.Separator(ui)
		p.prefabItems(f, d)
		command.MenuItem(f.UI, p.d.Commands, favorites.CommandNode, "")
		command.MenuItem(f.UI, p.d.Commands, command.EditRename, "")
		command.MenuItem(f.UI, p.d.Commands, "edit.batch_rename", i18n.Tr("editor.hierarchy.batch_rename", "Batch Rename…"))
		command.MenuItem(f.UI, p.d.Commands, command.EditDuplicate, "")
		command.MenuItem(f.UI, p.d.Commands, command.EditCopy, "")
		command.MenuItem(f.UI, p.d.Commands, command.EditCut, "")
		command.MenuItem(f.UI, p.d.Commands, command.EditPaste, "")
		if p.d.Ask != nil && widgets.MenuItemIcon(ui, i18n.Tr("editor.hierarchy.ask_agent", "Ask Agent…"), "", "panel.agent", true, false) {
			p.d.Ask(p.askPrompt(d))
		}
		widgets.Separator(ui)
		command.MenuItem(f.UI, p.d.Commands, command.EditDelete, "")
		widgets.EndContextMenu(ui)
	}
	p.sceneMenu(f, d)
}

func (p *Panel) drawHeader(f *services.Frame, d *document.SceneDocument, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	id := ui.ID("scene")
	ui.Rect(r, th.Header)
	name := d.Caption()
	if !d.OnDisk {
		name += "  " + i18n.Tr("editor.hierarchy.unsaved", "(unsaved)")
	}
	x := r.X + th.Padding
	if d.IsPrefab() {
		sz := r.H - 8
		widgets.IconAt(ui, mathx.Rect{X: x, Y: r.Y + 4, W: sz, H: sz}, "file.prefab", th.Accent)
		x += sz + 6
	}
	ui.PushClip(r)
	ui.Text(name, mathx.Vec2{X: x, Y: ui.TextY(r)}, th.Text)
	ui.PopClip()
	if ui.Pressed(id, r) && ui.DoubleClicked(id) && !d.IsPrefab() {
		p.d.Scenes.RenameScene(d)
	}
	if ui.RightClicked(id, r) {
		widgets.OpenContextMenu(ui, p.sceneMenuID)
	}
	widgets.Tooltip(ui, id, d.Path)
}

func (p *Panel) sceneMenu(f *services.Frame, d *document.SceneDocument) {
	ui := f.UI
	if !widgets.ContextMenu(ui, p.sceneMenuID) {
		return
	}
	command.MenuItem(f.UI, p.d.Commands, command.FileSave, i18n.Tr("editor.hierarchy.save_scene", "Save Scene"))
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.hierarchy.save_scene_as", "Save Scene As…"), p.d.Commands.Shortcut(command.FileSaveAs), "file.save", true, false) {
		p.d.Scenes.SaveSceneAs(d)
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.hierarchy.rename_scene", "Rename Scene…"), "", "edit.rename", true, false) {
		p.d.Scenes.RenameScene(d)
	}
	widgets.EndContextMenu(ui)
}

func (p *Panel) activeNode(d *document.SceneDocument) ids.NodeID {
	if it, ok := p.d.Selection.SelectionState().Active(); ok && it.Doc == d.ID {
		return it.Node
	}
	return ids.NodeID{}
}

func (p *Panel) track(d *document.SceneDocument) {
	sel := p.d.Selection.SelectionState()
	rev := sel.Revision()
	if rev == p.lastRev {
		return
	}
	p.lastRev = rev
	if it, ok := sel.Active(); ok && it.Doc == d.ID {
		p.expandTo(d, it.Node)
		p.reveal = it.Node
	}
}

func (p *Panel) expandTo(d *document.SceneDocument, id ids.NodeID) {
	s := d.Scene()
	h := s.Handle(id)
	if h.IsZero() {
		return
	}
	root := s.Root()
	for q := s.Parent(h); !q.IsZero() && q != root; q = s.Parent(q) {
		d.View.Expanded[s.ID(q)] = true
	}
}

func (p *Panel) build(d *document.SceneDocument) {
	s := d.Scene()
	p.rows = p.rows[:0]
	if p.filter != "" {
		q := strings.ToLower(p.filter)
		root := s.Root()
		for _, h := range s.PreOrder() {
			if h == root || !s.IsAlive(h) {
				continue
			}
			if strings.Contains(strings.ToLower(s.Name(h)), q) {
				p.rows = append(p.rows, row{h: h, id: s.ID(h)})
			}
		}
		hs := make([]scene.NodeHandle, len(p.rows))
		for i := range p.rows {
			hs[i] = p.rows[i].h
		}
		p.ambig = props.Ambiguous(s, hs)
		return
	}
	p.walk(s, d.View.Expanded, s.Root(), 0, 0)
}

func (p *Panel) walk(s *scene.Scene, expanded map[ids.NodeID]bool, h scene.NodeHandle, depth int, lines uint64) {
	for c := nextAlive(s, s.FirstChild(h)); !c.IsZero(); {
		next := nextAlive(s, s.NextSibling(c))
		id := s.ID(c)
		has := s.ChildCount(c) > 0
		last := next.IsZero()
		p.rows = append(p.rows, row{h: c, id: id, depth: depth, has: has, last: last, lines: lines})
		if has && expanded[id] {
			sub := lines
			if !last && depth > 0 && depth <= 64 {
				sub |= 1 << (depth - 1)
			}
			p.walk(s, expanded, c, depth+1, sub)
		}
		c = next
	}
}

func nextAlive(s *scene.Scene, c scene.NodeHandle) scene.NodeHandle {
	for !c.IsZero() && !s.IsAlive(c) {
		c = s.NextSibling(c)
	}
	return c
}

func (p *Panel) indexOf(id ids.NodeID) int {
	for i := range p.rows {
		if p.rows[i].id == id {
			return i
		}
	}
	return -1
}

func rowID(id ids.NodeID) core.ID {
	return core.IDSeq(core.ID(id.Hi), int(uint32(id.Lo)))
}

func (p *Panel) drawRow(f *services.Frame, d *document.SceneDocument, i int, rr mathx.Rect) {
	ui := f.UI
	s := d.Scene()
	rw := p.rows[i]
	selected := p.d.Selection.IsSelected(rw.id)
	if p.renaming == rw.id {
		p.drawRename(f, d, rw, rr, selected)
		return
	}
	label, tip := s.Name(rw.h), ""
	if p.filter != "" {
		_, label, tip = props.NodeLabel(s, rw.h, p.ambig[label])
	}
	icon, empty := props.NodeIconOf(s, rw.h)
	id := rowID(rw.id)
	ev := widgets.TreeRowRect(ui, id, rr, widgets.TreeRow{
		Depth:       rw.depth,
		Label:       label,
		HasChildren: rw.has,
		Open:        d.View.Expanded[rw.id],
		Selected:    selected,
		Focused:     f.Focused,
		Dim:         !s.EffectiveEnabled(rw.h),
		Icon:        icon,
		Marker:      empty,
		Guides:      p.filter == "",
		Last:        rw.last,
		Lines:       rw.lines,
	})
	if tip != "" {
		widgets.Tooltip(ui, id, tip)
	}
	p.decorate(f, d, rw, rr)
	if ev.Toggled {
		d.View.Expanded[rw.id] = !d.View.Expanded[rw.id]
	}
	if ev.Pressed {
		p.press(f, i)
	}
	if ev.DoubleClicked {
		p.focus(rw.id)
	}
	if ev.Released && p.pendingSingle == rw.id {
		p.d.Selection.Select(rw.id)
		p.pendingSingle = ids.NodeID{}
		p.sync()
	}
	if ev.RightClicked {
		if !selected {
			p.d.Selection.Select(rw.id)
			p.sync()
		}
		p.menuNode = rw.id
		widgets.OpenContextMenu(ui, p.menuID)
	}
	if ui.Active() == id {
		f.DnD.Source(ui, id, p.payload(d))
	}
	p.dropRow(f, d, rw, rr)
}

func (p *Panel) decorate(f *services.Frame, d *document.SceneDocument, rw row, rr mathx.Rect) {
	if p.d.Prefabs == nil {
		return
	}
	st := p.d.Prefabs.Row(d, rw.id)
	ui := f.UI
	th := &ui.Theme
	icon, col := "", th.Accent
	switch st.Status {
	case prefabs.StatusRoot:
		icon = "file.prefab"
	case prefabs.StatusNested:
		icon, col = "file.prefab", th.TextDim
	case prefabs.StatusAdded:
		icon = "ui.plus"
	case prefabs.StatusMissing:
		icon, col = "status.warning", th.Error
	}
	if st.Orphans {
		icon, col = "status.warning", th.Warn
	}
	if st.Status == prefabs.StatusRoot || st.Status == prefabs.StatusNested || st.Status == prefabs.StatusMissing {
		ui.Rect(mathx.Rect{X: rr.X, Y: rr.Y + 2, W: 2, H: rr.H - 4}, col)
	}
	if icon == "" {
		return
	}
	sz := rr.H - 6
	widgets.IconAt(ui, mathx.Rect{X: rr.X + rr.W - sz - 4, Y: rr.Y + 3, W: sz, H: sz}, icon, col)
}

func (p *Panel) prefabItems(f *services.Frame, d *document.SceneDocument) {
	pf := p.d.Prefabs
	if pf == nil || p.menuNode.IsZero() {
		return
	}
	ui := f.UI
	info, ok := pf.Info(d, p.menuNode)
	if !ok {
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.hierarchy.create_prefab", "Create Prefab"), "", "file.prefab", true, false) {
			p.report(pf.CreateFrom(d, p.d.Selection.Selected(), ""))
		}
		widgets.Separator(ui)
		return
	}
	if widgets.SubMenuIcon(ui, i18n.Tr("editor.hierarchy.prefab", "Prefab"), "file.prefab", 260) {
		live := !info.Missing
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.hierarchy.open_prefab", "Open Prefab"), "", "file.open", live, false) {
			p.report(pf.Open(info.Prefab))
		}
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.hierarchy.select_prefab_asset", "Select Prefab Asset"), "", "panel.assets", true, false) {
			pf.SelectAsset(info.Prefab)
		}
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.hierarchy.find_instances", "Find Instances"), "", "ui.search", true, false) {
			pf.FindInstances(info.Prefab)
		}
		widgets.Separator(ui)
		touched := live && info.Touched(p.menuNode)
		for i, t := range pf.Targets(d, p.menuNode) {
			ui.PushIDInt(i)
			if widgets.MenuItemIcon(ui, i18n.Tr("editor.hierarchy.apply_node_to", "Apply Node to {name}", i18n.Str("name", t.Name)), "", "prefab.apply", touched, false) {
				p.report(pf.Apply(d, prefabs.Change{Node: p.menuNode, Level: prefabs.LevelNode, Nearest: t.Nearest}))
			}
			if widgets.MenuItemIcon(ui, i18n.Tr("editor.hierarchy.apply_all_to", "Apply All to {name}", i18n.Str("name", t.Name)), "", "prefab.apply", live, false) {
				p.report(pf.Apply(d, prefabs.Change{Node: p.menuNode, Level: prefabs.LevelAll, Nearest: t.Nearest}))
			}
			ui.PopID()
		}
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.hierarchy.revert_node", "Revert Node"), "", "prefab.revert", touched, false) {
			p.report(pf.Revert(d, prefabs.Change{Node: p.menuNode, Level: prefabs.LevelNode}))
		}
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.hierarchy.revert_all", "Revert All"), "", "prefab.revert", live, false) {
			p.report(pf.Revert(d, prefabs.Change{Node: p.menuNode, Level: prefabs.LevelAll}))
		}
		widgets.Separator(ui)
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.hierarchy.unpack", "Unpack"), "", "prefab.unpack", true, false) {
			p.report(pf.Unpack(d, p.menuNode, false))
		}
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.hierarchy.unpack_completely", "Unpack Completely"), "", "prefab.unpack", true, false) {
			p.report(pf.Unpack(d, p.menuNode, true))
		}
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.hierarchy.create_variant", "Create Variant"), "", "prefab.variant", live, false) {
			p.report(pf.CreateVariant(info.Prefab))
		}
		widgets.EndSubMenu(ui)
	}
	widgets.Separator(ui)
}

func (p *Panel) focus(id ids.NodeID) {
	p.d.Selection.Select(id)
	p.sync()
	p.pendingSingle = ids.NodeID{}
	if p.d.Tools.Tool == context.ToolSelect {
		p.d.Tools.Tool = context.ToolMove
	}
	p.d.Commands.Run(command.SceneFrame)
}

func (p *Panel) press(f *services.Frame, i int) {
	rw := p.rows[i]
	mods := f.UI.Mods()
	switch {
	case mods.Primary():
		p.d.Selection.ToggleSelection(rw.id)
		p.anchor = rw.id
	case mods.Has(input.ModShift) && p.indexOf(p.anchor) >= 0:
		a := p.indexOf(p.anchor)
		lo, hi := min(a, i), max(a, i)
		span := make([]ids.NodeID, 0, hi-lo+1)
		for k := lo; k <= hi; k++ {
			span = append(span, p.rows[k].id)
		}
		p.d.Selection.Select(span...)
	default:
		if p.d.Selection.IsSelected(rw.id) && p.d.Selection.SelectionState().Len() > 1 {
			p.pendingSingle = rw.id
		} else {
			p.d.Selection.Select(rw.id)
		}
		p.anchor = rw.id
	}
	p.sync()
}

func (p *Panel) payload(d *document.SceneDocument) dnd.Payload {
	nodes := append([]ids.NodeID(nil), p.d.Selection.Selected()...)
	label := i18n.Tr("editor.hierarchy.nodes_nodes", "{nodes} nodes", i18n.Str("nodes", strconv.Itoa(len(nodes))))
	if len(nodes) == 1 {
		if h := d.Scene().Handle(nodes[0]); !h.IsZero() {
			label = d.Scene().Name(h)
		}
	}
	return dnd.Payload{Kind: context.KindNodes, Label: label, Data: context.NodesPayload{Doc: d.ID, Nodes: nodes}}
}

func contains(list []ids.NodeID, id ids.NodeID) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

func (p *Panel) accepts(d *document.SceneDocument, pl dnd.Payload, target ids.NodeID) bool {
	switch pl.Kind {
	case context.KindNodes:
		np, ok := pl.Data.(context.NodesPayload)
		return ok && np.Doc == d.ID && !contains(np.Nodes, target)
	case context.KindAsset:
		ap, ok := pl.Data.(context.AssetPayload)
		if !ok {
			return false
		}
		if ap.Kind == asset.ArtScript {
			s := d.Scene()
			return target != s.ID(s.Root())
		}
		return p.d.Templates != nil && p.d.Templates.Accepts(ap.Kind, nil)
	}
	return false
}

func zoneOf(y float32, r mathx.Rect) int {
	t := (y - r.Y) / r.H
	switch {
	case t < 0.25:
		return zoneBefore
	case t > 0.75:
		return zoneAfter
	}
	return zoneInto
}

func (p *Panel) dropRow(f *services.Frame, d *document.SceneDocument, rw row, rr mathx.Rect) {
	ui := f.UI
	dn := f.DnD
	if !dn.Hovering(ui, rr) {
		return
	}
	pl, _ := dn.Payload()
	if !p.accepts(d, pl, rw.id) {
		dn.Target(ui, rr, "reject")
		return
	}
	zone := zoneOf(ui.Mouse().Y, rr)
	if zone != zoneInto {
		y := rr.Y
		if zone == zoneAfter {
			y = rr.Y + rr.H - 2
		}
		ui.Rect(mathx.Rect{X: rr.X, Y: y, W: rr.W, H: 2}, ui.Theme.Accent)
	}
	if got, ok := dn.Target(ui, rr, context.KindNodes, context.KindAsset); ok {
		p.drop(d, got, rw.id, zone)
	}
}

func (p *Panel) emptyArea(f *services.Frame, d *document.SceneDocument, r mathx.Rect) {
	ui := f.UI
	id := ui.ID("empty")
	if ui.Pressed(id, r) && !ui.Primary() && !ui.Mods().Has(input.ModShift) {
		p.d.Selection.Select()
		p.sync()
	}
	if ui.RightClicked(id, r) {
		p.d.Selection.Select()
		p.sync()
		p.menuNode = ids.NodeID{}
		widgets.OpenContextMenu(ui, p.menuID)
	}
	if !f.DnD.Hovering(ui, r) {
		return
	}
	s := d.Scene()
	rootID := s.ID(s.Root())
	pl, _ := f.DnD.Payload()
	if !p.accepts(d, pl, rootID) {
		f.DnD.Target(ui, r, "reject")
		return
	}
	if got, ok := f.DnD.Target(ui, r, context.KindNodes, context.KindAsset); ok {
		p.drop(d, got, rootID, zoneInto)
	}
}

func (p *Panel) drop(d *document.SceneDocument, pl dnd.Payload, target ids.NodeID, zone int) {
	s := d.Scene()
	switch pl.Kind {
	case context.KindNodes:
		np, _ := pl.Data.(context.NodesPayload)
		p.move(d, np.Nodes, target, zone)
	case context.KindAsset:
		ap, _ := pl.Data.(context.AssetPayload)
		if ap.Kind == asset.ArtScript {
			p.d.Scenes.AttachScript([]ids.NodeID{target}, ap.ID)
			return
		}
		parent, index := target, -1
		if zone != zoneInto {
			th := s.Handle(target)
			parent = s.ID(s.Parent(th))
			index = commands.ChildIndex(s, th)
			if zone == zoneAfter {
				index++
			}
		}
		t, ok := p.d.Templates.ForAsset(ap.Kind, nil, ap.ID, path.Base(ap.Name))
		if !ok {
			return
		}
		if err := p.d.Scenes.Instantiate(t, parent, index); err != nil {
			p.d.Notices.Notify(context.LevelError, err.Error())
		}
	}
}

func (p *Panel) move(d *document.SceneDocument, nodes []ids.NodeID, target ids.NodeID, zone int) {
	s := d.Scene()
	th := s.Handle(target)
	if th.IsZero() {
		return
	}
	tops := clipboard.TopLevel(s, nodes)
	if len(tops) == 0 {
		return
	}
	tx := d.History.Begin("Move")
	anchor := th
	for _, n := range tops {
		if n == th {
			continue
		}
		parent, index := target, -1
		switch zone {
		case zoneBefore:
			ph := s.Parent(th)
			idx := commands.ChildIndex(s, th)
			if s.Parent(n) == ph && commands.ChildIndex(s, n) < idx {
				idx--
			}
			parent, index = s.ID(ph), idx
		case zoneAfter:
			ph := s.Parent(anchor)
			idx := commands.ChildIndex(s, anchor) + 1
			if s.Parent(n) == ph && commands.ChildIndex(s, n) < idx {
				idx--
			}
			parent, index = s.ID(ph), idx
		}
		if p.d.Prefabs != nil && parent != s.ID(s.Parent(n)) && p.d.Prefabs.Locked(d, s.ID(n)) {
			tx.Abort()
			p.d.Notices.Notify(context.LevelWarn, i18n.Note("editor.hierarchy.n_comes_from_a_prefab", "{n} comes from a prefab and keeps its parent, unpack the instance to move it", i18n.Str("n", s.Name(n))))
			return
		}
		cmd := commands.NewMutation("Reparent", commands.EnvFor(s), commands.Move(s.ID(n), parent, index, true))
		if err := tx.Exec(cmd); err != nil {
			tx.Abort()
			p.d.Notices.Notify(context.LevelError, err.Error())
			return
		}
		if zone == zoneAfter {
			anchor = n
		}
	}
	tx.Commit()
	if zone == zoneInto && th != s.Root() {
		d.View.Expanded[target] = true
	}
}

func (p *Panel) drawRename(f *services.Frame, d *document.SceneDocument, rw row, rr mathx.Rect, selected bool) {
	ui := f.UI
	th := &ui.Theme
	if selected {
		ui.Rect(rr, th.Selection)
	}
	fid := ui.ID("rename")
	x := rr.X + float32(rw.depth)*th.Indent + th.Indent + 2
	fr := mathx.Rect{X: x, Y: rr.Y + 1, W: max(40, rr.X+rr.W-x-2), H: rr.H - 2}
	if p.renameStart {
		p.renameStart = false
		p.renameText = d.Scene().Name(rw.h)
		ui.SetFocus(fid)
		ui.EditBuffer(fid, p.renameText).SelectAll()
	}
	if _, committed := widgets.TextFieldRect(ui, fid, fr, &p.renameText, false); committed {
		if err := d.Props.Rename(nil, rw.id, p.renameText); err != nil {
			p.d.Notices.Notify(context.LevelError, err.Error())
		}
	}
	if ui.Focus() != fid {
		p.renaming = ids.NodeID{}
	}
}

func (p *Panel) create(d *document.SceneDocument, parent ids.NodeID, at *mathx.Vec2) {
	p.d.Scenes.Create(parent, templates.Place{}, at, func([]ids.NodeID) {
		if !parent.IsZero() {
			d.View.Expanded[parent] = true
		}
	})
}

// askPrompt names the selected nodes with their ids, the way agent commands address them.
func (p *Panel) askPrompt(d *document.SceneDocument) string {
	s := d.Scene()
	var names []string
	for _, id := range p.d.Selection.Selected() {
		if h := s.Handle(id); !h.IsZero() && s.IsAlive(h) {
			names = append(names, s.Name(h)+" (#"+id.String()+")")
		}
	}
	if len(names) == 0 {
		return "In " + d.Path + ": "
	}
	return "In " + d.Path + ", " + strings.Join(names, ", ") + ": "
}
