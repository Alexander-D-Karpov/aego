package lightinggolden

import (
	"bytes"
	"fmt"
	"slices"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render"
	"aego/render/lighting"
	"aego/render/material"
	"aego/render/postfx"
	"aego/render/shader"
	"aego/render/shaderx"
)

// RunExtendedPostProcess exercises the engine's actual fullscreen passes, not
// a standalone copy of their GLSL. The same cases run on GL33 and WebGL2.
func RunExtendedPostProcess(g *Rig, report *Report) {
	reg := postfx.NewRegistry()
	desc := func(id string) *postfx.Descriptor {
		d, _ := reg.Lookup(id)
		return d
	}
	view := g.View()
	view.Clear = mathx.Color{R: .1, G: .3, B: .6, A: 1}
	draw := g.Sprite(nil)
	draw.Position, draw.Size = mathx.V2(16, 16), mathx.V2(11, 13)
	draw.Tint = mathx.Color{R: .8, G: .4, B: .2, A: 1}
	frame := func(plan *postfx.Plan) ([]byte, error) {
		pixels, err := g.DrawView(view, nil, func(f *render.ViewFrame) {
			if plan != nil {
				f.PostProcess(plan)
			}
			f.Sprite(draw)
		})
		if err == nil {
			err = g.Renderer.PostProcessError()
		}
		return slices.Clone(pixels), err
	}
	// An active identity pass traverses the same linear/display resolve as the
	// effects. A bare view bypasses that resolve and is not a pixel reference.
	identity := postfx.Plan{Passes: []postfx.Pass{{Descriptor: &postfx.Descriptor{
		ID: "test.extended-identity", Phase: postfx.DisplayLinear,
		Fragment: `vec4 aego_effect(vec4 c,vec2 uv,vec2 size,vec4 p[4]) { return c; }`,
	}}}}
	baseline, err := frame(&identity)
	if err != nil {
		report.Failures = append(report.Failures, "extended-postfx-baseline: "+err.Error())
		return
	}
	for _, tc := range []struct {
		id     string
		params [4][4]float32
	}{
		{postfx.PixelateID, [4][4]float32{{4}}},
		{postfx.ChromaticID, [4][4]float32{{4}}},
		{postfx.CRTID, [4][4]float32{{.3}, {.4}}},
	} {
		report.Case("engine-"+tc.id, func() error {
			plan := postfx.Plan{Passes: []postfx.Pass{{Descriptor: desc(tc.id), Params: tc.params}}}
			pixels, err := frame(&plan)
			if err != nil {
				return err
			}
			if bytes.Equal(pixels, baseline) || g.Renderer.Stats().PostFXPasses != 1 {
				return fmt.Errorf("effect did not change output or execute exactly one pass")
			}
			return nil
		})
	}
	report.Case("engine-identity-lut", func() error {
		lut := make([]byte, 256*16*4)
		for b := 0; b < 16; b++ {
			for y := 0; y < 16; y++ {
				for x := 0; x < 16; x++ {
					j := 4 * (y*256 + b*16 + x)
					lut[j], lut[j+1], lut[j+2], lut[j+3] = byte(x*17), byte(y*17), byte(b*17), 255
				}
			}
		}
		tex, err := g.Texture(gpu.TextureDesc{Width: 256, Height: 16, Format: gpu.TexRGBA8, Data: lut, Label: "postfx-test.identity-lut"})
		if err != nil {
			return err
		}
		plan := postfx.Plan{Passes: []postfx.Pass{{Descriptor: desc(postfx.ColorGradeID), LUTID: ids.AssetID{Lo: 1}, LUT: tex, LUTSize: 16, Params: [4][4]float32{{1, 1, 1}, {}, {1, 1, 1}, {1, 1, 1}}}}}
		pixels, err := frame(&plan)
		if err != nil {
			return err
		}
		if g.Renderer.Stats().PostFXPasses != 1 {
			return fmt.Errorf("identity LUT pass was not executed")
		}
		return Compare(pixels, baseline, 2)
	})
	report.Case("engine-custom-material", func() error {
		src := `#pragma aego abi 1
#pragma postfx
void fragment() { vec4 c = aego_sample(aego_uv); aego_out = vec4(c.bgr, c.a); }
`
		u, diagnostics := shaderx.Parse([]byte(src), "postfx-test.aeshader", nil)
		if len(diagnostics) != 0 {
			return fmt.Errorf("shader parser: %v", diagnostics)
		}
		sh := asset.BuildShader(u).Runtime(ids.AssetID{}, "postfx-test")
		m, ok := material.New("postfx-test", sh, 0, shader.BlendOpaque)
		if !ok {
			return fmt.Errorf("cannot construct postfx material")
		}
		plan := postfx.Plan{Passes: []postfx.Pass{{Descriptor: desc(postfx.CustomID), Material: m}}}
		pixels, err := frame(&plan)
		if err != nil {
			return err
		}
		want := Pixel(baseline, 0, 0)
		want[0], want[2] = want[2], want[0]
		got := Pixel(pixels, 0, 0)
		if !NearPixel(got, [4]int{int(want[0]), int(want[1]), int(want[2]), int(want[3])}, 2) || g.Renderer.Stats().PostFXPasses != 1 {
			return fmt.Errorf("custom output %v, expected red/blue swap %v", got, want)
		}
		return nil
	})
	for _, tc := range []struct {
		name, flag string
		red        int
	}{{"display", "display_linear", 188}, {"scene", "scene_linear", 225}} {
		report.Case("engine-custom-phase-"+tc.name, func() error {
			u, ds := shaderx.Parse([]byte("#pragma aego abi 1\n#pragma postfx "+tc.flag+"\nvoid fragment(){ vec4 c=aego_sample(aego_uv); aego_out=vec4(c.rgb*0.5,c.a); }\n"), "gain.aeshader", nil)
			if len(ds) != 0 {
				return fmt.Errorf("invalid phase: %v", ds)
			}
			m, ok := material.New("gain", asset.BuildShader(u).Runtime(ids.AssetID{}, "gain"), 0, shader.BlendOpaque)
			if !ok {
				return fmt.Errorf("cannot build gain material")
			}
			out := g.View()
			out.Clear = mathx.Color{R: .75, A: 1}
			env := lighting.DefaultEnvironment()
			env.Exposure, env.ColorMode = 2, lighting.LDR
			in := lighting.Input{Environment: env, HasEnvironment: true, OutputLit: true}
			p := postfx.Plan{Passes: []postfx.Pass{{Descriptor: desc(postfx.CustomID), Material: m}}}
			pixels, err := g.DrawView(out, &in, func(f *render.ViewFrame) { f.PostProcess(&p) })
			if err != nil {
				return err
			}
			if err = g.Renderer.PostProcessError(); err != nil {
				return err
			}
			got := Pixel(pixels, 8, 8)
			if !NearPixel(got, [4]int{tc.red, 0, 0, 255}, 2) {
				return fmt.Errorf("%s: got %v, expected red %d", tc.name, got, tc.red)
			}
			return nil
		})
	}
}
