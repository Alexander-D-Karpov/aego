package editor

import (
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/gizmo"
	"aego/editor/core/services"
	"aego/editor/core/shape"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/editor/ext/inspect"
	"aego/engine/i18n"
	"aego/scene"
	"aego/tests/modulecheck/fakephys"
	"aego/ui/widgets"
)

const (
	LayerImpulse = "impulse"
	CmdReset     = fakephys.ID + ".reset"
	PanelID      = fakephys.ID + ".panel"
	bodyType     = "fakephys.Body"
)

type Plugin struct {
	Overlays      int
	SceneOverlays int
	Vectors       int
	Profiled      int
	Pages         int
	Resets        int
	Inspected     int
	Drawn         int
	Gizmos        int
}

func (p *Plugin) Hook(e *ext.Registry) error {
	if err := e.SettingsPage(fakephys.ID, p.page); err != nil {
		return err
	}
	e.GameOverlay(func(*shape.OverlayCtx) { p.Overlays++ })
	e.Overlay(func(*shape.OverlayCtx) { p.SceneOverlays++ })
	if err := e.Vectors(LayerImpulse, i18n.Text{En: "Fake Impulse"}, func(*context.VectorSink) { p.Vectors++ }); err != nil {
		return err
	}
	e.Profiler(i18n.Text{En: "Fake"}, true, func(*context.Editor) { p.Profiled++ })
	if err := e.Panel(workspace.Desc{Panel: &panel{p: p}, Near: workspace.Inspector}); err != nil {
		return err
	}
	if err := e.Inspector(bodyType, inspect.CustomFunc(p.inspect)); err != nil {
		return err
	}
	if err := e.Gizmo(bodyType, &fakeGizmo{p: p}); err != nil {
		return err
	}
	if err := e.Bounds(bodyType, bounds); err != nil {
		return err
	}
	return e.Command(command.Desc{ID: CmdReset, Name: "Reset Fake", Category: "Fake", Run: func() error {
		p.Resets++
		return nil
	}})
}

func (p *Plugin) page(ctx *context.Editor, draft any) bool {
	s, ok := draft.(*fakephys.Settings)
	if !ok {
		return false
	}
	p.Pages++
	ui := ctx.UI
	v := float64(s.Gain)
	e := widgets.DragNumber(ui, ui.ID("gain"), widgets.Field(ui, "Gain"), &v, widgets.NumberOpts{Speed: 0.1})
	if e.Changed {
		s.Gain = float32(v)
	}
	return e.Changed
}

func (p *Plugin) inspect(_ *context.Editor, _ []ids.NodeID, def func()) {
	p.Inspected++
	def()
}

func bounds(s *scene.Scene, _ asset.Resolver, h scene.NodeHandle) (shape.Shape, bool) {
	b, ok := scene.Get[fakephys.Body](s, h)
	if !ok || b.Size <= 0 {
		return shape.Shape{}, false
	}
	half := b.Size / 2
	return shape.Shape{Kind: shape.Rect, Local: mathx.Rect{X: -half, Y: -half, W: b.Size, H: b.Size}}, true
}

type panel struct {
	p *Plugin
}

func (*panel) ID() string {
	return PanelID
}

func (*panel) Title() string {
	return "Fake"
}

func (pn *panel) Draw(f *services.Frame, _ mathx.Rect) {
	pn.p.Drawn++
	widgets.LabelDim(f.UI, "fake module panel")
}

type fakeGizmo struct {
	p *Plugin
}

func (g *fakeGizmo) Draw(*gizmo.DrawCtx) {
	g.p.Gizmos++
}

func (*fakeGizmo) HitTest(*gizmo.HitCtx) (gizmo.Hit, bool) {
	return gizmo.Hit{}, false
}

func (*fakeGizmo) Begin(*gizmo.EditCtx, gizmo.Hit) {}

func (*fakeGizmo) Update(*gizmo.EditCtx) {}

func (*fakeGizmo) End(*gizmo.EditCtx) {}
