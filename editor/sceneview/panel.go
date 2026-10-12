package sceneview

import (
	"math"
	"path"
	"strconv"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/clipboard"
	"aego/editor/command"
	"aego/editor/context"
	"aego/editor/document"
	"aego/editor/gizmo"
	"aego/editor/templates"
	"aego/editor/workspace"
	"aego/engine"
	"aego/gpu"
	"aego/platform/input"
	"aego/render/debug"
	"aego/render/rt"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	ToolSelect uint8 = iota
	ToolMove
	ToolRotate
	ToolScale
)

type mode uint8

const (
	modeIdle mode = iota
	modePan
	modeGizmo
	modeBox
)

type world struct {
	w      *engine.World
	runner *frame.Runner
	scene  *scene.Scene
}

type Panel struct {
	ctx        *context.Editor
	hook       func()
	pool       *rt.Pool
	target     *rt.Target
	worlds     map[document.ID]*world
	cur        *world
	rect       mathx.Rect
	scale      float32
	view       view.View
	space      gizmo.Space
	mode       mode
	viewMode   Mode
	c3         *Controller3
	c3Doc      document.ID
	gizmo      gizmo.Transform
	gizmos     *gizmo.Registry
	active     gizmo.Gizmo
	edit       *gizmo.EditCtx
	hot        gizmo.Part
	hotGizmo   gizmo.Gizmo
	boxStart   mathx.Vec2
	boxEnd     mathx.Vec2
	boxAdd     bool
	pivot      mathx.Vec2
	rotation   float32
	activeNode ids.NodeID
	hasPivot   bool
	title      string
	titleKey   titleKey
	barRect    mathx.Rect
	Templates  *templates.Registry
	menuID     core.ID
	menuAt     mathx.Vec2
	menuAt3    mathx.Vec3
	panRight   bool
	panMoved   bool
	panStart   mathx.Vec2
	rightAt    mathx.Vec2
	rightArmed bool
}

func New(g *gizmo.Registry) *Panel {
	return &Panel{worlds: map[document.ID]*world{}, gizmos: g}
}

func (p *Panel) ID() string {
	return workspace.Scene
}

type titleKey struct {
	path  string
	dirty bool
	play  bool
}

func (p *Panel) Title() string {
	if p.title == "" {
		return "Scene"
	}
	return p.title
}

func (p *Panel) updateTitle(ctx *context.Editor, d *document.SceneDocument) {
	k := titleKey{path: d.Path, play: ctx.Docs.InPlay()}
	if real := ctx.Docs.Get(d.ID); real != nil {
		k.dirty = real.Dirty()
	}
	if k == p.titleKey && p.title != "" {
		return
	}
	p.titleKey = k
	t := path.Base(d.Path)
	if k.dirty {
		t += " *"
	}
	if k.play {
		t += " [Play]"
	}
	p.title = t
}

func (p *Panel) ZoomLabel(ctx *context.Editor) string {
	d := ctx.Doc()
	if d == nil {
		return "—"
	}
	z := d.View.Camera.Zoom
	if z <= 0 {
		z = 64
	}
	return strconv.Itoa(int(z/64*100+0.5)) + "%"
}

func (p *Panel) SetZoom(ctx *context.Editor, z float32) {
	if d := ctx.Doc(); d != nil {
		d.View.Camera.Zoom = z
	}
}

func (p *Panel) Scope() string {
	return command.ScopeScene
}

func resolver(ctx *context.Editor) asset.Resolver {
	if ctx.Loader != nil {
		return ctx.Loader
	}
	return nil
}

func (p *Panel) ensure(ctx *context.Editor) {
	if p.ctx != nil {
		return
	}
	p.ctx = ctx
	p.hook = p.render
	p.pool = rt.New(ctx.Engine.GPU(), gpu.TexSRGBA8, "editor.sceneview")
	p.c3 = NewController3()
}

func (p *Panel) Close() {
	if p.pool != nil {
		if p.target != nil {
			p.pool.Release(p.target)
			p.target = nil
		}
		p.pool.Close()
	}
	p.worlds = map[document.ID]*world{}
}

func (p *Panel) prune(ctx *context.Editor) {
	for id := range p.worlds {
		if ctx.Docs.Get(id) == nil {
			delete(p.worlds, id)
		}
	}
}

func (p *Panel) worldFor(ctx *context.Editor, d *document.SceneDocument) *world {
	wd := p.worlds[d.ID]
	if wd == nil {
		wd = &world{w: engine.NewWorld("edit"), runner: frame.New()}
		wd.w.SetService(frame.ServiceID, wd.runner)
		p.worlds[d.ID] = wd
	}
	if wd.scene != d.Scene() {
		wd.scene = d.Scene()
		wd.runner.Bind(wd.scene, ctx.Renderer, resolver(ctx))
		wd.w.SetService(scene.ServiceID, wd.scene)
	}
	return wd
}

func (p *Panel) refreshView(d *document.SceneDocument) {
	cam := d.View.Camera
	p.view = view.View{
		Camera:        view.Camera2D{Position: cam.Position, Zoom: cam.Zoom * p.scale},
		Viewport:      mathx.Rect{W: float32(p.target.W), H: float32(p.target.H)},
		Target:        p.target.Texture,
		TargetW:       p.target.BucketW,
		TargetH:       p.target.BucketH,
		PixelScale:    p.scale,
		PixelsPerUnit: 1,
		ZMax:          1,
		Clear:         p.ctx.UI.Theme.Background,
		DoClear:       true,
		Layers:        view.AllLayers,
		Label:         "sceneview",
	}
	p.space = gizmo.Space{View: p.view, Zoom: cam.Zoom, Scale: p.scale}
}

func (p *Panel) toPixels(m mathx.Vec2) mathx.Vec2 {
	return mathx.Vec2{X: (m.X - p.rect.X) * p.scale, Y: (m.Y - p.rect.Y) * p.scale}
}

func (p *Panel) worldAt(m mathx.Vec2) mathx.Vec2 {
	return p.view.ScreenToWorld(p.toPixels(m))
}

func (p *Panel) gizmoMode(tool uint8) gizmo.Mode {
	switch tool {
	case ToolRotate:
		return gizmo.ModeRotate
	case ToolScale:
		return gizmo.ModeScale
	}
	return gizmo.ModeMove
}

func (p *Panel) gizmoActive(ctx *context.Editor) bool {
	return p.hasPivot && ctx.Settings.Tool != ToolSelect
}

func (p *Panel) snap(ctx *context.Editor) gizmo.Snap {
	set := ctx.Settings
	return gizmo.Snap{
		Enabled: set.Snap != ctx.UI.Primary(),
		Move:    set.SnapMove,
		Rotate:  set.SnapRotate,
		Scale:   set.SnapScale,
	}
}

func (p *Panel) Draw(ctx *context.Editor, r mathx.Rect) {
	ui := ctx.UI
	p.ensure(ctx)
	p.prune(ctx)
	p.menuID = ctx.UI.ID("ctx")
	d := ctx.Doc()
	if d == nil {
		p.title = ""
		widgets.LabelDim(ui, "No scene open")
		return
	}
	p.updateTitle(ctx, d)
	p.syncMode(d)
	scale := ui.Scale()
	pw, ph := int(r.W*scale), int(r.H*scale)
	if pw < 8 || ph < 8 {
		return
	}
	target, err := p.pool.Resize(p.target, pw, ph, p.viewMode == Mode3D)
	if err != nil {
		ui.TextIn(r, err.Error(), ui.Theme.Error, core.AlignCenter)
		return
	}
	p.target = target
	p.rect, p.scale = r, scale
	if d.View.Camera.Zoom <= 0 {
		d.View.Camera.Zoom = 64
	}
	p.cur = p.worldFor(ctx, d)
	p.gizmo.Mode = p.gizmoMode(ctx.Settings.Tool)
	p.refreshView(d)
	ui.List().Texture(target.Region(), r, mathx.Color{R: 1, G: 1, B: 1, A: 1})
	p.toolbar(ctx, d, r)
	if p.viewMode == Mode3D {
		p.c3.SetViewport(float32(target.W), float32(target.H))
		p.input3(ctx, d, r)
		p.drop3(ctx, r)
		d.View.Orbit = p.c3.Orbit()
	} else {
		p.updatePivot(ctx, d)
		p.input(ctx, d, r)
		p.refreshView(d)
		p.updatePivot(ctx, d)
		p.drop(ctx, d, r)
	}
	p.contextMenu(ctx, d)
	ctx.OnRender(p.hook)
}

func (p *Panel) updatePivot(ctx *context.Editor, d *document.SceneDocument) {
	p.hasPivot = false
	it, ok := ctx.Selection.Active()
	if !ok || it.Doc != d.ID {
		return
	}
	s := d.Scene()
	h := s.Handle(it.Node)
	if h.IsZero() || !s.IsAlive(h) {
		return
	}
	m := s.World(h)
	p.pivot = mathx.Vec2{X: m[6], Y: m[7]}
	p.rotation = mathx.Atan2(m[1], m[0])
	p.activeNode = it.Node
	p.hasPivot = true
}

func (p *Panel) hitCtx(ui *core.Context, s *scene.Scene) *gizmo.HitCtx {
	return &gizmo.HitCtx{
		Space:    p.space,
		Scene:    s,
		Node:     p.activeNode,
		Pivot:    p.pivot,
		Rotation: p.rotation,
		MousePx:  p.toPixels(ui.Mouse()),
	}
}

func (p *Panel) hitGizmo(ctx *context.Editor, d *document.SceneDocument) (gizmo.Gizmo, gizmo.Hit, bool) {
	if !p.hasPivot {
		return nil, gizmo.Hit{}, false
	}
	s := d.Scene()
	hc := p.hitCtx(ctx.UI, s)
	if p.gizmos != nil {
		var found gizmo.Gizmo
		var hit gizmo.Hit
		p.gizmos.Each(s, s.Handle(p.activeNode), func(g gizmo.Gizmo) {
			if found != nil {
				return
			}
			if h, ok := g.HitTest(hc); ok {
				found, hit = g, h
			}
		})
		if found != nil {
			return found, hit, true
		}
	}
	if p.gizmoActive(ctx) {
		if h, ok := p.gizmo.HitTest(hc); ok {
			return &p.gizmo, h, true
		}
	}
	return nil, gizmo.Hit{}, false
}

func (p *Panel) input(ctx *context.Editor, d *document.SceneDocument, r mathx.Rect) {
	ui := ctx.UI
	id := ui.ID("view")
	cam := &d.View.Camera
	over := ui.MouseOver(id, r)
	onBar := p.barRect.Contains(ui.Mouse())
	if over && !onBar && p.mode == modeIdle && (ui.MousePressed(input.MouseMiddle) || ui.MousePressed(input.MouseRight)) {
		p.mode = modePan
		p.panRight = ui.MousePressed(input.MouseRight) && !ui.MousePressed(input.MouseMiddle)
		p.panStart = ui.Mouse()
		p.panMoved = false
		ui.CaptureMouse(id)
	}
	switch p.mode {
	case modePan:
		if ui.MouseDown(input.MouseMiddle) || ui.MouseDown(input.MouseRight) {
			if ui.Mouse().Sub(p.panStart).Len() > 4 {
				p.panMoved = true
			}
			cam.Position = cam.Position.Sub(ui.MouseDelta().Scale(1 / cam.Zoom))
			if p.panMoved {
				ui.SetCursorShape(core.CursorMove)
			}
		} else {
			p.mode = modeIdle
			ui.ReleaseMouse()
			if p.panRight && !p.panMoved {
				p.openMenu(ctx, d)
			}
		}
	case modeGizmo:
		p.dragGizmo(ctx, d)
	case modeBox:
		p.dragBox(ctx, d)
	}
	if over {
		if w := ui.WheelY(); w != 0 {
			before := p.worldAt(ui.Mouse())
			cam.Zoom = mathx.Clamp(cam.Zoom*float32(math.Pow(1.15, float64(w))), 1, 4096)
			p.refreshView(d)
			after := p.worldAt(ui.Mouse())
			cam.Position = cam.Position.Add(before.Sub(after))
			p.refreshView(d)
		}
	}
	if p.mode == modeIdle && !onBar && ui.Pressed(id, r) {
		p.press(ctx, d, id)
	}
	p.hot, p.hotGizmo = gizmo.PartNone, nil
	if p.mode == modeIdle && over {
		if g, h, ok := p.hitGizmo(ctx, d); ok {
			p.hot, p.hotGizmo = h.Part, g
		}
	}
}

func (p *Panel) openMenu(ctx *context.Editor, d *document.SceneDocument) {
	s := d.Scene()
	p.menuAt = p.worldAt(ctx.UI.Mouse())
	if h, ok := Pick(s, shapes(ctx), p.menuAt); ok {
		if id := s.ID(h); !ctx.IsSelected(id) {
			ctx.Select(id)
		}
	}
	widgets.OpenContextMenu(ctx.UI, p.menuID)
}

func (p *Panel) createAt(ctx *context.Editor, at mathx.Vec2) {
	if p.Templates == nil {
		return
	}
	if ctx.Settings.Snap {
		at = snapVec(at, ctx.Settings.SnapMove)
	}
	templates.Menu(ctx, p.Templates, ids.NodeID{}, templates.Place{At2: &at}, nil)
}

func (p *Panel) contextMenu(ctx *context.Editor, d *document.SceneDocument) {
	ui := ctx.UI
	if !widgets.ContextMenu(ui, p.menuID) {
		return
	}
	if widgets.SubMenu(ui, "Create", 200) {
		if p.viewMode == Mode3D {
			p.createAt3(ctx, p.menuAt3)
		} else {
			p.createAt(ctx, p.menuAt)
		}
		widgets.EndSubMenu(ui)
	}
	widgets.Separator(ui)
	ctx.CommandItem(command.EditDuplicate, "Duplicate")
	ctx.CommandItem(command.EditCopy, "Copy")
	ctx.CommandItem(command.EditCut, "Cut")
	ctx.CommandItem(command.EditPaste, "Paste")
	widgets.Separator(ui)
	ctx.CommandItem(command.EditDelete, "Delete")
	widgets.Separator(ui)
	ctx.CommandItem(command.SceneFrame, "Frame Selection")
	widgets.EndContextMenu(ui)
}

func (p *Panel) toolbar(ctx *context.Editor, d *document.SceneDocument, r mathx.Rect) {
	ui := ctx.UI
	th := &ui.Theme
	ui.PushClip(r)
	defer ui.PopClip()
	three := p.viewMode == Mode3D
	modeLabel := "2D"
	if three {
		modeLabel = "3D"
	}
	zoom := p.ZoomLabel(ctx)
	h := th.BarHeight
	bw := h - 6 + 2
	w := 6 + 7*bw + 18 + ui.TextSize(modeLabel).X + 16 + 6
	projLabel := "Persp"
	if p.c3.Cam.Ortho {
		projLabel = "Ortho"
	}
	if three {
		w += ui.TextSize(projLabel).X + 16
	} else {
		w += ui.TextSize(zoom).X + 16
	}
	bar := mathx.Rect{X: r.X + 6, Y: r.Y + 6, W: min(w, r.W-12), H: h}
	p.barRect = bar
	tb := widgets.BeginToolbar(ui, bar)
	ui.Outline(bar, th.BarBorder)
	tools := [4]struct {
		icon string
		tip  string
	}{
		{"tool.select", "Select (Q)"},
		{"tool.move", "Move (W)"},
		{"tool.rotate", "Rotate (E)"},
		{"tool.scale", "Scale (R)"},
	}
	for i, t := range tools {
		if tb.Button(ui.IDInt(300+i), t.icon, t.tip, ctx.Settings.Tool == uint8(i), true) {
			ctx.Settings.Tool = uint8(i)
		}
	}
	tb.Separator()
	if tb.Button(ui.ID("grid"), "view.grid", "Grid", ctx.Settings.Grid, true) {
		ctx.Settings.Grid = !ctx.Settings.Grid
	}
	if tb.Button(ui.ID("snap"), "view.snap", "Snap (hold Ctrl to invert)", ctx.Settings.Snap, true) {
		ctx.Settings.Snap = !ctx.Settings.Snap
	}
	if clicked, _ := tb.TextButton(ui.ID("mode"), modeLabel, "Switch between the 2D and 3D view"); clicked {
		p.toggleMode(ctx, d)
	}
	if three {
		if clicked, _ := tb.TextButton(ui.ID("proj"), projLabel, "Switch between the perspective and orthographic 3D view"); clicked {
			p.ToggleOrtho()
		}
	}
	zid := ui.ID("zoom")
	if !three {
		if clicked, zr := tb.TextButton(zid, zoom, "Zoom"); clicked {
			if ui.PopupOpen(zid) {
				ui.ClosePopup(zid)
			} else {
				ui.CloseAllPopups()
				ui.OpenPopup(zid, mathx.Vec2{X: zr.X, Y: zr.Y + zr.H})
			}
		}
	}
	tb.Separator()
	cid := ui.ID("create")
	createX := tb.X()
	if tb.Button(cid, "ui.plus", "Create", ui.PopupOpen(cid), true) {
		if ui.PopupOpen(cid) {
			ui.ClosePopup(cid)
		} else {
			ui.CloseAllPopups()
			ui.OpenPopup(cid, mathx.Vec2{X: createX, Y: bar.Y + bar.H})
		}
	}
	if ui.BeginPopup(zid, 120) {
		if widgets.MenuItem(ui, "Fit", "", true, false) {
			p.FrameAll(ctx)
		}
		for _, z := range [...]struct {
			label string
			zoom  float32
		}{{"50%", 32}, {"100%", 64}, {"200%", 128}, {"400%", 256}} {
			if widgets.MenuItem(ui, z.label, "", true, false) {
				p.SetZoom(ctx, z.zoom)
			}
		}
		ui.EndPopup()
	}
	if ui.BeginPopup(cid, 200) {
		if three {
			p.createAt3(ctx, p.c3.Cam.Target)
		} else {
			p.createAt(ctx, d.View.Camera.Position)
		}
		ui.EndPopup()
	}
}

func (p *Panel) press(ctx *context.Editor, d *document.SceneDocument, id core.ID) {
	ui := ctx.UI
	s := d.Scene()
	mouse := p.worldAt(ui.Mouse())
	if g, h, ok := p.hitGizmo(ctx, d); ok {
		p.beginGizmo(ctx, d, id, g, h, mouse)
		return
	}
	mods := ui.Mods()
	if h, ok := Pick(s, shapes(ctx), mouse); ok {
		nid := s.ID(h)
		switch {
		case mods.Primary():
			ctx.ToggleSelection(nid)
		case mods.Has(input.ModShift):
			ctx.AddToSelection(nid)
		default:
			if !ctx.IsSelected(nid) {
				ctx.Select(nid)
			}
		}
		if ui.DoubleClicked(id) && ctx.Settings.Tool == ToolSelect {
			ctx.Settings.Tool = ToolMove
			p.gizmo.Mode = gizmo.ModeMove
		}
		if ctx.Settings.Tool == ToolMove && ctx.IsSelected(nid) && !mods.Primary() {
			p.updatePivot(ctx, d)
			p.beginGizmo(ctx, d, id, &p.gizmo, gizmo.Hit{Part: gizmo.PartXY}, mouse)
		}
		return
	}
	p.mode = modeBox
	p.boxStart, p.boxEnd = ui.Mouse(), ui.Mouse()
	p.boxAdd = mods.Primary() || mods.Has(input.ModShift)
	ui.CaptureMouse(id)
}

func gizmoLabel(m gizmo.Mode) string {
	switch m {
	case gizmo.ModeRotate:
		return "Rotate"
	case gizmo.ModeScale:
		return "Scale"
	}
	return "Move"
}

func (p *Panel) beginGizmo(ctx *context.Editor, d *document.SceneDocument, id core.ID, g gizmo.Gizmo, h gizmo.Hit, mouse mathx.Vec2) {
	if reason, locked := d.History.Locked(); locked {
		ctx.Notify(context.LevelWarn, reason)
		return
	}
	s := d.Scene()
	tops := clipboard.TopLevel(s, ctx.Selected())
	if len(tops) == 0 {
		return
	}
	nodes := make([]ids.NodeID, len(tops))
	for i, t := range tops {
		nodes[i] = s.ID(t)
	}
	label := "Edit"
	if g == gizmo.Gizmo(&p.gizmo) {
		label = gizmoLabel(p.gizmo.Mode)
	}
	p.edit = gizmo.NewEdit(s, nodes, p.activeNode, mouse, p.snap(ctx))
	p.edit.Tx = d.History.Begin(label)
	p.active = g
	g.Begin(p.edit, h)
	p.mode = modeGizmo
	ctx.UI.CaptureMouse(id)
}

func (p *Panel) dragGizmo(ctx *context.Editor, d *document.SceneDocument) {
	ui := ctx.UI
	e, g := p.edit, p.active
	finish := func() {
		p.edit, p.active = nil, nil
		p.mode = modeIdle
		ui.ReleaseMouse()
	}
	if e == nil || g == nil {
		finish()
		return
	}
	if ui.KeyPressed(input.KeyEscape) {
		gizmo.Cancel(e)
		e.Tx.Abort()
		finish()
		return
	}
	if !ui.MouseDown(input.MouseLeft) {
		g.End(e)
		if e.Err != nil {
			e.Tx.Abort()
			ctx.Notify(context.LevelError, e.Err.Error())
		} else {
			e.Tx.Commit()
		}
		finish()
		return
	}
	e.Mouse = p.worldAt(ui.Mouse())
	e.Snap = p.snap(ctx)
	g.Update(e)
}

func (p *Panel) dragBox(ctx *context.Editor, d *document.SceneDocument) {
	ui := ctx.UI
	p.boxEnd = ui.Mouse()
	if ui.MouseDown(input.MouseLeft) {
		return
	}
	p.mode = modeIdle
	ui.ReleaseMouse()
	if p.boxEnd.Sub(p.boxStart).Len() < 4 {
		if !p.boxAdd {
			ctx.Select()
		}
		return
	}
	a, b := p.worldAt(p.boxStart), p.worldAt(p.boxEnd)
	box := mathx.AABB2{
		Min: mathx.Vec2{X: min(a.X, b.X), Y: min(a.Y, b.Y)},
		Max: mathx.Vec2{X: max(a.X, b.X), Y: max(a.Y, b.Y)},
	}
	found := BoxSelect(d.Scene(), shapes(ctx), box)
	if p.boxAdd {
		ctx.AddToSelection(found...)
	} else {
		ctx.Select(found...)
	}
}

func (p *Panel) frame(ctx *context.Editor, nodes []ids.NodeID) {
	d := ctx.Doc()
	if d == nil || p.rect.W <= 0 || p.rect.H <= 0 {
		return
	}
	s := d.Scene()
	s.UpdateTransforms()
	sh := shapes(ctx)
	if len(nodes) == 0 {
		root := s.Root()
		for _, h := range s.PreOrder() {
			if h != root && s.IsAlive(h) {
				nodes = append(nodes, s.ID(h))
			}
		}
	}
	var box mathx.AABB2
	has := false
	for _, id := range nodes {
		h := s.Handle(id)
		if h.IsZero() || !s.IsAlive(h) {
			continue
		}
		b, _ := Bounds(s, sh, h)
		if !has {
			box, has = b, true
			continue
		}
		box = box.Union(b)
	}
	if !has {
		return
	}
	cam := &d.View.Camera
	cam.Position = box.Center()
	size := box.Size()
	size.X, size.Y = max(size.X, 1), max(size.Y, 1)
	cam.Zoom = mathx.Clamp(min(p.rect.W/(size.X*1.4), p.rect.H/(size.Y*1.4)), 1, 4096)
}

func snapVec(v mathx.Vec2, step float32) mathx.Vec2 {
	if step <= 0 {
		return v
	}
	return mathx.Vec2{
		X: float32(math.Round(float64(v.X/step))) * step,
		Y: float32(math.Round(float64(v.Y/step))) * step,
	}
}

func (p *Panel) drop(ctx *context.Editor, d *document.SceneDocument, r mathx.Rect) {
	ui := ctx.UI
	dn := ctx.DnD
	if !dn.Hovering(ui, r) {
		return
	}
	pl, _ := dn.Payload()
	at := p.worldAt(dn.Pos(ui))
	if ctx.Settings.Snap {
		at = snapVec(at, ctx.Settings.SnapMove)
	}
	sp := scene.Space2D
	switch pl.Kind {
	case context.KindAsset:
		ap, ok := pl.Data.(context.AssetPayload)
		if !ok || p.Templates == nil || !p.Templates.Accepts(ap.Kind, &sp) {
			dn.Target(ui, r, "reject")
			return
		}
		if got, ok := dn.Target(ui, r, context.KindAsset); ok {
			p.spawn(ctx, got.Data.(context.AssetPayload), at)
		}
	case context.KindFiles:
		got, ok := dn.Target(ui, r, context.KindFiles)
		if !ok || ctx.Pipeline == nil || p.Templates == nil {
			return
		}
		files, _ := got.Data.([]string)
		created, batch, err := ctx.Pipeline.ImportExternal(files, "assets")
		ctx.ApplyBatch(batch)
		if err != nil {
			ctx.Notify(context.LevelError, err.Error())
			return
		}
		for i, id := range created {
			if e, ok := ctx.Pipeline.Entry(id); ok && p.Templates.Accepts(e.Kind, &sp) {
				p.spawn(ctx, context.AssetPayload{ID: id, Kind: e.Kind, Name: e.Name}, at.Add(mathx.Vec2{X: float32(i) * 0.5}))
			}
		}
	default:
		dn.Target(ui, r, "reject")
	}
}

func (p *Panel) spawn(ctx *context.Editor, ap context.AssetPayload, at mathx.Vec2) {
	sp := scene.Space2D
	t, ok := p.Templates.ForAsset(ap.Kind, &sp, ap.ID, path.Base(ap.Name))
	if !ok {
		return
	}
	if _, err := templates.Instantiate(ctx, t, ids.NodeID{}, -1, &at); err != nil {
		ctx.Notify(context.LevelError, err.Error())
	}
}

func (p *Panel) Frame(ctx *context.Editor) {
	if p.viewMode == Mode3D {
		if p.c3 != nil && p.cur != nil {
			p.c3.FrameSelection(p.host3())
		}
		return
	}
	p.frame(ctx, ctx.Selected())
}

func (p *Panel) FrameAll(ctx *context.Editor) {
	if p.viewMode == Mode3D {
		if p.c3 == nil || p.cur == nil {
			return
		}
		if b, ok := SceneBounds(p.cur.scene, shapes(ctx)); ok {
			p.c3.Cam.Frame(b)
		}
		return
	}
	p.frame(ctx, nil)
}

func (p *Panel) drawSelection(s *scene.Scene, dbg *debug.Draw, th *core.Theme) {
	ctx := p.ctx
	sh := shapes(ctx)
	for _, id := range ctx.Selected() {
		h := s.Handle(id)
		if h.IsZero() || !s.IsAlive(h) {
			continue
		}
		if q, ok := Quad(s, sh, h); ok {
			for i := 0; i < 4; i++ {
				dbg.Line(q[i], q[(i+1)%4], th.Accent)
			}
			continue
		}
		dbg.Cross(s.WorldPosition(h), p.space.PxToWorld(14), th.Accent)
	}
}
