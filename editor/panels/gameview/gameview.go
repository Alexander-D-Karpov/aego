package gameview

import (
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/play"
	"aego/editor/core/services"
	"aego/editor/core/shape"
	"aego/editor/core/vectors"
	"aego/editor/core/workspace"
	"aego/engine/i18n"
	"aego/gpu"
	"aego/platform/input"
	"aego/render/debug"
	"aego/render/rt"
	"aego/render/view"
	"aego/ui/core"
	"aego/ui/theme"
	"aego/ui/widgets"
)

const CommandOverlays = context.OverlayGame

type Panel struct {
	play      *play.Controller
	vp        *services.Viewport
	pool      *rt.Pool
	target    *rt.Target
	hook      func()
	rect      mathx.Rect
	bar       mathx.Rect
	tl        *TimelineBar
	scale     float32
	hasCamera bool
	pending   bool
}

func New(c *play.Controller, vp *services.Viewport) *Panel {
	return &Panel{play: c, vp: vp, hasCamera: true, tl: NewTimelineBar(c)}
}

func Register(cmds *command.Registry, o *context.Overlays) error {
	return cmds.Register(command.Desc{
		ID:       CommandOverlays,
		Name:     "Game View Overlays",
		Category: "Window",
		Order:    260,
		Checked:  func() bool { return o.On(CommandOverlays) },
		Run: func() error {
			o.Toggle(CommandOverlays)
			return nil
		},
	})
}

func (p *Panel) ID() string {
	return workspace.Game
}

func (p *Panel) Title() string {
	return i18n.Tr("editor.gameview.title", "Game")
}

func (p *Panel) Release() {
	p.play.SetCapture(false, mathx.Vec2{}, mathx.Vec2{})
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

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	p.tl.Apply()
	th := &ui.Theme
	if p.pool == nil {
		p.hook = p.render
		p.pool = rt.New(p.vp.GPU, gpu.TexSRGBA8, "editor.gameview")
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
	ui.List().Texture(target.Region(), r, mathx.White)
	p.pending = true
	p.vp.OnRender(p.hook)

	if p.play.State() == play.Stopped {
		p.bar = mathx.Rect{}
		p.tl.Reset()
		ui.TextIn(r, "Press Play (Ctrl+P)", th.TextDim, core.AlignCenter)
		return
	}
	id := ui.ID("view")
	origin := mathx.Vec2{X: r.X, Y: r.Y}
	p.bar = mathx.Rect{}
	p.tl.Clear()
	if !p.play.Captured() && !p.vp.Bare {
		p.toolbar(ui, r)
		p.tl.Draw(ui, r)
	}
	m := ui.Mouse()
	onBar := p.bar.Contains(m) || p.tl.Holds(m)
	switch {
	case p.play.Captured():
		p.play.SetCapture(true, origin, mathx.Vec2{X: r.W, Y: r.H})
		if ui.MousePressed(input.MouseLeft) && !r.Contains(ui.Mouse()) {
			p.Release()
		}
	case !onBar && ui.Pressed(id, r):
		p.play.SetCapture(true, origin, mathx.Vec2{X: r.W, Y: r.H})
	}
	if p.play.Captured() {
		ui.List().RectOutline(r, th.Accent, 2)
		if hint := p.play.ReleaseHint(); hint != "" {
			hr := mathx.Rect{X: r.X, Y: r.Y + r.H - th.RowHeight, W: r.W, H: th.RowHeight}
			ui.TextIn(hr, hint, th.TextDim, core.AlignCenter)
		}
	}
	if !p.hasCamera {
		ui.TextIn(r, "No active camera in the scene", th.Warn, core.AlignCenter)
	}
	for _, hud := range p.vp.GameHUD {
		hud(ui, r)
	}
	widgets.Tooltip(ui, id, i18n.Tr("editor.gameview.click_to_send_input_to", "Click to send input to the game"))
}

func (p *Panel) toolbar(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	o := p.vp.Overlay
	overlays := "Overlays: off"
	if o.On(CommandOverlays) {
		overlays = "Overlays: on"
	}
	vecs := "Vectors: off"
	if o.On(vectors.CommandToggle) {
		vecs = "Vectors: on"
	}
	w := ui.TextSize(overlays).X + ui.TextSize(vecs).X + 44
	bar := mathx.Rect{X: r.X + 6, Y: r.Y + 6, W: min(w, r.W-12), H: th.BarHeight}
	ui.PushClip(r)
	tb := widgets.BeginToolbar(ui, bar)
	ui.Outline(bar, th.BarBorder)
	if clicked, _ := tb.TextButton(ui.ID("overlays"), overlays, i18n.Tr("editor.gameview.draw_module_overlays_and_vectors", "Draw module overlays and vectors over the game")); clicked {
		o.Toggle(CommandOverlays)
	}
	if clicked, _ := tb.TextButton(ui.ID("vectors"), vecs, i18n.Tr("editor.gameview.draw_velocity_force_contact_and", "Draw velocity, force, contact and field vectors (V in the Scene View)")); clicked {
		if !o.On(vectors.CommandToggle) {
			o.Set(CommandOverlays, true)
		}
		o.Toggle(vectors.CommandToggle)
	}
	ui.PopClip()
	p.bar = bar
}

func (p *Panel) render() {
	if !p.pending || p.target == nil {
		return
	}
	p.pending = false
	t := p.target
	p.hasCamera = p.play.Render(t)
	if p.play.State() != play.Stopped && p.hasCamera {
		if p.vp.Overlay.On(CommandOverlays) {
			Overlays(p.vp, p.play)
		}
		return
	}
	v := view.View{
		Target:     t.Texture,
		TargetW:    t.BucketW,
		TargetH:    t.BucketH,
		Viewport:   mathx.Rect{W: float32(t.W), H: float32(t.H)},
		PixelScale: p.scale,
		Camera:     view.Camera2D{Zoom: 1},
		Clear:      p.viewportBg(),
		DoClear:    true,
		Label:      "gameview.clear",
	}
	p.vp.Renderer.View(v).End()
}

func Overlays(vp *services.Viewport, pc *play.Controller) {
	s := pc.Scene()
	if s == nil || vp == nil || vp.Shapes == nil || vp.Renderer == nil {
		return
	}
	col := viewportText
	if vp.UI != nil {
		col = vp.UI.Theme.ViewportText
	}
	set := shape.Set{Reg: vp.Shapes}
	oc := shape.OverlayCtx{Scene: s, World: pc.World(), Game: true, Color: col}
	for _, v := range pc.Views() {
		if v.Is3D() {
			continue
		}
		v.DoClear = false
		v.Label = "gameview.overlay"
		f := vp.Renderer.View(v)
		fv := f.View()
		oc.Debug = f.Debug()
		oc.Debug.Space(debug.Overlay)
		oc.View = *fv
		oc.Px = fv.ScreenToWorld(mathx.Vec2{X: 1}).Sub(fv.ScreenToWorld(mathx.Vec2{})).Len()
		set.Overlays(&oc)
		f.End()
	}
}

// viewportText labels overlays when no UI is attached; a theme gives the same colour (D345).
var viewportText = theme.Editor().ViewportText

func (p *Panel) viewportBg() mathx.Color {
	if p.vp != nil && p.vp.UI != nil {
		return p.vp.UI.Theme.ViewportBg
	}
	return theme.Editor().ViewportBg
}
