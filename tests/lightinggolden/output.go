package lightinggolden

import (
	"fmt"
	"math"
	"slices"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render"
	"aego/render/lighting"
	"aego/render/texture"
	"aego/render/view"
	"aego/vfs"
)

func RunOutputs(g *Rig, report *Report) {
	report.Case("LDR-target-and-display-encoding", func() error {
		in := Input(lighting.LDR)
		in.Environment.AmbientIntensity = 2
		d := g.Sprite(g.Lit)
		got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		o, ok := g.Renderer.LightingOutput(g.View().OutputID)
		report.Outputs["LDR-target-and-display-encoding"] = o
		if !ok || o.Format != gpu.TexRGBA8 || o.Active != lighting.LDR {
			return fmt.Errorf("unexpected output: %+v", o)
		}
		return report.Image("LDR-target-and-display-encoding", got, Reference(in, [4]float64{}, WhiteSurface()), 2)
	})
	if !g.Device.Caps().RGBA16FRenderable {
		report.Unverified = append(report.Unverified, "real RGBA16F/HDR tests: backend capability is unavailable")
		return
	}
	for _, alpha := range []float32{1, 128.0 / 255} {
		name := "HDR-Reinhard-opaque"
		if alpha < 1 {
			name = "HDR-Reinhard-transparent-PMA"
		}
		report.Case(name, func() error {
			in := Input(lighting.HDR)
			in.Environment.AmbientIntensity = 2.5
			in.Environment.Exposure = 1.25
			d := g.Sprite(g.Lit)
			d.Tint = mathx.Color{R: 1, G: .2, B: .05, A: alpha}
			s := WhiteSurface()
			s.Color = PMA(d.Tint)
			got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
			if err != nil {
				return err
			}
			o, ok := g.Renderer.LightingOutput(g.View().OutputID)
			report.Outputs[name] = o
			if !ok || o.Format != gpu.TexRGBA16F || o.Active != lighting.HDR || o.Bytes != Size*Size*8 {
				return fmt.Errorf("not real RGBA16F: %+v", o)
			}
			report.Images[name+"-checkerboard"] = Checkerboard(got)
			return report.Image(name, got, Reference(in, [4]float64{}, s), 2)
		})
	}
	report.Case("linear-HDR-readback-keeps-energy-and-zero-coverage", func() error {
		h, err := g.Texture(gpu.TextureDesc{Width: Size, Height: Size, Format: gpu.TexRGBA16F, Label: "lighting-test.linear"})
		if err != nil {
			return err
		}
		v := g.View()
		v.Target = h
		v.OutputPolicy = view.OutputLinear
		in := Input(lighting.HDR)
		in.Environment.AmbientIntensity = 3
		d := g.Sprite(g.Lit)
		a := g.Sprite(g.Additive)
		a.Tint = mathx.Color{R: .5, G: .25, B: .125, A: .5}
		// An offscreen Lit draw enables the requested lighting output without
		// contributing coverage to the additive sample in the middle.
		d.Position = mathx.Vec2{X: 2, Y: 2}
		d.Size = mathx.Vec2{X: 2, Y: 2}
		if err := g.Frame(v, &in, func(v *render.ViewFrame) { v.Sprite(d); v.Sprite(a) }); err != nil {
			return err
		}
		reader, ok := g.Device.(gpu.TextureReader)
		if !ok {
			return fmt.Errorf("backend lacks float subresource readback")
		}
		pixels := make([]float32, Size*Size*4)
		if err := reader.ReadTexturePixelsFloat(h, gpu.TextureSubresource{}, 0, 0, Size, Size, pixels); err != nil {
			return err
		}
		i := (16*Size + 16) * 4
		expected := PMA(a.Tint)
		for c := 0; c < 3; c++ {
			if math.Abs(float64(pixels[i+c])-expected[c]) > .002 {
				return fmt.Errorf("additive RGB lost at alpha zero: %v", pixels[i:i+4])
			}
		}
		if pixels[i+3] != 0 {
			return fmt.Errorf("additive changed coverage: %v", pixels[i:i+4])
		}
		i = (2*Size + 2) * 4
		if pixels[i] < 2.9 || pixels[i+3] < .99 {
			return fmt.Errorf("HDR energy was clamped: %v", pixels[i:i+4])
		}
		if g.Renderer.Stats().ResolvePasses != 0 {
			return fmt.Errorf("linear output was resolved")
		}
		return nil
	})
	report.Case("additive-opaque-background-resolves-once", func() error {
		in := Input(lighting.HDR)
		in.Environment.AmbientIntensity = .1
		v := g.View()
		v.Clear = mathx.Color{R: .1, G: .1, B: .1, A: 1}
		d := g.Sprite(g.Lit)
		d.Tint = mathx.Color{R: 1, G: 1, B: 1, A: 128.0 / 255}
		a := g.Sprite(g.Additive)
		a.Tint = mathx.Color{R: 1, G: .25, B: 0, A: 128.0 / 255}
		s, as := WhiteSurface(), WhiteSurface()
		s.Color = PMA(d.Tint)
		as.Color = PMA(a.Tint)
		as.Lit = false
		as.Additive = true
		got, err := g.DrawView(v, &in, func(v *render.ViewFrame) { v.Sprite(d); v.Sprite(a) })
		if err != nil {
			return err
		}
		if g.Renderer.Stats().ResolvePasses != 1 {
			return fmt.Errorf("resolve count %d", g.Renderer.Stats().ResolvePasses)
		}
		return report.Image("additive-opaque-background-resolves-once", got, Reference(in, [4]float64{.1, .1, .1, 1}, s, as), 2)
	})
	runCameraOutput(g, report)
	runDisplayComposition(g, report)
	runIntermediateComposition(g, report)
	runSRGBTransparentOutput(g, report)
}

func runIntermediateComposition(g *Rig, report *Report) {
	for _, background := range []struct {
		name  string
		color [4]float64
	}{
		{"cool", [4]float64{.05, .1, .2, 1}}, {"warm", [4]float64{.4, .25, .08, 1}},
	} {
		name := "linear-additive-intermediate-before-tone-" + background.name
		report.Case(name, func() error {
			h, err := g.Texture(gpu.TextureDesc{Width: Size, Height: Size, Format: gpu.TexRGBA16F, Label: "lighting-test.emitted-energy"})
			if err != nil {
				return err
			}
			in := Input(lighting.HDR)
			v := g.View()
			v.Target = h
			v.OutputPolicy = view.OutputLinear
			emitted := g.Sprite(g.Additive)
			emitted.Tint = mathx.Color{R: .75, G: .35, B: .1, A: 128.0 / 255}
			if err = g.Frame(v, &in, func(v *render.ViewFrame) {
				for range 3 {
					v.Sprite(emitted)
				}
			}); err != nil {
				return err
			}
			var center [4]float32
			if err = g.Device.(gpu.TextureReader).ReadTexturePixelsFloat(h, gpu.TextureSubresource{}, 16, 16, 1, 1, center[:]); err != nil {
				return err
			}
			if center[0] <= 1 || center[3] != 0 {
				return fmt.Errorf("intermediate did not preserve HDR energy with zero alpha: %v", center)
			}
			composite := g.Sprite(g.Unlit)
			composite.Region = texture.Whole(h, Size, Size)
			composite.Size = mathx.Vec2{X: Size, Y: Size}
			v = g.View()
			v.Clear = mathx.Color{R: float32(background.color[0]), G: float32(background.color[1]), B: float32(background.color[2]), A: 1}
			got, err := g.DrawView(v, &in, func(v *render.ViewFrame) { v.Sprite(composite) })
			if err != nil {
				return err
			}
			s := WhiteSurface()
			s.Lit = false
			s.Additive = true
			s.Color = PMA(emitted.Tint)
			if g.Renderer.Stats().ResolvePasses != 1 {
				return fmt.Errorf("final composition resolved %d times", g.Renderer.Stats().ResolvePasses)
			}
			// Background and energy are added in linear HDR, then one resolve.
			// Tone(background)+Tone(energy) gives a visibly different reference.
			return report.Image(name, got, Reference(in, background.color, s, s, s), 2)
		})
	}
}

func decodeSRGB(x float64) float64 {
	if x <= .04045 {
		return x / 12.92
	}
	return math.Pow((x+.055)/1.055, 2.4)
}

func runSRGBTransparentOutput(g *Rig, report *Report) {
	report.Case("transparent-SRGBA8-output-samples-once-into-linear-UI", func() error {
		h, err := g.Texture(gpu.TextureDesc{Width: Size, Height: Size, Format: gpu.TexSRGBA8, Label: "lighting-test.srgb-transparent"})
		if err != nil {
			return err
		}
		in := Input(lighting.HDR)
		in.Environment.AmbientIntensity = 2
		d := g.Sprite(g.Lit)
		d.Tint = mathx.Color{R: 1, G: .25, B: .05, A: 128.0 / 255}
		v := g.View()
		v.Target = h
		if err = g.Frame(v, &in, func(v *render.ViewFrame) { v.Sprite(d) }); err != nil {
			return err
		}
		ui := g.Sprite(g.Unlit)
		ui.Region = texture.Whole(h, Size, Size)
		ui.Size = mathx.Vec2{X: Size, Y: Size}
		v = g.View()
		v.Clear = mathx.Color{R: .15, G: .25, B: .4, A: 1}
		got, err := g.DrawView(v, nil, func(v *render.ViewFrame) { v.Sprite(ui) })
		if err != nil {
			return err
		}
		color := PMA(d.Tint)
		a := color[3]
		background := [3]float64{quant(.15), quant(.25), quant(.4)}
		want := make([]byte, Size*Size*4)
		for y := 0; y < Size; y++ {
			for x := 0; x < Size; x++ {
				i := (y*Size + x) * 4
				inside := x >= 4 && x < 28 && y >= 4 && y < 28
				for c := 0; c < 3; c++ {
					linear := background[c]
					if inside {
						straight := color[c] / a * 2
						straight /= 1 + straight
						// Automatic attachment encoding stores encode(PMA linear).
						// Sampling decodes it; UI blends this linear PMA over its own
						// background, without the game's tone mapping a second time.
						stored := quant(srgb(straight * a))
						linear = decodeSRGB(stored) + background[c]*(1-a)
					}
					want[i+c] = byte(quant(linear)*255 + .5)
				}
				want[i+3] = 255
			}
		}
		if g.Renderer.Stats().ResolvePasses != 0 {
			return fmt.Errorf("UI was sent through game tone mapping")
		}
		return report.Image("transparent-SRGBA8-output-samples-once-into-linear-UI", got, want, 2)
	})
}

func runDisplayComposition(g *Rig, report *Report) {
	report.Case("display-resolve-keeps-existing-backdrop", func() error {
		background := make([]byte, Size*Size*4)
		for i := 0; i < len(background); i += 4 {
			background[i], background[i+1], background[i+2], background[i+3] = 40, 80, 120, 255
		}
		in := Input(lighting.HDR)
		in.Environment.AmbientIntensity = 1.5
		d := g.Sprite(g.Lit)
		d.Tint = mathx.Color{R: .8, G: .1, B: .3, A: 128.0 / 255}
		v := g.View()
		v.DoClear = false
		s := WhiteSurface()
		s.Color = PMA(d.Tint)
		foreground := Reference(in, [4]float64{}, s)
		want := slices.Clone(background)
		for i := 0; i < len(want); i += 4 {
			a := float64(foreground[i+3]) / 255
			for c := 0; c < 3; c++ {
				want[i+c] = byte(clamp01(float64(foreground[i+c])/255+float64(background[i+c])/255*(1-a))*255 + .5)
			}
		}
		var first []byte
		for frame := 0; frame < 3; frame++ {
			g.Device.UpdateTexture(g.Target, 0, 0, Size, Size, background)
			got, err := g.DrawView(v, &in, func(v *render.ViewFrame) { v.Sprite(d) })
			if err != nil {
				return err
			}
			if err = Compare(got, want, 2); err != nil {
				return err
			}
			if frame == 0 {
				first = slices.Clone(got)
			} else if err = Compare(got, first, 0); err != nil {
				return fmt.Errorf("previous linear frame accumulated: %w", err)
			}
		}
		return report.Image("display-resolve-keeps-existing-backdrop", first, want, 2)
	})
	report.Case("opaque-first-camera-allows-additive-second-camera", func() error {
		in := Input(lighting.HDR)
		in.Environment.AmbientIntensity = .5
		in.OutputLit = true
		first, second := g.View(), g.View()
		first.DeferResolve = true
		first.Clear = mathx.Color{R: .1, G: .1, B: .1, A: 1}
		second.DoClear = false
		second.ID.Node.Lo = 2
		a, b := g.Sprite(g.Lit), g.Sprite(g.Additive)
		a.Tint = mathx.Color{G: 1, A: 128.0 / 255}
		b.Tint = mathx.Color{R: 1, A: 128.0 / 255}
		f := g.Device.BeginFrame(Size, Size)
		if err := g.Renderer.BeginFrame(f, Size, Size, 1); err != nil {
			return err
		}
		v := g.Renderer.View(first)
		v.Lighting(&in)
		v.Sprite(a)
		v.End()
		v = g.Renderer.View(second)
		v.Lighting(&in)
		v.Sprite(b)
		v.End()
		g.Renderer.EndFrame()
		g.Device.EndFrame()
		if g.Renderer.Stats().LightingFailures > 0 {
			return g.Renderer.LightingError()
		}
		if err := g.Device.ReadPixels(g.Target, 0, 0, Size, Size, g.Pixels); err != nil {
			return err
		}
		sa, sb := WhiteSurface(), WhiteSurface()
		sa.Color = PMA(a.Tint)
		sb.Color = PMA(b.Tint)
		sb.Lit = false
		sb.Additive = true
		return report.Image("opaque-first-camera-allows-additive-second-camera", g.Pixels, Reference(in, [4]float64{.1, .1, .1, 1}, sa, sb), 2)
	})
	report.Case("transparent-display-additive-requires-explicit-policy", func() error {
		in := Input(lighting.HDR)
		a, b := g.Sprite(g.Lit), g.Sprite(g.Additive)
		g.AllowErrors = true
		defer func() { g.AllowErrors = false }()
		if err := g.Frame(g.View(), &in, func(v *render.ViewFrame) { v.Sprite(a); v.Sprite(b) }); err != nil {
			return err
		}
		if g.Renderer.Stats().LightingFailures == 0 || g.Renderer.LightingError() == nil {
			return fmt.Errorf("transparent additive output was silently treated as valid PMA display")
		}
		return nil
	})
}

func runCameraOutput(g *Rig, report *Report) {
	report.Case("two-cameras-share-linear-output-single-resolve", func() error {
		in := Input(lighting.HDR)
		in.Environment.AmbientIntensity = 1.8
		in.OutputLit = true
		first, second := g.View(), g.View()
		first.DeferResolve = true
		second.DoClear = false
		second.ID.Node = ids.NodeID{Lo: 2}
		a, b := g.Sprite(g.Lit), g.Sprite(g.Lit)
		a.Tint = mathx.Color{R: 1, A: 128.0 / 255}
		b.Tint = mathx.Color{B: 1, A: 128.0 / 255}
		f := g.Device.BeginFrame(Size, Size)
		if err := g.Renderer.BeginFrame(f, Size, Size, 1); err != nil {
			return err
		}
		v := g.Renderer.View(first)
		v.Lighting(&in)
		v.Sprite(a)
		v.End()
		v = g.Renderer.View(second)
		v.Lighting(&in)
		v.Sprite(b)
		v.End()
		g.Renderer.EndFrame()
		g.Device.EndFrame()
		if err := g.Device.ReadPixels(g.Target, 0, 0, Size, Size, g.Pixels); err != nil {
			return err
		}
		s, bs := WhiteSurface(), WhiteSurface()
		s.Color = PMA(a.Tint)
		bs.Color = PMA(b.Tint)
		stats := g.Renderer.Stats()
		report.Stats["two-cameras-share-linear-output-single-resolve"] = stats
		if stats.ResolvePasses != 1 {
			return fmt.Errorf("resolved %d times", stats.ResolvePasses)
		}
		if err := report.Image("two-cameras-share-linear-output-single-resolve", g.Pixels, Reference(in, [4]float64{}, s, bs), 2); err != nil {
			return err
		}
		return nil
	})
	report.Case("two-camera-outputs-isolate-ambient-and-targets", func() error {
		in := Input(lighting.HDR)
		in.Environment.Ambient = mathx.Color{R: 1, A: 1}
		in.Environment.AmbientIntensity = 1
		d := g.Sprite(g.Lit)
		a, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		a = slices.Clone(a)
		h, err := g.Texture(gpu.TextureDesc{Width: Size, Height: Size, Format: gpu.TexRGBA8})
		if err != nil {
			return err
		}
		v := g.View()
		v.Target = h
		v.OutputID = view.ID{Scope: 2}
		v.ID = view.ID{Scope: 2, Node: ids.NodeID{Lo: 2}}
		in.Environment.Ambient = mathx.Color{B: 1, A: 1}
		b, err := g.DrawView(v, &in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		if err := report.Image("camera-blue-output", b, Reference(in, [4]float64{}, WhiteSurface()), 2); err != nil {
			return err
		}
		if err := g.Device.ReadPixels(g.Target, 0, 0, Size, Size, g.Pixels); err != nil {
			return err
		}
		return report.Image("camera-red-output-retained", g.Pixels, a, 0)
	})
}

// CapabilityDevice deliberately alters one reported capability while forwarding
// the real driver's optional interfaces. This is a forced fallback test, never
// evidence that the physical device lacks an extension.
type CapabilityDevice struct {
	gpu.Device
	NoFloat, RejectFloatFramebuffer bool
}

func (d *CapabilityDevice) Caps() gpu.Capabilities {
	c := d.Device.Caps()
	if d.NoFloat {
		c.RGBA16FRenderable = false
	}
	return c
}
func (d *CapabilityDevice) TextureInfo(h gpu.TextureHandle) (gpu.TextureDesc, bool) {
	return d.Device.(gpu.TextureDescriber).TextureInfo(h)
}
func (d *CapabilityDevice) ValidateRenderTarget(p gpu.PassDesc) error {
	if d.RejectFloatFramebuffer {
		if desc, ok := d.TextureInfo(p.Target); ok && desc.Format == gpu.TexRGBA16F {
			return fmt.Errorf("injected RGBA16F framebuffer rejection")
		}
	}
	return d.Device.(gpu.RenderTargetValidator).ValidateRenderTarget(p)
}
func (d *CapabilityDevice) ContextGeneration() uint64 {
	if c, ok := d.Device.(gpu.ContextState); ok {
		return c.ContextGeneration()
	}
	return 1
}
func (d *CapabilityDevice) ContextLost() bool {
	if c, ok := d.Device.(gpu.ContextState); ok {
		return c.ContextLost()
	}
	return false
}
func (d *CapabilityDevice) ResourceStats() gpu.ResourceStats {
	return d.Device.(gpu.ResourceReporter).ResourceStats()
}
func (d *CapabilityDevice) ReadTexturePixels(t gpu.TextureHandle, s gpu.TextureSubresource, x, y, w, h int, b []byte) error {
	return d.Device.(gpu.TextureReader).ReadTexturePixels(t, s, x, y, w, h, b)
}
func (d *CapabilityDevice) ReadTexturePixelsFloat(t gpu.TextureHandle, s gpu.TextureSubresource, x, y, w, h int, b []float32) error {
	return d.Device.(gpu.TextureReader).ReadTexturePixelsFloat(t, s, x, y, w, h, b)
}

func RunFallbacks(dev gpu.Device, files vfs.FS, report *Report) {
	for _, entry := range []struct {
		name            string
		noFloat, reject bool
	}{
		{"forced-no-float-capability", true, false}, {"injected-float-framebuffer-failure", false, true},
	} {
		report.Case(entry.name, func() error {
			d := &CapabilityDevice{Device: dev, NoFloat: entry.noFloat, RejectFloatFramebuffer: entry.reject}
			g, err := New(d, d.Caps(), files)
			if err != nil {
				return err
			}
			defer g.Close()
			in := Input(lighting.Auto)
			in.Environment.AmbientIntensity = .4
			s := g.Sprite(g.Lit)
			got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(s) })
			if err != nil {
				return err
			}
			o, ok := g.Renderer.LightingOutput(g.View().OutputID)
			report.Outputs[entry.name] = o
			if !ok || o.Active != lighting.LDR || o.Format != gpu.TexRGBA8 || o.Reason == "" {
				return fmt.Errorf("missing explicit fallback reason: %+v", o)
			}
			in.Environment.ColorMode = lighting.LDR
			return report.Image(entry.name, got, Reference(in, [4]float64{}, WhiteSurface()), 2)
		})
	}
}
