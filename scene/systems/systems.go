package systems

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
	Sprites3D     int
	Meshes        int
	MissingAssets int
}

type Runner struct {
	scene    *scene.Scene
	render   *render.Renderer
	res      asset.Resolver
	views    []view.View
	order    []int
	draws    []sprite.Draw
	draws3   []sprite.Draw3D
	meshes   []mesh.Draw
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
		views:  make([]view.View, 0, 4),
		draws:  make([]sprite.Draw, 0, 4096),
		draws3: make([]sprite.Draw3D, 0, 1024),
		meshes: make([]mesh.Draw, 0, 1024),
		scale:  1,
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

func (r *Runner) Views() []view.View {
	return r.views
}

func (r *Runner) Renderer() *render.Renderer {
	return r.render
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

func runnerFor(def *Runner, ctx *engine.Context) *Runner {
	if ctx.World != nil {
		if wr, ok := engine.ServiceOf[*Runner](ctx.World, RunnerServiceID); ok && wr != nil {
			return wr
		}
	}
	return def
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

type transformSystem struct {
	r *Runner
}

func (s transformSystem) Run(ctx *engine.Context) {
	r := runnerFor(s.r, ctx)
	m := ctx.Engine.Metrics()
	r.Prepare(m.PixelW, m.PixelH, m.Density())
}

type extractSystem struct {
	r *Runner
}

func (s extractSystem) Run(ctx *engine.Context) {
	r := runnerFor(s.r, ctx)
	r.Extract(r.render)
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
	r.stats.Sprites = 0
	r.stats.Sprites3D = 0
	r.stats.Meshes = 0
	r.stats.MissingAssets = 0
	if r.scene == nil {
		r.views = r.views[:0]
		return
	}
	r.scene.Flush()
	r.scene.UpdateTransforms()
	r.buildViews()
}

func (r *Runner) Extract(dst *render.Renderer) {
	if dst == nil || len(r.views) == 0 {
		return
	}
	for i := range r.views {
		v := r.views[i]
		frame := dst.View(v)
		mask := uint64(v.Layers)
		if v.Projection == view.Perspective3D {
			r.collect3(dst, mask)
			frame.Meshes(r.meshes)
			frame.Sprites3D(r.draws3)
		} else {
			r.collect(mask)
			frame.Sprites(r.draws)
		}
		frame.End()
	}
}

func (r *Runner) viewport(vp mathx.Rect) mathx.Rect {
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

func (r *Runner) addView(v view.View, order int32) {
	r.views = append(r.views, v)
	r.order = append(r.order, int(order))
}

func (r *Runner) buildViews() {
	r.views = r.views[:0]
	r.order = r.order[:0]
	if r.hasFixed {
		v := r.fixed
		if r.targetW > 0 {
			v.Target, v.TargetW, v.TargetH = r.target, r.targetW, r.targetH
			if v.Projection == view.Perspective3D {
				v.DepthTarget = r.depth
			}
		}
		r.views = append(r.views, v)
		r.stats.Views = 1
		return
	}
	tw, th := r.pixelW, r.pixelH
	if r.targetW > 0 && r.targetH > 0 {
		tw, th = r.targetW, r.targetH
	}
	sc := r.scene
	scene.Query1[components.Camera2D](sc).Each(func(h scene.NodeHandle, cam *components.Camera2D) {
		if !cam.Active || sc.Space(h) != scene.Space2D || sc.Under3D(h) {
			return
		}
		world := sc.World(h)
		v := view.View{
			Viewport:   r.viewport(cam.Viewport),
			Target:     r.target,
			TargetW:    tw,
			TargetH:    th,
			PixelScale: r.scale,
			Camera: view.Camera2D{
				Position: mathx.Vec2{X: world[6], Y: world[7]},
				Rotation: rotationOf(world),
				Zoom:     cam.Zoom,
			},
			Clear:   cam.ClearColor,
			DoClear: cam.Clear,
			Scale:   cam.Scale,
			Virtual: cam.Virtual,
			Layers:  view.LayerMask(uint64(cam.Mask)),
			Label:   sc.Name(h),
		}
		r.addView(v, cam.Order)
	})
	scene.Query1[components.Camera3D](sc).Each(func(h scene.NodeHandle, cam *components.Camera3D) {
		if !cam.Active || sc.Space(h) != scene.Space3D {
			return
		}
		v := view.PerspectiveView(sc.World4(h), mathx.Deg2Rad(cam.FOV), cam.Near, cam.Far, r.viewport(cam.Viewport))
		v.Target, v.TargetW, v.TargetH = r.target, tw, th
		v.DepthTarget = r.depth
		v.PixelScale = r.scale
		v.Clear, v.DoClear = cam.ClearColor, cam.Clear
		v.Layers = view.LayerMask(uint64(cam.Mask))
		v.Label = sc.Name(h)
		r.addView(v, cam.Order)
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

func scaleX(m mathx.Mat3) float32 {
	return mathx.Sqrt(m[0]*m[0] + m[1]*m[1])
}

func scaleY(m mathx.Mat3) float32 {
	return mathx.Sqrt(m[3]*m[3] + m[4]*m[4])
}

func spriteSize(sr *components.SpriteRenderer, region mathx.Vec2) mathx.Vec2 {
	if sr.Size.X == 0 && sr.Size.Y == 0 {
		return region
	}
	return sr.Size
}

func (r *Runner) collect(mask uint64) {
	r.draws = r.draws[:0]
	sc := r.scene
	scene.Query1[components.SpriteRenderer](sc).Each(func(h scene.NodeHandle, sr *components.SpriteRenderer) {
		if sc.Layers(h)&mask == 0 || sc.Space(h) != scene.Space2D || sc.Under3D(h) {
			return
		}
		region, ok := sr.Sprite.Region(r.res)
		if !ok {
			r.stats.MissingAssets++
			return
		}
		world := sc.World(h)
		r.draws = append(r.draws, sprite.Draw{
			Region:   region,
			Position: mathx.Vec2{X: world[6], Y: world[7]},
			Size:     spriteSize(sr, region.Size()),
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

func (r *Runner) collect3(dst *render.Renderer, mask uint64) {
	r.meshes = r.meshes[:0]
	r.draws3 = r.draws3[:0]
	sc := r.scene
	scene.Query1[components.MeshRenderer](sc).Each(func(h scene.NodeHandle, mr *components.MeshRenderer) {
		if sc.Space(h) != scene.Space3D || sc.Layers(h)&mask == 0 {
			return
		}
		m, ok := mr.Mesh.Mesh(r.res)
		if !ok {
			r.stats.MissingAssets++
			return
		}
		region, _ := mr.Texture.Region(r.res)
		r.meshes = append(r.meshes, mesh.Draw{
			Mesh:        m,
			World:       sc.World4(h),
			Texture:     region,
			Sampler:     dst.Sampler(gpu.SamplerDesc{MinFilter: mr.Filter, MagFilter: mr.Filter}),
			Color:       mr.Color,
			Blend:       mr.Blend,
			DoubleSided: mr.DoubleSided,
			Layer:       int16(mr.Layer),
		})
		r.stats.Meshes++
	})
	scene.Query1[components.SpriteRenderer](sc).Each(func(h scene.NodeHandle, sr *components.SpriteRenderer) {
		if !sc.Under3D(h) || sc.Layers(h)&mask == 0 {
			return
		}
		region, ok := sr.Sprite.Region(r.res)
		if !ok {
			r.stats.MissingAssets++
			return
		}
		r.draws3 = append(r.draws3, sprite.Draw3D{
			Region: region,
			World:  sc.World4(h),
			Size:   spriteSize(sr, region.Size()),
			Origin: sr.Origin,
			Tint:   sr.Tint,
			Layer:  int16(sr.Layer),
			Order:  int16(sr.Order),
			Blend:  sr.Blend,
			FlipX:  sr.FlipX,
			FlipY:  sr.FlipY,
		})
		r.stats.Sprites3D++
	})
}
