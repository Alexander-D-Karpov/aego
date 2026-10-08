package sceneview

import (
	"aego/core/mathx"
	"aego/editor/gizmo"
	"aego/engine"
	"aego/render/debug"
	"aego/scene"
	"aego/scene/components"
	"aego/ui/core"
)

func (p *Panel) render() {
	if p.viewMode == Mode3D {
		p.render3D()
		return
	}
	wd, t := p.cur, p.target
	if p.ctx == nil || wd == nil || wd.scene == nil || t == nil {
		return
	}
	ctx := p.ctx
	th := &ctx.UI.Theme
	r := ctx.Renderer

	grid := p.view
	grid.Label = "sceneview.grid"
	f := r.View(grid)
	if ctx.Settings.Grid {
		dbg := f.Debug()
		dbg.Space(debug.World)
		step := ctx.Settings.SnapMove
		if step <= 0 {
			step = 1
		}
		drawGrid(dbg, f.View(), p.space, step, th)
	}
	f.End()

	sv := p.view
	sv.DoClear = false
	sv.Label = "sceneview.scene"
	wd.runner.SetTarget(t.Texture, t.BucketW, t.BucketH)
	wd.runner.SetViewportSize(t.W, t.H)
	wd.runner.SetFixedView(sv)
	e := ctx.Engine
	e.RunPhase(wd.w, engine.PhaseRenderPrepare, engine.ScopeEditor, 0, 0)
	e.RunPhase(wd.w, engine.PhaseRender, engine.ScopeEditor, 0, 0)

	ov := p.view
	ov.DoClear = false
	ov.Label = "sceneview.overlay"
	f = r.View(ov)
	dbg := f.Debug()
	dbg.Space(debug.Overlay)
	p.drawCameras(wd.scene, dbg, th)
	p.drawSelection(wd.scene, dbg, th)
	p.drawGizmos(wd.scene, dbg, th)
	p.drawBox(dbg, th)
	f.End()
}

func (p *Panel) drawCameras(s *scene.Scene, dbg *debug.Draw, th *core.Theme) {
	win := p.ctx.Project.Window
	col := th.TextDim
	col.A = 0.7
	scene.Query1[components.Camera2D](s).Each(func(h scene.NodeHandle, cam *components.Camera2D) {
		if cam.Zoom <= 0 {
			return
		}
		m := s.World(h)
		pos := mathx.Vec2{X: m[6], Y: m[7]}
		size := mathx.Vec2{X: float32(win.Width) / cam.Zoom, Y: float32(win.Height) / cam.Zoom}
		dbg.RectRot(pos, size, mathx.Atan2(m[1], m[0]), col)
		dbg.Cross(pos, p.space.PxToWorld(10), col)
	})
}

func (p *Panel) drawGizmos(s *scene.Scene, dbg *debug.Draw, th *core.Theme) {
	if !p.hasPivot {
		return
	}
	dc := &gizmo.DrawCtx{
		Debug:    dbg,
		Space:    p.space,
		Scene:    s,
		Node:     p.activeNode,
		Pivot:    p.pivot,
		Rotation: p.rotation,
		Colors: gizmo.Colors{
			X:      th.AxisX,
			Y:      th.AxisY,
			Center: th.Text,
			Rotate: th.Accent,
			Hot:    th.AxisHot,
		},
	}
	parts := func(g gizmo.Gizmo) (gizmo.Part, gizmo.Part) {
		hot, active := gizmo.PartNone, gizmo.PartNone
		if p.hotGizmo == g {
			hot = p.hot
		}
		if p.active == g && p.edit != nil {
			active = p.edit.Part
		}
		return hot, active
	}
	if p.gizmos != nil {
		p.gizmos.Each(s, s.Handle(p.activeNode), func(g gizmo.Gizmo) {
			dc.Hot, dc.Active = parts(g)
			g.Draw(dc)
		})
	}
	if p.gizmoActive(p.ctx) {
		dc.Hot, dc.Active = parts(&p.gizmo)
		p.gizmo.Draw(dc)
	}
}

func (p *Panel) drawBox(dbg *debug.Draw, th *core.Theme) {
	if p.mode != modeBox {
		return
	}
	a, b := p.worldAt(p.boxStart), p.worldAt(p.boxEnd)
	box := mathx.AABB2{
		Min: mathx.Vec2{X: min(a.X, b.X), Y: min(a.Y, b.Y)},
		Max: mathx.Vec2{X: max(a.X, b.X), Y: max(a.Y, b.Y)},
	}
	dbg.AABB(box, th.Accent)
}
