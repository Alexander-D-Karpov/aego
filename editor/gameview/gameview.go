package gameview

import (
	"aego/core/mathx"
	"aego/editor/context"
	"aego/editor/play"
	"aego/editor/workspace"
	"aego/gpu"
	"aego/platform/input"
	"aego/render/rt"
	"aego/render/view"
	"aego/ui/core"
	"aego/ui/widgets"
)

type Panel struct {
	play      *play.Controller
	ctx       *context.Editor
	pool      *rt.Pool
	target    *rt.Target
	hook      func()
	rect      mathx.Rect
	scale     float32
	hasCamera bool
	pending   bool
}

func New(c *play.Controller) *Panel {
	return &Panel{play: c, hasCamera: true}
}

func (p *Panel) ID() string {
	return workspace.Game
}

func (p *Panel) Title() string {
	return "Game"
}

func (p *Panel) Release() {
	p.play.SetCapture(false, mathx.Vec2{}, 0)
}

func (p *Panel) Close() {
	if p.pool == nil {
		return
	}
	if p.target != nil {
		p.pool.Release(p.target)
		p.target = nil
	}
	p.pool.Close()
}

func (p *Panel) Draw(ctx *context.Editor, r mathx.Rect) {
	ui := ctx.UI
	th := &ui.Theme
	if p.ctx == nil {
		p.ctx = ctx
		p.hook = p.render
		p.pool = rt.New(ctx.Engine.GPU(), gpu.TexSRGBA8, "editor.gameview")
	}
	scale := ui.Scale()
	pw, ph := int(r.W*scale), int(r.H*scale)
	if pw < 8 || ph < 8 {
		return
	}
	target, err := p.pool.Resize(p.target, pw, ph, true)
	if err != nil {
		ui.TextIn(r, err.Error(), th.Error, core.AlignCenter)
		return
	}
	p.target, p.rect, p.scale = target, r, scale
	ui.List().Texture(target.Region(), r, mathx.Color{R: 1, G: 1, B: 1, A: 1})
	p.pending = true
	ctx.OnRender(p.hook)

	if p.play.State() == play.Stopped {
		ui.TextIn(r, "Press Play (Ctrl+P)", th.TextDim, core.AlignCenter)
		return
	}
	id := ui.ID("view")
	origin := mathx.Vec2{X: r.X, Y: r.Y}
	switch {
	case p.play.Captured():
		p.play.SetCapture(true, origin, scale)
		if ui.KeyPressed(input.KeyEscape) || (ui.MousePressed(input.MouseLeft) && !r.Contains(ui.Mouse())) {
			p.Release()
		}
	case ui.Pressed(id, r):
		p.play.SetCapture(true, origin, scale)
	}
	if p.play.Captured() {
		ui.List().RectOutline(r, th.Accent, 2)
		hint := mathx.Rect{X: r.X, Y: r.Y + r.H - th.RowHeight, W: r.W, H: th.RowHeight}
		ui.TextIn(hint, "Esc releases input", th.TextDim, core.AlignCenter)
	}
	if !p.hasCamera {
		ui.TextIn(r, "No active camera in the scene", th.Warn, core.AlignCenter)
	}
	widgets.Tooltip(ui, id, "Click to send input to the game")
}

func (p *Panel) render() {
	if !p.pending || p.target == nil {
		return
	}
	p.pending = false
	t := p.target
	p.hasCamera = p.play.Render(t)
	if p.play.State() != play.Stopped && p.hasCamera {
		return
	}
	v := view.View{
		Target:     t.Texture,
		TargetW:    t.BucketW,
		TargetH:    t.BucketH,
		Viewport:   mathx.Rect{W: float32(t.W), H: float32(t.H)},
		PixelScale: p.scale,
		Camera:     view.Camera2D{Zoom: 1},
		Clear:      mathx.Color{R: 0.02, G: 0.02, B: 0.03, A: 1},
		DoClear:    true,
		Label:      "gameview.clear",
	}
	p.ctx.Renderer.View(v).End()
}
