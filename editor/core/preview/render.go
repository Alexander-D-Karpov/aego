package preview

import (
	"math"

	"aego/asset"
	"aego/core/mathx"
	"aego/editor/core/shape"
	"aego/editor/core/tasks"
	"aego/engine"
	"aego/gpu"
	"aego/render/rt"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
)

type sceneJob struct {
	kind   asset.ArtifactKind
	name   string
	data   []byte
	target *rt.Target
	runner *frame.Runner
	out    pixels
}

func (s *Service) startScene(e asset.Entry, it *item, spec tasks.Spec) {
	key := it.key
	j := &sceneJob{kind: e.Kind, name: e.Name}
	art, _ := e.Artifact(e.Kind)
	files := s.d.Pipeline.Artifacts()
	cached := false
	it.task = s.d.Tasks.Work(spec, func(*tasks.Task) error {
		if p, err := s.load(key); err == nil {
			j.out, cached = p, true
			return nil
		}
		data, err := files.ReadFile(art.Path)
		j.data = data
		return err
	}).Then(func(t *tasks.Task) {
		if it.key != key {
			return
		}
		if cached || t.State() != tasks.Done {
			s.finish(it, key, j.out, t)
			return
		}
		it.task = s.d.Tasks.Step(spec, tasks.LaneRender, s.renderStep(j)).Then(func(dt *tasks.Task) {
			if it.key != key || dt.State() != tasks.Done {
				s.release(j)
				s.finish(it, key, j.out, dt)
				return
			}
			it.task = s.d.Tasks.Step(spec, tasks.LaneMain, s.readStep(j)).Then(func(rt *tasks.Task) {
				s.release(j)
				s.finish(it, key, j.out, rt)
				if rt.State() == tasks.Done {
					out := j.out
					s.d.Tasks.Work(tasks.Spec{Title: spec.Title, Kind: "preview"}, func(*tasks.Task) error { return s.save(key, out) })
				}
			})
		})
	})
}

func (s *Service) renderStep(j *sceneJob) tasks.Step {
	return func(*tasks.Task) (bool, error) {
		return true, s.draw(j)
	}
}

func (s *Service) readStep(j *sceneJob) tasks.Step {
	return func(*tasks.Task) (bool, error) {
		buf := make([]byte, Size*Size*4)
		if err := s.d.GPU.ReadPixels(j.target.Texture, 0, 0, Size, Size, buf); err != nil {
			return true, err
		}
		j.out = pixels{buf: buf, w: Size, h: Size}
		return true, nil
	}
}

func (s *Service) draw(j *sceneJob) error {
	sc, _, err := sceneio.Load(j.data, j.name, s.d.Types, s.d.Resolver, sceneio.LoadOptions{})
	if err != nil {
		return err
	}
	if s.pool == nil {
		s.pool = rt.New(s.d.GPU, gpu.TexSRGBA8, "editor.preview")
	}
	tgt, err := s.pool.Acquire(Size, Size, true)
	if err != nil {
		return err
	}
	j.target = tgt
	w := engine.NewWorld("preview")
	r := frame.New()
	r.Bind(sc, s.d.Renderer, s.d.Resolver)
	w.SetService(frame.ServiceID, r)
	w.SetService(scene.ServiceID, sc)
	j.runner = r
	r.SetTarget(tgt.Texture, tgt.BucketW, tgt.BucketH)
	r.SetDepthTarget(tgt.Depth)
	r.SetViewportSize(Size, Size)
	clear := s.d.Background
	if j.kind == asset.ArtPrefab {
		clear = mathx.Color{}
	}
	e := s.d.Engine
	fixed := false
	if j.kind == asset.ArtScene {
		e.RunPhase(w, engine.PhaseRenderPrepare, engine.ScopeEditor, 0, 0)
		if v, ok := cameraView(r.Views(), s.reference(), tgt); ok {
			r.SetFixedView(v)
			fixed = true
		} else if len(r.Views()) > 0 {
			e.RunPhase(w, engine.PhaseRender, engine.ScopeEditor, 0, 0)
			return nil
		}
	}
	if !fixed {
		r.SetFixedView(Fit(sc, shape.Set{Reg: s.d.Shapes, Res: s.d.Resolver}, tgt, clear))
	}
	e.RunPhase(w, engine.PhaseRenderPrepare, engine.ScopeEditor, 0, 0)
	e.RunPhase(w, engine.PhaseRender, engine.ScopeEditor, 0, 0)
	return nil
}

func (s *Service) release(j *sceneJob) {
	if j.runner != nil {
		j.runner.Close()
		j.runner = nil
	}
	if j.target != nil && s.pool != nil {
		s.pool.Release(j.target)
		j.target = nil
	}
}

func Fit(sc *scene.Scene, set shape.Set, tgt *rt.Target, clear mathx.Color) view.View {
	sc.UpdateTransforms()
	var b2 mathx.AABB2
	var b3 mathx.AABB3
	has2, has3 := false, false
	add2 := func(b mathx.AABB2) {
		if has2 {
			b2 = b2.Union(b)
		} else {
			b2, has2 = b, true
		}
	}
	add3 := func(b mathx.AABB3) {
		if has3 {
			b3 = mathx.AABB3{Min: mathx.Vec3Min(b3.Min, b.Min), Max: mathx.Vec3Max(b3.Max, b.Max)}
		} else {
			b3, has3 = b, true
		}
	}
	root := sc.Root()
	for _, h := range sc.PreOrder() {
		if h == root || !sc.IsAlive(h) || !sc.EffectiveEnabled(h) {
			continue
		}
		sh, ok := set.Of(sc, h)
		switch {
		case ok && sh.Kind == shape.Box:
			add3(sh.Box)
		case ok && sh.Kind == shape.Rect:
			add2(rectBounds(sc, h, sh.Local))
		case sc.Space(h) == scene.Space3D:
			p := sc.WorldPosition3D(h)
			add3(mathx.AABB3{Min: p, Max: p})
		default:
			p := sc.WorldPosition(h)
			add2(mathx.AABB2{Min: p, Max: p})
		}
	}
	if has3 && (sc.DefaultSpace() == scene.Space3D || !has2) {
		return fit3(b3, tgt, clear)
	}
	return fit2(b2, has2, tgt, clear)
}

func rectBounds(sc *scene.Scene, h scene.NodeHandle, lr mathx.Rect) mathx.AABB2 {
	m := sc.World(h)
	pts := [4]mathx.Vec2{
		m.Apply(mathx.Vec2{X: lr.X, Y: lr.Y}),
		m.Apply(mathx.Vec2{X: lr.X + lr.W, Y: lr.Y}),
		m.Apply(mathx.Vec2{X: lr.X + lr.W, Y: lr.Y + lr.H}),
		m.Apply(mathx.Vec2{X: lr.X, Y: lr.Y + lr.H}),
	}
	b := mathx.AABB2{Min: pts[0], Max: pts[0]}
	for _, p := range pts[1:] {
		b = b.Union(mathx.AABB2{Min: p, Max: p})
	}
	return b
}

func target(v view.View, tgt *rt.Target, clear mathx.Color) view.View {
	v.Target, v.TargetW, v.TargetH = tgt.Texture, tgt.BucketW, tgt.BucketH
	v.Clear, v.DoClear, v.Label = clear, true, "preview"
	return v
}

func fit2(b mathx.AABB2, has bool, tgt *rt.Target, clear mathx.Color) view.View {
	zoom := float32(64)
	if has {
		sz := b.Size()
		zoom = min(Size/max(sz.X, 1), Size/max(sz.Y, 1)) / 1.15
	}
	v := view.View{
		Camera:        view.Camera2D{Position: b.Center(), Zoom: zoom},
		Viewport:      mathx.Rect{W: Size, H: Size},
		PixelScale:    1,
		PixelsPerUnit: 1,
		ZMax:          1,
		Layers:        view.AllLayers,
	}
	return target(v, tgt, clear)
}

func fit3(b mathx.AABB3, tgt *rt.Target, clear mathx.Color) view.View {
	c := b.Min.Add(b.Max).Scale(0.5)
	r := max(b.Max.Sub(b.Min).Len()*0.5, 0.25)
	fov := mathx.Pi / 3
	d := r / float32(math.Sin(float64(fov)/2)) * 1.1
	rot := mathx.QuatYawPitch(0.6, -0.45)
	pos := c.Add(rot.Rotate(mathx.Vec3{Z: d}))
	eye := mathx.Mat4TRS(pos, rot, mathx.Vec3{X: 1, Y: 1, Z: 1})
	v := view.PerspectiveView(eye, fov, mathx.Clamp(d*0.002, 0.01, 1), max(d*50, 1000), mathx.Rect{W: Size, H: Size})
	v.DepthTarget = tgt.Depth
	return target(v, tgt, clear)
}

func cameraView(views []view.View, ref mathx.Vec2, tgt *rt.Target) (view.View, bool) {
	if len(views) == 0 || views[0].Is3D() {
		return view.View{}, false
	}
	v := views[0]
	if v.Virtual == (view.View{}).Virtual && ref.Y > 0 {
		v.Camera.Zoom *= Size / ref.Y
	}
	v.Viewport = mathx.Rect{W: Size, H: Size}
	v.Target, v.TargetW, v.TargetH = tgt.Texture, tgt.BucketW, tgt.BucketH
	v.PixelScale = 1
	v.Label = "preview"
	return v, true
}

func (s *Service) reference() mathx.Vec2 {
	if s.d.Reference != nil {
		if v := s.d.Reference(); v.Y > 0 {
			return v
		}
	}
	return mathx.Vec2{X: 1280, Y: 720}
}
