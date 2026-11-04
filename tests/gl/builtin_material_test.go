//go:build aego_gltest

package gl

import (
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/render"
	"aego/render/material"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/tests/materialgolden"
)

func TestBuiltinMaterialImagesMatchLinearPremultipliedReferences(t *testing.T) {
	r, dev, target := materialRig(t)
	d := texture.Solid(4, 4, mathx.Color{R: 1, A: .5})
	d.SetPixel(0, 0, mathx.Color{})
	h, err := r.Textures().Upload(d, texture.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	region := texture.Whole(h, 4, 4)
	textures := map[ids.AssetID]material.Tex{}
	materials := map[string]*material.Material{}
	for _, name := range materialgolden.Stage12aBuiltins {
		id, _ := asset.BuiltinMaterialNamed(name)
		data, _ := asset.BuiltinMaterial(id)
		doc, err := asset.DecodeMaterial(data, name)
		if err != nil {
			t.Fatal(err)
		}
		data, _ = asset.BuiltinShader(doc.Shader)
		artifact, err := asset.DecodeShader(data)
		if err != nil {
			t.Fatal(err)
		}
		m, problems := doc.Build(name, artifact.Runtime(doc.Shader, name), func(id ids.AssetID) (material.Tex, bool) {
			if tex, ok := textures[id]; ok {
				return tex, true
			}
			data, ok := asset.BuiltinTexture(id)
			if !ok {
				return material.Tex{}, false
			}
			h, err := r.Textures().Upload(data, texture.UploadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			tex := material.Tex{Texture: h}
			textures[id] = tex
			return tex, true
		})
		if m == nil {
			t.Fatalf("%s: %v", name, problems)
		}
		materials[name] = m
	}
	pixels := make([]byte, 32*32*4)
	draw := func(m *material.Material, golden string, checker, twice bool) {
		t.Helper()
		drawMaterialFrame(t, r, dev, target, func(v *render.ViewFrame) {
			if checker {
				for y := 0; y < 4; y++ {
					for x := 0; x < 4; x++ {
						c := float32(.25)
						if (x+y)%2 != 0 {
							c = .75
						}
						v.Sprite(sprite.Draw{Region: r.White(), Position: mathx.Vec2{X: float32(x * 8), Y: float32(y * 8)}, Size: mathx.Vec2{X: 8, Y: 8}, Tint: mathx.Color{R: c, G: c, B: c, A: 1}})
					}
				}
			}
			d := sprite.Draw{Region: region, Position: mathx.Vec2{X: 16, Y: 16}, Size: mathx.Vec2{X: 24, Y: 24}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: m}
			v.Sprite(d)
			if twice {
				v.Sprite(d)
			}
		})
		if err := dev.ReadPixels(target, 0, 0, 32, 32, pixels); err != nil {
			t.Fatal(err)
		}
		if err := materialgolden.Compare(golden, pixels, 2); err != nil {
			t.Fatalf("%s: %v", golden, err)
		}
	}
	for _, name := range materialgolden.Stage12aBuiltins {
		draw(materials[name], "builtin-"+name, false, false)
	}
	unlit := materials["Unlit"]
	unlit.Set("Opacity", [4]float32{.5})
	draw(unlit, "opacity", false, false)
	unlit.Reset("Opacity")
	draw(unlit, "checker-unlit", true, false)
	flash := materials["Flash"]
	flash.Set("Flash", [4]float32{1})
	draw(flash, "flash", false, false)
	draw(flash, "checker-flash-restored", true, false)
	dissolve := materials["Dissolve"]
	dissolve.Set("Threshold", [4]float32{1})
	draw(dissolve, "dissolved", false, false)
	draw(materials["Additive"], "additive", false, true)
}
