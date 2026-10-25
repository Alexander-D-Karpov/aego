//go:build aego_golden

package golden

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render/sprite"
	"aego/render/view"
)

// A sprite tinted #00000000 is transparent black: it leaves no pixel, whatever its blend mode.
func TestZeroTintDrawsNothing(t *testing.T) {
	s := paletteSession(t)
	e := s.Engine
	const w, h = 64, 64
	draw := func(tint mathx.Color) []byte {
		t.Helper()
		tgt, err := e.GPU().CreateTexture(gpu.TextureDesc{Width: w, Height: h, Format: gpu.TexRGBA8, Label: "golden.tint"})
		if err != nil {
			t.Fatal(err)
		}
		frame := e.GPU().BeginFrame(w, h)
		if err := s.Renderer.BeginFrame(frame, w, h, 1); err != nil {
			t.Fatal(err)
		}
		v := view.ScreenView(w, h, 1)
		v.Target, v.Clear, v.DoClear, v.Label = tgt, mathx.Color{A: 1}, true, "golden.tint"
		f := s.Renderer.View(v)
		for i, blend := range []sprite.BlendMode{sprite.BlendPremultiplied, sprite.BlendAdditive} {
			f.Sprite(sprite.Draw{Region: s.Renderer.White(), Position: mathx.Vec2{X: float32(8 + i*28), Y: 8}, Size: mathx.Vec2{X: 24, Y: 48}, Tint: tint, Blend: blend})
		}
		f.End()
		s.Renderer.EndFrame()
		e.GPU().EndFrame()
		buf := make([]byte, w*h*4)
		if err := e.GPU().ReadPixels(tgt, 0, 0, w, h, buf); err != nil {
			t.Fatal(err)
		}
		return buf
	}
	lit := func(buf []byte) int {
		n := 0
		for i := 0; i < len(buf); i += 4 {
			if buf[i] != 0 || buf[i+1] != 0 || buf[i+2] != 0 {
				n++
			}
		}
		return n
	}
	if n := lit(draw(mathx.Color{R: 1, G: 1, B: 1, A: 1})); n == 0 {
		t.Fatal("a white sprite drew nothing; the test does not reach the screen")
	}
	if n := lit(draw(mathx.Color{})); n != 0 {
		t.Fatalf("a #00000000 sprite lit %d pixels", n)
	}
}
