package render

import (
	"fmt"
	"math"
	"time"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/lighting"
	"aego/render/rt"
	"aego/render/shader"
	"aego/render/view"
)

type OutputInfo struct {
	Requested, Active lighting.ColorMode
	Format            gpu.TextureFormat
	Width, Height     int
	Bytes             uint64
	Reason            string
}

type backgroundCheck struct {
	color, linear mathx.Color
	exposure      float32
	generation    uint64
	tested, match bool
}

type lightingOutput struct {
	fxRect        mathx.Rect
	fxCoverage    view.OutputCoverage
	fxOpaque      bool
	fxRegions     []mathx.Rect
	fxCuts        [2][]mathx.Rect
	fxPool        *rt.Pool
	background    backgroundCheck
	fxBackground  backgroundCheck
	info          OutputInfo
	color, depth  gpu.TextureHandle
	lastFrame     uint64
	destination   gpu.TextureHandle
	subresource   gpu.TextureSubresource
	additive      bool
	opaque        bool
	pending       bool
	clears        []mathx.Rect
	environment   lighting.Environment
	hdrFailed     bool
	hdrGeneration uint64
	hdrReason     string
}

func (r *Renderer) LightingOutput(id view.ID) (OutputInfo, bool) {
	o := r.lightState.outputs[id]
	if o == nil {
		return OutputInfo{}, false
	}
	return o.info, true
}

func (r *Renderer) prepareLightingOutput(f *ViewFrame, env lighting.Environment) bool {
	if r.frame == nil {
		return true
	}
	describe, ok := r.dev.(gpu.TextureDescriber)
	var target gpu.TextureDesc
	known := false
	if ok && !f.view.Target.IsZero() {
		target, known = describe.TextureInfo(f.view.Target)
	}
	if f.view.OutputPolicy == view.OutputLinear {
		if !known || target.Format != gpu.TexRGBA16F {
			r.lightingFailure(fmt.Errorf("LGT008: a linear lighting output requires an explicit RGBA16F target"))
			return false
		}
		if f.postProcess == nil {
			// Draw directly; coverage and RGB are preserved until a later composition.
			return true
		}
		env.ColorMode = lighting.HDR
	}
	s := &r.lightState
	if s.outputs == nil {
		s.outputs = make(map[view.ID]*lightingOutput)
	}
	id := f.view.OutputID
	if id == (view.ID{}) {
		id = view.ID{Scope: ^uint64(0), Node: ids.NodeID{Lo: uint64(r.stats.Views)}}
	}
	o := s.outputs[id]
	if o == nil {
		if len(s.outputs) >= maxLightingViews {
			r.lightingFailure(fmt.Errorf("LGT008: more than 16 active lighting outputs"))
			return false
		}
		o = &lightingOutput{}
		s.outputs[id] = o
	}
	first := o.lastFrame != s.frame || !o.pending
	if !first && (o.destination != f.view.Target || o.subresource != f.view.TargetSubresource) {
		r.lightingFailure(fmt.Errorf("LGT008: cameras sharing an output must use the same target subresource"))
		return false
	}
	if !first && o.environment != env {
		r.lightingFailure(fmt.Errorf("LGT003: cameras sharing a lighting output have different environments; use one environment or separate logical outputs"))
		return false
	}
	o.lastFrame = s.frame
	f.outputOriginal = f.view
	f.output = o
	format := gpu.TexRGBA8
	reason := ""
	if env.ColorMode != lighting.LDR {
		if r.dev.Caps().RGBA16FRenderable {
			format = gpu.TexRGBA16F
		} else {
			reason = "RGBA16F render/draw/readback capability is unavailable; using LDR"
		}
	}
	w, h := f.view.TargetW, f.view.TargetH
	if known {
		w = max(1, target.Width>>f.view.TargetSubresource.Mip)
		h = max(1, target.Height>>f.view.TargetSubresource.Mip)
	}
	if f.view.OutputPolicy == view.OutputLinear {
		format = gpu.TexRGBA16F
	}
	if f.view.OutputPolicy != view.OutputLinear && format == gpu.TexRGBA16F && o.hdrFailed && o.hdrGeneration == s.generation && o.info.Width == w && o.info.Height == h {
		format, reason = gpu.TexRGBA8, o.hdrReason
	}
	needsDepth := f.view.Is3D()
	if o.color.IsZero() || o.info.Width != w || o.info.Height != h || o.info.Format != format || needsDepth && o.depth.IsZero() || o.destination != f.view.Target || o.subresource != f.view.TargetSubresource {
		if err := r.resizeLightingOutput(o, w, h, format, needsDepth); err != nil {
			if format == gpu.TexRGBA16F && f.view.OutputPolicy != view.OutputLinear {
				reason = "RGBA16F framebuffer failed: " + err.Error() + "; using LDR"
				o.hdrFailed, o.hdrGeneration, o.hdrReason = true, s.generation, reason
				format = gpu.TexRGBA8
				err = r.resizeLightingOutput(o, w, h, format, needsDepth)
			}
			if err != nil {
				r.lightingFailure(err)
				return false
			}
		}
	}
	if format == gpu.TexRGBA16F {
		o.hdrFailed, o.hdrReason = false, ""
	}
	o.destination, o.subresource = f.view.Target, f.view.TargetSubresource
	o.info.Requested = env.ColorMode
	o.info.Reason = reason
	o.info.Active = lighting.LDR
	if format == gpu.TexRGBA16F {
		o.info.Active = lighting.HDR
	}
	f.outputEnvironment = env
	f.view.TargetW, f.view.TargetH = w, h
	f.trackResolveBounds = f.trackResolveBounds && first
	if first {
		o.environment = env
		o.additive, o.opaque, o.pending = false, false, true
		o.clears = o.clears[:0]
		o.fxOpaque = true
		o.fxCoverage.Reset()
		o.fxRect = mathx.Rect{}
		o.fxRegions = o.fxRegions[:0]
		if !r.caps.ESSL || !f.view.DoClear || !coversTarget(f.view.ClearRegion(), w, h) {
			p := r.frame.BeginPass(gpu.PassDesc{Target: o.color, Color: gpu.LoadClear, ClearRect: mathx.Rect{W: float32(w), H: float32(h)}, Label: "lighting.output-begin"})
			p.End()
		}
	}
	if f.postProcess != nil {
		o.includePostProcessRect(f.view.Viewport, w, h)
		if first && !f.view.DeferResolve {
			o.fxOpaque = f.view.DoClear && f.view.Clear.A >= 1
		} else {
			o.fxCoverage.Add(f.view, 0) // Views arrive in actual submission order.
			if !f.view.DeferResolve {
				o.fxOpaque = o.fxCoverage.Opaque()
			}
		}
	}
	o.additive = o.additive || f.hasAdditive
	if f.view.DoClear {
		rect := f.view.ClearRegion()
		o.clears = append(o.clears, rect)
		if f.view.Clear.A < 1 {
			o.opaque = false
		} else if coversTarget(rect, w, h) {
			o.opaque = true
		}
	}
	f.view.Target, f.view.DepthTarget = o.color, o.depth
	f.view.TargetSubresource, f.view.DepthSubresource = gpu.TextureSubresource{}, gpu.TextureSubresource{}
	return true
}

func (r *Renderer) resizeLightingOutput(o *lightingOutput, w, h int, format gpu.TextureFormat, depth bool) error {
	color, err := r.dev.CreateTexture(gpu.TextureDesc{Width: w, Height: h, Format: format, Label: "lighting.scene"})
	if err != nil {
		return err
	}
	var z gpu.TextureHandle
	if depth {
		z, err = r.dev.CreateTexture(gpu.TextureDesc{Width: w, Height: h, Format: gpu.TexDepth24Stencil8, Label: "lighting.scene-depth"})
		if err != nil {
			r.dev.DestroyTexture(color)
			return err
		}
	}
	desc := gpu.PassDesc{Target: color, DepthTarget: z, Color: gpu.LoadClear, ClearRect: mathx.Rect{W: float32(w), H: float32(h)}, Label: "lighting.output-initialize"}
	if validator, ok := r.dev.(gpu.RenderTargetValidator); ok {
		err = validator.ValidateRenderTarget(desc)
	}
	if err != nil {
		r.dev.DestroyTexture(color)
		if !z.IsZero() {
			r.dev.DestroyTexture(z)
		}
		return err
	}
	r.dropLightingOutput(o)
	o.background = backgroundCheck{}
	o.color, o.depth = color, z
	o.info = OutputInfo{Format: format, Width: w, Height: h, Bytes: uint64(w * h * format.StorageBytesPerPixel())}
	if depth {
		o.info.Bytes += uint64(w * h * 4)
	}
	p := r.frame.BeginPass(desc)
	p.End()
	return nil
}

func (r *Renderer) dropLightingOutput(o *lightingOutput) {
	if o == nil {
		return
	}
	o.closePostProcessPool()
	if !o.color.IsZero() {
		r.dev.DestroyTexture(o.color)
	}
	if !o.depth.IsZero() {
		r.dev.DestroyTexture(o.depth)
	}
}

func (r *Renderer) resolveLighting(f *ViewFrame) {
	o := f.output
	if o == nil {
		return
	}
	f.view = f.outputOriginal
	if f.view.DeferResolve {
		return
	}
	start := time.Now()
	defer func() { r.stats.LightingResolveNS += uint64(time.Since(start)) }()
	o.pending = false
	// A driver can lose its context inside the preceding sprite/material pass.
	// A fresh frame will produce this logical output after restoration.
	if r.contextLost() {
		return
	}
	if f.postProcess != nil && (o.fxRect.W <= 0 || o.fxRect.H <= 0) {
		r.clearPostProcessDestination(f)
		return
	}
	if f.view.OutputPolicy == view.OutputLinear {
		if err := r.resolvePostProcess(f); err != nil {
			if r.contextLost() {
				return
			}
			r.postProcessFailure(err)
			f.postProcess = &linearCopyPlan
			if err := r.resolvePostProcess(f); err != nil && !r.contextLost() {
				r.postProcessFailure(err)
			}
		}
		return
	}
	// A display RGBA image cannot represent background-independent emitted
	// energy with zero coverage. Keep it linear until its final opaque composite.
	if o.additive && !o.opaque && !(f.postProcess != nil && o.fxOpaque) {
		r.lightingFailure(fmt.Errorf("LGT008: transparent display output with Additive needs a linear RGBA16F intermediate or an opaque background before resolve"))
		return
	}
	if f.postProcess != nil {
		if err := r.resolvePostProcess(f); err == nil || r.contextLost() {
			return
		} else {
			r.postProcessFailure(err)
		}
	}
	s := &r.lightState
	if s.resolveShader.IsZero() {
		h, err := r.loadShader("lighting.resolve", "resolve.vert", "resolve.frag")
		if err != nil {
			r.lightingFailure(err)
			return
		}
		s.resolveShader = h
		p, err := r.dev.CreatePipeline(gpu.PipelineDesc{Shader: h, Blend: gpu.BlendPremultiplied(), Primitive: gpu.PrimTriangleStrip, Label: "lighting.resolve"})
		if err != nil {
			r.dev.DestroyShader(h)
			s.resolveShader = gpu.ShaderHandle{}
			r.lightingFailure(err)
			return
		}
		s.resolvePipe = p
		ub, err := r.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferUniform, Size: 16, Dynamic: true, Label: "lighting.resolve"})
		if err != nil {
			r.dev.DestroyPipeline(s.resolvePipe)
			r.dev.DestroyShader(s.resolveShader)
			s.resolvePipe, s.resolveShader = gpu.PipelineHandle{}, gpu.ShaderHandle{}
			r.lightingFailure(err)
			return
		}
		s.resolveUB = ub
		s.resolveData = make([]byte, 16)
	}
	exposure, tone, flip, encode := f.outputEnvironment.Exposure, float32(0), float32(-1), float32(0)
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
	shader.PutLightingVec4(s.resolveData[:], 0, [4]float32{exposure, tone, flip, encode})
	r.dev.UpdateBuffer(s.resolveUB, 0, s.resolveData[:])
	viewport := mathx.Rect{W: float32(o.info.Width), H: float32(o.info.Height)}
	replace := false
	for _, rect := range o.clears {
		if r.caps.ESSL && (f.view.Target.IsZero() && r.fbW == o.info.Width && r.fbH == o.info.Height || !f.view.Target.IsZero()) && coversTarget(rect, o.info.Width, o.info.Height) {
			replace = true
			break
		}
	}
	pipe := s.resolvePipe
	if replace {
		if s.resolveReplace.IsZero() {
			h, err := r.dev.CreatePipeline(gpu.PipelineDesc{Shader: s.resolveShader, Primitive: gpu.PrimTriangleStrip, Label: "lighting.resolve-replace"})
			if err != nil {
				r.lightingFailure(err)
				return
			}
			s.resolveReplace = h
		}
		pipe = s.resolveReplace
	} else {
		for _, rect := range o.clears {
			p := r.frame.BeginPass(gpu.PassDesc{Target: f.view.Target, TargetSubresource: f.view.TargetSubresource, Color: gpu.LoadClear, ClearRect: rect, Label: "lighting.output-clear"})
			p.End()
		}
	}
	desc := gpu.PassDesc{Target: f.view.Target, TargetSubresource: f.view.TargetSubresource, Color: gpu.LoadKeep, Label: "lighting.resolve"}
	var cut, witness mathx.Rect
	var candidate mathx.Color
	check := false
	clipped := false
	probe, canProbe := r.dev.(interface {
		MatchWindowClear(int, int, mathx.Color) bool
	})
	if canProbe && f.trackResolveBounds && f.hasResolveBounds && replace && tone == 1 && encode == 0 && f.view.Target.IsZero() && !f.view.Is3D() && viewport.W == float32(r.fbW) && viewport.H == float32(r.fbH) {
		world, over, screen := r.dbg.Counts()
		if world+over+screen == 0 {
			if rect, ok := f.resolveRect(viewport); ok {
				cached := &o.background
				same := cached.tested && cached.generation == s.generation
				if same && cached.match && cached.color == f.view.Clear && cached.exposure == exposure {
					desc.Color, desc.ClearColor, desc.ClearRect = gpu.LoadClear, cached.linear, viewport
					cut, clipped = rect, true
				} else if !same {
					if c, ok := resolveBackground(f.view.Clear, exposure); ok {
						candidate = c
						witness, check = backgroundWitness(rect, viewport)
					}
				}
			}
		}
	}
	draw := func(desc gpu.PassDesc, rect mathx.Rect, scissor bool) {
		p := r.frame.BeginPass(desc)
		p.SetViewport(viewport)
		if scissor {
			p.SetScissor(rect, true)
		}
		p.SetPipeline(pipe)
		p.SetUniformBuffer(0, s.resolveUB)
		p.SetTexture(0, o.color, r.defSampler)
		p.Draw(0, 4)
		p.End()
	}
	draw(desc, cut, clipped)
	if check {
		matched := probe.MatchWindowClear(int(witness.X), int(witness.Y), candidate)
		o.background = backgroundCheck{color: f.view.Clear, exposure: exposure, generation: s.generation, tested: true, match: matched, linear: candidate}
		if !matched {
			draw(desc, witness, true)
		}
	}
	r.stats.ResolvePasses++
}

func coversTarget(rect mathx.Rect, w, h int) bool {
	return rect.X <= 0 && rect.Y <= 0 && rect.X+rect.W >= float32(w) && rect.Y+rect.H >= float32(h)
}

func (f *ViewFrame) resolveRect(target mathx.Rect) (mathx.Rect, bool) {
	return f.projectBounds(f.resolveBounds, target)
}

func (f *ViewFrame) projectBounds(b mathx.AABB2, target mathx.Rect) (mathx.Rect, bool) {
	v := f.view
	m := v.ViewMatrix()
	extent := max(mathx.Abs(b.Min.X), mathx.Abs(b.Min.Y), mathx.Abs(b.Max.X), mathx.Abs(b.Max.Y))
	if extent*(mathx.Abs(m[0])+mathx.Abs(m[1])+mathx.Abs(m[3])+mathx.Abs(m[4]))+mathx.Abs(m[6])+mathx.Abs(m[7]) > 1<<20 {
		return mathx.Rect{}, false
	}
	p0, p1 := v.WorldToScreen(b.Min), v.WorldToScreen(b.Max)
	p2, p3 := v.WorldToScreen(mathx.Vec2{X: b.Min.X, Y: b.Max.Y}), v.WorldToScreen(mathx.Vec2{X: b.Max.X, Y: b.Min.Y})
	x0 := max(0, float32(math.Floor(float64(min(p0.X, p1.X, p2.X, p3.X))))-2)
	y0 := max(0, float32(math.Floor(float64(min(p0.Y, p1.Y, p2.Y, p3.Y))))-2)
	x1 := min(target.W, float32(math.Ceil(float64(max(p0.X, p1.X, p2.X, p3.X))))+2)
	y1 := min(target.H, float32(math.Ceil(float64(max(p0.Y, p1.Y, p2.Y, p3.Y))))+2)
	r := mathx.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
	return r, r.W > 0 && r.H > 0 && r.W*r.H < target.W*target.H*0.95
}

// Only clear directly if HDR storage rounding cannot change the displayed byte.
func resolveBackground(c mathx.Color, exposure float32) (mathx.Color, bool) {
	if c.A != 1 || !(exposure > 0 && exposure <= 65504) {
		return mathx.Color{}, false
	}
	result := mathx.Color{A: 1}
	for i, x := range [3]float32{c.R, c.G, c.B} {
		if !(x >= 0 && x <= 65504) {
			return mathx.Color{}, false
		}
		e := x/1024 + 1.0/(1<<24)
		encode := func(v float32) int {
			v *= exposure
			return int(mathx.Clamp(mathx.LinearToSRGB(v/(1+v)), 0, 1)*255 + .5)
		}
		lo, hi := encode(max(0, x-e)), encode(min(65504, x+e))
		if lo != hi {
			return mathx.Color{}, false
		}
		val := mathx.SRGBToLinear(float32(lo) / 255)
		switch i {
		case 0:
			result.R = val
		case 1:
			result.G = val
		case 2:
			result.B = val
		}
	}
	return result, true
}

func backgroundWitness(cut, target mathx.Rect) (mathx.Rect, bool) {
	for _, p := range [4]mathx.Vec2{{}, {X: target.W - 8}, {Y: target.H - 8}, {X: target.W - 8, Y: target.H - 8}} {
		r := mathx.Rect{X: p.X, Y: p.Y, W: 8, H: 8}
		if p.X >= 0 && p.Y >= 0 && target.W >= 8 && target.H >= 8 && !r.ToAABB().Intersects(cut.ToAABB()) {
			return r, true
		}
	}
	return mathx.Rect{}, false
}
