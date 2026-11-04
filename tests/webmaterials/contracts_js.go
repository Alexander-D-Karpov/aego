//go:build js && wasm

package main

import (
	"aego/core/mathx"
	"aego/gpu"
	"aego/render"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/render/view"
)

// These assertions deliberately exercise the actual linked WebGL programs.
// Schema/CPU unit tests alone cannot establish driver sampler or block bindings.
func (g *rig) contractChecks(out *report) {
	unused := g.material("unused")
	unused.Set("Unused", [4]float32{.9})
	img := g.frame(func(v *render.ViewFrame) { g.draw(v, unused, nil) })
	p, declared := unused.Shader().Schema.Param("Unused")
	value, stored := unused.Get("Unused")
	out.check("optimized-out-field-retains-schema-and-explicit-value", declared && p.Default[0] == .75 && stored && value[0] == .9 && unused.Explicit("Unused") && near(pixel(img, 16, 16), [4]int{0, 255, 0, 255}), value)

	sample := g.material("color-sample")
	for _, c := range []struct {
		name   string
		format gpu.TextureFormat
		want   int
	}{{"srgb-color-samples-linear", gpu.TexSRGBA8, 55}, {"raw-data-samples-unchanged", gpu.TexRGBA8, 128}} {
		data := texture.New(1, 1, c.format)
		copy(data.Pixels, []byte{128, 128, 128, 255})
		data.NonColor = c.format == gpu.TexRGBA8
		h, err := g.r.Textures().Upload(data, texture.UploadOptions{})
		if err != nil {
			panic(err)
		}
		img = g.frame(func(v *render.ViewFrame) {
			v.Sprite(sprite.Draw{Region: texture.Whole(h, 1, 1), Size: mathx.Vec2{X: 32, Y: 32}, Tint: mathx.White, Material: sample})
		})
		out.check(c.name, near(pixel(img, 16, 16), [4]int{c.want, c.want, c.want, 255}), pixel(img, 16, 16))
	}

	shared := g.material("world-time")
	a, b := *shared.DefaultLanes(), *shared.DefaultLanes()
	a[0], b[0] = .2, .8
	f := g.d.BeginFrame(32, 32)
	if err := g.r.BeginFrame(f, 32, 32, 1); err != nil {
		panic(err)
	}
	draw := func(x float32, clock render.Clock, lanes *[16]float32) {
		g.r.SetClock(clock)
		v := view.ScreenView(32, 32, 1)
		v.Target, v.DoClear = g.target, true
		v.Viewport = mathx.Rect{X: x, W: 16, H: 32}
		vf := g.r.View(v)
		vf.Sprite(sprite.Draw{Region: g.white, Position: mathx.Vec2{X: 16, Y: 16}, Size: mathx.Vec2{X: 32, Y: 32}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: shared, Lanes: lanes})
		vf.End()
	}
	draw(0, render.Clock{Time: .25, Unscaled: .5}, &a)
	draw(16, render.Clock{Time: .4, Unscaled: .2}, &b)
	draw(0, render.Clock{Time: .25, Unscaled: .5}, &a)
	g.r.EndFrame()
	g.d.EndFrame()
	if err := g.d.ReadPixels(g.target, 0, 0, 32, 32, g.pixels); err != nil {
		panic(err)
	}
	out.check("two-preview-clocks-and-overrides-are-isolated", near(pixel(g.pixels, 8, 16), [4]int{64, 128, 51, 255}) && near(pixel(g.pixels, 24, 16), [4]int{102, 51, 204, 255}) && shared.DefaultLanes()[0] == .5 && !shared.Explicit("Amount"), []any{pixel(g.pixels, 8, 16), pixel(g.pixels, 24, 16)})
	g.r.SetClock(render.Clock{})

	sampler := g.material("sampler-reload-old")
	data := texture.New(1, 1, gpu.TexRGBA8)
	data.NonColor = true
	copy(data.Pixels, []byte{64, 191, 0, 255})
	h, err := g.r.Textures().Upload(data, texture.UploadOptions{})
	if err != nil {
		panic(err)
	}
	sampler.SetTexture("DataTex", material.Tex{Texture: h})
	sampler.Set("Weight", [4]float32{.25})
	img = g.frame(func(v *render.ViewFrame) { g.draw(v, sampler, nil) })
	before := pixel(img, 16, 16)
	next := g.shader("sampler-reload-new")
	g.r.Materials().QueueReload(sampler.Shader(), next, []*material.Material{sampler}, nil)
	g.commit()
	img = g.frame(func(v *render.ViewFrame) { g.draw(v, sampler, nil) })
	tp, _ := next.Schema.Param("DataTex")
	out.check("relink-remaps-sampler-slot-and-material-block", near(before, [4]int{16, 48, 0, 64}) && near(pixel(img, 16, 16), [4]int{16, 48, 0, 64}) && sampler.Shader() == next && tp.Slot == shader.TexUser+1, []any{before, pixel(img, 16, 16), tp.Slot})
}
