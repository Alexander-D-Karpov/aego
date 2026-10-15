package sceneview

import (
	"aego/core/mathx"
	"aego/editor/core/gizmo"
	"aego/render/debug"
	"aego/ui/core"
)

func theme3(th *core.Theme) Theme3 {
	grid := th.TextDim
	grid.A = 0.25
	cam := th.TextDim
	cam.A = 0.7
	box := th.Accent
	box.A = 0.8
	return Theme3{
		Selection: th.Accent,
		Camera:    cam,
		Box:       box,
		Grid:      grid,
		Gizmo:     gizmo.Colors3{X: th.AxisX, Y: th.AxisY, Z: th.AxisZ, Center: th.Text, Hot: th.AxisHot},
	}
}

func (p *Panel) render3D() {
	wd, t := p.cur, p.target
	if p.pool == nil || wd == nil || wd.scene == nil || t == nil || p.c3 == nil {
		return
	}
	r := p.d.Viewport.Renderer
	host := p.host3()
	th := theme3(&p.ui().Theme)
	c := p.c3
	c.SetViewport(float32(t.W), float32(t.H))

	base := c.View()
	base.Target, base.TargetW, base.TargetH = t.Texture, t.BucketW, t.BucketH
	base.DepthTarget = t.Depth
	base.PixelScale = p.view.PixelScale
	base.Clear = p.view.Clear
	base.DoClear = true

	grid := base
	grid.Label = "sceneview3d.grid"
	f := r.View(grid)
	if p.d.Tools.Grid {
		dbg := f.Debug()
		dbg.Space(debug.World)
		DrawGrid3(dbg, c.Cam, p.d.Tools.SnapMove, th)
	}
	f.End()

	sv := base
	sv.DoClear = false
	sv.Label = "sceneview3d.scene"
	wd.runner.SetTarget(t.Texture, t.BucketW, t.BucketH)
	wd.runner.SetDepthTarget(t.Depth)
	wd.runner.SetViewportSize(t.W, t.H)
	wd.runner.SetFixedView(sv)
	p.d.Viewport.RenderWorld(wd.w)

	ov := base
	ov.DoClear = false
	ov.Label = "sceneview3d.overlay"
	f = r.View(ov)
	c.DrawOverlay(host, f.Debug(), th)
	f.End()

	f = r.View(c.AxesView(base))
	dbg := f.Debug()
	dbg.Space(debug.Overlay)
	dbg.Axes3(mathx.Mat4Identity(), 1)
	f.End()
}
