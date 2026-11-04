package asset

import (
	"math"
	"sync"

	"aego/gpu"
	"aego/render/texture"
)

var particlePuff = sync.OnceValue(func() *texture.Data { return particleMask(false) })
var particleStreak = sync.OnceValue(func() *texture.Data { return particleMask(true) })

func particleMask(streak bool) *texture.Data {
	const n = 24
	pixels := make([]byte, n*n*4)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx, dy := (float64(x)+.5)*2/n-1, (float64(y)+.5)*2/n-1
			if streak {
				dx *= 5.5
			} else {
				dx += .07 * math.Cos(8*dy)
			}
			r := math.Sqrt(dx*dx + dy*dy)
			v := min(max(1-r, 0)*1.5, 1.)
			b := byte(255*v*v*(3-2*v) + .5)
			i := 4 * (n*y + x)
			pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = b, b, b, b
		}
	}
	return &texture.Data{Width: n, Height: n, Format: gpu.TexRGBA8, Pixels: pixels, Premultiplied: true}
}
