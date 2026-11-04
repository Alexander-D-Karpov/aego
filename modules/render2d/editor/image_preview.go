package editor

import (
	"fmt"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/engine/i18n"
	"aego/gpu"
	"aego/platform/input"
	"aego/render/lighting"
	"aego/render/material"
	"aego/render/rt"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/render/view"
	"aego/scene/frame"
	"aego/ui/core"
	"aego/ui/widgets"
)

// These controls belong to one asset document's presentation. No Scene, Play
// world or source material participates in this preview session.
type imagePreviewControls struct {
	Lit, Normal, Emission, FlipX, FlipY bool
	Position                            mathx.Vec2
	Color                               mathx.Color
	Height, Intensity, Ambient          float32
}

func defaultImagePreviewControls() imagePreviewControls {
	return imagePreviewControls{Lit: true, Normal: true, Emission: true,
		Position: mathx.Vec2{X: -1.5, Y: -1}, Color: mathx.White,
		Height: 1.5, Intensity: 2, Ambient: .15}
}

type imageSurface struct {
	Albedo   texture.Region
	Normal   texture.Region
	Emission texture.Emission
	Size     mathx.Vec2
	Reason   string
}

type imagePreview struct {
	controls     imagePreviewControls
	ready        bool
	mat          *material.Material
	viewID       view.ID
	pool         *rt.Pool
	target       *rt.Target
	candidate    *imagePixels
	texture      gpu.TextureHandle
	normalRegion texture.Region
	light        [1]lighting.Light
	input        lighting.Input
	err          string
	serial       uint64
}

func (p *imagePreview) init() {
	if !p.ready {
		p.controls, p.ready = defaultImagePreviewControls(), true
		p.viewID = frame.New().OutputID()
	}
}

func (p *imagePreview) lighting() *lighting.Input {
	p.init()
	p.input.Environment = lighting.DefaultEnvironment()
	p.input.Environment.ColorMode = lighting.LDR
	p.input.Environment.AmbientIntensity = p.controls.Ambient
	p.input.Environment.MaxShadowedLights = 0
	p.input.HasEnvironment = true
	p.input.Disabled = !p.controls.Lit
	p.light[0] = lighting.Light{ID: ids.NodeID{Lo: 1}, Type: lighting.Point,
		Position: p.controls.Position, Direction: mathx.Vec2{X: 1},
		Color: p.controls.Color, Intensity: p.controls.Intensity,
		Height: p.controls.Height, Range: 10, Falloff: 1, Layers: ^uint64(0)}
	p.input.Lights = p.light[:]
	return &p.input
}

func (p *imagePreview) ensure(ed *context.Editor, w, h int) error {
	p.init()
	if ed.Renderer == nil || ed.Renderer.Caps().Renderer == "null" {
		return fmt.Errorf("%s", i18n.Tr("editor.image.no_gpu", "Lit preview is unavailable: this session has no graphics device. Source pixels and metadata remain available."))
	}
	if p.mat == nil {
		// Decode the same prepared builtin artifact that exported games use.
		// The private definition prevents preview settings from changing any
		// shared material, while its program still uses the normal material cache.
		data, ok := asset.BuiltinShader(asset.BuiltinShaderLit)
		if !ok {
			return fmt.Errorf("prepared Lit shader is unavailable")
		}
		a, err := asset.DecodeShader(data)
		if err != nil {
			return err
		}
		sh := a.Runtime(asset.BuiltinShaderLit, "image.preview.lit")
		m, ok := material.New("image.preview", sh, 0, shader.BlendPremultiplied)
		if !ok {
			return fmt.Errorf("prepared Lit schema is invalid")
		}
		p.mat = m
		ed.Renderer.Materials().Track(m)
	}
	if p.pool == nil {
		p.pool = rt.New(ed.Renderer.Device(), gpu.TexSRGBA8, "image.preview")
	}
	target, err := p.pool.Resize(p.target, w, h, true)
	if err != nil {
		return err
	}
	p.target = target
	p.pool.Trim(0)
	if validator, ok := ed.Renderer.Device().(gpu.RenderTargetValidator); ok {
		if err := validator.ValidateRenderTarget(gpu.PassDesc{Target: target.Texture, DepthTarget: target.Depth}); err != nil {
			return err
		}
	}
	return nil
}

// candidateRegion uploads immutable CPU candidate bytes on the render thread.
// WebGL restoration retains these engine-owned bytes; no importer runs there.
func (p *imagePreview) candidateRegion(ed *context.Editor, pixels *imagePixels, rect [4]int) (texture.Region, error) {
	if pixels == nil {
		return texture.Region{}, nil
	}
	if p.candidate != pixels || p.texture.IsZero() {
		d := pixels.data
		if d.Format != gpu.TexRGBA8 || pixels.semantic != asset.TextureNormal {
			return p.normalRegion, fmt.Errorf("normal candidate must be a prepared linear normal texture")
		}
		h, err := ed.Renderer.Device().CreateTexture(gpu.TextureDesc{Width: d.Width, Height: d.Height, Format: d.Format, Data: d.Pixels, Label: "image.preview.normal"})
		if err != nil {
			return p.normalRegion, err
		}
		old := p.texture
		p.texture, p.candidate = h, pixels
		if !old.IsZero() {
			ed.Renderer.Device().DestroyTexture(old)
		}
	}
	r := texture.Whole(p.texture, pixels.data.Width, pixels.data.Height)
	if rect[2] > 0 && rect[3] > 0 && rect[0] >= 0 && rect[1] >= 0 && rect[0]+rect[2] <= pixels.data.Width && rect[1]+rect[3] <= pixels.data.Height {
		r = r.Sub(rect[0], rect[1], rect[2], rect[3])
	}
	p.normalRegion = r
	return r, nil
}

func (p *imagePreview) setSurface(surface imageSurface) {
	if p.mat == nil {
		return
	}
	p.mat.Set("NormalStrength", [4]float32{1})
	p.mat.Set("EmissionColor", [4]float32{1, 1, 1, 1})
	p.mat.Set("EmissionIntensity", [4]float32{0})
	p.mat.Reset("NormalMap")
	p.mat.Reset("EmissionMap")
	if p.controls.Normal && !surface.Normal.IsZero() {
		p.mat.SetTexture("NormalMap", material.Tex{Texture: surface.Normal.Texture, Region: surface.Normal, NormalMap: true, RegionOnly: true})
	}
	if p.controls.Emission && surface.Emission.Mode != texture.EmissionNone && !surface.Emission.Region.IsZero() {
		p.mat.SetTexture("EmissionMap", material.Tex{Texture: surface.Emission.Region.Texture, Region: surface.Emission.Region, EmissionMode: surface.Emission.Mode, RegionOnly: true})
		p.mat.Set("EmissionIntensity", [4]float32{1})
	}
}

func (p *imagePreview) draw(ed *context.Editor, area mathx.Rect, surface imageSurface, candidate *imagePixels, rect [4]int) {
	p.init()
	ui := ed.UI
	if area.W < 8 || area.H < 8 {
		return
	}
	if surface.Reason != "" {
		imagePreviewReason(ui, area, surface.Reason)
		return
	}
	scale := ui.Scale()
	w, h := min(1024, max(8, int(area.W*scale))), min(1024, max(8, int(area.H*scale)))
	if err := p.ensure(ed, w, h); err != nil {
		imagePreviewReason(ui, area, err.Error())
		return
	}
	if surface.Albedo.IsZero() {
		surface.Albedo = ed.Renderer.White()
	}
	if surface.Size.X <= 0 || surface.Size.Y <= 0 {
		surface.Size = mathx.Vec2{X: 2, Y: 2}
	}
	id := ui.ID("image.preview.light")
	zoom := min(float32(w)/surface.Size.X, float32(h)/surface.Size.Y) * .78
	if ui.MouseOver(id, area) && ui.MousePressed(input.MouseLeft) {
		ui.CaptureMouse(id)
	}
	if ui.Captured() == id {
		mouse := ui.Mouse()
		p.controls.Position = mathx.Vec2{X: (mouse.X - area.X - area.W*.5) * float32(w) / area.W / zoom, Y: (mouse.Y - area.Y - area.H*.5) * float32(h) / area.H / zoom}
		if ui.MouseReleased(input.MouseLeft) || !ui.MouseDown(input.MouseLeft) {
			ui.ReleaseMouse()
		}
	}
	drawImageBackground(ui, area, 0)
	ui.List().Texture(p.target.Region(), area, mathx.White)
	serial, target := p.serial, p.target
	ed.OnRender(func() {
		if serial != p.serial || p.target != target || p.mat == nil {
			return
		}
		p.err = ""
		if candidate != nil {
			var err error
			surface.Normal, err = p.candidateRegion(ed, candidate, rect)
			if err != nil {
				p.err = err.Error()
			}
		}
		p.setSurface(surface)
		if !p.controls.Normal {
			surface.Normal = texture.Region{}
		}
		if !p.controls.Emission {
			surface.Emission = texture.Emission{}
		}
		r := ed.Renderer
		f := r.View(view.View{ID: p.viewID, OutputID: p.viewID, Target: target.Texture, DepthTarget: target.Depth,
			TargetW: target.BucketW, TargetH: target.BucketH, Viewport: mathx.Rect{W: float32(w), H: float32(h)},
			PixelScale: 1, Camera: view.Camera2D{Zoom: zoom}, DoClear: true, Label: "Image Lit preview"})
		if err := f.Lighting(p.lighting()); err != nil {
			p.err = err.Error()
		}
		f.Sprite(sprite.Draw{VisibilityLayers: ^uint64(0), Region: surface.Albedo, Normal: surface.Normal, Emission: surface.Emission,
			Size: surface.Size, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: p.mat,
			FlipX: p.controls.FlipX, FlipY: p.controls.FlipY})
		f.End()
		if !r.Materials().Ready(p.mat) {
			p.err = i18n.Tr("editor.image.lit_failed", "The Lit preview program could not compile on this graphics device. See Problems.")
		}
		if r.LightingError() != nil {
			p.err = r.LightingError().Error()
		}
	})
	if p.err != "" {
		ui.Text(p.err, mathx.Vec2{X: area.X + 6, Y: area.Y + 6}, ui.Theme.Error)
	}
	point := mathx.Vec2{X: area.X + area.W*.5 + p.controls.Position.X*zoom*area.W/float32(w), Y: area.Y + area.H*.5 + p.controls.Position.Y*zoom*area.H/float32(h)}
	if area.Contains(point) {
		ui.Line(point.Sub(mathx.Vec2{X: 5}), point.Add(mathx.Vec2{X: 5}), 1, ui.Theme.Accent)
		ui.Line(point.Sub(mathx.Vec2{Y: 5}), point.Add(mathx.Vec2{Y: 5}), 1, ui.Theme.Accent)
	}
}

func (p *imagePreview) fields(ed *context.Editor) {
	p.init()
	ui := ed.UI
	widgets.LabelWrapped(ui, i18n.Tr("editor.image.preview_controls", "Preview only · drag in the surface to move the light"), false)
	widgets.Checkbox(ui, i18n.Tr("editor.image.lit", "Lit"), &p.controls.Lit)
	widgets.Checkbox(ui, i18n.Tr("editor.image.normal_enabled", "Normal enabled"), &p.controls.Normal)
	widgets.Checkbox(ui, i18n.Tr("editor.image.emission_enabled", "Emission enabled"), &p.controls.Emission)
	widgets.Checkbox(ui, i18n.Tr("editor.image.flip_x", "Flip X"), &p.controls.FlipX)
	widgets.Checkbox(ui, i18n.Tr("editor.image.flip_y", "Flip Y"), &p.controls.FlipY)
	widgets.ColorField(ui, ui.ID("light.color"), widgets.Field(ui, i18n.Tr("editor.image.light_color", "Light color")), &p.controls.Color, false)
	for _, f := range []struct {
		id, label string
		value     *float32
		max       float64
	}{{"height", i18n.Tr("editor.image.height", "Light height"), &p.controls.Height, 10}, {"intensity", i18n.Tr("editor.image.intensity", "Light intensity"), &p.controls.Intensity, 16}, {"ambient", i18n.Tr("editor.image.ambient", "Ambient intensity"), &p.controls.Ambient, 4}} {
		widgets.DragFloat32(ui, ui.ID(f.id), widgets.Field(ui, f.label), f.value, widgets.NumberOpts{Speed: .01, Min: 0, HasMin: true, Max: f.max, HasMax: true})
	}
}

func imagePreviewReason(ui *core.Context, area mathx.Rect, reason string) {
	ui.PushClip(area)
	ui.Region(area)
	widgets.LabelWrapped(ui, reason, true)
	ui.PopCursor()
	ui.PopClip()
}

func (p *imagePreview) close(ed *context.Editor) {
	p.serial++ // invalidate already queued closures before releasing handles
	if ed.Renderer != nil {
		if p.mat != nil {
			ed.Renderer.Materials().Release(p.mat)
			ed.Renderer.Materials().Retire(p.mat.Shader())
			p.mat = nil
		}
		if !p.texture.IsZero() {
			ed.Renderer.Device().DestroyTexture(p.texture)
			p.texture = gpu.TextureHandle{}
		}
		p.candidate = nil
		p.normalRegion = texture.Region{}
		_ = ed.Renderer.ReleaseLightingView(p.viewID, p.viewID)
	}
	if p.pool != nil {
		p.pool.Release(p.target)
		p.target = nil
		p.pool.Close()
		p.pool = nil
	}
}
