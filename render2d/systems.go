package render2d

import (
	"aego/core/mathx"
	"aego/engine"
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
	scene.Query1[Camera2D](sc).Each(func(h scene.NodeHandle, cam *Camera2D) {
		if !cam.Active || sc.Space(h) != scene.Space2D || sc.Under3D(h) {
			return
		}
		world := sc.World(h)
		r.AddView(view.View{
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
		}, cam.Order)
	})
}

type extract struct{}

func (extract) Run(ctx *engine.Context) {
	r := frame.Of(ctx)
	if r == nil || r.Scene() == nil {
		return
	}
	views := r.Views()
	for i := range views {
		mask := uint64(views[i].Layers)
		if views[i].Is3D() {
			collect3(r, r.Bucket(i), mask)
		} else {
			collect(r, r.Bucket(i), mask)
		}
	}
}

func collect(r *frame.Runner, b *frame.Bucket, mask uint64) {
	sc, res, st := r.Scene(), r.Resolver(), r.Counters()
	scene.Query1[SpriteRenderer](sc).Each(func(h scene.NodeHandle, sr *SpriteRenderer) {
		if sc.Layers(h)&mask == 0 || sc.Space(h) != scene.Space2D || sc.Under3D(h) {
			return
		}
		region, ok := sr.Sprite.Region(res)
		if !ok {
			st.MissingAssets++
			return
		}
		world := sc.World(h)
		b.Sprites = append(b.Sprites, sprite.Draw{
			Region:   region,
			Position: mathx.Vec2{X: world[6], Y: world[7]},
			Size:     Size(sr, region.Size()),
			Scale:    mathx.Vec2{X: ScaleX(world), Y: ScaleY(world)},
			Rotation: Rotation(world),
			Origin:   sr.Origin,
			Tint:     sr.Tint,
			Layer:    int16(sr.Layer),
			Order:    int16(sr.Order),
			Blend:    sr.Blend,
			FlipX:    sr.FlipX,
			FlipY:    sr.FlipY,
		})
		st.Sprites++
	})
}

func collect3(r *frame.Runner, b *frame.Bucket, mask uint64) {
	sc, res, st := r.Scene(), r.Resolver(), r.Counters()
	scene.Query1[SpriteRenderer](sc).Each(func(h scene.NodeHandle, sr *SpriteRenderer) {
		if !sc.Under3D(h) || sc.Layers(h)&mask == 0 {
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
