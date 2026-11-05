package render

import (
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/lighting"
	"aego/render/shader"
	"aego/render/texture"
	"aego/render/view"
	"fmt"
)

// ReleaseLightingView releases camera/output-owned auxiliary textures for a closing preview, between completed views or outside a frame.
// outputID must be exclusive to that session; renderer-wide shaders, samplers and reusable UBO capacity remain renderer-owned.
func (r *Renderer) ReleaseLightingView(viewID, outputID view.ID) error {
	if r == nil {
		return nil
	}
	if r.frame != nil && !r.vf.ended {
		return fmt.Errorf("lighting resources cannot be released while a view is being prepared")
	}
	if o := r.lightState.outputs[outputID]; o != nil && o.pending {
		return fmt.Errorf("lighting output must finish compositing before release")
	}
	if v := r.lightState.views[viewID]; v != nil {
		r.dropLightingView(v)
		delete(r.lightState.views, viewID)
	}
	if o := r.lightState.outputs[outputID]; o != nil {
		r.dropLightingOutput(o)
		delete(r.lightState.outputs, outputID)
	}
	return nil
}

type lightingState struct {
	frame, generation                    uint64
	views                                map[view.ID]*lightingView
	outputs                              map[view.ID]*lightingOutput
	resolveShader                        gpu.ShaderHandle
	resolvePipe, resolveReplace          gpu.PipelineHandle
	resolveUB                            gpu.BufferHandle
	resolveData                          []byte
	used                                 []bool
	shadowVerts                          []mathx.Vec2
	shadowBytes                          []byte
	shadowCapacity                       int
	shadowShader, cookieShader           gpu.ShaderHandle
	shadowPipe, cookiePipe               gpu.PipelineHandle
	shadowVB, cookieUB                   gpu.BufferHandle
	copyData                             []byte
	error                                error
	lastError                            string
	sets                                 []gpu.BufferHandle
	uploadedSets                         [][shader.LightingSize]byte
	validSets                            []bool
	data                                 []byte
	surface                              []byte
	surfaceUB                            gpu.BufferHandle
	flatNormal, emptyShadow, emptyCookie gpu.TextureHandle
	sampler                              gpu.SamplerHandle
}

func (r *Renderer) initLighting() bool {
	s := &r.lightState
	if !s.flatNormal.IsZero() {
		return true
	}
	fail := func(err error) bool {
		for _, h := range [...]gpu.TextureHandle{s.flatNormal, s.emptyShadow, s.emptyCookie} {
			if !h.IsZero() {
				r.dev.DestroyTexture(h)
			}
		}
		if !s.surfaceUB.IsZero() {
			r.dev.DestroyBuffer(s.surfaceUB)
		}
		s.flatNormal, s.emptyShadow, s.emptyCookie = gpu.TextureHandle{}, gpu.TextureHandle{}, gpu.TextureHandle{}
		s.surfaceUB = gpu.BufferHandle{}
		r.lightingFailure(err)
		return false
	}
	var err error
	s.flatNormal, err = r.dev.CreateTexture(gpu.TextureDesc{Width: 1, Height: 1, Format: gpu.TexRGBA8, Data: []byte{128, 128, 255, 255}, Label: "lighting.flat-normal"})
	if err != nil {
		return fail(err)
	}
	s.emptyShadow, err = r.dev.CreateTexture(gpu.TextureDesc{Dimension: gpu.Texture2DArray, Layers: 1, Width: 1, Height: 1, Format: gpu.TexR8, Data: []byte{255}, Label: "lighting.unshadowed"})
	if err != nil {
		return fail(err)
	}
	s.emptyCookie, err = r.dev.CreateTexture(gpu.TextureDesc{Dimension: gpu.Texture2DArray, Layers: 1, Width: 1, Height: 1, Format: gpu.TexR8, Data: []byte{0}, Label: "lighting.no-cookie"})
	if err != nil {
		return fail(err)
	}
	s.surfaceUB, err = r.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferUniform, Size: shader.SurfaceSize, Dynamic: true, Label: "lighting.surface"})
	if err != nil {
		return fail(err)
	}
	s.sampler = r.Sampler(gpu.SamplerDesc{MinFilter: gpu.FilterLinear, MagFilter: gpu.FilterLinear})
	// Separate pointer-free backing stores are required by native cgo uploads;
	// an embedded array would expose the enclosing state's maps and slices.
	s.data = make([]byte, shader.LightingSize)
	s.surface = make([]byte, shader.SurfaceSize)
	return true
}

func (r *Renderer) contextLost() bool {
	state, ok := r.dev.(gpu.ContextState)
	return ok && state.ContextLost()
}

func (r *Renderer) prepareLighting(f *ViewFrame) bool {
	if r.contextLost() {
		return false
	}
	if f.lightingInvalid {
		return false
	}
	env := lighting.DefaultEnvironment()
	if f.lighting != nil && f.lighting.HasEnvironment {
		env = f.lighting.Environment
	}
	if f.lighting != nil && (f.lighting.Disabled || f.lighting.Debug != lighting.DebugLighting) {
		// Inspection compares the same assets in display-linear LDR. It must
		// not silently darken the unshaded reference through HDR tone mapping.
		env.ColorMode, env.Exposure = lighting.LDR, 1
	}
	wantOutput := f.litCount > 0 || env.ColorMode == lighting.HDR || f.lighting != nil && f.lighting.OutputLit
	if !wantOutput {
		if f.postProcess == nil {
			return true
		}
		env.ColorMode, env.Exposure = lighting.LDR, 1
	}
	if err := env.Validate(); err != nil {
		r.lightingFailure(err)
		return false
	}
	if !r.prepareLightingOutput(f, env) {
		if r.contextLost() {
			return false
		}
		if !wantOutput && f.view.OutputPolicy != view.OutputLinear {
			f.output, f.postProcess = nil, nil
			r.postProcessFailure(r.LightingError())
			return true
		}
		return false
	}
	if f.litCount == 0 {
		return true
	}
	if !r.initLighting() {
		return false
	}
	s := &r.lightState
	// Reserve a small block on first use so an additional visible light set
	// does not create a WebGL object during an otherwise warm camera pan.
	const setBlock = 8
	needed := (len(f.sets) + setBlock - 1) / setBlock * setBlock
	for len(s.sets) < needed {
		h, err := r.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferUniform, Size: shader.LightingSize, Dynamic: true, Label: "lighting.set"})
		if err != nil {
			r.lightingFailure(err)
			return false
		}
		s.sets = append(s.sets, h)
		s.uploadedSets = append(s.uploadedSets, [shader.LightingSize]byte{})
		s.validSets = append(s.validSets, false)
	}
	if !r.prepareLightingAux(f, env) {
		return false
	}
	for k, set := range f.sets {
		clear(s.data[:])
		ambient := mathx.Color{R: env.Ambient.R * env.AmbientIntensity, G: env.Ambient.G * env.AmbientIntensity, B: env.Ambient.B * env.AmbientIntensity, A: 1}
		if f.lighting != nil && f.lighting.Disabled {
			ambient = mathx.Color{R: 1, G: 1, B: 1, A: 1}
		}
		shader.PutLightingVec4(s.data[:], 0, [4]float32{ambient.R, ambient.G, ambient.B, float32(set.Count)})
		for j := 0; j < set.Count; j++ {
			l := &f.lighting.Lights[set.Indices[j]]
			dir := l.Direction
			length := mathx.Sqrt(dir.X*dir.X + dir.Y*dir.Y)
			if length > 1e-8 {
				dir = dir.Scale(1 / length)
			} else {
				dir = mathx.Vec2{X: 1}
			}
			shader.PutLightingVec4(s.data[:], 16+j*16, [4]float32{l.Position.X, l.Position.Y, l.Height, float32(l.Type)})
			shader.PutLightingVec4(s.data[:], 144+j*16, [4]float32{l.Color.R * l.Intensity, l.Color.G * l.Intensity, l.Color.B * l.Intensity, l.Range})
			shader.PutLightingVec4(s.data[:], 272+j*16, [4]float32{dir.X, dir.Y, mathx.Cos(l.InnerAngle / 2), mathx.Cos(l.OuterAngle / 2)})
			var top float32
			shadowLayer, cookieLayer := float32(-1), float32(-1)
			if resources := f.lightResources; resources != nil {
				shadowLayer = float32(resources.shadowFor[set.Indices[j]])
				cookieLayer = float32(resources.cookieFor[set.Indices[j]])
				if shadowLayer >= 0 {
					top = resources.shadow[int(shadowLayer)].certifiedTop
				}
			}
			shader.PutLightingVec4(s.data[:], 400+j*16, [4]float32{l.Falloff, l.Elevation, shadowLayer, min(l.ShadowSoftness, 4) / float32(env.ShadowResolution)})
			shader.PutLightingVec4(s.data[:], 528+j*16, [4]float32{cookieLayer, top, 0, 0})
		}
		if f.lighting != nil {
			mode := float32(f.lighting.Debug)
			if f.lighting.Disabled && f.lighting.Debug == lighting.DebugLighting {
				mode = 3 // Unshaded inspection also bypasses material emission.
			}
			putF32(s.data[540:], mode)
		}
		if !s.validSets[k] || s.uploadedSets[k] != *(*[shader.LightingSize]byte)(s.data) {
			r.dev.UpdateBuffer(s.sets[k], 0, s.data[:])
			s.uploadedSets[k], s.validSets[k] = *(*[shader.LightingSize]byte)(s.data), true
			r.stats.LightingUBOBytes += shader.LightingSize
		}
	}
	r.stats.LightingResources = len(s.sets) + 4
	return true
}

func (r *Renderer) bindLighting(p gpu.Pass, set uint32, normal, emission gpu.TextureHandle, channels bool) bool {
	s := &r.lightState
	if set == 0 || int(set) > len(s.sets) {
		return false
	}
	if normal.IsZero() {
		normal = s.flatNormal
	}
	p.SetUniformBuffer(shader.SlotLighting, s.sets[set-1])
	p.SetTexture(shader.TexNormal, normal, s.sampler)
	shadows, cookies := s.emptyShadow, s.emptyCookie
	if resources := r.vf.lightResources; resources != nil {
		if !resources.shadows.IsZero() {
			shadows = resources.shadows
		}
		if !resources.cookies.IsZero() {
			cookies = resources.cookies
		}
	}
	p.SetTexture(shader.TexShadow, shadows, s.sampler)
	p.SetTexture(shader.TexCookie, cookies, s.sampler)
	if channels {
		if emission.IsZero() {
			emission = r.white.Texture
		}
		p.SetTexture(shader.TexEmission, emission, s.sampler)
	}
	return true
}

func (r *Renderer) bindSurface(p gpu.Pass, normal texture.Region, emission texture.Emission, mode texture.EmissionMode) {
	s := &r.lightState
	clear(s.surface[:])
	if !normal.IsZero() {
		shader.PutLightingVec4(s.surface[:], 0, [4]float32{normal.UVMin.X, normal.UVMin.Y, normal.UVMax.X, normal.UVMax.Y})
		shader.PutLightingVec4(s.surface[:], 16, normal.LocalRect())
	}
	if emission.Valid() {
		e := emission.Region
		shader.PutLightingVec4(s.surface[:], 32, [4]float32{e.UVMin.X, e.UVMin.Y, e.UVMax.X, e.UVMax.Y})
		shader.PutLightingVec4(s.surface[:], 48, e.LocalRect())
	}
	shader.PutLightingVec4(s.surface[:], 64, [4]float32{float32(mode), 0, 0, 0})
	r.dev.UpdateBuffer(s.surfaceUB, 0, s.surface[:])
	p.SetUniformBuffer(shader.SlotSurface, s.surfaceUB)
	r.stats.LightingUBOBytes += shader.SurfaceSize
}

func (r *Renderer) closeLighting() {
	s := &r.lightState
	for _, h := range s.sets {
		r.dev.DestroyBuffer(h)
	}
	if !s.surfaceUB.IsZero() {
		r.dev.DestroyBuffer(s.surfaceUB)
	}
	for _, h := range [...]gpu.TextureHandle{s.flatNormal, s.emptyShadow, s.emptyCookie} {
		if !h.IsZero() {
			r.dev.DestroyTexture(h)
		}
	}
	for _, v := range s.views {
		r.dropLightingView(v)
	}
	for _, o := range s.outputs {
		r.dropLightingOutput(o)
	}
	for _, h := range [...]gpu.BufferHandle{s.shadowVB, s.cookieUB, s.resolveUB} {
		if !h.IsZero() {
			r.dev.DestroyBuffer(h)
		}
	}
	r.destroyPipe(s.shadowPipe)
	r.destroyPipe(s.cookiePipe)
	r.destroyShader(s.shadowShader)
	r.destroyShader(s.cookieShader)
	r.destroyPipe(s.resolveReplace)
	r.destroyPipe(s.resolvePipe)
	r.destroyShader(s.resolveShader)
	*s = lightingState{}
}

func (r *Renderer) lightingFailure(err error) {
	if r.contextLost() {
		return
	}
	if err == nil {
		return
	}
	if r.lightState.error == nil || r.lightState.error.Error() != err.Error() {
		r.lightState.error = err
		if r.lightState.lastError != err.Error() {
			r.lightState.lastError = err.Error()
			r.log.Warn("lighting resource unavailable", log.F("reason", err.Error()))
		}
	}
	r.stats.LightingFailures++
}

func (r *Renderer) LightingError() error { return r.lightState.error }

// LightingResources returns a borrowed preview handle; do not destroy it or retain it across a subsequent frame, resize or context generation.
// The layer count is capacity; LightingShadowLayers lists active layers.
func (r *Renderer) LightingResources(id view.ID) (gpu.TextureHandle, int, int, bool) {
	v := r.lightState.views[id]
	if v == nil || v.shadows.IsZero() || v.lastFrame != r.lightState.frame {
		return gpu.TextureHandle{}, 0, 0, false
	}
	return v.shadows, v.resolution, v.shadowLayers, true
}

type ShadowInfo struct {
	Layer      int
	Light      ids.NodeID
	Resolution int
	Texture    gpu.TextureHandle
}

// LightingShadowLayers appends the current active masks, without exposing
// inactive cached content as a live light. dst is caller-owned scratch storage.
func (r *Renderer) LightingShadowLayers(id view.ID, dst []ShadowInfo) []ShadowInfo {
	v := r.lightState.views[id]
	if v == nil || v.lastFrame != r.lightState.frame || v.shadows.IsZero() {
		return dst
	}
	for i := 0; i < v.shadowLayers; i++ {
		if layer := &v.shadow[i]; layer.active && layer.valid {
			dst = append(dst, ShadowInfo{Layer: i, Light: layer.light, Resolution: v.resolution, Texture: v.shadows})
		}
	}
	return dst
}

// LightingDropped is borrowed until the next preparation of this view. A light
// appears once when at least one compatible receiver exceeded its light budget.
func (r *Renderer) LightingDropped(id view.ID) []ids.NodeID {
	v := r.lightState.views[id]
	if v == nil || v.lastFrame != r.lightState.frame {
		return nil
	}
	return v.dropped
}

func (r *Renderer) beginLightingFrame() {
	s := &r.lightState
	s.frame++
	s.error = nil
	r.fx.err = nil
	if context, ok := r.dev.(gpu.ContextState); ok {
		generation := context.ContextGeneration()
		if generation != s.generation {
			r.closePostProcess()
			clear(s.validSets)
			for _, o := range s.outputs {
				o.closePostProcessPool()
			}
			s.generation = generation
			s.error = nil
		}
	}
	for id, v := range s.views {
		if s.frame-v.lastFrame > 2 {
			r.dropLightingView(v)
			delete(s.views, id)
		}
	}
	for id, o := range s.outputs {
		if s.frame-o.lastFrame > 2 {
			r.dropLightingOutput(o)
			delete(s.outputs, id)
		}
	}
}

func (r *Renderer) lightingResourceStats() {
	s := &r.lightState
	for _, v := range s.views {
		if !v.shadows.IsZero() {
			r.stats.ShadowBytes += uint64(v.resolution * v.resolution * v.shadowLayers)
			r.stats.LightingResources++
		}
		if !v.cookies.IsZero() {
			r.stats.CookieBytes += maxCookieLayers * cookieResolution * cookieResolution
			r.stats.LightingResources++
		}
	}
	for _, o := range s.outputs {
		r.stats.LightingTargetBytes += o.info.Bytes
		r.stats.LightingResources++
	}
}
