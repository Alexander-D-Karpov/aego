package gameview

import (
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/play"
	"aego/editor/core/services"
	"aego/editor/core/shape"
	"aego/editor/core/vectors"
	"aego/editor/core/workspace"
	"aego/engine"
	"aego/engine/i18n"
	"aego/gpu"
	"aego/platform/input"
	"aego/render/debug"
	"aego/render/rt"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/ui/core"
	"aego/ui/theme"
	"aego/ui/widgets"
)

const CommandOverlays = context.OverlayGame

type Panel struct {
	play          *play.Controller
	vp            *services.Viewport
	pool          *rt.Pool
	target        *rt.Target
	hook          func()
	rect          mathx.Rect
	bar           mathx.Rect
	tl            *TimelineBar
	scale         float32
	lastTick      uint64
	lastScene     *scene.Scene
	lastPreview   frame.PostPreview
	renderFrame   uint64
	hasCamera     bool
	previewWorld  *engine.World
	previewRunner *frame.Runner
	previewScene  *scene.Scene
	bypassFX      bool
	pending       bool
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
	p.releasePreview()
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

	p.previewScene = nil
	if p.play.State() == play.Stopped {
		p.bar = mathx.Rect{}
		p.tl.Reset()
		if p.vp.CameraPreview != nil {
			p.previewScene = p.vp.CameraPreview()
		}
		if p.previewScene != nil {
			if !p.vp.Bare {
				p.toolbar(ui, r)
			}
			return
		}
		ui.TextIn(r, i18n.Tr("editor.gameview.press_play", "Press Play (Ctrl+P)"), th.TextDim, core.AlignCenter)
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
		ui.TextIn(r, i18n.Tr("editor.gameview.no_active_camera", "No active camera in the scene"), th.Warn, core.AlignCenter)
	}
	for _, hud := range p.vp.GameHUD {
		hud(ui, r)
	}
	widgets.Tooltip(ui, id, i18n.Tr("editor.gameview.click_to_send_input_to", "Click to send input to the game"))
}

func (p *Panel) toolbar(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	o := p.vp.Overlay
	overlays := i18n.Tr("editor.gameview.overlays_off", "Overlays: off")
	if o.On(CommandOverlays) {
		overlays = i18n.Tr("editor.gameview.overlays_on", "Overlays: on")
	}
	vecs := i18n.Tr("editor.gameview.vectors_off", "Vectors: off")
	if o.On(vectors.CommandToggle) {
		vecs = i18n.Tr("editor.gameview.vectors_on", "Vectors: on")
	}
	fx := i18n.Tr("editor.gameview.postfx_on", "PostFX: on")
	if p.bypassFX {
		fx = i18n.Tr("editor.gameview.postfx_off", "PostFX: bypass")
	}
	w := ui.TextSize(overlays).X + ui.TextSize(vecs).X + ui.TextSize(fx).X + 66
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
	if clicked, _ := tb.TextButton(ui.ID("postfx-preview"), fx, i18n.Tr("editor.gameview.postfx_bypass", "Compare the authored camera output with effects bypassed in this Game View")); clicked {
		p.bypassFX = !p.bypassFX
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
	preview := frame.PostPreview{Bypass: p.bypassFX}
	if p.vp.EffectPreview != nil {
		preview.Camera, preview.Instance = p.vp.EffectPreview()
	}
	p.lastPreview = preview
	p.renderFrame = p.vp.UI.Frame()
	if p.previewScene != nil && p.play.State() == play.Stopped {
		if p.previewRunner == nil || p.previewRunner.Scene() != p.previewScene {
			p.releasePreview()
			p.previewWorld = engine.NewWorld("camera preview")
			p.previewRunner = frame.New()
			p.previewRunner.Bind(p.previewScene, p.vp.Renderer, p.previewScene.Resolver())
			p.previewWorld.SetService(frame.ServiceID, p.previewRunner)
			p.previewWorld.SetService(scene.ServiceID, p.previewScene)
		}
		p.previewRunner.SetPostPreview(preview)
		p.previewRunner.SetPooledTarget(t)
		p.vp.RenderWorld(p.previewWorld)
		p.hasCamera = len(p.previewRunner.Views()) > 0
		p.lastScene, p.lastTick = p.previewScene, 0
		if p.hasCamera {
			return
		}
	} else {
		p.releasePreview()
	}
	p.hasCamera = p.play.RenderPreview(t, preview)
	p.lastScene = p.play.Scene()
	p.lastTick = p.play.Ticks()
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

func (p *Panel) releasePreview() {
	if p.previewRunner != nil {
		for _, v := range p.previewRunner.Views() {
			_ = p.vp.Renderer.ReleaseLightingView(v.ID, v.OutputID)
		}
		p.previewRunner.Close()
		p.previewRunner, p.previewWorld = nil, nil
	}
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

// PostOutput identifies the Game View presentation used by this document's
// latest visible frame. A dock switch or Play/Stop invalidates a stale world.
func (p *Panel) PostOutput(d *document.SceneDocument) (*engine.World, view.ID, frame.PostPreview) {
	if d != nil && p.target != nil && p.hasCamera && p.vp.UI.Frame()-p.renderFrame <= 1 {
		if p.play.State() == play.Stopped {
			if p.previewRunner != nil && p.previewScene == d.Scene() && p.lastScene == p.previewScene {
				return p.previewWorld, p.previewRunner.OutputID(), p.lastPreview
			}
		} else if p.lastScene == p.play.Scene() && (d.ID == p.play.Doc() || d.Scene() == p.lastScene) {
			if r, ok := engine.ServiceOf[*frame.Runner](p.play.World(), frame.ServiceID); ok {
				return p.play.World(), r.OutputID(), p.lastPreview
			}
		}
	}
	return nil, view.ID{}, frame.PostPreview{}
}

func (p *Panel) LastRender() (*rt.Target, uint64) {
	if p.target != nil && !p.pending && p.hasCamera && p.play.State() == play.Stopped && p.previewScene != nil && p.lastScene == p.previewScene && p.vp.CameraPreview != nil && p.vp.CameraPreview() == p.previewScene {
		return p.target, p.lastTick
	}
	if p.target == nil || p.pending || !p.hasCamera || p.play.State() == play.Stopped || p.lastScene != p.play.Scene() {
		return nil, 0
	}
	return p.target, p.lastTick
}
