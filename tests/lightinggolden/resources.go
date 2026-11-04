package lightinggolden

import (
	"embed"
	"fmt"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render"
	"aego/render/lighting"
	"aego/render/material"
	"aego/render/texture"
)

//go:embed fixtures/*.aesh
var preparedFixtures embed.FS

func RunSurfaceResources(g *Rig, report *Report) {
	RunEmissionChannels(g, report)
	report.Case("normal-own-atlas-region-trim-and-frame-mapping", func() error {
		// An unrelated island intentionally surrounds the normal. Sampling the
		// albedo UV or reading past the normal rectangle gives a distinct normal.
		pixels := make([]byte, 16*8*4)
		for y := 0; y < 8; y++ {
			for x := 0; x < 16; x++ {
				i := (y*16 + x) * 4
				pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = 255, 128, 128, 255
				if x >= 10 && x < 14 && y >= 2 && y < 6 {
					pixels[i], pixels[i+1], pixels[i+2] = 191, 223, 223
				}
			}
		}
		nh, err := g.Texture(gpu.TextureDesc{Width: 16, Height: 8, Format: gpu.TexRGBA8, Data: pixels})
		if err != nil {
			return err
		}
		for i := range pixels {
			pixels[i] = 255
		}
		ah, err := g.Texture(gpu.TextureDesc{Width: 16, Height: 8, Format: gpu.TexRGBA8, Data: pixels})
		if err != nil {
			return err
		}
		normal := texture.Whole(nh, 16, 8).Sub(10, 2, 4, 4)
		normal.SourceW, normal.SourceH, normal.TrimX, normal.TrimY = 8, 8, 2, 2
		albedo := texture.Whole(ah, 16, 8).Sub(0, 0, 4, 8)
		albedo.SourceW, albedo.SourceH, albedo.TrimX = 8, 8, 2
		d := g.Sprite(g.Lit)
		d.Region = albedo
		d.Normal = normal
		in := Input(lighting.LDR)
		in.Debug = lighting.DebugNormals
		s := WhiteSurface()
		s.Width = 12
		s.NormalAt = func(u, v float64) [3]float64 {
			if v < .25 || v > .75 {
				return [3]float64{0, 0, 1}
			}
			return NormalReference([3]byte{191, 223, 223}, s.Basis, false, false)
		}
		got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		if err = report.Image("normal-own-atlas-region-trim-and-frame-mapping", got, Reference(in, [4]float64{}, s), 2); err != nil {
			return err
		}
		// A later frame can have another independently packed normal rectangle.
		d.Normal = texture.Whole(nh, 16, 8).Sub(2, 1, 4, 4)
		s.NormalAt = nil
		s.Normal = NormalReference([3]byte{255, 128, 128}, s.Basis, false, false)
		got, err = g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		if err = report.Image("normal-second-frame-own-region", got, Reference(in, [4]float64{}, s), 2); err != nil {
			return err
		}
		d.Normal = texture.Region{}
		s.Normal = [3]float64{0, 0, 1}
		got, err = g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		return report.Image("normal-missing-frame-flat", got, Reference(in, [4]float64{}, s), 2)
	})
	report.Case("six-user-samplers-plus-normal-emission-shadow-cookie", func() error {
		data, err := preparedFixtures.ReadFile("fixtures/resources.aesh")
		if err != nil {
			return err
		}
		artifact, err := asset.DecodeShader(data)
		if err != nil {
			return err
		}
		sh := artifact.Runtime(ids.AssetID{}, "lighting-six-samplers")
		m, ok := material.New("lighting-six-samplers", sh, 0, sh.Blend)
		if !ok {
			return fmt.Errorf("cannot create fixture material")
		}
		for i, name := range []string{"One", "Two", "Three", "Four", "Five", "Six"} {
			r := []byte{47, 71, 97, 131, 173, 223}[i]
			h, err := g.Texture(gpu.TextureDesc{Width: 1, Height: 1, Format: gpu.TexRGBA8, Data: []byte{r, 0, 0, 255}})
			if err != nil {
				return err
			}
			if !m.SetTexture(name, material.Tex{Texture: h}) {
				return fmt.Errorf("failed to bind %s", name)
			}
		}
		if !m.SetTexture("NormalMap", material.Tex{Texture: g.Normal.Texture, Region: g.Normal, NormalMap: true}) {
			return fmt.Errorf("normal reserved binding rejected")
		}
		eh, err := g.Texture(gpu.TextureDesc{Width: 1, Height: 1, Format: gpu.TexRGBA8, Data: []byte{128, 128, 128, 0}})
		if err != nil {
			return err
		}
		if !m.SetTexture("EmissionMap", material.Tex{Texture: eh, EmissionMode: texture.EmissionRGB}) {
			return fmt.Errorf("emission reserved binding rejected")
		}
		in := ShadowInput(lighting.Sprite)
		in.Lights[0].Cookie = g.Cookie
		d := g.Sprite(m)
		s := WhiteSurface()
		s.Normal = NormalReference([3]byte{191, 223, 223}, s.Basis, false, false)
		s.Emission = [3]float64{.1 * 128 / 255, .15 * 128 / 255, .2 * 128 / 255}
		got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		return report.Image("six-user-samplers-plus-normal-emission-shadow-cookie", got, Reference(in, [4]float64{}, s), 2)
	})
	report.Case("shadow-array-layer-preview-is-real-resource", func() error {
		h, err := g.Texture(gpu.TextureDesc{Width: Size, Height: Size, Format: gpu.TexRGBA8})
		if err != nil {
			return err
		}
		in := ShadowInput(lighting.Point)
		in.Lights[0].Position = mathx.Vec2{X: 16, Y: 6}
		in.Points = []mathx.Vec2{{X: -64, Y: 15}, {X: 96, Y: 15}}
		d := g.Sprite(g.Lit)
		frame := g.Device.BeginFrame(Size, Size)
		if err = g.Renderer.BeginFrame(frame, Size, Size, 1); err != nil {
			return err
		}
		v := g.Renderer.View(g.View())
		v.Lighting(&in)
		v.Sprite(d)
		v.End()
		layers := g.Renderer.LightingShadowLayers(g.View().ID, nil)
		if len(layers) != 1 {
			return fmt.Errorf("active shadow layers %v", layers)
		}
		err = g.Renderer.PreviewShadowLayer(g.View().ID, layers[0].Layer, h, Size, Size)
		g.Renderer.EndFrame()
		g.Device.EndFrame()
		if err != nil {
			return err
		}
		if err = g.Device.ReadPixels(h, 0, 0, Size, Size, g.Pixels); err != nil {
			return err
		}
		want := make([]byte, Size*Size*4)
		for y := 0; y < Size; y++ {
			for x := 0; x < Size; x++ {
				i := (y*Size + x) * 4
				c := byte(255)
				worldY := 6 - 40 + (float64(y)+.5)*80/Size
				if worldY >= 15 {
					c = 0
				}
				want[i], want[i+1], want[i+2], want[i+3] = c, c, c, 255
			}
		}
		return report.Image("shadow-array-layer-preview-is-real-resource", g.Pixels, want, 1)
	})
}
