//go:build aego_gltest

package gl

import (
	"fmt"
	"math"
	"testing"

	"aego/asset"
	"aego/core/log"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render"
	"aego/render/material"
	"aego/render/sprite"
	"aego/render/view"
)

// Pixel positions are checked against a CPU rectangle in top-left physical
// coordinates. An odd viewport origin/height and a four-pixel grid distinguish
// window Y orientation from the offscreen projection; grid=1 masks that bug.
func TestPixelSnapPhysicalGridAcrossDPRViewportAndTargets(t *testing.T) {
	e := newGLEngine(t)
	defer e.Close()
	dev := e.GPU()
	r, err := render.New(dev, e.Caps(), e.VFS(), log.Nop(), render.Options{MaxSprites: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.Materials().OnError = func(sh *material.Shader, variant int, err error) {
		t.Errorf("shader %s variant %d: %v", sh.Name, variant, err)
	}
	data, _ := asset.BuiltinMaterial(asset.BuiltinMaterialPixelSnap)
	doc, err := asset.DecodeMaterial(data, "PixelSnap")
	if err != nil {
		t.Fatal(err)
	}
	data, _ = asset.BuiltinShader(doc.Shader)
	artifact, err := asset.DecodeShader(data)
	if err != nil {
		t.Fatal(err)
	}
	m, ps := doc.Build("PixelSnap", artifact.Runtime(doc.Shader, "PixelSnap"), nil)
	if m == nil {
		t.Fatal(ps)
	}
	if !m.Set("GridPixels", [4]float32{4}) {
		t.Fatal("PixelSnap has no GridPixels parameter")
	}
	const size = 64
	target, err := dev.CreateTexture(gpu.TextureDesc{Width: size, Height: size, Format: gpu.TexRGBA8, Label: "pixelsnap-offscreen"})
	if err != nil {
		t.Fatal(err)
	}
	defer dev.DestroyTexture(target)
	backbuffer, ok := dev.(interface{ ReadBackbuffer(int, int, []byte) error })
	if !ok {
		t.Fatal("native backend does not expose backbuffer readback")
	}
	viewport := mathx.Rect{X: 3, Y: 5, W: 45, H: 39}
	position, extent := mathx.Vec2{X: 5.2, Y: 6.3}, mathx.Vec2{X: 8.4, Y: 7.1}
	pixels := make([]byte, size*size*4)
	for _, offscreen := range []bool{false, true} {
		for _, dpr := range []float32{1, 2} {
			for _, strength := range []float32{0, 1} {
				name := fmt.Sprintf("offscreen=%v DPR=%g strength=%g", offscreen, dpr, strength)
				if !m.Set("Strength", [4]float32{strength}) {
					t.Fatal("PixelSnap has no Strength parameter")
				}
				frame := dev.BeginFrame(size, size)
				if err := r.BeginFrame(frame, size, size, dpr); err != nil {
					t.Fatal(err)
				}
				v := view.ScreenView(size, size, dpr)
				v.Viewport = viewport
				v.Camera.Position = mathx.Vec2{X: viewport.W / (2 * dpr), Y: viewport.H / (2 * dpr)}
				v.DoClear = true
				v.ClearWholeTarget = true // The pixel oracle checks the full attachment, not only the camera viewport.
				if offscreen {
					v.Target = target
				}
				vf := r.View(v)
				vf.Sprite(sprite.Draw{Region: r.White(), Position: position, Size: extent, Tint: mathx.White, Material: m})
				vf.End()
				r.EndFrame()
				dev.EndFrame()
				if offscreen {
					err = dev.ReadPixels(target, 0, 0, size, size, pixels)
				} else {
					err = backbuffer.ReadBackbuffer(size, size, pixels)
				}
				if err != nil {
					t.Fatalf("%s readback: %v", name, err)
				}
				snap := func(p float32) float32 {
					if strength == 0 {
						return p
					}
					return float32(math.Floor(float64(p)/4+0.5) * 4)
				}
				left, top := snap(viewport.X+position.X*dpr), snap(viewport.Y+position.Y*dpr)
				right, bottom := snap(viewport.X+(position.X+extent.X)*dpr), snap(viewport.Y+(position.Y+extent.Y)*dpr)
				for y := 0; y < size; y++ {
					row := y
					if !offscreen {
						row = size - 1 - y // Native window readback is bottom-up.
					}
					for x := 0; x < size; x++ {
						visible := float32(x)+0.5 >= left && float32(x)+0.5 < right && float32(y)+0.5 >= top && float32(y)+0.5 < bottom
						want := byte(0)
						if visible {
							want = 255
						}
						at := (row*size + x) * 4
						if pixels[at] != want || pixels[at+1] != want || pixels[at+2] != want {
							t.Fatalf("%s pixel (%d,%d)=%v, expected RGB %d in rectangle [%g,%g]→[%g,%g]", name, x, y, pixels[at:at+4], want, left, top, right, bottom)
						}
						if offscreen && pixels[at+3] != want {
							t.Fatalf("%s alpha (%d,%d)=%d, expected %d", name, x, y, pixels[at+3], want)
						}
					}
				}
			}
		}
	}
}
