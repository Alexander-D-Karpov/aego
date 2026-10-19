package render3d

import (
	"aego/core/mathx"
	"aego/engine"
	"aego/gpu"
	"aego/render/mesh"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
)

type cameras struct{}

func cameraView(cam *Camera3D, world mathx.Mat4, vp mathx.Rect) view.View {
	if cam.Orthographic {
		return view.OrthoView(world, cam.OrthoSize, cam.Near, cam.Far, vp)
	}
	return view.PerspectiveView(world, mathx.Deg2Rad(cam.FOV), cam.Near, cam.Far, vp)
}

func (cameras) Run(ctx *engine.Context) {
	r := frame.Of(ctx)
	if r == nil || r.Fixed() || r.Scene() == nil {
		return
	}
	sc := r.Scene()
	target, tw, th := r.Target()
	scene.Query1[Camera3D](sc).Each(func(h scene.NodeHandle, cam *Camera3D) {
		if !cam.Active || sc.Space(h) != scene.Space3D {
			return
		}
		v := cameraView(cam, sc.World4(h), r.Viewport(cam.Viewport))
		v.Target, v.TargetW, v.TargetH = target, tw, th
		v.DepthTarget = r.Depth()
		v.PixelScale = r.PixelScale()
		v.Clear, v.DoClear = cam.ClearColor, cam.Clear
		v.Layers = view.LayerMask(uint64(cam.Mask))
		v.Label = sc.Name(h)
		r.AddView(v, cam.Order)
	})
}

type extract struct{}

func (extract) Run(ctx *engine.Context) {
	r := frame.Of(ctx)
	if r == nil || r.Scene() == nil || r.Renderer() == nil {
		return
	}
	views := r.Views()
	for i := range views {
		if views[i].Is3D() {
			collect(r, r.Bucket(i), uint64(views[i].Layers))
		}
	}
}

func collect(r *frame.Runner, b *frame.Bucket, mask uint64) {
	sc, res, st, dst := r.Scene(), r.Resolver(), r.Counters(), r.Renderer()
	scene.Query1[MeshRenderer](sc).Each(func(h scene.NodeHandle, mr *MeshRenderer) {
		if sc.Space(h) != scene.Space3D || sc.Layers(h)&mask == 0 || r.Hidden(h) {
			return
		}
		m, ok := mr.Mesh.Mesh(res)
		if !ok {
			st.MissingAssets++
			return
		}
		region, _ := mr.Texture.Region(res)
		b.Meshes = append(b.Meshes, mesh.Draw{
			Mesh:        m,
			World:       sc.World4(h),
			Texture:     region,
			Sampler:     dst.Sampler(gpu.SamplerDesc{MinFilter: mr.Filter, MagFilter: mr.Filter}),
			Color:       mr.Color,
			Blend:       mr.Blend,
			DoubleSided: mr.DoubleSided,
			Layer:       int16(mr.Layer),
		})
		st.Meshes++
	})
}
