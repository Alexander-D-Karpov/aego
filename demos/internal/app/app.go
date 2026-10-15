package app

import (
	"aego/core/mathx"
	"aego/demos"
	"aego/engine"
	"aego/platform/input"
	"aego/render"
	renderhost "aego/render/host"
	"aego/render/view"
	"aego/ui/core"
	"aego/ui/overlay"
)

type Base struct {
	demos.Base
	Engine   *engine.Engine
	Renderer *render.Renderer
	UI       *core.Context
	Overlay  *overlay.Stats
	Module   *renderhost.Module
	Show     bool
}

func (b *Base) Attach(e *engine.Engine, m *renderhost.Module) {
	b.Engine = e
	b.Module = m
	b.Renderer = m.Renderer()
	b.UI = core.New(b.Renderer.UIFont())
	b.Overlay = overlay.NewStats(b.UI)
	b.Show = true
}

func (b *Base) BeginUI() {
	m := b.Engine.Metrics()
	scale := m.DisplayScale
	if scale <= 0 {
		scale = 1
	}
	size := mathx.Vec2{X: float32(m.PixelW) / scale, Y: float32(m.PixelH) / scale}
	b.UI.Begin(b.Engine.Input(), size, scale)
}

func (b *Base) SetOverlay(on bool) {
	b.Show = on
}

func (b *Base) ToggleOverlay() {
	if b.Engine.Input().KeyPressed(input.KeyF1) {
		b.Show = !b.Show
	}
}

func (b *Base) DrawOverlay(extra func(*core.Context)) {
	if !b.Show {
		return
	}
	stats := b.Module.Stats()
	b.Overlay.Draw(b.Engine.Stats(), &stats, b.Renderer.Caps(), extra)
}

func (b *Base) FlushUI() {
	list := b.UI.End()
	if list.Empty() {
		return
	}
	m := b.Engine.Metrics()
	scale := m.DisplayScale
	if scale <= 0 {
		scale = 1
	}
	v := view.ScreenView(m.PixelW, m.PixelH, scale)
	v.Label = "ui"
	f := b.Renderer.View(v)
	list.Flush(f, b.UI.Font(), b.Renderer.White(), 0)
	f.End()
}

func (b *Base) WorldView() view.View {
	m := b.Engine.Metrics()
	scale := m.DisplayScale
	if scale <= 0 {
		scale = 1
	}
	return view.View{
		Viewport:   mathx.Rect{W: float32(m.PixelW), H: float32(m.PixelH)},
		TargetW:    m.PixelW,
		TargetH:    m.PixelH,
		PixelScale: scale,
		Clear:      mathx.Color{R: 0.04, G: 0.05, B: 0.07, A: 1},
		DoClear:    true,
		Camera:     view.Camera2D{Zoom: 64},
		Label:      "world",
	}
}
