package frame

import (
	"sync/atomic"

	"aego/asset"
	"aego/core/clock"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine"
	"aego/gpu"
	"aego/render"
	renderhost "aego/render/host"
	"aego/render/lighting"
	"aego/render/mesh"
	"aego/render/postfx"
	"aego/render/rt"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/render/view"
	"aego/scene"
)

const (
	SystemTransform = "engine.transform"
	SystemSubmit    = "engine.views_submit"
	SystemPresent   = "engine.present"
	SystemOverlays  = "engine.overlays"
	ServiceID       = "scene.runner"
)

type Stats struct {
	Views         int
	Sprites       int
	Sprites3D     int
	Meshes        int
	MissingAssets int
}

type Bucket struct {
	PostProcess      *postfx.Plan
	PostProcessOwner bool
	Lighting         lighting.Input
	Sprites          []sprite.Draw
	Sprites3D        []sprite.Draw3D
	Meshes           []mesh.Draw
}

func (b *Bucket) reset() {
	b.PostProcess, b.PostProcessOwner = nil, false
	b.Lighting.Reset()
	b.Sprites = b.Sprites[:0]
	b.Sprites3D = b.Sprites3D[:0]
	b.Meshes = b.Meshes[:0]
}

type Runner struct {
	beforeSubmit        func(*Runner)
	postPreview         PostPreview
	previewPlan         postfx.Plan
	lightingOff         bool
	lightingDebug       lighting.DebugMode
	lightingEnvironment ids.NodeID
	identity            uint64
	hidden              func(scene.NodeHandle) bool
	scene               *scene.Scene
	render              *render.Renderer
	res                 asset.Resolver
	views               []view.View
	order               []int32
	perm                []int
	buckets             []Bucket
	stats               Stats
	pixelW              int
	pixelH              int
	scale               float32
	target              gpu.TextureHandle
	depth               gpu.TextureHandle
	targetW             int
	targetH             int
	vpW                 int
	vpH                 int
	fixedBackdrop       func(*render.ViewFrame)
	fixed               view.View
	hasFixed            bool
	preview2DIn3D       bool
	captureView         view.ID
	factor              float32
	rscale              float32
	pooledTarget        bool
	external            bool
	scaling             bool
	pool                *rt.Pool
	scaled              *rt.Target
	outW                int
	outH                int
	outScale            float32
}

var nextIdentity atomic.Uint64

func New() *Runner {
	return &Runner{
		identity: nextIdentity.Add(1),
		views:    make([]view.View, 0, 4),
		order:    make([]int32, 0, 4),
		perm:     make([]int, 0, 4),
		scale:    1,
		factor:   1,
		rscale:   1,
	}
}

// ViewID is stable across transform changes, resize and target replacement.
// A runner owns a separate output even when another world reuses its camera IDs.
func (r *Runner) ViewID(node ids.NodeID) view.ID {
	return view.ID{Scope: r.identity, Node: node}
}

func (r *Runner) OutputID() view.ID { return view.ID{Scope: r.identity} }

// SetLightingView changes only presentation/inspection state owned by this
// runner; it does not write to the edit scene, a prefab or a simulation keyframe.
func (r *Runner) SetLightingView(enabled bool, debug lighting.DebugMode) {
	r.lightingOff = !enabled
	if debug > lighting.DebugShadows {
		debug = lighting.DebugLighting
	}
	r.lightingDebug = debug
}

func (r *Runner) LightingView() (bool, lighting.DebugMode) { return !r.lightingOff, r.lightingDebug }

// SetLightingEnvironmentPreview selects a session-only environment without changing cameras, scene components or simulation state; zero restores the scene's normal camera/output selection.
func (r *Runner) SetLightingEnvironmentPreview(id ids.NodeID) { r.lightingEnvironment = id }
func (r *Runner) LightingEnvironmentPreview() ids.NodeID      { return r.lightingEnvironment }

func (r *Runner) SetTarget(t gpu.TextureHandle, w, h int) {
	r.unscale()
	r.target, r.targetW, r.targetH = t, w, h
	r.external, r.pooledTarget = true, false
}

// SetPooledTarget grants this output a dedicated attachment including bucket padding, which a full logical camera clear may initialize.
// Arbitrary external targets do not grant such ownership implicitly.
func (r *Runner) SetPooledTarget(t *rt.Target) {
	if t == nil {
		r.ClearTarget()
		return
	}
	r.SetTarget(t.Texture, t.BucketW, t.BucketH)
	r.depth, r.vpW, r.vpH, r.pooledTarget = t.Depth, t.W, t.H, true
}

func (r *Runner) SetDepthTarget(d gpu.TextureHandle) {
	r.depth = d
}

func (r *Runner) ClearTarget() {
	r.target, r.targetW, r.targetH = gpu.TextureHandle{}, 0, 0
	r.depth = gpu.TextureHandle{}
	r.vpW, r.vpH = 0, 0
	r.external, r.pooledTarget, r.scaling, r.factor = false, false, false, 1
}

func (r *Runner) SetViewportSize(w, h int) {
	r.vpW, r.vpH, r.pooledTarget = w, h, false
}

// SetFixedBackdrop draws a fixed presentation surface's opaque backdrop before the world; ordinary cameras do not get one.
// When coverage-free additive needs it, backdrop and world share a linear composite.
func (r *Runner) SetFixedBackdrop(draw func(*render.ViewFrame)) { r.fixedBackdrop = draw }

// SetPreview2DIn3D includes ordinary 2D sprites in a 3D editor preview. It
// does not change the scene or the behaviour of runtime cameras.
func (r *Runner) SetPreview2DIn3D(enabled bool) { r.preview2DIn3D = enabled }

func (r *Runner) Preview2DIn3D() bool { return r.preview2DIn3D }

func (r *Runner) SetFixedView(v view.View) {
	r.fixed = v
	r.hasFixed = true
}

func (r *Runner) ClearFixedView() {
	r.hasFixed = false
}

func (r *Runner) Fixed() bool {
	return r.hasFixed
}

func (r *Runner) Bind(s *scene.Scene, rnd *render.Renderer, res asset.Resolver) {
	r.scene = s
	r.render = rnd
	r.res = res
}

func (r *Runner) SetScene(s *scene.Scene) {
	r.scene = s
}

func (r *Runner) Scene() *scene.Scene {
	return r.scene
}

func (r *Runner) Renderer() *render.Renderer {
	return r.render
}

func (r *Runner) Resolver() asset.Resolver {
	return r.res
}

func (r *Runner) Stats() Stats {
	return r.stats
}

func (r *Runner) Counters() *Stats {
	return &r.stats
}

func (r *Runner) Views() []view.View {
	return r.views
}

func (r *Runner) ScreenToWorld(px mathx.Vec2) (mathx.Vec2, bool) {
	best, inside := -1, false
	for i := range r.views {
		v := &r.views[i]
		if v.Is3D() {
			continue
		}
		in := v.LetterboxViewport().Contains(px)
		if best >= 0 && ((inside && !in) || (in == inside && r.order[i] < r.order[best])) {
			continue
		}
		best, inside = i, in
	}
	if best < 0 {
		return mathx.Vec2{}, false
	}
	return r.views[best].ScreenToWorld(px), inside
}

// ViewOrder returns the camera priority used at submission. Equal priorities retain extraction order.
func (r *Runner) ViewOrder(i int) int32 { return r.order[i] }

func (r *Runner) Bucket(i int) *Bucket {
	return &r.buckets[i]
}

func (r *Runner) PixelScale() float32 {
	return r.scale
}

func (r *Runner) Depth() gpu.TextureHandle {
	return r.depth
}

func (r *Runner) Target() (gpu.TextureHandle, int, int) {
	if r.targetW > 0 && r.targetH > 0 {
		return r.target, r.targetW, r.targetH
	}
	return r.target, r.pixelW, r.pixelH
}

func (r *Runner) Viewport(vp mathx.Rect) mathx.Rect {
	if vp.W <= 0 || vp.H <= 0 {
		vp = mathx.Rect{W: 1, H: 1}
	}
	return mathx.Rect{
		X: vp.X * float32(r.pixelW),
		Y: vp.Y * float32(r.pixelH),
		W: vp.W * float32(r.pixelW),
		H: vp.H * float32(r.pixelH),
	}
}

func (r *Runner) SelectCaptureView(id view.ID) { r.captureView = id }

func (r *Runner) AddView(v view.View, order int32) {
	if r.hasFixed || (r.captureView != (view.ID{}) && v.ID != r.captureView) {
		return
	}
	r.push(v, order)
}

func (r *Runner) push(v view.View, order int32) {
	// A full logical clear on the owned pool target also clears padding.
	// Keep sub-viewports and external/shared attachments isolated, including multiple cameras on one output.
	if (r.pooledTarget || r.scaling) && v.Target == r.target && v.TargetSubresource == (gpu.TextureSubresource{}) &&
		v.DoClear && v.Clear.A >= 1 && v.Viewport == (mathx.Rect{W: float32(r.vpW), H: float32(r.vpH)}) {
		v.ClearWholeTarget = true
	}
	if v.ID == (view.ID{}) {
		v.ID = r.OutputID()
	}
	if v.OutputID == (view.ID{}) {
		v.OutputID = r.OutputID()
	}
	r.views = append(r.views, v)
	r.order = append(r.order, order)
	i := len(r.views) - 1
	if i >= len(r.buckets) {
		r.buckets = append(r.buckets, Bucket{})
		return
	}
	r.buckets[i].reset()
}

func (r *Runner) Prepare(pixelW, pixelH int, scale float32) {
	switch {
	case r.vpW > 0 && r.vpH > 0:
		pixelW, pixelH = r.vpW, r.vpH
	case r.targetW > 0 && r.targetH > 0:
		pixelW, pixelH = r.targetW, r.targetH
	}
	f := r.factor
	if f <= 0 {
		f = 1
	}
	r.pixelW, r.pixelH = pixelW, pixelH
	r.scale = scale * f
	r.stats = Stats{}
	r.views = r.views[:0]
	r.order = r.order[:0]
	if r.scene == nil {
		return
	}
	r.scene.Flush()
	r.scene.UpdateTransforms()
	if !r.hasFixed {
		return
	}
	v := r.fixed
	if r.targetW > 0 {
		v.Target, v.TargetW, v.TargetH = r.target, r.targetW, r.targetH
		if v.Is3D() {
			v.DepthTarget = r.depth
		}
	}
	r.push(v, 0)
}

// PostPreview affects one presentation runner's primary output only. It is
// never written to a component, asset, replay or the game settings.
type PostPreview struct {
	Bypass   bool
	Camera   ids.NodeID
	Instance string
}

func (r *Runner) SetPostPreview(v PostPreview) { r.postPreview = v }

func (r *Runner) previewPostPlan(output view.ID, owner ids.NodeID, p *postfx.Plan) *postfx.Plan {
	if output != r.OutputID() || p == nil {
		return p
	}
	v := r.postPreview
	if v.Bypass {
		return nil
	}
	if v.Instance == "" || v.Camera != owner {
		return p
	}
	for i, pass := range p.Passes {
		if pass.InstanceID == v.Instance {
			r.previewPlan.Passes = append(r.previewPlan.Passes[:0], p.Passes[:i]...)
			r.previewPlan.Passes = append(r.previewPlan.Passes, p.Passes[i+1:]...)
			return &r.previewPlan
		}
	}
	return p
}

func (r *Runner) SetPreviewExtract(f func(*Runner)) { r.beforeSubmit = f }

func (r *Runner) Submit(dst *render.Renderer) {
	r.stats.Views = len(r.views)
	if dst == nil || len(r.views) == 0 {
		return
	}
	if r.beforeSubmit != nil {
		r.beforeSubmit(r)
	}
	r.perm = r.perm[:0]
	for i := range r.views {
		r.perm = append(r.perm, i)
	}
	for i := 1; i < len(r.perm); i++ {
		for j := i; j > 0 && r.order[r.perm[j]] < r.order[r.perm[j-1]]; j-- {
			r.perm[j], r.perm[j-1] = r.perm[j-1], r.perm[j]
		}
	}
	for _, i := range r.perm {
		v := r.views[i]
		b := &r.buckets[i]
		var plan *postfx.Plan
		owners := 0
		var owner ids.NodeID
		// All cameras of one output compose in linear space before one resolve, including earlier Unlit cameras when later cameras have Lit receivers.
		// Material and painter order stay unchanged.
		for _, other := range r.perm {
			if r.views[other].OutputID != v.OutputID {
				continue
			}
			ob := &r.buckets[other]
			if ob.PostProcessOwner {
				owners++
				owner = r.views[other].ID.Node
				plan = ob.PostProcess
			}
			for _, d := range ob.Sprites {
				if d.Material != nil && d.Material.Shader().Features&shader.FeatureLighting != 0 {
					b.Lighting.OutputLit = true
					break
				}
			}
			if !b.Lighting.OutputLit {
				for _, d := range ob.Meshes {
					if d.Material != nil && d.Material.Shader().Features&shader.FeatureLighting != 0 {
						b.Lighting.OutputLit = true
						break
					}
				}
			}
			if other != i && ob.Lighting.HasEnvironment && !b.Lighting.HasEnvironment {
				b.Lighting.Environment = ob.Lighting.Environment
				b.Lighting.HasEnvironment = true
			}
		}
		seen := false
		for _, other := range r.perm {
			if other == i {
				seen = true
				continue
			}
			if seen && r.views[other].OutputID == v.OutputID {
				v.DeferResolve = true
				break
			}
		}
		if owners > 1 {
			plan = nil
			if !v.DeferResolve {
				dst.PostProcessOwnerConflict(v.OutputID)
			}
		}
		plan = r.previewPostPlan(v.OutputID, owner, plan)
		if r.hasFixed && r.fixedBackdrop != nil {
			r.drawFixedBackdrop(dst, v, b, plan)
			v.DoClear = false
		}
		f := dst.View(v)
		f.PostProcess(plan)
		f.Lighting(&b.Lighting)
		if v.Is3D() {
			f.Meshes(b.Meshes)
			f.Sprites3D(b.Sprites3D)
		} else {
			f.Meshes(b.Meshes)
			f.Sprites(b.Sprites)
		}
		f.End()
	}
	if len(r.previewPlan.Passes) > 0 {
		clear(r.previewPlan.Passes[:cap(r.previewPlan.Passes)])
		r.previewPlan.Passes = r.previewPlan.Passes[:0]
	}
}

func (r *Runner) drawFixedBackdrop(dst *render.Renderer, v view.View, b *Bucket, plan *postfx.Plan) {
	v.Label = "presentation.backdrop"
	v.Clear.A, v.DoClear, v.DeferResolve = 1, true, false
	// Source-over leaves the display backdrop below the resolved world; additive preserves coverage and needs the backdrop in linear space before exposure/tone mapping.
	linear := (plan != nil && len(plan.Passes) != 0 || b.Lighting.OutputLit || b.Lighting.HasEnvironment && b.Lighting.Environment.ColorMode == lighting.HDR) && b.additive()
	if linear {
		v.DeferResolve, v.ClearWholeTarget = true, true
	}
	f := dst.View(v)
	if linear {
		f.PostProcess(plan)
		f.Lighting(&b.Lighting)
	}
	r.fixedBackdrop(f)
	f.End()
}

func (b *Bucket) additive() bool {
	for i := range b.Sprites {
		d := &b.Sprites[i]
		if d.Blend == sprite.BlendAdditive || d.Material != nil && d.Material.Blend() == shader.BlendAdditive {
			return true
		}
	}
	for i := range b.Meshes {
		d := &b.Meshes[i]
		if d.Blend == sprite.BlendAdditive || d.Material != nil && d.Material.Blend() == shader.BlendAdditive {
			return true
		}
	}
	return false
}

func Of(ctx *engine.Context) *Runner {
	if ctx.World != nil {
		if r, ok := engine.ServiceOf[*Runner](ctx.World, ServiceID); ok && r != nil {
			return r
		}
	}
	if ctx.Engine != nil {
		if r, ok := engine.Service[*Runner](ctx.Engine, ServiceID); ok && r != nil {
			return r
		}
	}
	return nil
}

func Register(reg *engine.Registry) error {
	sys := reg.Systems()
	if err := sys.Register(engine.SystemDesc{
		ID:     SystemTransform,
		Phase:  engine.PhaseRenderPrepare,
		Scope:  engine.ScopeWorld,
		System: transform{},
	}); err != nil {
		return err
	}
	if err := sys.Register(engine.SystemDesc{
		ID:     SystemSubmit,
		Phase:  engine.PhaseRender,
		Scope:  engine.ScopeWorld,
		Before: []string{engine.SystemAppRender},
		System: submit{},
	}); err != nil {
		return err
	}
	var end []string
	if _, ok := sys.ScopeOf(renderhost.SystemEnd); ok {
		end = []string{renderhost.SystemEnd}
	}
	if err := sys.Register(engine.SystemDesc{
		ID:     SystemPresent,
		Phase:  engine.PhaseRenderUI,
		Scope:  engine.ScopeWorld,
		Before: end,
		System: present{},
	}); err != nil {
		return err
	}
	return sys.Register(engine.SystemDesc{
		ID:     SystemOverlays,
		Phase:  engine.PhaseRenderUI,
		Scope:  engine.ScopeWorld,
		After:  []string{SystemPresent},
		Before: end,
		System: anchor{},
	})
}

type transform struct{}

func (transform) Run(ctx *engine.Context) {
	r := Of(ctx)
	if r == nil {
		return
	}
	m := ctx.Engine.Metrics()
	d := m.Density()
	r.scaleTarget(ctx.Engine.GPU(), m.PixelW, m.PixelH, d)
	r.Prepare(m.PixelW, m.PixelH, d)
}

type submit struct{}

func (submit) Run(ctx *engine.Context) {
	if r := Of(ctx); r != nil {
		if r.render != nil && ctx.Time != nil {
			r.render.SetClock(render.Clock{
				Time: float32(ctx.Time.Span(clock.Game).Elapsed), Delta: float32(ctx.Dt),
				Unscaled: float32(ctx.Time.Span(clock.Real).Elapsed), Frame: ctx.Frame,
			})
		}
		r.Submit(r.render)
	}
}

type Output struct {
	Target  gpu.TextureHandle
	W, H    int
	TargetW int
	TargetH int
	Scale   float32
}

func (r *Runner) SetRenderScale(k float32) {
	r.rscale = k
}

func (r *Runner) RenderScale() float32 {
	if r.scaling {
		return r.factor
	}
	return 1
}

func (r *Runner) scaleTarget(dev gpu.Device, w, h int, density float32) {
	r.outW, r.outH, r.outScale = w, h, density
	k := r.rscale
	if r.external || k <= 0 || k == 1 || w <= 0 || h <= 0 || dev == nil {
		r.unscale()
		return
	}
	sw := max(1, int(float32(w)*k+0.5))
	sh := max(1, int(float32(h)*k+0.5))
	if r.pool == nil {
		r.pool = rt.New(dev, gpu.TexSRGBA8, SystemPresent)
	}
	t, err := r.pool.Resize(r.scaled, sw, sh, true)
	if err != nil {
		r.rscale = 1
		r.unscale()
		return
	}
	r.scaled = t
	r.target, r.targetW, r.targetH, r.depth = t.Texture, t.BucketW, t.BucketH, t.Depth
	r.vpW, r.vpH = t.W, t.H
	r.factor, r.scaling = k, true
}

func (r *Runner) unscale() {
	if !r.scaling {
		return
	}
	r.target, r.targetW, r.targetH, r.depth = gpu.TextureHandle{}, 0, 0, gpu.TextureHandle{}
	r.vpW, r.vpH = 0, 0
	r.factor, r.scaling = 1, false
}

func (r *Runner) Output() Output {
	switch {
	case r.scaling:
		return Output{W: r.outW, H: r.outH, TargetW: r.outW, TargetH: r.outH, Scale: r.outScale}
	case r.targetW > 0 && r.targetH > 0:
		w, h := r.vpW, r.vpH
		if w <= 0 || h <= 0 {
			w, h = r.targetW, r.targetH
		}
		return Output{Target: r.target, W: w, H: h, TargetW: r.targetW, TargetH: r.targetH, Scale: r.scale}
	}
	return Output{W: r.pixelW, H: r.pixelH, TargetW: r.pixelW, TargetH: r.pixelH, Scale: r.scale}
}

func (r *Runner) Present() {
	if !r.scaling || r.scaled == nil || r.render == nil || r.outScale <= 0 {
		return
	}
	v := view.ScreenView(r.outW, r.outH, r.outScale)
	v.DoClear, v.Clear, v.Label = true, mathx.Color{A: 1}, SystemPresent
	f := r.render.View(v)
	f.Sprite(sprite.Draw{
		Region: r.scaled.Region(),
		Size:   mathx.Vec2{X: float32(r.outW) / r.outScale, Y: float32(r.outH) / r.outScale},
		Tint:   mathx.Color{R: 1, G: 1, B: 1, A: 1},
	})
	f.End()
}

func (r *Runner) Close() {
	r.fixedBackdrop = nil
	r.unscale()
	if r.pool == nil {
		return
	}
	if r.scaled != nil {
		r.pool.Release(r.scaled)
		r.scaled = nil
	}
	r.pool.Close()
	r.pool = nil
}

type present struct{}

func (present) Run(ctx *engine.Context) {
	if r := Of(ctx); r != nil {
		r.Present()
	}
}

type anchor struct{}

func (anchor) Run(*engine.Context) {}

func (r *Runner) SetHidden(fn func(scene.NodeHandle) bool) {
	r.hidden = fn
}

func (r *Runner) Hidden(h scene.NodeHandle) bool {
	return r.hidden != nil && r.hidden(h)
}
