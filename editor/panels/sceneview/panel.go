package sceneview

import (
	"math"
	"path"
	"strconv"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/assetfiles"
	"aego/editor/core/clipboard"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/gizmo"
	"aego/editor/core/services"
	"aego/editor/core/shape"
	"aego/editor/core/templates"
	"aego/editor/core/vectors"
	"aego/editor/core/workspace"
	"aego/engine"
	"aego/gpu"
	"aego/platform/input"
	"aego/project"
	"aego/render/debug"
	"aego/render/rt"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/ui/core"
	"aego/ui/widgets"
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

type Deps struct {
	Capture   func(tex gpu.TextureHandle, w, h int, name string)
	Viewport  *services.Viewport
	Resolver  asset.Resolver
	Docs      services.Documents
	Selection services.Selection
	Tools     *context.SceneSettings
	Commands  *command.Registry
	Notices   services.Notices
	Scenes    services.SceneOps
	Files     *assetfiles.Files
	Pipeline  *pipeline.Pipeline
	Project   *project.Project
	Templates *templates.Registry
}

type Panel struct {
	d          Deps
	iso        isolation
	isoVer     uint64
	captureReq bool
	f          *services.Frame
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
	icon       string
	barRect    mathx.Rect
	menuID     core.ID
	menuAt     mathx.Vec2
	menuAt3    mathx.Vec3
	panRight   bool
	panMoved   bool
	panStart   mathx.Vec2
	rightAt    mathx.Vec2
	rightArmed bool
}

func New(g *gizmo.Registry, d Deps) *Panel {
	return &Panel{d: d, worlds: map[document.ID]*world{}, gizmos: g, c3: NewController3()}
}

func (p *Panel) ui() *core.Context {
	if p.f != nil {
		return p.f.UI
	}
	return p.d.Viewport.UI
}

func (p *Panel) shapes() shape.Set {
	return shape.Set{Reg: p.d.Viewport.Shapes, Res: p.d.Resolver, Hide: p.hidden}
}

func (p *Panel) docs() *document.Manager {
	return p.d.Docs.Documents()
}

func (p *Panel) item(id string, label string) {
	command.MenuItem(p.ui(), p.d.Commands, id, label)
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

func (p *Panel) updateTitle(d *document.SceneDocument) {
	k := titleKey{path: d.Path, play: p.docs().InPlay()}
	if real := p.docs().Get(d.ID); real != nil {
		k.dirty = real.Dirty()
	}
	if k == p.titleKey && p.title != "" {
		return
	}
	p.titleKey = k
	t := strings.TrimSuffix(path.Base(d.Path), path.Ext(d.Path))
	if k.play {
		t += " [Play]"
	}
	p.title = t
	p.icon = "file.scene"
	if d.IsPrefab() {
		p.icon = "file.prefab"
	}
}

func (p *Panel) Icon() string {
	if p.icon == "" {
		return "file.scene"
	}
	return p.icon
}

func (p *Panel) Dirty() bool {
	return p.titleKey.dirty
}

func (p *Panel) ZoomLabel() string {
	d := p.d.Docs.Doc()
	if d == nil {
		return "—"
	}
	z := d.View.Camera.Zoom
	if z <= 0 {
		z = 64
	}
	return strconv.Itoa(int(z/64*100+0.5)) + "%"
}

func (p *Panel) SetZoom(z float32) {
	if d := p.d.Docs.Doc(); d != nil {
		d.View.Camera.Zoom = z
	}
}

func (p *Panel) Scope() string {
	return command.ScopeScene
}

func (p *Panel) ensure() {
	if p.pool != nil {
		return
	}
	p.hook = p.render
	p.pool = rt.New(p.d.Viewport.GPU, gpu.TexSRGBA8, "editor.sceneview")
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

func (p *Panel) prune() {
	for id := range p.worlds {
		if p.docs().Get(id) == nil {
			delete(p.worlds, id)
		}
	}
}

func (p *Panel) worldFor(d *document.SceneDocument) *world {
	wd := p.worlds[d.ID]
	if wd == nil {
		wd = &world{w: engine.NewWorld("edit"), runner: frame.New()}
		wd.w.SetService(frame.ServiceID, wd.runner)
		p.worlds[d.ID] = wd
	}
	if wd.scene != d.Scene() {
		wd.scene = d.Scene()
		wd.runner.Bind(wd.scene, p.d.Viewport.Renderer, p.d.Resolver)
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
		Clear:         p.ui().Theme.Background,
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
	case context.ToolRotate:
		return gizmo.ModeRotate
	case context.ToolScale:
		return gizmo.ModeScale
	}
	return gizmo.ModeMove
}

func (p *Panel) gizmoActive() bool {
	return p.hasPivot && p.d.Tools.Tool != context.ToolSelect
}

func (p *Panel) snap() gizmo.Snap {
	set := *p.d.Tools
	return gizmo.Snap{
		Enabled: set.Snap != p.ui().Primary(),
		Move:    set.SnapMove,
		Rotate:  set.SnapRotate,
		Scale:   set.SnapScale,
	}
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	p.f = f
	ui := f.UI
	p.ensure()
	p.prune()
	p.menuID = ui.ID("ctx")
	d := p.d.Docs.Doc()
	if d == nil {
		p.title = ""
		widgets.LabelDim(ui, "No scene open")
		return
	}
	p.updateTitle(d)
	p.syncMode(d)
	p.updateIsolation(d)
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
	p.cur = p.worldFor(d)
	p.gizmo.Mode = p.gizmoMode(p.d.Tools.Tool)
	p.refreshView(d)
	ui.List().Texture(target.Region(), r, mathx.Color{R: 1, G: 1, B: 1, A: 1})
	p.toolbar(d, r)
	p.prefabBar(d, r)
	p.isolationBar(d, r)
	if p.viewMode == Mode3D {
		p.c3.SetViewport(float32(target.W), float32(target.H))
		p.input3(d, r)
		p.drop3(r)
		d.View.Orbit = p.c3.Orbit()
	} else {
		p.updatePivot(d)
		p.input(d, r)
		p.refreshView(d)
		p.updatePivot(d)
		p.drop(d, r)
	}
	p.contextMenu(d)
	p.d.Viewport.OnRender(p.hook)
}

func (p *Panel) updatePivot(d *document.SceneDocument) {
	p.hasPivot = false
	it, ok := p.d.Selection.SelectionState().Active()
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

func (p *Panel) hitGizmo(d *document.SceneDocument) (gizmo.Gizmo, gizmo.Hit, bool) {
	if !p.hasPivot {
		return nil, gizmo.Hit{}, false
	}
	s := d.Scene()
	hc := p.hitCtx(p.ui(), s)
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
	if p.gizmoActive() {
		if h, ok := p.gizmo.HitTest(hc); ok {
			return &p.gizmo, h, true
		}
	}
	return nil, gizmo.Hit{}, false
}

func (p *Panel) input(d *document.SceneDocument, r mathx.Rect) {
	ui := p.ui()
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
				p.openMenu(d)
			}
		}
	case modeGizmo:
		p.dragGizmo(d)
	case modeBox:
		p.dragBox(d)
	}
	if over {
		if w := ui.ZoomY(); w != 0 {
			before := p.worldAt(ui.Mouse())
			cam.Zoom = mathx.Clamp(cam.Zoom*float32(math.Pow(1.15, float64(w))), 1, 4096)
			p.refreshView(d)
			after := p.worldAt(ui.Mouse())
			cam.Position = cam.Position.Add(before.Sub(after))
			p.refreshView(d)
		}
	}
	if p.mode == modeIdle && !onBar && ui.Pressed(id, r) {
		p.press(d, id)
	}
	p.hot, p.hotGizmo = gizmo.PartNone, nil
	if p.mode == modeIdle && over {
		if g, h, ok := p.hitGizmo(d); ok {
			p.hot, p.hotGizmo = h.Part, g
		}
	}
}

func (p *Panel) openMenu(d *document.SceneDocument) {
	s := d.Scene()
	p.menuAt = p.worldAt(p.ui().Mouse())
	if h, ok := Pick(s, p.shapes(), p.menuAt); ok {
		if id := s.ID(h); !p.d.Selection.IsSelected(id) {
			p.d.Selection.Select(id)
		}
	}
	widgets.OpenContextMenu(p.ui(), p.menuID)
}

func (p *Panel) createAt(at mathx.Vec2) {
	if p.d.Templates == nil {
		return
	}
	if p.d.Tools.Snap {
		at = snapVec(at, p.d.Tools.SnapMove)
	}
	p.d.Scenes.CreateMenuAt(p.d.Templates, templates.Place{At2: &at})
}

func (p *Panel) contextMenu(d *document.SceneDocument) {
	ui := p.ui()
	if !widgets.ContextMenu(ui, p.menuID) {
		return
	}
	if widgets.SubMenuIcon(ui, "Create", "ui.plus", 200) {
		if p.viewMode == Mode3D {
			p.createAt3(p.menuAt3)
		} else {
			p.createAt(p.menuAt)
		}
		widgets.EndSubMenu(ui)
	}
	widgets.Separator(ui)
	p.item(command.EditDuplicate, "Duplicate")
	p.item(command.EditCopy, "Copy")
	p.item(command.EditCut, "Cut")
	p.item(command.EditPaste, "Paste")
	widgets.Separator(ui)
	p.item(command.EditDelete, "Delete")
	widgets.Separator(ui)
	p.item(command.SceneFrame, "Frame Selection")
	widgets.EndContextMenu(ui)
}

func (p *Panel) toolbar(d *document.SceneDocument, r mathx.Rect) {
	ui := p.ui()
	th := &ui.Theme
	ui.PushClip(r)
	defer ui.PopClip()
	three := p.viewMode == Mode3D
	modeLabel := "2D"
	if three {
		modeLabel = "3D"
	}
	zoom := p.ZoomLabel()
	h := th.BarHeight
	bw := h - 6 + 2
	w := 6 + 8*bw + 18 + ui.TextSize(modeLabel).X + 16 + 6
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
	ui.Shadow(bar, th.Radius.Popup)
	ui.RoundRect(bar, th.Radius.Popup, th.BarBg)
	tb := widgets.BeginToolbarIn(ui, bar)
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
		if tb.Button(ui.IDInt(300+i), t.icon, t.tip, p.d.Tools.Tool == uint8(i), true) {
			p.d.Tools.Tool = uint8(i)
		}
	}
	tb.Separator()
	if tb.Button(ui.ID("grid"), "view.grid", "Grid", p.d.Tools.Grid, true) {
		p.d.Tools.Grid = !p.d.Tools.Grid
	}
	if tb.Button(ui.ID("snap"), "view.snap", "Snap (hold Ctrl to invert)", p.d.Tools.Snap, true) {
		p.d.Tools.Snap = !p.d.Tools.Snap
	}
	if tb.Button(ui.ID("vectors"), "view.vectors", "Vectors (V)", p.d.Viewport.Overlay.On(vectors.CommandToggle), true) {
		p.d.Viewport.Overlay.Toggle(vectors.CommandToggle)
	}
	if clicked, _ := tb.TextButton(ui.ID("mode"), modeLabel, "Switch between the 2D and 3D view"); clicked {
		p.toggleMode(d)
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
		if widgets.MenuItemIcon(ui, "Fit", "", "view.zoom", true, false) {
			p.FrameAll()
		}
		for _, z := range [...]struct {
			label string
			zoom  float32
		}{{"50%", 32}, {"100%", 64}, {"200%", 128}, {"400%", 256}} {
			if widgets.MenuItem(ui, z.label, "", true, false) {
				p.SetZoom(z.zoom)
			}
		}
		ui.EndPopup()
	}
	if ui.BeginPopup(cid, 200) {
		if three {
			p.createAt3(p.c3.Cam.Target)
		} else {
			p.createAt(d.View.Camera.Position)
		}
		ui.EndPopup()
	}
}

func (p *Panel) press(d *document.SceneDocument, id core.ID) {
	ui := p.ui()
	s := d.Scene()
	mouse := p.worldAt(ui.Mouse())
	if g, h, ok := p.hitGizmo(d); ok {
		p.beginGizmo(d, id, g, h, mouse)
		return
	}
	mods := ui.Mods()
	if h, ok := Pick(s, p.shapes(), mouse); ok {
		nid := s.ID(h)
		switch {
		case mods.Primary():
			p.d.Selection.ToggleSelection(nid)
		case mods.Has(input.ModShift):
			p.d.Selection.AddToSelection(nid)
		default:
			if !p.d.Selection.IsSelected(nid) {
				p.d.Selection.Select(nid)
			}
		}
		if ui.DoubleClicked(id) && p.d.Tools.Tool == context.ToolSelect {
			p.d.Tools.Tool = context.ToolMove
			p.gizmo.Mode = gizmo.ModeMove
		}
		if p.d.Tools.Tool == context.ToolMove && p.d.Selection.IsSelected(nid) && !mods.Primary() {
			p.updatePivot(d)
			p.beginGizmo(d, id, &p.gizmo, gizmo.Hit{Part: gizmo.PartXY}, mouse)
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

func (p *Panel) beginGizmo(d *document.SceneDocument, id core.ID, g gizmo.Gizmo, h gizmo.Hit, mouse mathx.Vec2) {
	if reason, locked := d.History.Locked(); locked {
		p.d.Notices.Notify(context.LevelWarn, reason)
		return
	}
	s := d.Scene()
	tops := clipboard.TopLevel(s, p.d.Selection.Selected())
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
	p.edit = gizmo.NewEdit(s, nodes, p.activeNode, mouse, p.snap())
	p.edit.Tx = d.History.Begin(label)
	p.active = g
	g.Begin(p.edit, h)
	p.mode = modeGizmo
	p.ui().CaptureMouse(id)
}

func (p *Panel) dragGizmo(d *document.SceneDocument) {
	ui := p.ui()
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
			p.d.Notices.Notify(context.LevelError, e.Err.Error())
		} else {
			e.Tx.Commit()
		}
		finish()
		return
	}
	e.Mouse = p.worldAt(ui.Mouse())
	e.Snap = p.snap()
	g.Update(e)
}

func (p *Panel) dragBox(d *document.SceneDocument) {
	ui := p.ui()
	p.boxEnd = ui.Mouse()
	if ui.MouseDown(input.MouseLeft) {
		return
	}
	p.mode = modeIdle
	ui.ReleaseMouse()
	if p.boxEnd.Sub(p.boxStart).Len() < 4 {
		if !p.boxAdd {
			p.d.Selection.Select()
		}
		return
	}
	a, b := p.worldAt(p.boxStart), p.worldAt(p.boxEnd)
	box := mathx.AABB2{
		Min: mathx.Vec2{X: min(a.X, b.X), Y: min(a.Y, b.Y)},
		Max: mathx.Vec2{X: max(a.X, b.X), Y: max(a.Y, b.Y)},
	}
	found := BoxSelect(d.Scene(), p.shapes(), box)
	if p.boxAdd {
		p.d.Selection.AddToSelection(found...)
	} else {
		p.d.Selection.Select(found...)
	}
}

func (p *Panel) frame(nodes []ids.NodeID) {
	d := p.d.Docs.Doc()
	if d == nil || p.rect.W <= 0 || p.rect.H <= 0 {
		return
	}
	s := d.Scene()
	s.UpdateTransforms()
	sh := p.shapes()
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

func (p *Panel) drop(d *document.SceneDocument, r mathx.Rect) {
	ui := p.ui()
	dn := p.f.DnD
	if !dn.Hovering(ui, r) {
		return
	}
	pl, _ := dn.Payload()
	at := p.worldAt(dn.Pos(ui))
	if p.d.Tools.Snap {
		at = snapVec(at, p.d.Tools.SnapMove)
	}
	sp := scene.Space2D
	switch pl.Kind {
	case context.KindAsset:
		ap, ok := pl.Data.(context.AssetPayload)
		if !ok || p.d.Templates == nil || !p.d.Templates.Accepts(ap.Kind, &sp) {
			dn.Target(ui, r, "reject")
			return
		}
		if got, ok := dn.Target(ui, r, context.KindAsset); ok {
			p.spawn(got.Data.(context.AssetPayload), at)
		}
	case context.KindFiles:
		got, ok := dn.Target(ui, r, context.KindFiles)
		if !ok || p.d.Pipeline == nil || p.d.Templates == nil {
			return
		}
		files, _ := got.Data.([]string)
		created, err := p.d.Files.ImportExternal(files, "assets")
		if err != nil {
			p.d.Notices.Notify(context.LevelError, err.Error())
			return
		}
		for i, id := range created {
			if e, ok := p.d.Pipeline.Entry(id); ok && p.d.Templates.Accepts(e.Kind, &sp) {
				p.spawn(context.AssetPayload{ID: id, Kind: e.Kind, Name: e.Name}, at.Add(mathx.Vec2{X: float32(i) * 0.5}))
			}
		}
	default:
		dn.Target(ui, r, "reject")
	}
}

func (p *Panel) spawn(ap context.AssetPayload, at mathx.Vec2) {
	sp := scene.Space2D
	t, ok := p.d.Templates.ForAsset(ap.Kind, &sp, ap.ID, path.Base(ap.Name))
	if !ok {
		return
	}
	if err := p.d.Scenes.InstantiateAt(t, &at); err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
	}
}

func (p *Panel) Frame() {
	if p.viewMode == Mode3D {
		if p.c3 != nil && p.cur != nil {
			p.c3.FrameSelection(p.host3())
		}
		return
	}
	p.frame(p.d.Selection.Selected())
}

func (p *Panel) FrameAll() {
	if p.viewMode == Mode3D {
		if p.c3 == nil || p.cur == nil {
			return
		}
		if b, ok := SceneBounds(p.cur.scene, p.shapes()); ok {
			p.c3.Cam.Frame(b)
		}
		return
	}
	p.frame(nil)
}

func (p *Panel) drawSelection(s *scene.Scene, dbg *debug.Draw, th *core.Theme) {
	sh := p.shapes()
	for _, id := range p.d.Selection.Selected() {
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
