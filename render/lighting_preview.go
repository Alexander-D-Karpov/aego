package render

import (
	"encoding/binary"
	"fmt"
	"math"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render/view"
)

type shadowPreviewState struct {
	shader  gpu.ShaderHandle
	pipe    gpu.PipelineHandle
	uniform gpu.BufferHandle
	sampler gpu.SamplerHandle
	data    [16]byte
}

// PreviewShadowLayer draws an existing light-space visibility mask into a 2D
// preview target. Call it between views during BeginFrame/EndFrame. It borrows
// the current source at execution time; it never retains an array handle across
// a later lighting frame, resize or context generation.
//
// This diagnostic pass does not change a material, light, or camera setting.
// The target must be an existing RGBA8/SRGBA8 texture whose base level exactly
// matches width/height. Invalid requests do not draw to the window.
func (r *Renderer) PreviewShadowLayer(id view.ID, layer int, target gpu.TextureHandle, width, height int) error {
	if r == nil || r.frame == nil {
		return fmt.Errorf("shadow preview requires an active render frame")
	}
	if r.vf.pass != nil {
		return fmt.Errorf("shadow preview must run between completed views")
	}
	if state, ok := r.dev.(gpu.ContextState); ok && state.ContextLost() {
		return fmt.Errorf("shadow preview is unavailable while the graphics context is lost")
	}
	source, _, layers, ok := r.LightingResources(id)
	if !ok {
		return fmt.Errorf("this view has no prepared shadow masks")
	}
	if layer < 0 || layer >= layers {
		return fmt.Errorf("shadow preview layer %d is outside [0,%d)", layer, layers)
	}
	if cached := &r.lightState.views[id].shadow[layer]; !cached.active || !cached.valid {
		return fmt.Errorf("shadow preview layer %d is not active in the current view", layer)
	}
	describer, ok := r.dev.(gpu.TextureDescriber)
	if !ok {
		return fmt.Errorf("graphics device cannot validate a shadow preview target")
	}
	desc, ok := describer.TextureInfo(target)
	if !ok || desc.Dimension != gpu.Texture2D || desc.Width != width || desc.Height != height || width <= 0 || height <= 0 || desc.Format != gpu.TexRGBA8 && desc.Format != gpu.TexSRGBA8 {
		return fmt.Errorf("shadow preview needs a valid RGBA8/SRGBA8 2D target matching %dx%d", width, height)
	}
	sourceDesc, ok := describer.TextureInfo(source)
	if !ok || sourceDesc.Dimension != gpu.Texture2DArray || layer >= sourceDesc.LayerCount() {
		return fmt.Errorf("shadow preview source is no longer a valid array layer")
	}
	area := mathx.Rect{W: float32(width), H: float32(height)}
	pass := gpu.PassDesc{Target: target, Color: gpu.LoadClear, ClearRect: area, Label: "lighting.shadow-preview"}
	if validator, ok := r.dev.(gpu.RenderTargetValidator); ok {
		if err := validator.ValidateRenderTarget(pass); err != nil {
			return fmt.Errorf("shadow preview target: %w", err)
		}
	}
	if err := r.prepareShadowPreview(); err != nil {
		return err
	}
	s := r.shadowPreview
	binary.LittleEndian.PutUint32(s.data[:4], math.Float32bits(float32(layer)))
	r.dev.UpdateBuffer(s.uniform, 0, s.data[:])
	p := r.frame.BeginPass(pass)
	p.SetViewport(area)
	p.SetPipeline(s.pipe)
	p.SetUniformBuffer(0, s.uniform)
	p.SetTexture(0, source, s.sampler)
	p.Draw(0, 4)
	p.End()
	return nil
}

func (r *Renderer) prepareShadowPreview() (err error) {
	if r.shadowPreview != nil {
		return nil
	}
	// The helper has no asset/importer dependency. It only visualizes a GPU
	// resource already produced by the normal lighting renderer.
	prefix := "#version 330 core\n"
	if r.caps.ESSL {
		prefix = "#version 300 es\nprecision highp float;\nprecision highp int;\nprecision highp sampler2DArray;\n"
	}
	s := &shadowPreviewState{}
	r.shadowPreview = s
	defer func() {
		if err != nil {
			r.closeShadowPreview()
		}
	}()
	s.shader, err = r.dev.CreateShader(gpu.ShaderDesc{Label: "lighting.shadow-preview", Sources: []gpu.ShaderSource{
		{Stage: gpu.StageVertex, Source: prefix + `
out vec2 vUV;
void main() {
    vec2 p = vec2(float(gl_VertexID & 1), float(gl_VertexID >> 1));
    vUV = p;
    gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}`},
		{Stage: gpu.StageFragment, Source: prefix + `
in vec2 vUV;
uniform sampler2DArray tex0;
layout(std140) uniform ub0 { vec4 options; };
out vec4 outColor;
void main() {
    float visibility = texture(tex0, vec3(vUV, options.x)).r;
    outColor = vec4(vec3(visibility), 1.0);
}`},
	}})
	if err != nil {
		return fmt.Errorf("shadow preview shader: %w", err)
	}
	s.pipe, err = r.dev.CreatePipeline(gpu.PipelineDesc{Shader: s.shader, Primitive: gpu.PrimTriangleStrip, Label: "lighting.shadow-preview"})
	if err != nil {
		return err
	}
	s.uniform, err = r.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferUniform, Size: len(s.data), Dynamic: true, Label: "lighting.shadow-preview"})
	if err != nil {
		return err
	}
	s.sampler, err = r.dev.CreateSampler(gpu.SamplerDesc{MinFilter: gpu.FilterLinear, MagFilter: gpu.FilterLinear})
	return err
}

func (r *Renderer) closeShadowPreview() {
	if r.shadowPreview == nil {
		return
	}
	s := r.shadowPreview
	if !s.pipe.IsZero() {
		r.dev.DestroyPipeline(s.pipe)
	}
	if !s.shader.IsZero() {
		r.dev.DestroyShader(s.shader)
	}
	if !s.uniform.IsZero() {
		r.dev.DestroyBuffer(s.uniform)
	}
	if !s.sampler.IsZero() {
		r.dev.DestroySampler(s.sampler)
	}
	r.shadowPreview = nil
}
