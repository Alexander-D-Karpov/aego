package lightinggolden

import (
	"fmt"
	"slices"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/render"
	"aego/render/lighting"
	"aego/render/mesh"
	"aego/render/sprite"
)

func (g *Rig) Mesh(normal bool) mesh.Draw {
	d := mesh.Draw{VisibilityLayers: ^uint64(0), Mesh: g.Quad, World: mathx.Mat4Translate(mathx.Vec3{X: 16, Y: 16}), Texture: g.Renderer.White(), Color: mathx.White, Blend: sprite.BlendPremultiplied, DoubleSided: true, Is2D: true, Material: g.LitMesh}
	if normal {
		d.Normal = g.Normal
	}
	return d
}

func (g *Rig) imageCase(report *Report, name string, in lighting.Input, surface Surface, draw func(*render.ViewFrame)) {
	report.Case(name, func() error {
		got, err := g.Draw(&in, draw)
		if err != nil {
			return err
		}
		report.Stats[name] = g.Renderer.Stats()
		if output, ok := g.Renderer.LightingOutput(g.View().OutputID); ok {
			report.Outputs[name] = output
		}
		return report.Image(name, got, Reference(in, [4]float64{}, surface), 2)
	})
}

// Run exercises the same GPU commands and independently computed image on both
// real drivers. Context loss is added by the browser harness, not simulated here.
func Run(g *Rig, report *Report) {
	for _, entry := range []struct {
		name string
		kind lighting.Type
	}{
		{"point", lighting.Point}, {"spot", lighting.Spot}, {"directional", lighting.Directional}, {"sprite-cookie-letterbox", lighting.Sprite},
	} {
		l := Light(entry.kind)
		if l.Type == lighting.Spot {
			l.Position = mathx.Vec2{X: 7, Y: 15}
			l.OuterAngle = 2.4
			l.InnerAngle = .8
		}
		if l.Type == lighting.Sprite {
			l.Cookie = g.Cookie
			l.Range = 18
		}
		in := Input(lighting.LDR, l)
		d := g.Sprite(g.Lit)
		g.imageCase(report, "light-"+entry.name, in, WhiteSurface(), func(v *render.ViewFrame) { v.Sprite(d) })
	}
	for _, entry := range []struct {
		name                          string
		height, intensity, lightRange float32
	}{
		{"height-zero-flat-normal", 0, .75, 30}, {"height-positive-flat-normal", 8, .75, 30},
		{"zero-intensity", 8, 0, 30}, {"finite-range-boundary", 8, .75, 10},
	} {
		l := Light(lighting.Point)
		l.Height, l.Intensity, l.Range = entry.height, entry.intensity, entry.lightRange
		in := Input(lighting.LDR, l)
		d := g.Sprite(g.Lit)
		g.imageCase(report, entry.name, in, WhiteSurface(), func(v *render.ViewFrame) { v.Sprite(d) })
	}
	for _, entry := range []struct {
		name         string
		basis        [4]float32
		flipX, flipY bool
	}{
		{"identity", [4]float32{1, 0, 0, 1}, false, false},
		{"flip-x", [4]float32{1, 0, 0, 1}, true, false},
		{"flip-y", [4]float32{1, 0, 0, 1}, false, true},
		{"flip-xy", [4]float32{1, 0, 0, 1}, true, true},
		{"rotation", [4]float32{0, 1, -1, 0}, false, false},
		{"negative-scale", [4]float32{-1, 0, 0, 1}, false, false},
		{"nonuniform-scale", [4]float32{.75, 0, 0, 1.25}, false, false},
		// Exact binary coordinates, with pixel centers > 0.068 pixels from
		// edges, keep this a lighting test instead of testing implementation-
		// dependent subpixel raster rounding on almost-on-edge samples.
		{"parent-shear-reflection", [4]float32{-.75, .125, .25, .875}, false, false},
		{"reflection-and-flip", [4]float32{-.75, .125, .25, .875}, true, true},
	} {
		d := g.Sprite(g.Lit)
		d.Normal = g.Normal
		d.Linear = entry.basis
		d.HasLinear = true
		if entry.flipX {
			d.Region = d.Region.FlippedX()
		}
		if entry.flipY {
			d.Region = d.Region.FlippedY()
		}
		s := WhiteSurface()
		for i, b := range entry.basis {
			s.Basis[i] = float64(b)
		}
		s.Normal = NormalReference([3]byte{191, 223, 223}, s.Basis, entry.flipX, entry.flipY)
		in := Input(lighting.LDR)
		in.Debug = lighting.DebugNormals
		g.imageCase(report, "normal-"+entry.name, in, s, func(v *render.ViewFrame) { v.Sprite(d) })
		// Evaluate actual lighting too; a debug-normal image alone does not prove
		// that the diffuse loop consumes the receiver's normal.
		l := Light(lighting.Point)
		l.Position = mathx.Vec2{X: 8, Y: 7}
		in = Input(lighting.LDR, l)
		g.imageCase(report, "normal-shading-"+entry.name, in, s, func(v *render.ViewFrame) { v.Sprite(d) })
	}
	for _, entry := range []struct {
		name       string
		basis      [4]float32
		degenerate bool
	}{
		{"normal", [4]float32{1, 0, 0, 1}, false},
		{"normal-affine", [4]float32{-.75, .125, .25, .875}, false},
		{"degenerate-uv-flat", [4]float32{1, 0, 0, 1}, true},
	} {
		d := g.Mesh(true)
		d.World = mathx.Mat4FromMat3(mathx.Mat3{entry.basis[0], entry.basis[1], 0, entry.basis[2], entry.basis[3], 0, 16, 16, 1})
		if entry.degenerate {
			d.Mesh = g.DegenerateQuad
		}
		s := WhiteSurface()
		for i, b := range entry.basis {
			s.Basis[i] = float64(b)
		}
		if !entry.degenerate {
			s.Normal = NormalReference([3]byte{191, 223, 223}, s.Basis, false, false)
		}
		in := Input(lighting.LDR)
		in.Debug = lighting.DebugNormals
		g.imageCase(report, "mesh-"+entry.name, in, s, func(v *render.ViewFrame) { v.Mesh(d) })
		l := Light(lighting.Point)
		l.Position = mathx.Vec2{X: 8, Y: 7}
		in = Input(lighting.LDR, l)
		g.imageCase(report, "mesh-shading-"+entry.name, in, s, func(v *render.ViewFrame) { v.Mesh(d) })
	}
	runAmbientEmission(g, report)
	runAlphaOrder(g, report)
	runShadows(g, report)
}

func runAmbientEmission(g *Rig, report *Report) {
	in := Input(lighting.LDR)
	in.Environment.Ambient = mathx.Color{R: .2, G: .35, B: .6, A: 1}
	in.Environment.AmbientIntensity = .8
	d := g.Sprite(g.Lit)
	g.imageCase(report, "ambient-without-direct-lights", in, WhiteSurface(), func(v *render.ViewFrame) { v.Sprite(d) })
	report.Case("emission-preserves-transparent-coverage", func() error {
		if !g.Lit.Set("EmissionColor", [4]float32{.6, .15, .025, 1}) || !g.Lit.Set("EmissionIntensity", [4]float32{.8}) {
			return fmt.Errorf("lit emission missing from declared schema")
		}
		defer g.Lit.Reset("EmissionColor")
		defer g.Lit.Reset("EmissionIntensity")
		in := Input(lighting.LDR)
		d := g.Sprite(g.Lit)
		d.Tint = mathx.Color{R: 1, G: 1, B: 1, A: .5}
		s := WhiteSurface()
		s.Color = PMA(d.Tint)
		s.Emission = [3]float64{.48, .12, .02}
		got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		return report.Image("emission-preserves-transparent-coverage", got, Reference(in, [4]float64{}, s), 2)
	})
}

func runAlphaOrder(g *Rig, report *Report) {
	report.Case("mixed-sprite-mesh-lit-unlit-A-B-A-painter-order", func() error {
		in := Input(lighting.LDR)
		in.Environment.AmbientIntensity = .7
		a := g.Sprite(g.Lit)
		a.Tint = mathx.Color{R: 1, A: 128.0 / 255}
		b := g.Mesh(false)
		b.Material = nil
		b.Color = mathx.Color{G: 1, A: 128.0 / 255}
		c := g.Sprite(g.Lit)
		c.Tint = mathx.Color{B: 1, A: 128.0 / 255}
		sa, sb, sc := WhiteSurface(), WhiteSurface(), WhiteSurface()
		sa.Color = PMA(a.Tint)
		sb.Color = PMA(b.Color)
		sb.Lit = false
		sc.Color = PMA(c.Tint)
		got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(a); v.Mesh(b); v.Sprite(c) })
		if err != nil {
			return err
		}
		report.Stats["mixed-sprite-mesh-lit-unlit-A-B-A-painter-order"] = g.Renderer.Stats()
		want := Reference(in, [4]float64{}, sa, sb, sc)
		if err := report.Image("mixed-sprite-mesh-lit-unlit-A-B-A-painter-order", got, want, 2); err != nil {
			return err
		}
		// Checkerboard uses the actual PMA output. Save it as a second visual so
		// coverage mistakes cannot hide against an opaque black background.
		report.Images["mixed-A-B-A-checkerboard"] = Checkerboard(got)
		return nil
	})
}

func Checkerboard(image []byte) []byte {
	out := slices.Clone(image)
	for y := 0; y < Size; y++ {
		for x := 0; x < Size; x++ {
			i := (y*Size + x) * 4
			a := float64(image[i+3]) / 255
			b := .2
			if ((x/4)+(y/4))%2 == 0 {
				b = .8
			}
			for c := 0; c < 3; c++ {
				out[i+c] = byte(clamp01(float64(image[i+c])/255+b*(1-a))*255 + .5)
			}
			out[i+3] = 255
		}
	}
	return out
}

func ShadowInput(kind lighting.Type) lighting.Input {
	l := Light(kind)
	l.Position = mathx.Vec2{X: 6, Y: 16}
	l.Range = 40
	l.Shadows = true
	l.Height = 15
	l.InnerAngle = 1.7
	l.OuterAngle = 2.7
	in := Input(lighting.LDR, l)
	in.Points = []mathx.Vec2{{X: 15, Y: -64}, {X: 15, Y: 96}}
	in.Occluders = []lighting.Occluder{{ID: ids.NodeID{Lo: 8}, GeometryRevision: 1, TransformRevision: 1, First: 0, Count: 2, Layers: ^uint64(0)}}
	return in
}

func runShadows(g *Rig, report *Report) {
	report.Case("two-shadow-array-layers-filter-without-cross-layer-bleed", func() error {
		in := ShadowInput(lighting.Point)
		in.Environment.ShadowResolution = 512
		in.Lights[0].Position.X = 6
		in.Lights[0].Range = 32
		in.Lights[0].Color = mathx.Color{R: 1, A: 1}
		in.Lights[0].ShadowSoftness = 1
		other := in.Lights[0]
		other.ID.Lo = 2
		other.Position.X = 26
		other.Color = mathx.Color{G: 1, A: 1}
		in.Lights = append(in.Lights, other)
		d := g.Sprite(g.Lit)
		got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		if g.Renderer.Stats().DirtyShadowMasks != 2 {
			return fmt.Errorf("expected separate masks: %+v", g.Renderer.Stats())
		}
		return report.Image("two-shadow-array-layers-filter-without-cross-layer-bleed", got, Reference(in, [4]float64{}, WhiteSurface()), 2)
	})
	for _, entry := range []struct {
		name string
		kind lighting.Type
	}{{"point", lighting.Point}, {"spot", lighting.Spot}, {"cookie", lighting.Sprite}} {
		in := ShadowInput(entry.kind)
		if entry.kind == lighting.Sprite {
			in.Lights[0].Cookie = g.Cookie
		}
		d := g.Sprite(g.Lit)
		g.imageCase(report, "shadow-"+entry.name, in, WhiteSurface(), func(v *render.ViewFrame) { v.Sprite(d) })
	}
	report.Case("shadow-overlap-is-binary-union", func() error {
		in := ShadowInput(lighting.Point)
		in.Debug = lighting.DebugShadows
		d := g.Sprite(g.Lit)
		before, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		before = slices.Clone(before)
		duplicate := in.Occluders[0]
		duplicate.ID.Lo++
		in.Occluders = append(in.Occluders, duplicate)
		got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		if err := Compare(got, before, 0); err != nil {
			return err
		}
		return report.Image("shadow-overlap-is-binary-union", got, Reference(in, [4]float64{}, WhiteSurface()), 1)
	})
	report.Case("offscreen-caster-shadows-visible-receiver", func() error {
		in := ShadowInput(lighting.Point)
		in.Lights[0].Position.X = -6
		in.Points[0].X = -2
		in.Points[1].X = -2
		d := g.Sprite(g.Lit)
		got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		return report.Image("offscreen-caster-shadows-visible-receiver", got, Reference(in, [4]float64{}, WhiteSurface()), 1)
	})
	for _, onEdge := range []bool{false, true} {
		name := "closed-occluder-light-inside"
		if onEdge {
			name = "closed-occluder-light-on-edge"
		}
		report.Case(name, func() error {
			in := ShadowInput(lighting.Point)
			in.Points = []mathx.Vec2{{X: 4, Y: 14}, {X: 8, Y: 14}, {X: 8, Y: 18}, {X: 4, Y: 18}}
			in.Occluders[0].Count = 4
			in.Occluders[0].Closed = true
			if onEdge {
				in.Lights[0].Position.X = 4
			}
			d := g.Sprite(g.Lit)
			got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
			if err != nil {
				return err
			}
			// Entire mask is shadowed by the documented inside/boundary policy.
			black := Input(lighting.LDR)
			return report.Image(name, got, Reference(black, [4]float64{}, WhiteSurface()), 1)
		})
	}
	report.Case("shadow-cache-color-does-not-rebuild-geometry", func() error {
		in := ShadowInput(lighting.Point)
		d := g.Sprite(g.Lit)
		if _, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) }); err != nil {
			return err
		}
		in.Lights[0].Color = mathx.Color{R: .3, G: .6, B: 1, A: 1}
		in.Lights[0].Intensity = .9
		got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		stats := g.Renderer.Stats()
		report.Stats["shadow-cache-color-does-not-rebuild-geometry"] = stats
		if stats.DirtyShadowMasks != 0 || stats.ShadowCacheHits < 1 {
			return fmt.Errorf("color-only change rebuilt shadows: %+v", stats)
		}
		if err := report.Image("shadow-cache-color-does-not-rebuild-geometry", got, Reference(in, [4]float64{}, WhiteSurface()), 2); err != nil {
			return err
		}
		in.Points[0].X = 19
		in.Points[1].X = 19
		in.Occluders[0].GeometryRevision++
		if _, err = g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) }); err != nil {
			return err
		}
		if g.Renderer.Stats().DirtyShadowMasks != 1 {
			return fmt.Errorf("shape revision did not rebuild mask: %+v", g.Renderer.Stats())
		}
		return nil
	})
}
