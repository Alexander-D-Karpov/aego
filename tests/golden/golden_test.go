//go:build aego_golden

package golden

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/core/log"
	"aego/core/mathx"
	"aego/engine"
	"aego/gpu"
	"aego/render"
	rendermodule "aego/render/module"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/render/view"
)

const size = 256

var update = flag.Bool("update", false, "rewrite reference images")

type sceneFunc func(*render.Renderer, gpu.TextureHandle)

type testCase struct {
	name      string
	tolerance int
	scene     sceneFunc
}

func TestGolden(t *testing.T) {
	mod := rendermodule.New(render.Options{MaxSprites: 4096})
	e, err := engine.NewBuilder(engine.Config{
		Title:    "golden",
		Width:    size,
		Height:   size,
		Hidden:   true,
		ResRoot:  "../../demos/res",
		UserRoot: t.TempDir(),
		AppID:    "aego-golden",
	}).WithLogger(log.Nop()).Use(mod).Build()
	if err != nil {
		t.Skipf("no GL context available: %v", err)
	}
	defer e.Close()

	renderer := e.Caps().Renderer
	t.Logf("golden renderer: %s", renderer)
	if !softwareRenderer(renderer) {
		if os.Getenv("AEGO_GOLDEN_ALLOW_HARDWARE") == "" {
			t.Fatalf("golden references require a software rasterizer, got %q; "+
				"run with SDL_VIDEODRIVER=offscreen LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe, "+
				"or set AEGO_GOLDEN_ALLOW_HARDWARE=1 to compare anyway", renderer)
		}
		t.Logf("comparing against golden references on a hardware renderer %q, results are advisory", renderer)
	}
	if *update && !softwareRenderer(renderer) {
		t.Fatalf("refusing to write golden references from a hardware renderer %q", renderer)
	}
	r := mod.Renderer()

	target, err := e.GPU().CreateTexture(gpu.TextureDesc{
		Width: size, Height: size, Format: gpu.TexSRGBA8, Label: "golden.target",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases(t, r) {
		t.Run(tc.name, func(t *testing.T) {
			frame := e.GPU().BeginFrame(size, size)
			if err := r.BeginFrame(frame, size, size, 1); err != nil {
				t.Fatal(err)
			}
			tc.scene(r, target)
			r.EndFrame()
			e.GPU().EndFrame()

			buf := make([]byte, size*size*4)
			if err := e.GPU().ReadPixels(target, 0, 0, size, size, buf); err != nil {
				t.Fatal(err)
			}
			img := &image.NRGBA{Pix: buf, Stride: size * 4, Rect: image.Rect(0, 0, size, size)}
			path := filepath.Join("testdata", tc.name+".png")
			if *update {
				writePNG(t, path, img)
				return
			}
			compare(t, path, img, tc.tolerance)
		})
	}
}

func softwareRenderer(name string) bool {
	n := strings.ToLower(name)
	for _, s := range []string{"llvmpipe", "softpipe", "swrast", "lavapipe", "null"} {
		if strings.Contains(n, s) {
			return true
		}
	}
	return false
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func compare(t *testing.T, path string, got *image.NRGBA, tolerance int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("reference missing, run with -update: %v", err)
	}
	defer f.Close()
	want, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	differing := 0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if diff(want.At(x, y), got.At(x, y)) > tolerance {
				differing++
			}
		}
	}
	if pct := float64(differing) * 100 / float64(size*size); pct > 0.5 {
		writePNG(t, filepath.Join("testdata", "actual-"+filepath.Base(path)), got)
		t.Fatalf("%.3f%% of pixels differ beyond tolerance %d", pct, tolerance)
	}
}

func diff(a, b color.Color) int {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	d := 0
	for _, p := range [4][2]uint32{{ar, br}, {ag, bg}, {ab, bb}, {aa, ba}} {
		v := int(p[0]>>8) - int(p[1]>>8)
		if v < 0 {
			v = -v
		}
		if v > d {
			d = v
		}
	}
	return d
}

func targetView(target gpu.TextureHandle, clear mathx.Color) view.View {
	return view.View{
		Target:     target,
		TargetW:    size,
		TargetH:    size,
		Viewport:   mathx.Rect{W: size, H: size},
		PixelScale: 1,
		Camera:     view.Camera2D{Zoom: 32},
		Clear:      clear,
		DoClear:    true,
		Label:      "golden",
	}
}

func cases(t *testing.T, r *render.Renderer) []testCase {
	t.Helper()
	white, err := r.Textures().Upload(texture.Solid(8, 8, mathx.Color{R: 1, G: 1, B: 1, A: 1}), texture.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	solid := texture.Whole(white, 8, 8)
	atlas := buildAtlas(t, r)
	black := mathx.Color{A: 1}

	return []testCase{
		{"alpha", 0, func(r *render.Renderer, tgt gpu.TextureHandle) {
			f := r.View(targetView(tgt, black))
			for i := 0; i < 3; i++ {
				f.Sprite(sprite.Draw{
					Region:   solid,
					Position: mathx.Vec2{X: float32(i)*0.6 - 0.6},
					Size:     mathx.Vec2{X: 2, Y: 2},
					Origin:   mathx.Vec2{X: 0.5, Y: 0.5},
					Tint:     mathx.Color{R: 1, G: 0.5, B: 0.2, A: 0.4},
					Order:    int16(i),
				})
			}
			f.End()
		}},
		{"rotation", 4, func(r *render.Renderer, tgt gpu.TextureHandle) {
			f := r.View(targetView(tgt, black))
			for i := 0; i < 6; i++ {
				f.Sprite(sprite.Draw{
					Region:   atlas,
					Position: mathx.Vec2{X: float32(i%3)*2 - 2, Y: float32(i/3)*2 - 1},
					Size:     mathx.Vec2{X: 1.5, Y: 1.5},
					Origin:   mathx.Vec2{X: 0.5, Y: 0.5},
					Rotation: float32(i) * 0.3,
					Tint:     mathx.Color{R: 1, G: 1, B: 1, A: 1},
				})
			}
			f.End()
		}},
		{"camera", 4, func(r *render.Renderer, tgt gpu.TextureHandle) {
			v := targetView(tgt, black)
			v.Camera = view.Camera2D{Position: mathx.Vec2{X: 1, Y: -0.5}, Rotation: 0.4, Zoom: 48}
			f := r.View(v)
			for y := -2; y <= 2; y++ {
				for x := -2; x <= 2; x++ {
					f.Sprite(sprite.Draw{
						Region:   atlas,
						Position: mathx.Vec2{X: float32(x), Y: float32(y)},
						Size:     mathx.Vec2{X: 0.9, Y: 0.9},
						Origin:   mathx.Vec2{X: 0.5, Y: 0.5},
						Tint:     mathx.Color{R: 1, G: 1, B: 1, A: 1},
					})
				}
			}
			f.End()
		}},
		{"atlas", 0, func(r *render.Renderer, tgt gpu.TextureHandle) {
			f := r.View(targetView(tgt, black))
			for i := 0; i < 4; i++ {
				f.Sprite(sprite.Draw{
					Region:   atlas.Grid(2, 2, i),
					Position: mathx.Vec2{X: float32(i%2)*2 - 1, Y: float32(i/2)*2 - 1},
					Size:     mathx.Vec2{X: 1.8, Y: 1.8},
					Origin:   mathx.Vec2{X: 0.5, Y: 0.5},
					Tint:     mathx.Color{R: 1, G: 1, B: 1, A: 1},
				})
			}
			f.End()
		}},
		{"clipping", 0, func(r *render.Renderer, tgt gpu.TextureHandle) {
			v := targetView(tgt, black)
			f := r.View(v)
			f.PushClip(mathx.Rect{X: 40, Y: 40, W: 120, H: 90})
			f.Sprite(sprite.Draw{
				Region: solid,
				Size:   mathx.Vec2{X: 8, Y: 8},
				Origin: mathx.Vec2{X: 0.5, Y: 0.5},
				Tint:   mathx.Color{R: 0.2, G: 0.9, B: 0.4, A: 1},
			})
			f.PopClip()
			f.End()
		}},
		{"srgb", 4, func(r *render.Renderer, tgt gpu.TextureHandle) {
			f := r.View(targetView(tgt, black))
			for i := 0; i < 8; i++ {
				g := float32(i) / 7
				f.Sprite(sprite.Draw{
					Region:   solid,
					Position: mathx.Vec2{X: float32(i)*0.9 - 3.2},
					Size:     mathx.Vec2{X: 0.85, Y: 4},
					Origin:   mathx.Vec2{X: 0.5, Y: 0.5},
					Tint:     mathx.Color{R: g, G: g, B: g, A: 1},
				})
			}
			f.End()
		}},
		{"text", 0, func(r *render.Renderer, tgt gpu.TextureHandle) {
			v := view.ScreenView(size, size, 1)
			v.Target = tgt
			v.Clear = black
			v.DoClear = true
			f := r.View(v)
			f.Text(r.DebugFont(), "aego\ngolden\n0123456789", mathx.Vec2{X: 16, Y: 24}, 2,
				mathx.Color{R: 1, G: 1, B: 1, A: 1}, 0, 0)
			f.End()
		}},
		{"offscreen_flip", 0, func(r *render.Renderer, tgt gpu.TextureHandle) {
			v := view.ScreenView(size, size, 1)
			v.Target = tgt
			v.Clear = black
			v.DoClear = true
			f := r.View(v)
			f.Sprite(sprite.Draw{
				Region: solid,
				Size:   mathx.Vec2{X: size, Y: 32},
				Tint:   mathx.Color{R: 1, G: 0.2, B: 0.1, A: 1},
			})
			f.Sprite(sprite.Draw{
				Region:   solid,
				Position: mathx.Vec2{Y: size - 32},
				Size:     mathx.Vec2{X: size, Y: 32},
				Tint:     mathx.Color{R: 0.1, G: 0.3, B: 1, A: 1},
			})
			f.End()
		}},
	}
}

func buildAtlas(t *testing.T, r *render.Renderer) texture.Region {
	t.Helper()
	d := texture.New(64, 64, gpu.TexSRGBA8)
	colors := [4]mathx.Color{
		mathx.ColorRGB8(0xE0, 0x7B, 0x39),
		mathx.ColorRGB8(0x52, 0xA7, 0xC9),
		mathx.ColorRGB8(0x8A, 0xC9, 0x52),
		mathx.ColorRGB8(0xC9, 0x52, 0x8A),
	}
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			cell := (x/32)%2 + ((y/32)%2)*2
			c := colors[cell]
			if (x/4+y/4)%2 == 0 {
				c = c.Lerp(mathx.Color{A: 1}, 0.3)
			}
			d.SetPixel(x, y, c)
		}
	}
	tex, err := r.Textures().Upload(d, texture.UploadOptions{Label: "golden.atlas"})
	if err != nil {
		t.Fatal(err)
	}
	return texture.Whole(tex, 64, 64)
}
