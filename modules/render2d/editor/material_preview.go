package editor

import (
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/engine/i18n"
	"aego/gpu"
	"aego/render"
	"aego/render/lighting"
	"aego/render/material"
	"aego/render/mesh"
	"aego/render/postfx"
	"aego/render/rt"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/render/view"
	"aego/scene/frame"
	"aego/ui/widgets"
	"fmt"
)

type materialPreview struct {
	mat             *material.Material
	stamp, revision uint64
	pool            *rt.Pool
	target          *rt.Target
	quad            *mesh.Mesh
	reason          string
	renderError     string
	viewID          view.ID
	lighting        lighting.Input
	lights          [2]lighting.Light
	customPlan      postfx.Plan
}

func (p *materialPreview) lightingInput() *lighting.Input {
	if p.viewID.Scope == 0 {
		p.viewID = frame.New().OutputID()
	}
	p.lighting.Environment = lighting.DefaultEnvironment()
	p.lighting.Environment.AmbientIntensity = .18
	p.lighting.Environment.ColorMode = lighting.LDR
	p.lighting.Environment.MaxShadowedLights = 0
	p.lighting.HasEnvironment = true
	p.lights = [2]lighting.Light{
		{ID: ids.NodeID{Lo: 1}, Type: lighting.Point, Position: mathx.Vec2{X: -2, Y: -1}, Direction: mathx.Vec2{X: 1}, Color: mathx.Color{R: 1, G: .8, B: .65, A: 1}, Intensity: 1.5, Range: 6, Height: 2, Falloff: 1, Layers: ^uint64(0)},
		{ID: ids.NodeID{Lo: 2}, Type: lighting.Directional, Direction: mathx.Vec2{X: -1}, Color: mathx.Color{R: .5, G: .65, B: 1, A: 1}, Intensity: .35, Elevation: .7, Layers: ^uint64(0)},
	}
	p.lighting.Lights = p.lights[:]
	return &p.lighting
}

func (p *materialPreview) update(ed *context.Editor, m *material.Material, stamp, revision uint64) {
	if p.mat != nil && p.stamp == stamp && p.revision == revision {
		return
	}
	p.stamp, p.revision = stamp, revision
	if ed.Renderer == nil || ed.Renderer.Caps().Renderer == "null" {
		p.reason = i18n.Tr("editor.material.preview_no_gpu", "Preview unavailable: this session has no graphics device.")
		return
	}
	p.reason = ""
	cache := ed.Renderer.Materials()
	if p.mat == nil {
		p.mat = m
		cache.Track(m)
	} else {
		cache.QueueMaterial(p.mat, m, nil)
	}
}

func (p *materialPreview) drawLast(ed *context.Editor) {
	if p.mat == nil {
		widgets.LabelWrapped(ed.UI, i18n.Tr("editor.material.preview_none", "No successfully compiled preview is available."), false)
		return
	}
	widgets.LabelWrapped(ed.UI, i18n.Tr("editor.material.preview_last_good", "Preview keeps its previous definition while the current source is invalid."), false)
	p.draw(ed)
}

func (p *materialPreview) draw(ed *context.Editor) {
	ui := ed.UI
	if p.reason != "" {
		widgets.LabelWrapped(ui, p.reason, true)
		return
	}
	if p.mat == nil || ed.Renderer == nil {
		return
	}
	if p.viewID.Scope == 0 {
		p.viewID = frame.New().OutputID()
	}
	if p.renderError != "" {
		widgets.LabelWrapped(ui, p.renderError, true)
	}
	cache := ed.Renderer.Materials()
	if cache.Pending(p.mat) {
		widgets.LabelWrapped(ui, i18n.Tr("editor.material.preview_pending", "Preview: separate session, compiling candidate; generation {generation} remains visible.", i18n.Int("generation", int64(p.mat.Generation()))), false)
	} else if p.mat.Failed() {
		widgets.LabelWrapped(ui, i18n.Tr("editor.material.preview_rejected", "Preview: candidate rejected; previous generation {generation} is retained. See Problems.", i18n.Int("generation", int64(p.mat.Generation()))), true)
	} else {
		widgets.LabelWrapped(ui, i18n.Tr("editor.material.preview_committed", "Preview: separate material session, generation {generation}.", i18n.Int("generation", int64(p.mat.Generation()))), false)
	}
	const size = 192
	if err := p.prepare(ed, size); err != nil {
		widgets.LabelWrapped(ui, err.Error(), true)
		return
	}
	target, mat := p.target, p.mat
	ed.OnRender(func() {
		if p.target == target && p.mat == mat {
			p.renderSurface(ed, size)
		}
	})
	area := ui.Cursor().Next(size)
	area.W = min(area.W, size)
	// Standard/PMA previews remain transparent over checkerboard. Additive
	// previews own an opaque backdrop so zero-coverage energy remains visible.
	cell := float32(12)
	for y := float32(0); y < area.H; y += cell {
		for x := float32(0); x < area.W; x += cell {
			shade := float32(.12)
			if (int(x/cell)+int(y/cell))%2 != 0 {
				shade = .3
			}
			ui.Rect(mathx.Rect{X: area.X + x, Y: area.Y + y, W: min(cell, area.W-x), H: min(cell, area.H-y)}, mathx.Color{R: shade, G: shade, B: shade, A: 1})
		}
	}
	ui.List().Texture(target.Region(), area, mathx.Color{R: 1, G: 1, B: 1, A: 1})
	if !ed.Renderer.Materials().Ready(mat) {
		widgets.LabelWrapped(ui, i18n.Tr("editor.material.preview_failed", "GPU compilation failed; see Problems for the compiler log."), true)
	}
}

func (p *materialPreview) close(ed *context.Editor) {
	if ed.Renderer != nil {
		if p.viewID.Scope != 0 {
			_ = ed.Renderer.ReleaseLightingView(p.viewID, p.viewID)
			p.viewID = view.ID{}
		}
		if p.mat != nil {
			ed.Renderer.Materials().Release(p.mat)
			ed.Renderer.Materials().Retire(p.mat.Shader())
			p.mat = nil
		}
		if p.quad != nil {
			p.quad.Destroy(ed.Renderer.Device())
			p.quad = nil
		}
	}
	if p.pool != nil {
		p.pool.Release(p.target)
		p.target = nil
		p.pool.Close()
		p.pool = nil
	}
}

func (p *materialPreview) prepare(ed *context.Editor, size int) error {
	if p.pool == nil {
		p.pool = rt.New(ed.Renderer.Device(), gpu.TexSRGBA8, "material.preview")
	}
	if p.target == nil {
		target, err := p.pool.Acquire(size, size, true)
		if err != nil {
			p.reason = err.Error()
			return err
		}
		p.target = target
	}
	if p.mat.Shader().Domain == shader.DomainMesh2D && p.quad == nil {
		data := mesh.Data{Vertices: []mesh.Vertex{
			{Pos: mathx.Vec3{X: -1, Y: -1}, Normal: mathx.Vec3{Z: 1}, UV: mathx.Vec2{}, Color: mesh.White},
			{Pos: mathx.Vec3{X: 1, Y: -1}, Normal: mathx.Vec3{Z: 1}, UV: mathx.Vec2{X: 1}, Color: mesh.White},
			{Pos: mathx.Vec3{X: 1, Y: 1}, Normal: mathx.Vec3{Z: 1}, UV: mathx.Vec2{X: 1, Y: 1}, Color: mesh.White},
			{Pos: mathx.Vec3{X: -1, Y: 1}, Normal: mathx.Vec3{Z: 1}, UV: mathx.Vec2{Y: 1}, Color: mesh.White},
		}, Indices: []uint32{0, 1, 2, 0, 2, 3}}
		data.ComputeBounds()
		quad, err := mesh.Upload(ed.Renderer.Device(), &data, "material.preview.quad")
		if err != nil {
			p.reason = err.Error()
			return err
		}
		p.quad = quad
	}
	return nil
}

func (p *materialPreview) renderSurface(ed *context.Editor, size int) {
	if p.viewID.Scope == 0 {
		p.viewID = frame.New().OutputID()
	}
	target, mat, quad := p.target, p.mat, p.quad
	r := ed.Renderer
	background := mathx.Color{}
	if mat.Blend() == shader.BlendAdditive {
		// Additive energy preserves existing coverage. Preview it against an
		// opaque backdrop, not a falsely compositable alpha-zero image.
		background = mathx.Color{R: .12, G: .12, B: .12, A: 1}
	}
	p.renderError = ""
	if mat.Shader().Domain == shader.DomainPostFX {
		if len(p.customPlan.Passes) == 0 {
			d, _ := postfx.NewRegistry().Lookup(postfx.CustomID)
			p.customPlan.Passes = []postfx.Pass{{Descriptor: d}}
		}
		p.customPlan.Passes[0].Material = mat
		if err := drawPostScene(r, p.viewID, target, size, &p.customPlan, "material.postfx-preview"); err != nil {
			p.renderError = err.Error()
		}
		return
	}
	f := r.View(view.View{ID: p.viewID, OutputID: p.viewID, Target: target.Texture, DepthTarget: target.Depth, TargetW: target.BucketW, TargetH: target.BucketH, Viewport: mathx.Rect{W: float32(size), H: float32(size)}, PixelScale: 1, Camera: view.Camera2D{Zoom: float32(size) / 3}, DoClear: true, Clear: background, Label: i18n.Tr("editor.material.preview_name", "Material preview")})
	if mat.Shader().Features&shader.FeatureLighting != 0 {
		if err := f.Lighting(p.lightingInput()); err != nil {
			p.renderError = err.Error()
		}
	}
	normal, _, _ := mat.Normal()
	white := mathx.Color{R: 1, G: 1, B: 1, A: 1}
	switch mat.Shader().Domain {
	case shader.DomainMesh2D:
		f.Mesh(mesh.Draw{VisibilityLayers: ^uint64(0), Normal: normal, Mesh: quad, World: mathx.Mat4Identity(), Texture: r.White(), Color: white, Material: mat, DoubleSided: true, Is2D: true})
	case shader.DomainSprite:
		f.Sprite(sprite.Draw{VisibilityLayers: ^uint64(0), Normal: normal, Region: r.White(), Size: mathx.Vec2{X: 2, Y: 2}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: white, Material: mat})
	}
	f.End()
}

// Render into a private linear camera target, then bind its colour as MainTex to resolve the fullscreen material into the preview; no sprite draw owns that material.
func drawPostScene(r *render.Renderer, id view.ID, target *rt.Target, size int, plan *postfx.Plan, label string) error {
	prev := r.Stats()
	f := r.View(view.View{ID: id, OutputID: id, Target: target.Texture, TargetW: target.BucketW, TargetH: target.BucketH, Viewport: mathx.Rect{W: float32(size), H: float32(size)}, Camera: view.Camera2D{Zoom: float32(size) / 3}, Layers: view.AllLayers, PixelsPerUnit: 1, PixelScale: 1, DoClear: true, Clear: mathx.Color{A: 1}, Label: label})
	f.PostProcess(plan)
	drawPostSample(f, r.White())
	f.End()
	next := r.Stats()
	if next.PostFXFailures != prev.PostFXFailures {
		return r.PostProcessError()
	}
	if len(plan.Passes) > 0 && next.PostFXPasses == prev.PostFXPasses {
		return fmt.Errorf("PostFX preview did not execute its full-screen pass")
	}
	return nil
}

// The same high-contrast scene makes channel, vignette and spatial effects
// visible on both the material document and the Assets tile.
func drawPostSample(f *render.ViewFrame, white texture.Region) {
	for j := range 4 {
		for i := range 4 {
			tint := mathx.Color{R: float32(i+1) / 4, G: float32(j+1) / 4, B: float32((i + j) % 2), A: 1}
			f.Sprite(sprite.Draw{VisibilityLayers: ^uint64(0), Region: white, Size: mathx.Vec2{X: .75, Y: .75}, Position: mathx.Vec2{X: (float32(i) - 1.5) * .75, Y: (float32(j) - 1.5) * .75}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: tint})
		}
	}
}
