package render2d

import (
	"aego/contracts/camera2d"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/module"
	"aego/render/material"
	"aego/render/mesh"
	"aego/render/postfx"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
)

type cameras struct{}

func (cameras) Run(ctx *engine.Context) {
	r := frame.Of(ctx)
	if r == nil || r.Fixed() || r.Scene() == nil {
		return
	}
	sc := r.Scene()
	target, tw, th := r.Target()
	adj, _ := engine.ServiceOf[camera2d.Adjuster](ctx.World, camera2d.ServiceID)
	scene.Query1[Camera2D](sc).Each(func(h scene.NodeHandle, cam *Camera2D) {
		if !cam.Active || sc.Space(h) != scene.Space2D || sc.Under3D(h) {
			return
		}
		world := sc.RenderWorld(h)
		v := view.View{
			ID:         r.ViewID(sc.ID(h)),
			OutputID:   r.OutputID(),
			Viewport:   r.Viewport(cam.Viewport),
			Target:     target,
			TargetW:    tw,
			TargetH:    th,
			PixelScale: r.PixelScale(),
			Camera: view.Camera2D{
				Position: mathx.Vec2{X: world[6], Y: world[7]},
				Rotation: Rotation(world),
				Zoom:     cam.Zoom,
			},
			Clear:   cam.ClearColor,
			DoClear: cam.Clear,
			Scale:   cam.Scale,
			Virtual: cam.Virtual,
			Layers:  view.LayerMask(uint64(cam.Mask)),
			Label:   sc.Name(h),
		}
		if adj != nil {
			v = adj.Adjust(h, v)
		}
		r.AddView(v, cam.Order)
	})
}

type extract struct{ effects *postfx.Registry }

func (e extract) Run(ctx *engine.Context) {
	r := frame.Of(ctx)
	if r == nil || r.Scene() == nil {
		return
	}
	ls, ok := engine.ServiceOf[*lightingState](ctx.World, lightingServiceID)
	if !ok || ls.scene != r.Scene() {
		Module{}.attachLighting(&module.AttachContext{World: ctx.World, Scene: r.Scene()})
		ls, _ = engine.ServiceOf[*lightingState](ctx.World, lightingServiceID)
	}
	ls.prepare(r, ctx.World)
	views := r.Views()
	for i := range views {
		mask := uint64(views[i].Layers)
		if views[i].Is3D() {
			collect3(r, r.Bucket(i), mask)
		} else {
			collect(r, r.Bucket(i), mask, ls)
			collectMeshes(r, r.Bucket(i), mask, ls)
			ls.prepareView(r, views[i], &r.Bucket(i).Lighting)
		}
	}
	ls.preparePostProcess(r, e.effects)
	ls.finish()
}

func collect(r *frame.Runner, b *frame.Bucket, mask uint64, ls *lightingState) {
	sc, res, st := r.Scene(), r.Resolver(), r.Counters()
	scene.Query1[SpriteRenderer](sc).Each(func(h scene.NodeHandle, sr *SpriteRenderer) {
		if sc.Layers(h)&mask == 0 || sc.Space(h) != scene.Space2D || sc.Under3D(h) || r.Hidden(h) {
			return
		}
		region, ok := sr.Sprite.Region(res)
		if !ok {
			st.MissingAssets++
			return
		}
		var mat *material.Material
		var lanes *[16]float32
		if !sr.Material.IsZero() {
			mat, ok = sr.Material.Material(res)
			if !ok || mat.Shader().Domain != shader.DomainSprite {
				st.MissingAssets++
				return
			}
			if o, has := scene.Get[MaterialOverride](sc, h); has {
				var err error
				lanes, err = o.Lanes(mat)
				if err != nil {
					st.MissingAssets++
					return
				}
			}
		}
		world := sc.RenderWorld(h)
		draw := sprite.Draw{
			VisibilityLayers: sc.Layers(h),
			Region:           region,
			Material:         mat,
			Lanes:            lanes,
			Size:             Size(sr, region.Size()),
			Origin:           sr.Origin,
			Tint:             sr.Tint,
			Layer:            int16(sr.Layer),
			Order:            int16(sr.Order),
			Blend:            sr.Blend,
			FlipX:            sr.FlipX,
			FlipY:            sr.FlipY,
		}
		if mat != nil && mat.Shader().Features&shader.FeatureLighting != 0 {
			draw.Lighting = true
			draw.Normal = ls.normal(mat, sr.Sprite.ID, res, sc.ID(h), "engine.SpriteRenderer.material")
			draw.Emission = ls.emission(mat, sr.Sprite.ID, res, sc.ID(h), "engine.SpriteRenderer.material")
		}
		draw.SetTransform(world)
		b.Sprites = append(b.Sprites, draw)
		st.Sprites++
	})
}

func collect3(r *frame.Runner, b *frame.Bucket, mask uint64) {
	sc, res, st := r.Scene(), r.Resolver(), r.Counters()
	scene.Query1[SpriteRenderer](sc).Each(func(h scene.NodeHandle, sr *SpriteRenderer) {
		if !sc.Under3D(h) || sc.Layers(h)&mask == 0 || r.Hidden(h) {
			return
		}
		region, ok := sr.Sprite.Region(res)
		if !ok {
			st.MissingAssets++
			return
		}
		b.Sprites3D = append(b.Sprites3D, sprite.Draw3D{
			Region: region,
			World:  sc.World4(h),
			Size:   Size(sr, region.Size()),
			Origin: sr.Origin,
			Tint:   sr.Tint,
			Layer:  int16(sr.Layer),
			Order:  int16(sr.Order),
			Blend:  sr.Blend,
			FlipX:  sr.FlipX,
			FlipY:  sr.FlipY,
		})
		st.Sprites3D++
	})
}

func collectMeshes(r *frame.Runner, b *frame.Bucket, mask uint64, ls *lightingState) {
	sc, res, st := r.Scene(), r.Resolver(), r.Counters()
	scene.Query1[Mesh2D](sc).Each(func(h scene.NodeHandle, mr *Mesh2D) {
		if sc.Layers(h)&mask == 0 || sc.Space(h) != scene.Space2D || sc.Under3D(h) || r.Hidden(h) {
			return
		}
		geometry, ok := mr.Mesh.Mesh(res)
		if !ok {
			st.MissingAssets++
			return
		}
		var mat *material.Material
		var lanes *[16]float32
		if !mr.Material.IsZero() {
			mat, ok = mr.Material.Material(res)
			if !ok || mat.Shader().Domain != shader.DomainMesh2D {
				st.MissingAssets++
				return
			}
			if o, has := scene.Get[MaterialOverride](sc, h); has {
				var err error
				lanes, err = o.Lanes(mat)
				if err != nil {
					st.MissingAssets++
					return
				}
			}
		}
		region, _ := mr.Texture.Region(res)
		draw := mesh.Draw{VisibilityLayers: sc.Layers(h), Mesh: geometry, World: mathx.Mat4FromMat3(sc.RenderWorld(h)), Texture: region, Color: mr.Tint, Layer: int16(mr.Layer), Material: mat, Lanes: lanes, Is2D: true, Order: int16(mr.Order)}
		if mat != nil && mat.Shader().Features&shader.FeatureLighting != 0 {
			draw.Normal = ls.normal(mat, mr.Texture.ID, res, sc.ID(h), "engine.Mesh2D.material")
			draw.Emission = ls.emission(mat, mr.Texture.ID, res, sc.ID(h), "engine.Mesh2D.material")
		}
		b.Meshes = append(b.Meshes, draw)
		st.Meshes++
	})
}
