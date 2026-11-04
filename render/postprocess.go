package render

import (
	"fmt"
	"math"

	"aego/core/log"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/lighting"
	"aego/render/postfx"
	"aego/render/rt"
	"aego/render/shader"
	"aego/render/view"
)

const postProcessBlock = 128

type postProcessProgram struct {
	shader gpu.ShaderHandle
	pipe   [2]gpu.PipelineHandle
	err    error
}

type postProcessState struct {
	programs   map[*postfx.Descriptor]*postProcessProgram
	uniform    gpu.BufferHandle
	auxUniform [4]gpu.BufferHandle
	data       []byte
	err        error
	lastError  string
}

// PostProcess borrows the effective stack until End. All composing views
// receive the same plan; only the last view submits it.
func (f *ViewFrame) PostProcess(p *postfx.Plan) {
	f.postProcess = nil
	f.brightThreshold = 0
	if p == nil || len(p.Passes) == 0 {
		return
	}
	if f.view.OutputPolicy == view.OutputLinear {
		for _, pass := range p.Passes {
			if pass.Descriptor == nil || pass.Phase() != postfx.SceneLinear {
				return
			}
		}
	}
	f.postProcess = p
	first := &p.Passes[0]
	if f.submission == 0 && first.Descriptor != nil && first.Descriptor.LumaThreshold != nil {
		f.brightThreshold = first.Descriptor.LumaThreshold(first.Params)
	}
}

func (r *Renderer) PostProcessOwnerConflict(id view.ID) {
	r.postProcessFailure(fmt.Errorf("multiple PostProcess owners on output %v", id))
}

func (r *Renderer) PostProcessError() error { return r.fx.err }

func (r *Renderer) postProcessFailure(err error) {
	s := &r.fx
	s.err = err
	if msg := err.Error(); msg != s.lastError {
		s.lastError = msg
		r.log.Warn("post-processing bypassed", log.F("reason", msg))
	}
	r.stats.PostFXFailures++
}

func (o *lightingOutput) includePostProcessRect(rect mathx.Rect, w, h int) {
	cut := view.RasterRect(rect, w, h)
	if cut.W <= 0 || cut.H <= 0 {
		return
	}
	x, y := int(cut.X), int(cut.Y)
	x1, y1 := x+int(cut.W), y+int(cut.H)
	o.addPostProcessRegion(cut)
	if b := o.fxRect; b.W > 0 && b.H > 0 {
		x1, y1 = max(x1, int(b.X+b.W)), max(y1, int(b.Y+b.H))
		x, y = min(x, int(b.X)), min(y, int(b.Y))
	}
	o.fxRect = mathx.Rect{X: float32(x), Y: float32(y), W: float32(x1 - x), H: float32(y1 - y)}
}

// Keep the union disjoint: the final source-over draw must never blend an
// overlapping camera twice or paint an unrelated output between two cameras.
func (o *lightingOutput) addPostProcessRegion(rect mathx.Rect) {
	if len(o.fxRegions) == 0 {
		o.fxRegions = append(o.fxRegions, rect)
		return
	}
	cur, next := append(o.fxCuts[0][:0], rect), o.fxCuts[1][:0]
	for _, b := range o.fxRegions {
		next = next[:0]
		for _, a := range cur {
			x, y := max(a.X, b.X), max(a.Y, b.Y)
			right, bottom := a.X+a.W, a.Y+a.H
			x1, y1 := min(right, b.X+b.W), min(bottom, b.Y+b.H)
			if x >= x1 || y >= y1 {
				next = append(next, a)
				continue
			}
			if a.Y < y {
				next = append(next, mathx.Rect{X: a.X, Y: a.Y, W: a.W, H: y - a.Y})
			}
			if y1 < bottom {
				next = append(next, mathx.Rect{X: a.X, Y: y1, W: a.W, H: bottom - y1})
			}
			if a.X < x {
				next = append(next, mathx.Rect{X: a.X, Y: y, W: x - a.X, H: y1 - y})
			}
			if x1 < right {
				next = append(next, mathx.Rect{X: x1, Y: y, W: right - x1, H: y1 - y})
			}
		}
		cur, next = next, cur
		if len(cur) == 0 {
			break
		}
	}
	o.fxRegions = append(o.fxRegions, cur...)
	o.fxCuts[0], o.fxCuts[1] = cur, next
}

func (o *lightingOutput) closePostProcessPool() {
	if o.fxPool != nil {
		o.fxPool.Close()
		o.fxPool = nil
	}
}

func (r *Renderer) closePostProcess() {
	s := &r.fx
	for _, p := range s.programs {
		for _, h := range p.pipe {
			r.destroyPipe(h)
		}
		r.destroyShader(p.shader)
	}
	for _, h := range s.auxUniform {
		if !h.IsZero() {
			r.dev.DestroyBuffer(h)
		}
	}
	if !s.uniform.IsZero() {
		r.dev.DestroyBuffer(s.uniform)
	}
	*s = postProcessState{}
}

func (r *Renderer) postProcessProgram(d *postfx.Descriptor, final, auxiliary bool) error {
	s := &r.fx
	if s.programs == nil {
		s.programs = make(map[*postfx.Descriptor]*postProcessProgram)
	}
	p := s.programs[d]
	if p == nil {
		p = &postProcessProgram{}
		s.programs[d] = p
		header := "#version 330 core\n"
		if r.caps.ESSL {
			header = "#version 300 es\nprecision highp float;\nprecision highp int;\nprecision highp sampler2D;\n"
		}
		fragment := postProcessFragment
		if auxiliary {
			header += "#define AEGO_AUX_PASS 1\n"
		} else if d.CenterAuxSample {
			fragment = shader.PostProcessBoundedAux
		}
		p.shader, p.err = r.dev.CreateShader(gpu.ShaderDesc{Label: d.ID, Sources: []gpu.ShaderSource{
			{Stage: gpu.StageVertex, Source: header + postProcessUniforms + postProcessVertex},
			{Stage: gpu.StageFragment, Source: header + postProcessUniforms + fragment + d.Fragment + postProcessMain},
		}})
	}
	i := 0
	blend := gpu.BlendState{}
	if final {
		i, blend = 1, gpu.BlendPremultiplied()
	}
	if p.err == nil && p.pipe[i].IsZero() {
		p.pipe[i], p.err = r.dev.CreatePipeline(gpu.PipelineDesc{Label: d.ID, Shader: p.shader, Blend: blend, Primitive: gpu.PrimTriangleStrip})
	}
	return p.err
}

var linearCopy = postfx.Descriptor{ID: "postfx.copy-linear", Phase: postfx.SceneLinear,
	Fragment: `vec4 aego_effect(vec4 c, vec2 uv, vec2 size, vec4 params[4]) { return c; }`}
var linearCopyPlan = postfx.Plan{Passes: []postfx.Pass{{Descriptor: &linearCopy}}}

// A full, non-discarding write replaces source-over on transparent black.
func replacePostClear(o *lightingOutput, d *postfx.Descriptor) bool {
	return d != nil && d.WritesAllPixels && !d.UsesMaterial && o.opaque && o.fxOpaque &&
		len(o.clears) == 1 && len(o.fxRegions) == 1 && o.clears[0] == o.fxRect && o.fxRegions[0] == o.fxRect
}

func (r *Renderer) auxiliaryScissor(f *ViewFrame, pass *postfx.Pass) mathx.Rect {
	return r.auxiliaryBounds(f, pass, false)
}

func (r *Renderer) auxiliaryBounds(f *ViewFrame, pass *postfx.Pass, brightOnly bool) mathx.Rect {
	o, d := f.output, pass.Descriptor
	full := mathx.Rect{W: float32(o.info.Width), H: float32(o.info.Height)}
	if d.AuxiliarySupport == nil || !f.trackResolveBounds || !f.hasResolveBounds || o.fxRect != full ||
		len(o.clears) != 1 || o.clears[0] != full {
		return mathx.Rect{}
	}
	if world, over, screen := r.dbg.Counts(); world+over+screen != 0 {
		return mathx.Rect{}
	}
	padding, ok := d.AuxiliarySupport(f.view.Clear, pass.Params)
	if !ok {
		return mathx.Rect{}
	}
	b := f.resolveBounds
	if brightOnly && d.LumaThreshold != nil && f.brightThreshold > 0 && d.LumaThreshold(pass.Params) == f.brightThreshold {
		if !f.hasBrightBounds {
			// Every input sample is below threshold. One valid texel keeps the
			// normal auxiliary chain and its black-outside clear intact.
			return mathx.Rect{W: 1, H: 1}
		}
		b = f.brightBounds
	}
	bounds, bounded := f.projectBounds(b, full)
	if !bounded {
		return mathx.Rect{}
	}
	// Round outwards using actual (ceil-divided) half sizes on odd viewports.
	hw, hh := float32((int(full.W)+1)/2), float32((int(full.H)+1)/2)
	x := max(0, float32(math.Floor(float64((bounds.X-padding)*hw/full.W))))
	y := max(0, float32(math.Floor(float64((bounds.Y-padding)*hh/full.H))))
	x1 := min(hw, float32(math.Ceil(float64((bounds.X+bounds.W+padding)*hw/full.W))))
	y1 := min(hh, float32(math.Ceil(float64((bounds.Y+bounds.H+padding)*hh/full.H))))
	if (x1-x)*(y1-y) >= .85*hw*hh {
		return mathx.Rect{}
	}
	return mathx.Rect{X: x, Y: y, W: x1 - x, H: y1 - y}
}

func upscaleSupport(clip, dst mathx.Rect) mathx.Rect {
	hw, hh := float32((int(dst.W)+1)/2), float32((int(dst.H)+1)/2)
	x := max(0, float32(math.Floor(float64((clip.X-1)*dst.W/hw))))
	y := max(0, float32(math.Floor(float64((clip.Y-1)*dst.H/hh))))
	x1 := min(dst.W, float32(math.Ceil(float64((clip.X+clip.W+1)*dst.W/hw))))
	y1 := min(dst.H, float32(math.Ceil(float64((clip.Y+clip.H+1)*dst.H/hh))))
	return mathx.Rect{X: x, Y: y, W: x1 - x, H: y1 - y}
}

func (r *Renderer) resolvePostProcess(f *ViewFrame) error {
	s, o, passes := &r.fx, f.output, f.postProcess.Passes
	if len(passes) == 0 {
		return fmt.Errorf("empty post-processing plan at resolve")
	}
	replace := replacePostClear(o, passes[len(passes)-1].Descriptor)
	sparseCopy := len(passes) == 1 && replace && passes[0].Descriptor.LumaThreshold != nil &&
		f.trackResolveBounds && f.sceneGroups.count > 0 && r.postProcessProgram(&linearCopy, false, false) == nil
	display := false
	var half [2]*rt.Target
	needHalf := false
	for i, p := range passes {
		d := p.Descriptor
		if d == nil {
			return fmt.Errorf("post-processing: missing descriptor at %d", i)
		}
		if display && p.Phase() == postfx.SceneLinear {
			return fmt.Errorf("%s: scene effect follows a display effect", d.ID)
		}
		if d.Alpha == postfx.OpaqueOnly && !o.opaque && !o.fxOpaque {
			return fmt.Errorf("%s requires an opaque output", d.ID)
		}
		display = display || p.Phase() == postfx.DisplayLinear
		if d.UsesMaterial {
			if _, ok := r.mats.PreparePost(p.Material, i == len(passes)-1); !ok {
				return fmt.Errorf("%s: postfx material or pipeline not ready", d.ID)
			}
		} else if err := r.postProcessProgram(d, i == len(passes)-1 && !replace, false); err != nil {
			return fmt.Errorf("%s: %w", d.ID, err)
		}
		if !p.LUTID.IsZero() && (p.LUT.IsZero() || (p.LUTSize != 16 && p.LUTSize != 32)) {
			return fmt.Errorf("%s: unresolved or invalid LUT", d.ID)
		}
		for j, sub := range d.Auxiliary() {
			needHalf = true
			if s.auxUniform[j].IsZero() {
				h, err := r.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferUniform, Size: postProcessBlock, Dynamic: true, Label: "postfx.aux-params"})
				if err != nil {
					return err
				}
				s.auxUniform[j] = h
			}
			if err := r.postProcessProgram(sub, false, true); err != nil {
				return err
			}
		}

	}
	if s.uniform.IsZero() {
		var err error
		s.uniform, err = r.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferUniform, Size: postProcessBlock, Dynamic: true, Label: "postfx.params"})
		if err != nil {
			return err
		}
		s.data = make([]byte, postProcessBlock)
	}

	// Validate the complete chain before touching the destination. At most two
	// targets are live regardless of stack length, including failure paths.
	rect := o.fxRect
	var targets [2]*rt.Target
	n := min(len(passes)-1, len(targets))
	if (n > 0 || needHalf) && o.fxPool == nil {
		o.fxPool = rt.New(r.dev, o.info.Format, "postfx.scratch")
	}
	if n > 0 || needHalf {
		defer func() {
			for _, t := range targets {
				o.fxPool.Release(t)
			}
			for _, t := range half {
				o.fxPool.Release(t)
			}
			keep := 2
			if needHalf {
				keep += 2
			}
			o.fxPool.Trim(keep)
		}()
	}
	for i := 0; i < n; i++ {
		t, err := o.fxPool.Acquire(int(rect.W), int(rect.H), false)
		if err != nil {
			return err
		}
		targets[i] = t
		r.stats.PostFXTargets++
		if v, ok := r.dev.(gpu.RenderTargetValidator); ok {
			if err := v.ValidateRenderTarget(gpu.PassDesc{Target: t.Texture}); err != nil {
				return err
			}
		}
	}
	if needHalf {
		for i := range half {
			t, err := o.fxPool.Acquire((int(rect.W)+1)/2, (int(rect.H)+1)/2, false)
			if err != nil {
				return err
			}
			half[i] = t
			r.stats.PostFXTargets++
			if val, ok := r.dev.(gpu.RenderTargetValidator); ok {
				if err = val.ValidateRenderTarget(gpu.PassDesc{Target: t.Texture}); err != nil {
					return err
				}
			}
		}
	}
	if v, ok := r.dev.(gpu.RenderTargetValidator); ok {
		if err := v.ValidateRenderTarget(gpu.PassDesc{Target: f.view.Target, TargetSubresource: f.view.TargetSubresource}); err != nil {
			return err
		}
	}
	src := o.color
	for i := range passes {
		dst := f.view.Target
		if i != len(passes)-1 {
			dst = targets[i%2].Texture
		}
		if src == dst || !dst.IsZero() && (passes[i].LUT == dst || passes[i].Material != nil && passes[i].Material.UsesTexture(dst)) {
			return fmt.Errorf("post-processing pass %d reads its render attachment", i)
		}
		src = dst
	}
	exposure, tone, flip, encode := r.postProcessResolveParameters(f)
	src, srcRect := o.color, rect
	display = false
	if r.inspection.active {
		defer func() { r.inspection.instance = "" }()
	}
	for i, pass := range passes {
		if r.inspection.active {
			r.inspection.instance = pass.InstanceID
		}
		last := i == len(passes)-1
		d := pass.Descriptor
		toDisplay := !display && pass.Phase() == postfx.DisplayLinear
		display = display || toDisplay
		pd := gpu.PassDesc{Color: gpu.LoadKeep, Label: d.ID}
		dstRect := mathx.Rect{W: rect.W, H: rect.H}
		params := [4]float32{exposure, tone, 1, 0}
		var ops [4]float32
		if toDisplay {
			ops[0] = 1
		}
		var pipe gpu.PipelineHandle
		if d.UsesMaterial {
			pipe, _ = r.mats.PreparePost(pass.Material, last)
		} else if last && !replace {
			pipe = s.programs[d].pipe[1]
		} else {
			pipe = s.programs[d].pipe[0]
		}
		var extra gpu.TextureHandle
		var clip, sceneClip, finalClip, witness mathx.Rect
		var candidate mathx.Color
		var verify bool
		var info [4]float32
		if !pass.LUT.IsZero() {
			extra = pass.LUT
			info[0] = float32(pass.LUTSize)
		}
		if subs := d.Auxiliary(); len(subs) > 0 {
			if i == 0 {
				sceneClip = r.auxiliaryScissor(f, &pass)
				clip = r.auxiliaryBounds(f, &pass, true)
			}
			extra, info = r.runPostAux(src, srcRect, rect, half, d, pass.Params, clip)
			if d.CenterAuxSample {
				dst := mathx.Rect{W: info[0], H: info[1]}
				guard, _ := auxSampleWindow(clip, dst)
				ops[2], ops[3] = guard.X, guard.Y
				info[2], info[3] = guard.X+guard.W, guard.Y+guard.H
			}
		}
		if last {
			if !replace {
				r.clearPostProcessDestination(f)
			} else {
				r.stats.PostFXReplacedClears++
			}
			pd.Target, pd.TargetSubresource, dstRect = f.view.Target, f.view.TargetSubresource, rect
			params[2], params[3] = flip, encode
			if !display && f.view.OutputPolicy != view.OutputLinear {
				ops[1] = 1
			}
		} else {
			pd.Target = targets[i%2].Texture
		}
		probe, canProbe := r.dev.(interface {
			MatchWindowClear(int, int, mathx.Color) bool
		})
		if last && i == 0 && replace && d.PreservesOutsideAux && sceneClip.W > 0 && canProbe &&
			f.view.Target.IsZero() && tone == 1 && encode == 0 && dstRect.W == float32(r.fbW) && dstRect.H == float32(r.fbH) {
			// Include the outer texel sampled by the bilinear upscale. The earlier
			// support proof already includes the source geometry and every blur tap.
			area := upscaleSupport(sceneClip, dstRect)
			cached := &o.fxBackground
			if cached.tested && cached.generation == r.lightState.generation && cached.color == f.view.Clear && cached.exposure == exposure {
				if cached.match {
					pd.Color, pd.ClearColor, pd.ClearRect = gpu.LoadClear, cached.linear, dstRect
					finalClip = area
				}
			} else if value, ok := resolveBackground(f.view.Clear, exposure); ok {
				candidate = value
				witness, verify = backgroundWitness(area, dstRect)
			}
		}
		shader.PutLightingVec4(s.data, 0, params)
		shader.PutLightingVec4(s.data, 16, [4]float32{srcRect.X, srcRect.Y, srcRect.W, srcRect.H})
		shader.PutLightingVec4(s.data, 32, ops)
		for j, v := range pass.Params {
			shader.PutLightingVec4(s.data, 48+16*j, v)
		}
		shader.PutLightingVec4(s.data, 112, info)
		r.dev.UpdateBuffer(s.uniform, 0, s.data)
		p := r.frame.BeginPass(pd)
		p.SetViewport(dstRect)
		p.SetPipeline(pipe)
		p.SetUniformBuffer(0, s.uniform)
		p.SetTexture(0, src, r.defSampler)
		if extra.IsZero() && d.LUT {
			extra = r.white.Texture
		} // complete active tex1 even when the LUT is absent
		if !extra.IsZero() {
			p.SetTexture(1, extra, r.Sampler(gpu.SamplerDesc{MinFilter: gpu.FilterLinear, MagFilter: gpu.FilterLinear}))
		}
		if d.UsesMaterial {
			r.mats.BindPostParameters(p, pass.Material)
		}
		if finalClip.W > 0 && clip.W > 0 && clip != sceneClip && sparseCopy {
			f.drawSparsePost(p, pipe, s.programs[&linearCopy].pipe[0], upscaleSupport(clip, dstRect), finalClip, dstRect)
		} else if finalClip.W > 0 {
			p.SetScissor(finalClip, true)
			p.Draw(0, 4)
		} else if last && len(o.fxRegions) > 1 {
			var area float64
			for _, region := range o.fxRegions {
				area += float64(region.W) * float64(region.H)
			}
			if area < float64(rect.W)*float64(rect.H) {
				for _, region := range o.fxRegions {
					p.SetScissor(region, true)
					p.Draw(0, 4)
				}
			} else {
				p.Draw(0, 4)
			}
		} else {
			p.Draw(0, 4)
		}
		if verify {
			matched := probe.MatchWindowClear(int(witness.X), int(witness.Y), candidate)
			o.fxBackground = backgroundCheck{color: f.view.Clear, exposure: exposure, generation: r.lightState.generation, tested: true, match: matched, linear: candidate}
			if !matched {
				p.SetScissor(witness, true)
				p.Draw(0, 4)
			}
		}
		p.End()
		src, srcRect = pd.Target, dstRect
		r.stats.PostFXPasses++
	}
	r.stats.ResolvePasses++
	return nil
}

const postProcessUniforms = shader.PostProcessUniforms
const postProcessVertex = shader.PostProcessVertex
const postProcessFragment = shader.PostProcessFragment
const postProcessMain = shader.PostProcessMain

func (r *Renderer) postProcessResolveParameters(f *ViewFrame) (exposure, tone, flip, encode float32) {
	o := f.output
	exposure, flip = f.outputEnvironment.Exposure, -1
	if o.info.Active == lighting.HDR {
		tone = 1
	}
	if f.lighting != nil && f.lighting.Debug != lighting.DebugLighting {
		exposure, tone = 1, 0
	}
	if f.view.Flipped(r.caps) {
		flip = 1
	}
	if describer, ok := r.dev.(gpu.TextureDescriber); ok && !f.view.Target.IsZero() {
		if desc, known := describer.TextureInfo(f.view.Target); known && desc.Format == gpu.TexRGBA8 {
			encode = 1
		}
	}
	return
}

func (r *Renderer) clearPostProcessDestination(f *ViewFrame) {
	o := f.output
	// Camera clears replace the destination only in their requested rectangles.
	// Resolve then composites once over any retained editor/display backdrop.
	// The scene's actual clear color is already part of the linear intermediate.
	for _, rect := range o.clears {
		p := r.frame.BeginPass(gpu.PassDesc{Target: f.view.Target, TargetSubresource: f.view.TargetSubresource, Color: gpu.LoadClear, ClearRect: rect, Label: "lighting.output-clear"})
		p.End()
	}
}

// The final auxiliary texture can be sampled by the full-frame composite. Its
// pixels outside clip must be black. Earlier textures are read solely by their
// immediate successor. Draw its entire input footprint, including a texel on
// each side for bilinear filtering, so stale pool contents cannot contribute.
func auxiliaryRegions(d *postfx.Descriptor, params [4][4]float32, clip, dst, output mathx.Rect) ([4]mathx.Rect, bool) {
	defaults := [4]mathx.Rect{clip, clip, clip, clip}
	if clip.W <= 0 || d.HalfReadOffset == nil {
		return defaults, false
	}
	regions := defaults
	for i := len(d.Auxiliary()) - 1; i > 0; i-- {
		x, y := d.HalfReadOffset(i, params)
		if !(x >= 0 && y >= 0) || math.IsInf(float64(x), 0) || math.IsInf(float64(y), 0) {
			return defaults, false
		}
		px := float32(math.Ceil(float64(x*dst.W/output.W))) + 1
		py := float32(math.Ceil(float64(y*dst.H/output.H))) + 1
		consumer := regions[i]
		left, bottom := max(float32(0), consumer.X-px), max(float32(0), consumer.Y-py)
		right, top := min(dst.W, consumer.X+consumer.W+px), min(dst.H, consumer.Y+consumer.H+py)
		regions[i-1] = mathx.Rect{X: left, Y: bottom, W: right - left, H: top - bottom}
	}
	return regions, true
}

// Clamp the center to two black texels past the support. A further two
// texels in the written region cover bilinear filtering and its rounding.
func auxSampleWindow(clip, dst mathx.Rect) (guard, writes mathx.Rect) {
	if clip.W <= 0 {
		return dst, dst
	}
	expand := func(p float32) mathx.Rect {
		x, y := max(0, clip.X-p), max(0, clip.Y-p)
		right, top := min(dst.W, clip.X+clip.W+p), min(dst.H, clip.Y+clip.H+p)
		return mathx.Rect{X: x, Y: y, W: right - x, H: top - y}
	}
	return expand(2), expand(4)
}

// runPostAux works on two half-sized targets and never samples its destination.
// src is held by the caller until the final composite. Targets have already been validated.
func (r *Renderer) runPostAux(src gpu.TextureHandle, source, output mathx.Rect, targets [2]*rt.Target, descriptor *postfx.Descriptor, params [4][4]float32, clip mathx.Rect) (gpu.TextureHandle, [4]float32) {
	s := &r.fx
	lin := r.Sampler(gpu.SamplerDesc{MinFilter: gpu.FilterLinear, MagFilter: gpu.FilterLinear})
	var info [4]float32
	dst := mathx.Rect{W: float32((int(output.W) + 1) / 2), H: float32((int(output.H) + 1) / 2)}
	steps := descriptor.Auxiliary()
	drawClip := clip
	boundedSample := descriptor.CenterAuxSample && clip.W > 0 && descriptor.HalfReadOffset != nil
	if boundedSample {
		_, drawClip = auxSampleWindow(clip, dst)
	}
	regions, bounded := auxiliaryRegions(descriptor, params, drawClip, dst, output)
	for i, sub := range steps {
		t := targets[i%2]
		info = [4]float32{dst.W, dst.H, output.W, output.H}
		shader.PutLightingVec4(s.data, 0, [4]float32{1, 0, 1, 0})
		shader.PutLightingVec4(s.data, 16, [4]float32{source.X, source.Y, source.W, source.H})
		shader.PutLightingVec4(s.data, 32, [4]float32{})
		for j, v := range params {
			shader.PutLightingVec4(s.data, 48+16*j, v)
		}
		shader.PutLightingVec4(s.data, 112, info)
		r.dev.UpdateBuffer(s.auxUniform[i], 0, s.data)
		desc := gpu.PassDesc{Target: t.Texture, Color: gpu.LoadKeep, Label: sub.ID}
		if clip.W > 0 && (!bounded || i == len(steps)-1 && !boundedSample) {
			desc.Color, desc.ClearColor = gpu.LoadClear, mathx.Color{A: 1}
		}
		p := r.frame.BeginPass(desc)
		p.SetViewport(dst)
		area := dst
		if clip.W > 0 {
			p.SetScissor(regions[i], true)
			area = regions[i]
		}
		r.stats.PostFXAuxPixels += uint64(area.W) * uint64(area.H)
		p.SetPipeline(s.programs[sub].pipe[0])
		p.SetUniformBuffer(0, s.auxUniform[i])
		p.SetTexture(0, src, lin)
		p.Draw(0, 4)
		p.End()
		r.stats.PostFXPasses++
		src = t.Texture
		source = dst
	}
	return src, info
}
