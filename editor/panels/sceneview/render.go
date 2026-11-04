package sceneview

import (
	"aego/core/mathx"
	"aego/editor/core/gizmo"
	"aego/editor/core/shape"
	"aego/engine"
	"aego/render"
	"aego/render/debug"
	"aego/scene"
	"aego/ui/core"
)

func (p *Panel) render() {
	if p.viewMode == Mode3D {
		p.render3D()
		return
	}
	wd, t := p.cur, p.target
	if p.pool == nil || wd == nil || wd.scene == nil || t == nil {
		return
	}
	th := &p.ui().Theme
	r := p.d.Viewport.Renderer

	sv := p.view
	sv.DoClear = false
	sv.Label = "sceneview.scene"
	wd.runner.SetPooledTarget(t)
	wd.runner.SetFixedView(sv)
	wd.runner.SetHidden(p.runnerHide())
	p.d.Viewport.RenderWorld(wd.w)
	p.capturedScene, p.capturedTarget, p.capturedTick = wd.scene, t, wd.w.Time.Tick()
	p.flushCapture()

	ov := p.view
	ov.DoClear = false
	ov.Label = "sceneview.overlay"
	f := r.View(ov)
	dbg := f.Debug()
	dbg.Space(debug.Overlay)
	p.drawOverlays(wd.scene, dbg, th)
	p.drawSelection(wd.scene, dbg, th)
	p.drawAgent(wd.scene, dbg, th)
	p.drawGizmos(wd.scene, dbg, th)
	p.drawBox(dbg, th)
	f.End()
}

// The fixed runner chooses the correct target after extraction; drawing the
// grid earlier on the display target would lose it in an additive HDR scene.
func (p *Panel) drawFixedBackdrop(f *render.ViewFrame) {
	if !p.d.Tools.Grid {
		return
	}
	dbg := f.Debug()
	dbg.Space(debug.World)
	if p.viewMode == Mode3D && p.c3 != nil {
		DrawGrid3(dbg, p.c3.Cam, p.d.Tools.SnapMove, theme3(&p.ui().Theme))
		return
	}
	step := p.d.Tools.SnapMove
	if step <= 0 {
		step = 1
	}
	drawGrid(dbg, f.View(), p.space, step, &p.ui().Theme)
}

func (p *Panel) drawOverlays(s *scene.Scene, dbg *debug.Draw, th *core.Theme) {
	win := p.d.Project.Window
	col := th.ViewportText
	var presentationWorld *engine.World
	if p.cur != nil {
		presentationWorld = p.cur.w
	}
	p.shapes().Overlays(&shape.OverlayCtx{
		Scene:             s,
		World:             p.worldOf(s),
		PresentationWorld: presentationWorld,
		Debug:             dbg,
		View:              p.view,
		Window:            mathx.Vec2{X: float32(win.Width), Y: float32(win.Height)},
		Px:                p.space.PxToWorld(1),
		Color:             col,
	})
}

func (p *Panel) worldOf(s *scene.Scene) *engine.World {
	if p.docs().ActiveScene() == s {
		return p.docs().ActiveWorld()
	}
	return nil
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
			Center: th.ViewportLine,
			Rotate: th.Accent,
			Hot:    th.AxisHot,
			Field:  th.FieldGizmo,
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
	if p.gizmoActive() {
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
