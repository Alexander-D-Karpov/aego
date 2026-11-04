package render

import (
	"fmt"
	"time"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/lighting"
	"aego/render/shader"
	"aego/render/texture"
	"aego/render/view"
)

const maxCookieLayers = 32
const cookieResolution = 256
const maxLightingViews = 16

type shadowLayer struct {
	certifiedTop float32
	key          uint64
	light        ids.NodeID
	valid        bool
	active       bool
}
type cookieLayer struct {
	region     texture.Region
	generation uint64
	valid      bool
}
type lightingView struct {
	dropped                  []ids.NodeID
	lastFrame                uint64
	shadows, cookies         gpu.TextureHandle
	resolution, shadowLayers int
	shadowFBOGeneration      uint64
	shadow                   [lighting.MaxShadows]shadowLayer
	cookie                   [maxCookieLayers]cookieLayer
	shadowFor, cookieFor     []int
}

func (r *Renderer) viewLighting(f *ViewFrame) *lightingView {
	s := &r.lightState
	if s.views == nil {
		s.views = make(map[view.ID]*lightingView)
	}
	id := f.view.ID
	if id == (view.ID{}) {
		id = view.ID{Scope: ^uint64(0), Node: ids.NodeID{Lo: uint64(r.stats.Views)}}
	}
	v := s.views[id]
	if v == nil {
		if len(s.views) >= maxLightingViews {
			// Evict the least recently used inactive view; no target handle/label
			// participates in logical identity. Allocation stays bounded in previews.
			var oldest view.ID
			age := ^uint64(0)
			for key, entry := range s.views {
				if entry.lastFrame < age {
					age, oldest = entry.lastFrame, key
				}
			}
			r.dropLightingView(s.views[oldest])
			delete(s.views, oldest)
		}
		v = &lightingView{}
		s.views[id] = v
	}
	v.lastFrame = s.frame
	return v
}

func (r *Renderer) dropLightingView(v *lightingView) {
	if v == nil {
		return
	}
	if !v.shadows.IsZero() {
		r.dev.DestroyTexture(v.shadows)
	}
	if !v.cookies.IsZero() {
		r.dev.DestroyTexture(v.cookies)
	}
}

func (r *Renderer) prepareLightingAux(f *ViewFrame, env lighting.Environment) bool {
	if f.lighting == nil || f.lighting.Disabled || f.litCount == 0 || r.frame == nil {
		return true
	}
	start := time.Now()
	defer func() { r.stats.LightingAuxNS += uint64(time.Since(start)) }()
	s := &r.lightState
	v := r.viewLighting(f)
	f.lightResources = v
	v.dropped = v.dropped[:0]
	for i, drop := range f.droppedLights {
		if drop {
			v.dropped = append(v.dropped, f.lighting.Lights[i].ID)
		}
	}
	for layer := range v.shadow {
		v.shadow[layer].active = false
	}
	n := len(f.lighting.Lights)
	if cap(v.shadowFor) < n {
		v.shadowFor = make([]int, n, max(lighting.MaxShadows, n))
		v.cookieFor = make([]int, n, max(lighting.MaxShadows, n))
	} else {
		v.shadowFor = v.shadowFor[:n]
		v.cookieFor = v.cookieFor[:n]
	}
	for i := 0; i < n; i++ {
		v.shadowFor[i], v.cookieFor[i] = -1, -1
	}
	if cap(s.used) < n {
		s.used = make([]bool, n, max(lighting.MaxShadows, n))
	} else {
		s.used = s.used[:n]
		clear(s.used)
	}
	for _, set := range f.sets {
		for j := 0; j < set.Count; j++ {
			s.used[set.Indices[j]] = true
		}
	}
	var selected [lighting.MaxShadows]int
	count := 0
	for i := range f.lighting.Lights {
		l := &f.lighting.Lights[i]
		if !s.used[i] || !l.Shadows || l.Type == lighting.Directional || l.ShadowCasterMask == 0 {
			continue
		}
		at := count
		for at > 0 && shadowBefore(l, &f.lighting.Lights[selected[at-1]]) {
			at--
		}
		if at >= env.MaxShadowedLights {
			r.stats.DroppedShadowLights++
			continue
		}
		if count < env.MaxShadowedLights {
			count++
		} else {
			r.stats.DroppedShadowLights++
		}
		for j := count - 1; j > at; j-- {
			selected[j] = selected[j-1]
		}
		selected[at] = i
	}
	if count > 0 {
		if !r.prepareShadowArray(v, env.ShadowResolution, env.MaxShadowedLights) {
			return false
		}
		var layers [lighting.MaxShadows]int
		var occupied [lighting.MaxShadows]bool
		for at := 0; at < count; at++ {
			layers[at] = -1
			id := f.lighting.Lights[selected[at]].ID
			for layer := 0; layer < v.shadowLayers; layer++ {
				if v.shadow[layer].valid && v.shadow[layer].light == id && !occupied[layer] {
					layers[at] = layer
					occupied[layer] = true
					break
				}
			}
		}
		for at := 0; at < count; at++ {
			if layers[at] < 0 {
				for layer := 0; layer < v.shadowLayers; layer++ {
					if !occupied[layer] {
						layers[at] = layer
						occupied[layer] = true
						break
					}
				}
			}
		}
		for at := 0; at < count; at++ {
			layer := layers[at]
			index := selected[at]
			light := &f.lighting.Lights[index]
			key := lighting.ShadowKey(light, f.lighting, env.ShadowResolution, s.generation)
			if !v.shadow[layer].valid || v.shadow[layer].key != key {
				start := time.Now()
				verts, err := lighting.BuildShadow(light, f.lighting, s.shadowVerts[:0])
				r.stats.ShadowGeometryNS += uint64(time.Since(start))
				if err != nil {
					r.lightingFailure(err)
					continue
				}
				s.shadowVerts = verts
				if !r.drawShadow(v, layer, verts) {
					continue
				}
				// The filter can extend to the light softness plus one texel
				// of linear support. One additional texel covers raster snapping.
				top := float32(1)
				for _, v := range verts {
					top = min(top, v.Y)
				}
				top = max(0, top-(min(light.ShadowSoftness, 4)+2)/float32(env.ShadowResolution))
				v.shadow[layer] = shadowLayer{key: key, light: light.ID, valid: true, certifiedTop: top}
				r.stats.DirtyShadowMasks++
			} else {
				r.stats.ShadowCacheHits++
			}
			v.shadowFor[index] = layer
			v.shadow[layer].active = true
		}
	}
	// Cookies share one fixed-resolution array. Regions retain aspect ratio,
	// are letterboxed with zero mask, and are copied only on source/generation change.
	usedCookies := 0
	for i := range f.lighting.Lights {
		l := &f.lighting.Lights[i]
		if !s.used[i] || l.Type != lighting.Sprite || l.Cookie.IsZero() {
			continue
		}
		if usedCookies >= maxCookieLayers {
			r.stats.DroppedCookieLights++
			continue
		}
		layer := usedCookies
		usedCookies++
		if v.cookies.IsZero() && !r.prepareCookieArray(v) {
			return false
		}
		if !v.cookie[layer].valid || v.cookie[layer].region != l.Cookie || v.cookie[layer].generation != s.generation {
			if !r.drawCookie(v, layer, l.Cookie) {
				continue
			}
			v.cookie[layer] = cookieLayer{region: l.Cookie, generation: s.generation, valid: true}
			r.stats.CookieCopies++
		}
		v.cookieFor[i] = layer
	}
	return true
}

func shadowBefore(a, b *lighting.Light) bool {
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	// Conservative total emitted influence is camera-independent, so panning a
	// view cannot churn otherwise identical light-space shadow resources.
	ai := a.Intensity * max(a.Color.R, max(a.Color.G, a.Color.B)) * a.Range
	bi := b.Intensity * max(b.Color.R, max(b.Color.G, b.Color.B)) * b.Range
	if ai != bi {
		return ai > bi
	}
	if a.ID.Hi != b.ID.Hi {
		return a.ID.Hi < b.ID.Hi
	}
	return a.ID.Lo < b.ID.Lo
}

func (r *Renderer) prepareShadowArray(v *lightingView, resolution, layers int) bool {
	gen := r.lightState.generation
	sameStorage := !v.shadows.IsZero() && v.resolution == resolution && v.shadowLayers == layers
	if sameStorage && v.shadowFBOGeneration == gen {
		return true
	}
	h := v.shadows
	if !sameStorage {
		var err error
		h, err = r.dev.CreateTexture(gpu.TextureDesc{Dimension: gpu.Texture2DArray, Width: resolution, Height: resolution, Layers: layers, Format: gpu.TexR8, Label: "lighting.shadow-array"})
		if err != nil {
			r.lightingFailure(err)
			return false
		}
	}
	if validator, ok := r.dev.(gpu.RenderTargetValidator); ok {
		// Context restore preserves the logical texture handle, but invalidates
		// every per-layer FBO. Prewarm idle capacity once per generation, just
		// as on first allocation, before publishing any layer for this draw.
		for layer := 0; layer < layers; layer++ {
			if err := validator.ValidateRenderTarget(gpu.PassDesc{Target: h, TargetSubresource: gpu.TextureSubresource{Layer: layer}}); err != nil {
				if !sameStorage {
					r.dev.DestroyTexture(h)
				}
				r.lightingFailure(err)
				return false
			}
		}
	}
	if !sameStorage {
		if !v.shadows.IsZero() {
			r.dev.DestroyTexture(v.shadows)
		}
		v.shadows, v.resolution, v.shadowLayers = h, resolution, layers
		clear(v.shadow[:])
	}
	v.shadowFBOGeneration = gen
	return true
}

func (r *Renderer) drawShadow(v *lightingView, layer int, verts []mathx.Vec2) bool {
	s := &r.lightState
	if s.shadowShader.IsZero() {
		h, err := r.loadShader("lighting.shadow", "shadow.vert", "shadow.frag")
		if err != nil {
			r.lightingFailure(err)
			return false
		}
		s.shadowShader = h
		p, err := r.dev.CreatePipeline(gpu.PipelineDesc{Shader: h, Layout: gpu.VertexLayout{Attrs: []gpu.VertexAttr{{Location: 0, Format: gpu.VFFloat2}}, Strides: [gpu.MaxVertexSlots]uint16{8}}, Primitive: gpu.PrimTriangles, Label: "lighting.shadow-union"})
		if err != nil {
			r.dev.DestroyShader(h)
			s.shadowShader = gpu.ShaderHandle{}
			r.lightingFailure(err)
			return false
		}
		s.shadowPipe = p
	}
	n := len(verts) * 8
	if cap(s.shadowBytes) < n {
		s.shadowBytes = make([]byte, max(n, cap(s.shadowBytes)*2, 8192))
	}
	s.shadowBytes = s.shadowBytes[:n]
	for i, p := range verts {
		putF32(s.shadowBytes[i*8:], p.X)
		putF32(s.shadowBytes[i*8+4:], p.Y)
	}
	if n > s.shadowCapacity {
		h, err := r.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferVertex, Size: max(n, s.shadowCapacity*2, 8192), Dynamic: true, Label: "lighting.shadow-vertices"})
		if err != nil {
			r.lightingFailure(err)
			return false
		}
		if !s.shadowVB.IsZero() {
			r.dev.DestroyBuffer(s.shadowVB)
		}
		s.shadowVB = h
		s.shadowCapacity = max(n, s.shadowCapacity*2, 8192)
	}
	if n > 0 {
		r.dev.UpdateBuffer(s.shadowVB, 0, s.shadowBytes)
		r.stats.ShadowUploadBytes += uint64(n)
	}
	viewport := mathx.Rect{W: float32(v.resolution), H: float32(v.resolution)}
	p := r.frame.BeginPass(gpu.PassDesc{Target: v.shadows, TargetSubresource: gpu.TextureSubresource{Layer: layer}, Color: gpu.LoadClear, ClearColor: mathx.White, ClearRect: viewport, Label: "lighting.shadow"})
	p.SetViewport(viewport)
	if n > 0 {
		p.SetPipeline(s.shadowPipe)
		p.SetVertexBuffer(0, s.shadowVB)
		p.Draw(0, len(verts))
		r.stats.ShadowDraws++
	}
	p.End()
	return true
}

func (r *Renderer) prepareCookieArray(v *lightingView) bool {
	h, err := r.dev.CreateTexture(gpu.TextureDesc{Dimension: gpu.Texture2DArray, Layers: maxCookieLayers, Width: cookieResolution, Height: cookieResolution, Format: gpu.TexR8, Label: "lighting.cookies"})
	if err != nil {
		r.lightingFailure(err)
		return false
	}
	if validator, ok := r.dev.(gpu.RenderTargetValidator); ok {
		if err = validator.ValidateRenderTarget(gpu.PassDesc{Target: h}); err != nil {
			r.dev.DestroyTexture(h)
			r.lightingFailure(err)
			return false
		}
	}
	v.cookies = h
	return true
}

func (r *Renderer) drawCookie(v *lightingView, layer int, region texture.Region) bool {
	s := &r.lightState
	if s.cookieShader.IsZero() {
		h, err := r.loadShader("lighting.cookie", "cookie.vert", "cookie.frag")
		if err != nil {
			r.lightingFailure(err)
			return false
		}
		s.cookieShader = h
		p, err := r.dev.CreatePipeline(gpu.PipelineDesc{Shader: h, Primitive: gpu.PrimTriangleStrip, Label: "lighting.cookie"})
		if err != nil {
			r.dev.DestroyShader(h)
			s.cookieShader = gpu.ShaderHandle{}
			r.lightingFailure(err)
			return false
		}
		s.cookiePipe = p
		ub, err := r.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferUniform, Size: 48, Dynamic: true, Label: "lighting.cookie-map"})
		if err != nil {
			r.dev.DestroyPipeline(s.cookiePipe)
			r.dev.DestroyShader(s.cookieShader)
			s.cookiePipe, s.cookieShader = gpu.PipelineHandle{}, gpu.ShaderHandle{}
			r.lightingFailure(err)
			return false
		}
		s.cookieUB = ub
		s.copyData = make([]byte, 48)
	}
	size := region.Size()
	if size.X <= 0 || size.Y <= 0 {
		r.lightingFailure(fmt.Errorf("LGT006: empty cookie mapping"))
		return false
	}
	fitX, fitY := size.X/max(size.X, size.Y), size.Y/max(size.X, size.Y)
	shader.PutLightingVec4(s.copyData[:], 0, [4]float32{region.UVMin.X, region.UVMin.Y, region.UVMax.X, region.UVMax.Y})
	shader.PutLightingVec4(s.copyData[:], 16, region.LocalRect())
	shader.PutLightingVec4(s.copyData[:], 32, [4]float32{fitX, fitY, 0, 0})
	r.dev.UpdateBuffer(s.cookieUB, 0, s.copyData[:])
	viewport := mathx.Rect{W: cookieResolution, H: cookieResolution}
	p := r.frame.BeginPass(gpu.PassDesc{Target: v.cookies, TargetSubresource: gpu.TextureSubresource{Layer: layer}, Color: gpu.LoadClear, ClearRect: viewport, Label: "lighting.cookie-copy"})
	p.SetViewport(viewport)
	p.SetPipeline(s.cookiePipe)
	p.SetUniformBuffer(0, s.cookieUB)
	p.SetTexture(0, region.Texture, s.sampler)
	p.Draw(0, 4)
	p.End()
	return true
}
