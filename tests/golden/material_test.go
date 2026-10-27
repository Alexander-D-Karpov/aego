//go:build aego_golden

package golden

import (
	"testing"

	"aego/app/playground"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render"
	"aego/render/sprite"
	"aego/render/texture"
)

// materialCases draw the playground's bench material, which reads aego_time and a parameter, at the bench's pinned time; web-smoke draws it on WebGL2.
func materialCases(t *testing.T, r *render.Renderer) []testCase {
	t.Helper()
	m, err := playground.BenchMaterial()
	if err != nil {
		t.Fatal(err)
	}
	white, err := r.Textures().Upload(texture.Solid(8, 8, mathx.Color{R: 1, G: 1, B: 1, A: 1}), texture.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	solid := texture.Whole(white, 8, 8)
	clock := playground.BenchClock
	caseClocks["material_time"] = &clock
	return []testCase{{"material_time", 2, func(r *render.Renderer, tgt gpu.TextureHandle) {
		f := r.View(targetView(tgt, mathx.Color{R: 0.07, G: 0.08, B: 0.1, A: 1}))
		for i := 0; i < 16; i++ {
			f.Sprite(sprite.Draw{
				Region:   solid,
				Position: mathx.Vec2{X: float32(i%4)*1.5 - 2.25, Y: float32(i/4)*1.5 - 2.25},
				Size:     mathx.Vec2{X: 1.2, Y: 1.2},
				Origin:   mathx.Vec2{X: 0.5, Y: 0.5},
				Tint:     mathx.Color{R: 1, G: 1 - float32(i)/16, B: float32(i) / 16, A: 1},
				Material: m,
			})
		}
		f.End()
	}}}
}
