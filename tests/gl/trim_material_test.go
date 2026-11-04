//go:build aego_gltest

package gl

import (
	"testing"

	"aego/core/mathx"
	"aego/render"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/tests/materialgolden"
)

func TestTrimmedAtlasOriginalUVAndFlipImages(t *testing.T) {
	r, dev, target := materialRig(t)
	sh := runtimeShader(t, "trim", `#pragma aego abi 1
#include "engine/sprite.glsl"
#pragma material
uniform int Code; // @instance @default(1)
#pragma end
void fragment(){
    vec4 sampleColor=aego_sample_local(aego_local);
    bool good=Code==16777217 && sampleColor.a>0.99 && sampleColor.r>0.99
      && aego_sample_local(vec2(0.05,0.05)).a==0.0;
    aego_out=good?vec4(aego_local,0.0,1.0):vec4(1.0,0.0,0.0,1.0);
}`)
	m, _ := material.New("trim", sh, 0, shader.BlendPremultiplied)
	lanes := *m.DefaultLanes()
	param, _ := sh.Schema.Param("Code")
	material.PutLaneInt(&lanes, param, 16777217)
	data := texture.Solid(5, 4, mathx.Color{G: 1, A: 1})
	for y := 1; y < 3; y++ {
		for x := 1; x < 4; x++ {
			data.SetPixel(x, y, mathx.White)
		}
	}
	h, err := r.Textures().Upload(data, texture.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	reg := texture.Whole(h, 5, 4).Sub(1, 1, 3, 2)
	reg.SourceW, reg.SourceH, reg.TrimX, reg.TrimY = 8, 6, 2, 1
	pixels := make([]byte, 32*32*4)
	for i, name := range []string{"trim-none", "trim-x", "trim-y", "trim-xy"} {
		drawMaterialFrame(t, r, dev, target, func(v *render.ViewFrame) {
			v.Sprite(sprite.Draw{Region: reg, Position: mathx.Vec2{X: 16, Y: 16}, Size: mathx.Vec2{X: 16, Y: 12}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: m, Lanes: &lanes, FlipX: i&1 != 0, FlipY: i&2 != 0})
		})
		if err := dev.ReadPixels(target, 0, 0, 32, 32, pixels); err != nil {
			t.Fatal(err)
		}
		if err := materialgolden.Compare(name, pixels, 2); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
