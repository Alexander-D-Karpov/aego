package frame

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/engine"
	"aego/gpu"
	"aego/render"
	renderhost "aego/render/host"
	"aego/render/mesh"
	"aego/render/rt"
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
	Sprites   []sprite.Draw
	Sprites3D []sprite.Draw3D
	Meshes    []mesh.Draw
}

func (b *Bucket) reset() {
	b.Sprites = b.Sprites[:0]
	b.Sprites3D = b.Sprites3D[:0]
	b.Meshes = b.Meshes[:0]
}

type Runner struct {
	hidden   func(scene.NodeHandle) bool
	scene    *scene.Scene
	render   *render.Renderer
	res      asset.Resolver
	views    []view.View
	order    []int32
	perm     []int
	buckets  []Bucket
	stats    Stats
	pixelW   int
	pixelH   int
	scale    float32
	target   gpu.TextureHandle
	depth    gpu.TextureHandle
	targetW  int
	targetH  int
	vpW      int
	vpH      int
	fixed    view.View
	hasFixed bool
	factor   float32
	rscale   float32
	external bool
	scaling  bool
	pool     *rt.Pool
	scaled   *rt.Target
	outW     int
	outH     int
	outScale float32
}

func New() *Runner {
	return &Runner{
		views:  make([]view.View, 0, 4),
		order:  make([]int32, 0, 4),
		perm:   make([]int, 0, 4),
		scale:  1,
		factor: 1,
		rscale: 1,
	}
}

func (r *Runner) SetTarget(t gpu.TextureHandle, w, h int) {
	r.unscale()
	r.target, r.targetW, r.targetH = t, w, h
	r.external = true
}

func (r *Runner) SetDepthTarget(d gpu.TextureHandle) {
	r.depth = d
}

func (r *Runner) ClearTarget() {
	r.target, r.targetW, r.targetH = gpu.TextureHandle{}, 0, 0
	r.depth = gpu.TextureHandle{}
	r.vpW, r.vpH = 0, 0
	r.external, r.scaling, r.factor = false, false, 1
}

func (r *Runner) SetViewportSize(w, h int) {
	r.vpW, r.vpH = w, h
}

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

func (r *Runner) AddView(v view.View, order int32) {
	if r.hasFixed {
		return
	}
	r.push(v, order)
}

func (r *Runner) push(v view.View, order int32) {
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

func (r *Runner) Submit(dst *render.Renderer) {
	r.stats.Views = len(r.views)
	if dst == nil || len(r.views) == 0 {
		return
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
		f := dst.View(v)
		if v.Is3D() {
			f.Meshes(b.Meshes)
			f.Sprites3D(b.Sprites3D)
		} else {
			f.Sprites(b.Sprites)
		}
		f.End()
	}
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
