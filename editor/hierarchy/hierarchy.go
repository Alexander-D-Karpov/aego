package hierarchy

import (
	"aego/asset"
	"aego/editor/assets"
	"path"
	"strconv"
	"strings"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/clipboard"
	"aego/editor/command"
	"aego/editor/context"
	"aego/editor/document"
	"aego/editor/history/commands"
	"aego/editor/sceneview"
	"aego/editor/templates"
	"aego/editor/workspace"
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
}

type Panel struct {
	templates     *templates.Registry
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
	doc           document.ID
}

func New(t *templates.Registry) *Panel {
	return &Panel{templates: t}
}

func (p *Panel) ID() string {
	return workspace.Hierarchy
}

func (p *Panel) Title() string {
	return "Hierarchy"
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

func (p *Panel) sync(ctx *context.Editor) {
	p.lastRev = ctx.Selection.Revision()
}

func (p *Panel) Draw(ctx *context.Editor, r mathx.Rect) {
	ui := ctx.UI
	th := &ui.Theme
	d := ctx.Doc()
	if d == nil {
		widgets.LabelDim(ui, "No scene open")
		return
	}
	if d.ID != p.doc {
		p.doc = d.ID
		p.scroll = 0
		p.anchor = ids.NodeID{}
		p.renaming = ids.NodeID{}
	}
	p.menuID = ui.ID("menu")
	p.track(ctx, d)
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
		if ui.PopupOpen(addID) {
			ui.ClosePopup(addID)
		} else {
			ui.CloseAllPopups()
			ui.OpenPopup(addID, mathx.Vec2{X: addR.X, Y: addR.Y + addR.H})
		}
	}
	ui.Rect(addR, ui.WidgetColor(addID))
	ui.TextIn(addR, "+", th.Text, core.AlignCenter)
	if ui.BeginPopup(addID, 200) {
		p.createItems(ctx, d, p.activeNode(ctx, d))
		ui.EndPopup()
	}

	top := bar.Y + bar.H + th.Spacing
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
		p.drawRow(ctx, d, i, rr)
	})
	if ctx.DnD.Active() {
		p.pendingSingle = ids.NodeID{}
	}
	end := max(list.Y+float32(len(p.rows))*th.RowHeight-p.scroll, list.Y)
	if end < list.Y+list.H {
		p.emptyArea(ctx, d, mathx.Rect{X: list.X, Y: end, W: list.W, H: list.Y + list.H - end})
	}
	if widgets.ContextMenu(ui, p.menuID) {
		if widgets.SubMenu(ui, "Create", 220) {
			p.createItems(ctx, d, p.menuNode)
			widgets.EndSubMenu(ui)
		}
		widgets.Separator(ui)
		ctx.CommandItem(command.EditRename, "Rename")
		ctx.CommandItem(command.EditDuplicate, "Duplicate")
		ctx.CommandItem(command.EditCopy, "Copy")
		ctx.CommandItem(command.EditCut, "Cut")
		ctx.CommandItem(command.EditPaste, "Paste")
		widgets.Separator(ui)
		ctx.CommandItem(command.EditDelete, "Delete")
		widgets.EndContextMenu(ui)
	}
}

func (p *Panel) activeNode(ctx *context.Editor, d *document.SceneDocument) ids.NodeID {
	if it, ok := ctx.Selection.Active(); ok && it.Doc == d.ID {
		return it.Node
	}
	return ids.NodeID{}
}

func (p *Panel) track(ctx *context.Editor, d *document.SceneDocument) {
	rev := ctx.Selection.Revision()
	if rev == p.lastRev {
		return
	}
	p.lastRev = rev
	if it, ok := ctx.Selection.Active(); ok && it.Doc == d.ID {
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
		return
	}
	p.walk(s, d.View.Expanded, s.Root(), 0)
}

func (p *Panel) walk(s *scene.Scene, expanded map[ids.NodeID]bool, h scene.NodeHandle, depth int) {
	s.Children(h, func(c scene.NodeHandle) bool {
		if !s.IsAlive(c) {
			return true
		}
		id := s.ID(c)
		has := s.ChildCount(c) > 0
		p.rows = append(p.rows, row{h: c, id: id, depth: depth, has: has})
		if has && expanded[id] {
			p.walk(s, expanded, c, depth+1)
		}
		return true
	})
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

func (p *Panel) drawRow(ctx *context.Editor, d *document.SceneDocument, i int, rr mathx.Rect) {
	ui := ctx.UI
	s := d.Scene()
	rw := p.rows[i]
	selected := ctx.IsSelected(rw.id)
	if p.renaming == rw.id {
		p.drawRename(ctx, d, rw, rr, selected)
		return
	}
	label := s.Name(rw.h)
	if p.filter != "" {
		label = s.Path(rw.h)
	}
	id := rowID(rw.id)
	ev := widgets.TreeRowRect(ui, id, rr, widgets.TreeRow{
		Depth:       rw.depth,
		Label:       label,
		HasChildren: rw.has,
		Open:        d.View.Expanded[rw.id],
		Selected:    selected,
		Focused:     ctx.FocusedPanel == workspace.Hierarchy,
		Dim:         !s.EffectiveEnabled(rw.h),
	})
	if ev.Toggled {
		d.View.Expanded[rw.id] = !d.View.Expanded[rw.id]
	}
	if ev.Pressed {
		p.press(ctx, i)
	}
	if ev.DoubleClicked {
		p.focus(ctx, rw.id)
	}
	if ev.Released && p.pendingSingle == rw.id {
		ctx.Select(rw.id)
		p.pendingSingle = ids.NodeID{}
		p.sync(ctx)
	}
	if ev.RightClicked {
		if !selected {
			ctx.Select(rw.id)
			p.sync(ctx)
		}
		p.menuNode = rw.id
		widgets.OpenContextMenu(ui, p.menuID)
	}
	if ui.Active() == id {
		ctx.DnD.Source(ui, id, p.payload(ctx, d))
	}
	p.dropRow(ctx, d, rw, rr)
}

func (p *Panel) focus(ctx *context.Editor, id ids.NodeID) {
	ctx.Select(id)
	p.sync(ctx)
	p.pendingSingle = ids.NodeID{}
	if ctx.Settings.Tool == sceneview.ToolSelect {
		ctx.Settings.Tool = sceneview.ToolMove
	}
	ctx.Commands.Run(command.SceneFrame)
}

func (p *Panel) press(ctx *context.Editor, i int) {
	rw := p.rows[i]
	mods := ctx.UI.Mods()
	switch {
	case mods.Primary():
		ctx.ToggleSelection(rw.id)
		p.anchor = rw.id
	case mods.Has(input.ModShift) && p.indexOf(p.anchor) >= 0:
		a := p.indexOf(p.anchor)
		lo, hi := min(a, i), max(a, i)
		span := make([]ids.NodeID, 0, hi-lo+1)
		for k := lo; k <= hi; k++ {
			span = append(span, p.rows[k].id)
		}
		ctx.Select(span...)
	default:
		if ctx.IsSelected(rw.id) && ctx.Selection.Len() > 1 {
			p.pendingSingle = rw.id
		} else {
			ctx.Select(rw.id)
		}
		p.anchor = rw.id
	}
	p.sync(ctx)
}

func (p *Panel) payload(ctx *context.Editor, d *document.SceneDocument) dnd.Payload {
	nodes := append([]ids.NodeID(nil), ctx.Selected()...)
	label := strconv.Itoa(len(nodes)) + " nodes"
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
		return context.AssetMatches("texture", ap.Kind) || context.AssetMatches("mesh", ap.Kind)
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

func (p *Panel) dropRow(ctx *context.Editor, d *document.SceneDocument, rw row, rr mathx.Rect) {
	ui := ctx.UI
	dn := ctx.DnD
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
		p.drop(ctx, d, got, rw.id, zone)
	}
}

func (p *Panel) emptyArea(ctx *context.Editor, d *document.SceneDocument, r mathx.Rect) {
	ui := ctx.UI
	id := ui.ID("empty")
	if ui.Pressed(id, r) && !ui.Primary() && !ui.Mods().Has(input.ModShift) {
		ctx.Select()
		p.sync(ctx)
	}
	if ui.RightClicked(id, r) {
		ctx.Select()
		p.sync(ctx)
		p.menuNode = ids.NodeID{}
		widgets.OpenContextMenu(ui, p.menuID)
	}
	if !ctx.DnD.Hovering(ui, r) {
		return
	}
	s := d.Scene()
	rootID := s.ID(s.Root())
	pl, _ := ctx.DnD.Payload()
	if !p.accepts(d, pl, rootID) {
		ctx.DnD.Target(ui, r, "reject")
		return
	}
	if got, ok := ctx.DnD.Target(ui, r, context.KindNodes, context.KindAsset); ok {
		p.drop(ctx, d, got, rootID, zoneInto)
	}
}

func (p *Panel) drop(ctx *context.Editor, d *document.SceneDocument, pl dnd.Payload, target ids.NodeID, zone int) {
	s := d.Scene()
	switch pl.Kind {
	case context.KindNodes:
		np, _ := pl.Data.(context.NodesPayload)
		p.move(ctx, d, np.Nodes, target, zone)
	case context.KindAsset:
		ap, _ := pl.Data.(context.AssetPayload)
		if ap.Kind == asset.ArtScript {
			assets.AttachScriptAsset(ctx, []ids.NodeID{target}, ap.ID)
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
		t, ok := templates.ForAsset(ctx, ap.ID, ap.Kind, path.Base(ap.Name))
		if !ok {
			return
		}
		if _, err := templates.Instantiate(ctx, t, parent, index, nil); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		}
	}
}

func (p *Panel) move(ctx *context.Editor, d *document.SceneDocument, nodes []ids.NodeID, target ids.NodeID, zone int) {
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
		cmd := &commands.Reparent{Node: s.ID(n), KeepWorld: true}
		switch zone {
		case zoneInto:
			cmd.NewParent = target
			cmd.NewIndex = -1
		case zoneBefore:
			parent := s.Parent(th)
			idx := commands.ChildIndex(s, th)
			if s.Parent(n) == parent && commands.ChildIndex(s, n) < idx {
				idx--
			}
			cmd.NewParent = s.ID(parent)
			cmd.NewIndex = idx
		case zoneAfter:
			parent := s.Parent(anchor)
			idx := commands.ChildIndex(s, anchor) + 1
			if s.Parent(n) == parent && commands.ChildIndex(s, n) < idx {
				idx--
			}
			cmd.NewParent = s.ID(parent)
			cmd.NewIndex = idx
		}
		if err := tx.Exec(cmd); err != nil {
			tx.Abort()
			ctx.Notify(context.LevelError, err.Error())
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

func (p *Panel) drawRename(ctx *context.Editor, d *document.SceneDocument, rw row, rr mathx.Rect, selected bool) {
	ui := ctx.UI
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
			ctx.Notify(context.LevelError, err.Error())
		}
	}
	if ui.Focus() != fid {
		p.renaming = ids.NodeID{}
	}
}

func (p *Panel) createItems(ctx *context.Editor, d *document.SceneDocument, parent ids.NodeID) {
	templates.Menu(ctx, p.templates, parent, templates.Place{}, func([]ids.NodeID) {
		if !parent.IsZero() {
			d.View.Expanded[parent] = true
		}
	})
}
