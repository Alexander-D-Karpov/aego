//go:build aego_gltest

package gl

import (
	"math"
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/render/view"
)

func TestTwoPreviewClocksAndOverridesReachIndependentDraws(t *testing.T) {
	r, dev, target := materialRig(t)
	sh := runtimeShader(t, "two-preview-worlds", `#pragma aego abi 1
#pragma material
uniform float Amount; // @instance @default(0.5)
#pragma end
void fragment(){aego_out=vec4(aego_time,aego_unscaled,Amount,1.0);}`)
	m, _ := material.New("shared", sh, 0, shader.BlendPremultiplied)
	a, b := *m.DefaultLanes(), *m.DefaultLanes()
	a[0], b[0] = .2, .8
	f := dev.BeginFrame(32, 32)
	if err := r.BeginFrame(f, 32, 32, 1); err != nil {
		t.Fatal(err)
	}
	draw := func(x float32, clock render.Clock, lanes *[16]float32) {
		r.SetClock(clock)
		v := view.ScreenView(32, 32, 1)
		v.Target, v.DoClear = target, true
		v.Viewport = mathx.Rect{X: x, W: 16, H: 32}
		vf := r.View(v)
		vf.Sprite(sprite.Draw{Region: r.White(), Position: mathx.Vec2{X: 16, Y: 16}, Size: mathx.Vec2{X: 32, Y: 32}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: m, Lanes: lanes})
		vf.End()
	}
	draw(0, render.Clock{Time: .25, Unscaled: .5}, &a)
	draw(16, render.Clock{Time: .4, Unscaled: .2}, &b)
	draw(0, render.Clock{Time: .25, Unscaled: .5}, &a)
	r.EndFrame()
	dev.EndFrame()
	for _, c := range []struct {
		x    int
		want [4]int
	}{{8, [4]int{64, 128, 51, 255}}, {24, [4]int{102, 51, 204, 255}}} {
		var got [4]byte
		if err := dev.ReadPixels(target, c.x, 16, 1, 1, got[:]); err != nil {
			t.Fatal(err)
		}
		for i, n := range c.want {
			if math.Abs(float64(int(got[i])-n)) > 2 {
				t.Fatalf("view at x=%d received stale time or instance state: %v, want %v", c.x, got, c.want)
			}
		}
	}
	if m.DefaultLanes()[0] != .5 || m.Explicit("Amount") {
		t.Fatal("preview changed the shared material")
	}
}

func TestColorAndDataTexturesUseDistinctSamplerSemantics(t *testing.T) {
	r, dev, target := materialRig(t)
	sh := runtimeShader(t, "color-vs-data", `#pragma aego abi 1
#include "engine/sprite.glsl"
void fragment(){aego_out=aego_sample();}`)
	m, _ := material.New("sample", sh, 0, shader.BlendPremultiplied)
	for _, c := range []struct {
		format gpu.TextureFormat
		want   int
	}{{gpu.TexSRGBA8, 55}, {gpu.TexRGBA8, 128}} {
		data := texture.New(1, 1, c.format)
		copy(data.Pixels, []byte{128, 128, 128, 255})
		data.NonColor = c.format == gpu.TexRGBA8
		h, err := r.Textures().Upload(data, texture.UploadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		px := drawMaterialFrame(t, r, dev, target, func(v *render.ViewFrame) {
			v.Sprite(sprite.Draw{Region: texture.Whole(h, 1, 1), Position: mathx.Vec2{X: 16, Y: 16}, Size: mathx.Vec2{X: 24, Y: 24}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: m})
		})
		for i := 0; i < 3; i++ {
			if math.Abs(float64(int(px[i])-c.want)) > 2 {
				t.Fatalf("format %v sampled as %v, want linear channel %d", c.format, px, c.want)
			}
		}
		if px[3] != 255 {
			t.Fatalf("format %v changed alpha: %v", c.format, px)
		}
	}
}

func TestOptimizedOutParameterRemainsInDeclaredMaterialSchema(t *testing.T) {
	r, dev, target := materialRig(t)
	sh := runtimeShader(t, "unused-parameter", `#pragma aego abi 1
#pragma material
uniform float Unused; // @default(0.75)
#pragma end
void fragment(){aego_out=vec4(0.0,1.0,0.0,1.0);}`)
	m, _ := material.New("m", sh, 0, shader.BlendPremultiplied)
	if !m.Set("Unused", [4]float32{.9}) {
		t.Fatal("cannot edit declared field")
	}
	px := drawMaterialFrame(t, r, dev, target, func(v *render.ViewFrame) {
		v.Sprite(sprite.Draw{Region: r.White(), Size: mathx.Vec2{X: 32, Y: 32}, Tint: mathx.White, Material: m})
	})
	if px != [4]byte{0, 255, 0, 255} {
		t.Fatalf("unused field prevented rendering: %v", px)
	}
	if p, ok := m.Shader().Schema.Param("Unused"); !ok || p.Default[0] != .75 {
		t.Fatal("driver optimization removed the declared schema/default")
	}
	if value, ok := m.Get("Unused"); !ok || value[0] != .9 || !m.Explicit("Unused") {
		t.Fatal("inactive GPU field lost its explicit semantic value")
	}
}

func TestSamplerAndBlockBindingsRefreshAfterRelink(t *testing.T) {
	r, dev, target := materialRig(t)
	sh := runtimeShader(t, "old-sampler-slot", `#pragma aego abi 1
#pragma material
uniform sampler2D DataTex;
uniform float Weight; // @default(0.5)
#pragma end
void fragment(){aego_out=texture(DataTex,vec2(0.5))*Weight;}`)
	m, _ := material.New("m", sh, 0, shader.BlendPremultiplied)
	data := texture.New(1, 1, gpu.TexRGBA8)
	data.NonColor = true
	copy(data.Pixels, []byte{64, 191, 0, 255})
	h, err := r.Textures().Upload(data, texture.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m.SetTexture("DataTex", material.Tex{Texture: h})
	m.Set("Weight", [4]float32{.25})
	draw := func(v *render.ViewFrame) {
		v.Sprite(sprite.Draw{Region: r.White(), Size: mathx.Vec2{X: 32, Y: 32}, Tint: mathx.White, Material: m})
	}
	before := drawMaterialFrame(t, r, dev, target, draw)
	next := runtimeShader(t, "new-sampler-slot-and-block", `#pragma aego abi 1
#pragma material
uniform vec3 Padding; // @default(0,0,0)
uniform sampler2D Auxiliary;
uniform sampler2D DataTex;
uniform float Weight; // @default(0.9)
#pragma end
void fragment(){aego_out=texture(DataTex,vec2(0.5))*Weight+vec4(Padding,0.0);}`)
	r.Materials().QueueReload(sh, next, []*material.Material{m}, nil)
	drawMaterialFrame(t, r, dev, target, draw)
	after := drawMaterialFrame(t, r, dev, target, draw)
	for _, got := range [][4]byte{before, after} {
		for i, n := range [4]int{16, 48, 0, 64} {
			if math.Abs(float64(int(got[i])-n)) > 2 {
				t.Fatalf("sampler/block was not rebound: before=%v after=%v", before, after)
			}
		}
	}
	p, _ := next.Schema.Param("DataTex")
	if m.Shader() != next || p.Slot != shader.TexUser+1 {
		t.Fatal("test did not commit the relocated sampler")
	}
}
