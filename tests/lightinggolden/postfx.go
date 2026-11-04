package lightinggolden

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render"
	"aego/render/lighting"
	"aego/render/postfx"
	"aego/render/view"
)

func RunPostProcess(g *Rig, report *Report) {
	reg := postfx.NewRegistry()
	vignette, _ := reg.Lookup(postfx.VignetteID)
	v := postfx.Pass{Descriptor: vignette, Params: [4][4]float32{{.75}, {.25, .7}, {.5}}}
	gain := func(id string, phase postfx.Phase, n float32) postfx.Pass {
		return postfx.Pass{Descriptor: &postfx.Descriptor{ID: id, Phase: phase, Fragment: `
vec4 aego_effect(vec4 c, vec2 uv, vec2 size, vec4 params[4]) { return vec4(c.rgb*params[0].x,c.a); }
`}, Params: [4][4]float32{{n}}}
	}
	p := postfx.Plan{Passes: []postfx.Pass{gain("test.scene-gain", postfx.SceneLinear, .5), v, gain("test.display-gain", postfx.DisplayLinear, .5)}}
	for _, mode := range []lighting.ColorMode{lighting.LDR, lighting.HDR} {
		if mode == lighting.HDR && !g.Device.Caps().RGBA16FRenderable {
			report.Unverified = append(report.Unverified, "postfx HDR pixels: RGBA16F unavailable")
			continue
		}
		report.Case(fmt.Sprintf("postfx-active-viewport-%d", mode), func() error {
			env := lighting.DefaultEnvironment()
			env.ColorMode, env.Exposure = mode, 2
			in := lighting.Input{HasEnvironment: true, Environment: env, OutputLit: true}
			back := g.View()
			back.Clear = mathx.Color{G: 1, A: 1}
			if err := g.Frame(back, nil, func(*render.ViewFrame) {}); err != nil {
				return err
			}
			out := g.View()
			out.Viewport = mathx.Rect{X: 7, Y: 3, W: 11, H: 17}
			out.Clear = mathx.Color{R: .5, A: 1}
			pixels, err := g.DrawView(out, &in, func(f *render.ViewFrame) { f.PostProcess(&p) })
			if err != nil {
				return err
			}
			if err = g.Renderer.PostProcessError(); err != nil {
				return err
			}
			if s := g.Renderer.Stats(); s.PostFXPasses != 3 || s.PostFXTargets != 2 || s.ResolvePasses != 1 {
				return fmt.Errorf("stats %+v", s)
			}
			for y := 0; y < Size; y++ {
				for x := 0; x < Size; x++ {
					want := [4]int{0, 255, 0, 255}
					if x >= 7 && x < 18 && y >= 3 && y < 20 {
						rgb := .5
						if mode == lighting.HDR {
							rgb = rgb / (1 + rgb)
						}
						dx, dy := float64(x-7)+.5-11*.25, float64(y-3)+.5-17*.7
						t := max(0., min(1., (2*math.Hypot(dx, dy)/11-.5)/.5))
						smooth := t * t * (3 - 2*t)
						rgb *= .5 * (1 - .75*smooth)
						want = [4]int{int(math.Round(255 * postFXEncode(rgb))), 0, 0, 255}
					}
					if got := Pixel(pixels, x, y); !NearPixel(got, want, 2) {
						return fmt.Errorf("(%d,%d) got %v want %v +/-2", x, y, got, want)
					}
				}
			}
			return nil
		})
	}
	report.Case("postfx-source-over-two-backdrops", func() error {
		out := g.View()
		out.DoClear = false
		plan := postfx.Plan{Passes: []postfx.Pass{v}}
		draw := g.Sprite(nil)
		draw.Tint = mathx.Color{R: 1, A: .5}
		var got [2][4]byte
		for i, bg := range []mathx.Color{{}, {B: 1, A: 1}} {
			back := g.View()
			back.Clear = bg
			if err := g.Frame(back, nil, func(*render.ViewFrame) {}); err != nil {
				return err
			}
			data, err := g.DrawView(out, nil, func(f *render.ViewFrame) { f.PostProcess(&plan); f.Sprite(draw) })
			if err != nil {
				return err
			}
			if err = g.Renderer.PostProcessError(); err != nil {
				return err
			}
			got[i] = Pixel(data, 6, 6)
		}
		// This pixel is fully vignetted: 25% linear red over two backgrounds.
		red := int(math.Round(postFXEncode(.25) * .5 * 255))
		if !NearPixel(got[0], [4]int{red, 0, 0, 128}, 2) || !NearPixel(got[1], [4]int{red, 0, 127, 255}, 2) {
			return fmt.Errorf("transparent=%v blue=%v expected premultiplied red %d", got[0], got[1], red)
		}
		return nil
	})
	report.Case("bloom-over-composed-opaque-cameras", func() error {
		bloom, _ := reg.NewInstance(postfx.BloomID, "halo")
		bloom.Parameters[0].Value.X = .15
		bloom.Parameters[1].Value.X = 1
		var plan postfx.Plan
		if err := reg.Build(&plan, true, []postfx.Instance{bloom}, postfx.Profile{}); err != nil {
			return err
		}
		v := g.View()
		v.Clear = mathx.Color{A: 1}
		lamp := g.Sprite(nil)
		lamp.Size = mathx.V2(5, 5)
		ref, err := g.DrawView(v, nil, func(f *render.ViewFrame) { f.PostProcess(&plan); f.Sprite(lamp) })
		if err != nil {
			return err
		}
		ref = slices.Clone(ref)
		r := g.Renderer
		if err := r.BeginFrame(g.Device.BeginFrame(Size, Size), Size, Size, 1); err != nil {
			return err
		}
		// Neither clear covers the full output alone. The final camera draws
		// the bright sprite without clearing the composed background.
		for _, area := range []mathx.Rect{{W: Size/2 + 1, H: Size}, {X: Size / 2, W: Size / 2, H: Size}} {
			base := v
			base.DeferResolve = true
			base.Viewport = area
			f := r.View(base)
			f.PostProcess(&plan)
			f.End()
		}
		v.DoClear = false
		f := r.View(v)
		f.PostProcess(&plan)
		f.Sprite(lamp)
		f.End()
		stats := r.EndFrame()
		g.Device.EndFrame()
		if err := r.PostProcessError(); err != nil {
			return err
		}
		if stats.PostFXPasses != 4 || stats.ResolvePasses != 1 {
			return fmt.Errorf("shared output: post passes %d, resolves %d", stats.PostFXPasses, stats.ResolvePasses)
		}
		if err := g.Device.ReadPixels(g.Target, 0, 0, Size, Size, g.Pixels); err != nil {
			return err
		}
		if err := Compare(ref, g.Pixels, 0); err != nil {
			return fmt.Errorf("single and composed Bloom differ: %w", err)
		}
		halo := Pixel(g.Pixels, Size/2+4, Size/2)
		if halo[0] == 0 || halo[3] != 255 {
			return fmt.Errorf("no opaque halo beyond lamp: %v", halo)
		}
		return nil
	})
	report.Case("postfx-fractional-camera-coverage", func() error {
		r := g.Renderer
		if err := r.BeginFrame(g.Device.BeginFrame(Size, Size), Size, Size, 1); err != nil {
			return err
		}
		v := g.View()
		v.Clear = mathx.White
		draw := func(rect mathx.Rect, alpha float32) { v.Viewport = rect; v.Clear.A = alpha; r.View(v).End() }
		draw(v.Viewport, 1)
		draw(mathx.Rect{X: .75, Y: .25, W: 2.5, H: 5.5}, 0)
		draw(mathx.Rect{W: 3, H: 6}, 1)
		r.EndFrame()
		g.Device.EndFrame()
		if err := g.Device.ReadPixels(g.Target, 0, 0, Size, Size, g.Pixels); err != nil {
			return err
		}
		if left, right := Pixel(g.Pixels, 2, 2), Pixel(g.Pixels, 3, 2); left[3] != 255 || right[3] != 0 {
			return fmt.Errorf("fractional alpha columns: %v %v", left, right)
		}
		bloom, _ := reg.NewInstance(postfx.BloomID, "halo")
		var plan postfx.Plan
		if err := reg.Build(&plan, true, []postfx.Instance{bloom}, postfx.Profile{}); err != nil {
			return err
		}
		if err := r.BeginFrame(g.Device.BeginFrame(Size, Size), Size, Size, 1); err != nil {
			return err
		}
		v = g.View()
		submit := func(rect mathx.Rect, alpha float32, last bool) {
			v.Viewport = rect
			v.Clear.A = alpha
			v.DeferResolve = !last
			f := r.View(v)
			f.PostProcess(&plan)
			f.End()
		}
		submit(v.Viewport, 1, false)
		submit(mathx.Rect{X: .75, Y: .25, W: 2.5, H: 5.5}, 0, false)
		submit(mathx.Rect{W: 3, H: 6}, 1, true)
		stats := r.EndFrame()
		g.Device.EndFrame()
		if err := r.PostProcessError(); err == nil || !strings.Contains(err.Error(), "opaque") || stats.PostFXPasses != 0 {
			return fmt.Errorf("accepted uncovered column 3: passes=%d error=%v", stats.PostFXPasses, err)
		}
		return nil
	})
	report.Case("postfx-shared-output-isolation", func() error {
		opaque := postfx.Pass{Descriptor: &postfx.Descriptor{ID: "test.opaque", Phase: postfx.DisplayLinear, Fragment: `
vec4 aego_effect(vec4 c,vec2 uv,vec2 size,vec4 params[4]) { return vec4(0.5, c.g, 0.0, 1.0); }
`}}
		plan := postfx.Plan{Passes: []postfx.Pass{opaque}}
		r := g.Renderer
		fr := g.Device.BeginFrame(Size, Size)
		if err := r.BeginFrame(fr, Size, Size, 1); err != nil {
			return err
		}
		vw := g.View()
		vw.Clear = mathx.Color{B: 1, A: 1}
		r.View(vw).End()
		vw.Viewport = mathx.Rect{X: 1, Y: 2, W: 11, H: 11}
		vw.Clear = mathx.Color{A: 1}
		vw.DeferResolve = true
		f := r.View(vw)
		f.PostProcess(&plan)
		f.End()
		vw.Viewport = mathx.Rect{X: 22, Y: 5, W: 1, H: 1}
		vw.Clear = mathx.Color{G: 1, A: 1}
		vw.DeferResolve = false
		f = r.View(vw)
		f.PostProcess(&plan)
		f.End()
		vw.Viewport = mathx.Rect{X: 4, Y: 6, W: 1, H: 1}
		vw.Clear = mathx.White
		r.View(vw).End()
		stats := r.EndFrame()
		g.Device.EndFrame()
		if err := r.PostProcessError(); err != nil {
			return err
		}
		if stats.ResolvePasses != 1 || stats.PostFXPasses != 1 || stats.PostFXTargets != 0 {
			return fmt.Errorf("unexpected resolve stats %+v", stats)
		}
		if err := g.Device.ReadPixels(g.Target, 0, 0, Size, Size, g.Pixels); err != nil {
			return err
		}
		for y := 0; y < Size; y++ {
			for x := 0; x < Size; x++ {
				want := [4]int{0, 0, 255, 255}
				switch {
				case x == 4 && y == 6:
					want = [4]int{255, 255, 255, 255}
				case x >= 1 && x < 12 && y >= 2 && y < 13:
					want = [4]int{188, 0, 0, 255}
				case x == 22 && y == 5:
					want = [4]int{188, 255, 0, 255}
				}
				if got := Pixel(g.Pixels, x, y); !NearPixel(got, want, 1) {
					return fmt.Errorf("(%d,%d) got %v want %v +/-1", x, y, got, want)
				}
			}
		}
		return nil
	})
	report.Case("postfx-one-texel-clamped-sampling", func() error {
		back := g.View()
		back.Clear = mathx.Color{G: 1, A: 1}
		if err := g.Frame(back, nil, func(*render.ViewFrame) {}); err != nil {
			return err
		}
		out := back
		out.Viewport = mathx.Rect{X: 4, Y: 7, W: 1, H: 1}
		out.Clear = mathx.Color{R: 1, A: 1}
		edge := &postfx.Descriptor{ID: "test.edge", Phase: postfx.DisplayLinear, Fragment: `
vec4 aego_effect(vec4 c,vec2 uv,vec2 size,vec4 params[4]) { return aego_sample(uv+vec2(-100.0,100.0)); }
`}
		plan := postfx.Plan{Passes: []postfx.Pass{{Descriptor: edge}, {Descriptor: edge}}}
		got, err := g.DrawView(out, nil, func(f *render.ViewFrame) { f.PostProcess(&plan) })
		if err != nil {
			return err
		}
		if err = g.Renderer.PostProcessError(); err != nil {
			return err
		}
		for y := 0; y < Size; y++ {
			for x := 0; x < Size; x++ {
				want := [4]byte{0, 255, 0, 255}
				if x == 4 && y == 7 {
					want = [4]byte{255, 0, 0, 255}
				}
				if c := Pixel(got, x, y); c != want {
					return fmt.Errorf("pixel (%d,%d) %v expected %v", x, y, c, want)
				}
			}
		}
		return nil
	})
	report.Case("postfx-empty-pixel-bypass", func() error {
		out := g.View()
		out.Clear = mathx.Color{R: .27, G: .08, B: .13, A: 1}
		baseline, err := g.DrawView(out, nil, func(*render.ViewFrame) {})
		if err != nil {
			return err
		}
		var ref [Size * Size * 4]byte
		copy(ref[:], baseline)
		old := g.Renderer.Stats()
		empty := postfx.Plan{}
		got, err := g.DrawView(out, nil, func(f *render.ViewFrame) { f.PostProcess(&empty) })
		if err != nil {
			return err
		}
		if s := g.Renderer.Stats(); s.DrawCalls != old.DrawCalls || s.ResolvePasses != old.ResolvePasses || s.PostFXTargets != 0 || s.PostFXPasses != 0 {
			return fmt.Errorf("bypass added work: %+v", s)
		}
		return Compare(got, ref[:], 0)
	})
	report.Case("postfx-invalid-plan-atomic-bypass", func() error {
		out := g.View()
		out.Clear = mathx.Color{R: .7, G: .25, A: 1}
		env := lighting.DefaultEnvironment()
		env.ColorMode = lighting.LDR
		in := lighting.Input{Environment: env, HasEnvironment: true, OutputLit: true}
		baseline, err := g.DrawView(out, &in, func(*render.ViewFrame) {})
		if err != nil {
			return err
		}
		var ref [Size * Size * 4]byte
		copy(ref[:], baseline)
		invalid := postfx.Plan{Passes: []postfx.Pass{v, {Descriptor: &postfx.Descriptor{ID: "test.bad", Phase: postfx.DisplayLinear, Fragment: "missing_function"}}}}
		got, err := g.DrawView(out, &in, func(f *render.ViewFrame) { f.PostProcess(&invalid) })
		if err != nil {
			return err
		}
		if s := g.Renderer.Stats(); g.Renderer.PostProcessError() == nil || s.PostFXPasses != 0 || s.PostFXTargets != 0 || s.ResolvePasses != 1 {
			return fmt.Errorf("partial plan or missing diagnostic: %+v", s)
		}
		return Compare(got, ref[:], 0)
	})
	if g.Device.Caps().RGBA16FRenderable {
		report.Case("postfx-linear-energy-without-coverage", func() error {
			target, err := g.Texture(gpu.TextureDesc{Width: Size, Height: Size, Format: gpu.TexRGBA16F})
			if err != nil {
				return err
			}
			out := g.View()
			out.Target, out.OutputPolicy = target, view.OutputLinear
			out.Clear = mathx.Color{R: 4, G: 2}
			env := lighting.DefaultEnvironment()
			env.ColorMode, env.Exposure = lighting.HDR, 3
			in := lighting.Input{Environment: env, HasEnvironment: true}
			sceneOnly := postfx.Plan{Passes: []postfx.Pass{gain("test.linear", postfx.SceneLinear, .5)}}
			for _, tc := range []struct {
				plan   *postfx.Plan
				r, g   float32
				passes uint32
			}{{&postfx.Plan{Passes: []postfx.Pass{v}}, 4, 2, 0}, {&sceneOnly, 2, 1, 1}} {
				if err = g.Frame(out, &in, func(f *render.ViewFrame) { f.PostProcess(tc.plan) }); err != nil {
					return err
				}
				if err = g.Renderer.PostProcessError(); err != nil {
					return err
				}
				if s := g.Renderer.Stats(); s.PostFXPasses != tc.passes {
					return fmt.Errorf("wanted %d passes: %+v", tc.passes, s)
				}
				var pixel [4]float32
				if err = g.Device.(gpu.TextureReader).ReadTexturePixelsFloat(target, gpu.TextureSubresource{}, 1, 1, 1, 1, pixel[:]); err != nil {
					return err
				}
				if pixel != ([4]float32{tc.r, tc.g, 0, 0}) {
					return fmt.Errorf("linear pixel %v expected R=%g G=%g A=0", pixel, tc.r, tc.g)
				}
			}
			return nil
		})
	}

}

func postFXEncode(n float64) float64 {
	if n <= .0031308 {
		return 12.92 * n
	}
	return 1.055*math.Pow(n, 1/2.4) - .055
}
