package frame

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/engine"
	"aego/gpu"
	"aego/render"
	"aego/render/mesh"
	"aego/render/sprite"
	"aego/render/view"
	"aego/scene"
)

const (
	SystemTransform = "engine.transform"
	SystemSubmit    = "engine.views_submit"
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
}

func New() *Runner {
	return &Runner{
		views: make([]view.View, 0, 4),
		order: make([]int32, 0, 4),
		perm:  make([]int, 0, 4),
		scale: 1,
	}
}

func (r *Runner) SetTarget(t gpu.TextureHandle, w, h int) {
	r.target, r.targetW, r.targetH = t, w, h
}

func (r *Runner) SetDepthTarget(d gpu.TextureHandle) {
	r.depth = d
}

func (r *Runner) ClearTarget() {
	r.target, r.targetW, r.targetH = gpu.TextureHandle{}, 0, 0
	r.depth = gpu.TextureHandle{}
	r.vpW, r.vpH = 0, 0
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
	r.pixelW, r.pixelH = pixelW, pixelH
	r.scale = scale
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
		if v.Projection == view.Perspective3D {
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
		if v.Projection == view.Perspective3D {
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
	return sys.Register(engine.SystemDesc{
		ID:     SystemSubmit,
		Phase:  engine.PhaseRender,
		Scope:  engine.ScopeWorld,
		Before: []string{engine.SystemAppRender},
		System: submit{},
	})
}

type transform struct{}

func (transform) Run(ctx *engine.Context) {
	r := Of(ctx)
	if r == nil {
		return
	}
	m := ctx.Engine.Metrics()
	r.Prepare(m.PixelW, m.PixelH, m.Density())
}

type submit struct{}

func (submit) Run(ctx *engine.Context) {
	if r := Of(ctx); r != nil {
		r.Submit(r.render)
	}
}
