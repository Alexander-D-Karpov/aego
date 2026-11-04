//go:build aego_gltest

package gl

import (
	"math"
	"testing"

	"aego/asset"
	"aego/core/curve"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/modules/particles"
	"aego/render"
	"aego/render/material"
	"aego/render/sprite"
	"aego/render/view"
	"aego/scene/frame"
)

type particleMaterial struct {
	asset.Resolver
	m *material.Material
}

func (p particleMaterial) Material(id ids.AssetID) (*material.Material, bool) {
	return p.m, id == asset.BuiltinMaterialAdditive
}
func (particleMaterial) Generation() uint32 { return 1 }

func TestParticleOpacityAndAdditivePreserveCoverage(t *testing.T) {
	r, dev, target := materialRig(t)
	data, _ := asset.BuiltinMaterial(asset.BuiltinMaterialAdditive)
	doc, err := asset.DecodeMaterial(data, "Additive")
	if err != nil {
		t.Fatal(err)
	}
	data, _ = asset.BuiltinShader(doc.Shader)
	sh, err := asset.DecodeShader(data)
	if err != nil {
		t.Fatal(err)
	}
	m, issues := doc.Build("Additive", sh.Runtime(doc.Shader, "Additive"), func(ids.AssetID) (material.Tex, bool) { return material.Tex{Texture: r.White().Texture}, true })
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	res := particleMaterial{m: m}
	pose := mathx.Mat3TRS(mathx.V2(16, 16), 0, mathx.V2(1, 1))
	for _, additive := range []bool{false, true} {
		c := particles.DefaultParticleSystem2D()
		c.PlayOnStart = false
		c.Rate = 0
		c.Lifetime = mathx.V2(1, 1)
		c.Size = mathx.V2(12, 12)
		c.Speed = mathx.V2(0, 0)
		c.Color = mathx.Color{R: 1, A: .8}
		c.AlphaOverLife = curve.Constant(.5)
		if additive {
			c.Material.ID = asset.BuiltinMaterialAdditive
		}
		p := particles.NewPreview(nil, res)
		if err := p.Configure(c, 21); err != nil {
			t.Fatal(err)
		}
		p.Step(0, pose)
		p.Burst(1)
		runner := frame.New()
		runner.Bind(nil, r, res)
		runner.AddView(view.ScreenView(32, 32, 1), 0)
		p.Extract(runner, pose)
		if len(runner.Bucket(0).Sprites) != 1 {
			t.Fatal("missing particle")
		}
		for _, bg := range []mathx.Color{{}, {R: .2, G: .2, B: .2, A: 1}, {R: .7, G: .7, B: .7, A: 1}} {
			px := drawMaterialFrame(t, r, dev, target, func(v *render.ViewFrame) {
				v.Sprite(sprite.Draw{Region: r.White(), Position: mathx.V2(16, 16), Size: mathx.V2(32, 32), Origin: mathx.V2(.5, .5), Tint: bg, Order: -1})
				v.Sprites(runner.Bucket(0).Sprites)
			})
			factor := float32(.6)
			alpha := float32(.4) + bg.A*factor
			if additive {
				factor = 1
				alpha = bg.A
			}
			want := [4]float32{min(float32(1), .4+bg.R*factor), bg.G * factor, bg.B * factor, alpha}
			for k, v := range want {
				if math.Abs(float64(px[k])-float64(v)*255) > 2 {
					t.Fatalf("additive=%v background=%v: got %v, want %v", additive, bg, px, want)
				}
			}
		}
		p.Close()
	}
}
