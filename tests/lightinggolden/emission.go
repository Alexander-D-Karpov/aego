package lightinggolden

import (
	"fmt"
	"math"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render"
	"aego/render/lighting"
	"aego/render/material"
	"aego/render/texture"
)

// The reference values are derived from authoring channel semantics. They do
// not read generated GLSL or bless a backend's output as another backend's golden.
func RunEmissionChannels(g *Rig, report *Report) {
	defer g.Lit.Reset("EmissionColor")
	defer g.Lit.Reset("EmissionIntensity")
	defer g.Lit.Reset("EmissionMap")
	defer g.LitMesh.Reset("EmissionColor")
	defer g.LitMesh.Reset("EmissionIntensity")
	g.Lit.Set("EmissionColor", [4]float32{.25, .5, .75, 1})
	g.Lit.Set("EmissionIntensity", [4]float32{1.5})
	in := Input(lighting.LDR)
	for _, tc := range []struct {
		name   string
		format gpu.TextureFormat
		mode   texture.EmissionMode
	}{
		{"rgb-linear-alpha-ignored", gpu.TexRGBA8, texture.EmissionRGB},
		{"rgb-srgb-decoded-once", gpu.TexSRGBA8, texture.EmissionRGB},
		{"mask-is-scalar-not-color", gpu.TexRGBA8, texture.EmissionMask},
	} {
		report.Case("emission-"+tc.name, func() error {
			h, err := g.Texture(gpu.TextureDesc{Width: 1, Height: 1, Format: tc.format, Data: []byte{51, 102, 153, 0}})
			if err != nil {
				return err
			}
			d := g.Sprite(g.Lit)
			d.Tint = mathx.Color{R: .8, G: .6, B: .4, A: .5}
			d.Emission = texture.Emission{Region: texture.Whole(h, 1, 1), Mode: tc.mode}
			s := WhiteSurface()
			s.Color = PMA(d.Tint)
			for c, sample := range []float64{.2, .4, .6} {
				if tc.mode == texture.EmissionMask {
					sample = .2
				}
				if tc.format == gpu.TexSRGBA8 {
					if sample <= .04045 {
						sample /= 12.92
					} else {
						sample = math.Pow((sample+.055)/1.055, 2.4)
					}
				}
				s.Emission[c] = sample * []float64{.25, .5, .75}[c] * 1.5
			}
			got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
			if err != nil {
				return err
			}
			return report.Image("emission-"+tc.name, got, Reference(in, [4]float64{}, s), 2)
		})
	}
	g.Lit.Set("EmissionColor", [4]float32{1, 1, 1, 1})
	g.Lit.Set("EmissionIntensity", [4]float32{1})
	pixels := make([]byte, 8*8*4)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			i := (y*8 + x) * 4
			pixels[i], pixels[i+2], pixels[i+3] = 255, 255, 0 // conspicuous unrelated atlas island
			if x >= 3 && x < 7 && y >= 1 && y < 5 {
				pixels[i], pixels[i+1], pixels[i+2] = 0, 128, 0
			}
		}
	}
	h, err := g.Texture(gpu.TextureDesc{Width: 8, Height: 8, Format: gpu.TexRGBA8, Data: pixels})
	if err != nil {
		report.Failures = append(report.Failures, "emission atlas: "+err.Error())
		return
	}
	region := texture.Whole(h, 8, 8).Sub(3, 1, 4, 4)
	region.SourceW, region.SourceH, region.TrimX, region.TrimY = 8, 8, 1, 2
	for _, flip := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
		name := fmt.Sprintf("emission-independent-atlas-trim-flip-%t-%t", flip[0], flip[1])
		d := g.Sprite(g.Lit)
		d.FlipX, d.FlipY = flip[0], flip[1]
		d.Emission = texture.Emission{Region: region, Mode: texture.EmissionRGB}
		s := WhiteSurface()
		s.EmissionAt = func(u, v float64) [3]float64 {
			if flip[0] {
				u = 1 - u
			}
			if flip[1] {
				v = 1 - v
			}
			if u < .125 || u > .625 || v < .25 || v > .75 {
				return [3]float64{}
			}
			return [3]float64{0, 128.0 / 255, 0}
		}
		g.imageCase(report, name, in, s, func(v *render.ViewFrame) { v.Sprite(d) })
	}
	report.Case("emission-material-map-overrides-frame-and-reset-inherits", func() error {
		d := g.Sprite(g.Lit)
		d.Emission = texture.Emission{Region: region, Mode: texture.EmissionRGB}
		if !g.Lit.SetTexture("EmissionMap", material.Tex{}) {
			return fmt.Errorf("explicit empty emission rejected")
		}
		s := WhiteSurface()
		s.Emission = [3]float64{1, 1, 1}
		got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		if err = report.Image("emission-explicit-empty-overrides-pair", got, Reference(in, [4]float64{}, s), 2); err != nil {
			return err
		}
		g.Lit.Reset("EmissionMap")
		s.Emission = [3]float64{}
		s.EmissionAt = func(u, v float64) [3]float64 {
			if u < .125 || u > .625 || v < .25 || v > .75 {
				return [3]float64{}
			}
			return [3]float64{0, 128.0 / 255, 0}
		}
		got, err = g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		return report.Image("emission-reset-inherits-frame", got, Reference(in, [4]float64{}, s), 2)
	})
	report.Case("emission-mesh-own-region", func() error {
		g.LitMesh.Set("EmissionColor", [4]float32{1, 1, 1, 1})
		g.LitMesh.Set("EmissionIntensity", [4]float32{1})
		d := g.Mesh(false)
		d.Emission = texture.Emission{Region: region, Mode: texture.EmissionRGB}
		s := WhiteSurface()
		s.EmissionAt = func(u, v float64) [3]float64 {
			if u < .125 || u > .625 || v < .25 || v > .75 {
				return [3]float64{}
			}
			return [3]float64{0, 128.0 / 255, 0}
		}
		got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Mesh(d) })
		if err != nil {
			return err
		}
		return report.Image("emission-mesh-own-region", got, Reference(in, [4]float64{}, s), 2)
	})
}
