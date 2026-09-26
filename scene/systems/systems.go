package systems

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/engine"
	"aego/gpu"
	"aego/render"
	"aego/render/sprite"
	"aego/render/view"
	"aego/scene"
	"aego/scene/components"
)

const (
	SystemTransform = "engine.transform"
	SystemExtract   = "engine.sprite_extract"
	RunnerServiceID = "scene.runner"
)

type Stats struct {
	Views         int
	Sprites       int
	MissingAssets int
}

type Runner struct {
	scene    *scene.Scene
	render   *render.Renderer
	res      asset.Resolver
	views    []view.View
	order    []int
	draws    []sprite.Draw
	stats    Stats
	pixelW   int
	pixelH   int
	scale    float32
	target   gpu.TextureHandle
	targetW  int
	targetH  int
	fixed    view.View
	hasFixed bool
}

func (r *Runner) SetTarget(t gpu.TextureHandle, w, h int) {
	r.target, r.targetW, r.targetH = t, w, h
}

func (r *Runner) ClearTarget() {
	r.target, r.targetW, r.targetH = gpu.TextureHandle{}, 0, 0
}

func (r *Runner) SetFixedView(v view.View) {
	r.fixed = v
	r.hasFixed = true
}

func (r *Runner) ClearFixedView() {
	r.hasFixed = false
}

func (r *Runner) Views() []view.View {
	return r.views
}

func (r *Runner) Renderer() *render.Renderer {
	return r.render
}

func runnerFor(def *Runner, ctx *engine.Context) *Runner {
	if ctx.World != nil {
		if wr, ok := engine.ServiceOf[*Runner](ctx.World, RunnerServiceID); ok && wr != nil {
			return wr
		}
	}
	return def
}

func New() *Runner {
	return &Runner{
		views: make([]view.View, 0, 4),
		draws: make([]sprite.Draw, 0, 4096),
		scale: 1,
	}
}

func (r *Runner) Bind(s *scene.Scene, rnd *render.Renderer, res asset.Resolver) {
	r.scene = s
	r.render = rnd
	r.res = res
}

func (r *Runner) Stats() Stats {
	return r.stats
}

func (r *Runner) SetScene(s *scene.Scene) {
	r.scene = s
}

func (r *Runner) Scene() *scene.Scene {
	return r.scene
}

func (r *Runner) Extract(dst *render.Renderer) {
	if dst == nil || len(r.views) == 0 {
		return
	}
	for i := range r.views {
		v := r.views[i]
		frame := dst.View(v)
		r.collect(uint64(v.Layers))
		frame.Sprites(r.draws)
		frame.End()
	}
}

func (r *Runner) Prepare(pixelW, pixelH int, scale float32) {
	if r.targetW > 0 && r.targetH > 0 {
		pixelW, pixelH = r.targetW, r.targetH
	}
	r.pixelW, r.pixelH = pixelW, pixelH
	r.scale = scale
	r.stats.Sprites = 0
	r.stats.MissingAssets = 0
	if r.scene == nil {
		r.views = r.views[:0]
		return
	}
	r.scene.Flush()
	r.scene.UpdateTransforms()
	r.buildViews()
}

func Register(reg *engine.Registry, run *Runner) error {
	sys := reg.Systems()
	if err := sys.Register(engine.SystemDesc{
		ID:     SystemTransform,
		Phase:  engine.PhaseRenderPrepare,
		Scope:  engine.ScopeWorld,
		System: transformSystem{r: run},
	}); err != nil {
		return err
	}
	return sys.Register(engine.SystemDesc{
		ID:     SystemExtract,
		Phase:  engine.PhaseRender,
		Scope:  engine.ScopeWorld,
		Before: []string{engine.SystemAppRender},
		System: extractSystem{r: run},
	})
}

func (s transformSystem) Run(ctx *engine.Context) {
	r := runnerFor(s.r, ctx)
	m := ctx.Engine.Metrics()
	r.Prepare(m.PixelW, m.PixelH, m.Density())
}

func (s extractSystem) Run(ctx *engine.Context) {
	r := runnerFor(s.r, ctx)
	r.Extract(r.render)
}

type transformSystem struct {
	r *Runner
}

func (r *Runner) buildViews() {
	r.views = r.views[:0]
	r.order = r.order[:0]
	if r.hasFixed {
		v := r.fixed
		if r.targetW > 0 {
			v.Target, v.TargetW, v.TargetH = r.target, r.targetW, r.targetH
		}
		r.views = append(r.views, v)
		r.stats.Views = 1
		return
	}
	sc := r.scene
	scene.Query1[components.Camera](sc).Each(func(h scene.NodeHandle, cam *components.Camera) {
		if !cam.Active {
			return
		}
		world := sc.World(h)
		vp := cam.Viewport
		if vp.W <= 0 || vp.H <= 0 {
			vp = mathx.Rect{W: 1, H: 1}
		}
		v := view.View{
			Viewport: mathx.Rect{
				X: vp.X * float32(r.pixelW),
				Y: vp.Y * float32(r.pixelH),
				W: vp.W * float32(r.pixelW),
				H: vp.H * float32(r.pixelH),
			},
			Target:     r.target,
			TargetW:    r.pixelW,
			TargetH:    r.pixelH,
			PixelScale: r.scale,
			Camera: view.Camera2D{
				Position: mathx.Vec2{X: world[6], Y: world[7]},
				Rotation: rotationOf(world),
				Zoom:     cam.Zoom,
			},
			DoClear: cam.Clear,
			Scale:   cam.Scale,
			Virtual: cam.Virtual,
			Layers:  view.LayerMask(uint64(cam.Mask)),
			Label:   sc.Name(h),
		}
		v.Clear = cam.ClearColor
		r.views = append(r.views, v)
		r.order = append(r.order, int(cam.Order))
	})
	for i := 1; i < len(r.views); i++ {
		for j := i; j > 0 && r.order[j] < r.order[j-1]; j-- {
			r.views[j], r.views[j-1] = r.views[j-1], r.views[j]
			r.order[j], r.order[j-1] = r.order[j-1], r.order[j]
		}
	}
	r.stats.Views = len(r.views)
}

func rotationOf(m mathx.Mat3) float32 {
	return mathx.Atan2(m[1], m[0])
}

type extractSystem struct {
	r *Runner
}

func (r *Runner) collect(mask uint64) {
	r.draws = r.draws[:0]
	sc := r.scene
	scene.Query1[components.SpriteRenderer](sc).Each(func(h scene.NodeHandle, sr *components.SpriteRenderer) {
		if sc.Layers(h)&mask == 0 {
			return
		}
		region, ok := sr.Sprite.Region(r.res)
		if !ok {
			r.stats.MissingAssets++
			return
		}
		world := sc.World(h)
		size := sr.Size
		if size.X == 0 && size.Y == 0 {
			size = region.Size()
		}
		r.draws = append(r.draws, sprite.Draw{
			Region:   region,
			Position: mathx.Vec2{X: world[6], Y: world[7]},
			Size:     size,
			Scale:    mathx.Vec2{X: scaleX(world), Y: scaleY(world)},
			Rotation: rotationOf(world),
			Origin:   sr.Origin,
			Tint:     sr.Tint,
			Layer:    int16(sr.Layer),
			Order:    int16(sr.Order),
			Blend:    sr.Blend,
			FlipX:    sr.FlipX,
			FlipY:    sr.FlipY,
		})
		r.stats.Sprites++
	})
}

func scaleX(m mathx.Mat3) float32 {
	return mathx.Sqrt(m[0]*m[0] + m[1]*m[1])
}

func scaleY(m mathx.Mat3) float32 {
	return mathx.Sqrt(m[3]*m[3] + m[4]*m[4])
}
