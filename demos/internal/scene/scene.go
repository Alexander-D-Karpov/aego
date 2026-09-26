package scene

import (
	"math/rand"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render"
	"aego/render/sprite"
	"aego/render/texture"
)

const AtlasSize = 128

func BuildAtlas(r *render.Renderer) (texture.Region, error) {
	d := texture.New(AtlasSize, AtlasSize, gpu.TexSRGBA8)
	colors := [4]mathx.Color{
		mathx.ColorRGB8(0xE0, 0x7B, 0x39),
		mathx.ColorRGB8(0x52, 0xA7, 0xC9),
		mathx.ColorRGB8(0x8A, 0xC9, 0x52),
		mathx.ColorRGB8(0xC9, 0x52, 0x8A),
	}
	for cell := 0; cell < 4; cell++ {
		ox := (cell % 2) * 64
		oy := (cell / 2) * 64
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				if x < 2 || y < 2 || x >= 62 || y >= 62 {
					continue
				}
				c := colors[cell]
				if (x/8+y/8)%2 == 0 {
					c = c.Lerp(mathx.Color{R: 0, G: 0, B: 0, A: 1}, 0.25)
				}
				if cell == 3 {
					c.A = float32(x) / 63
				}
				d.SetPixel(ox+x, oy+y, c)
			}
		}
	}
	tex, err := r.Textures().Upload(d, texture.UploadOptions{Label: "demo.atlas"})
	if err != nil {
		return texture.Region{}, err
	}
	return texture.Whole(tex, AtlasSize, AtlasSize), nil
}

func Cell(atlas texture.Region, i int) texture.Region {
	return atlas.Sub((i%2)*64+2, (i/2)*64+2, 60, 60)
}

func Field(atlas texture.Region, n int, extent float32, seed int64) []sprite.Draw {
	rng := rand.New(rand.NewSource(seed))
	out := make([]sprite.Draw, n)
	for i := range out {
		out[i] = sprite.Draw{
			Region:   Cell(atlas, rng.Intn(4)),
			Position: mathx.Vec2{X: (rng.Float32()*2 - 1) * extent, Y: (rng.Float32()*2 - 1) * extent},
			Size:     mathx.Vec2{X: 1, Y: 1},
			Origin:   mathx.Vec2{X: 0.5, Y: 0.5},
			Rotation: rng.Float32() * mathx.TwoPi,
			Tint:     mathx.Color{R: 1, G: 1, B: 1, A: 1},
			Z:        rng.Float32(),
			Layer:    int16(rng.Intn(3)),
		}
	}
	return out
}
