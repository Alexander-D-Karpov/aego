package lightinggolden

import (
	"fmt"
	"slices"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/render"
	"aego/render/lighting"
)

func RunSelection(g *Rig, report *Report) {
	report.Case("light-mask-bit63-and-bit0-remain-distinct", func() error {
		red, green := Light(lighting.Directional), Light(lighting.Directional)
		red.Layers = uint64(1) << 63
		red.Color = mathx.Color{R: 1, A: 1}
		red.Elevation = mathx.Pi / 2
		green.Layers = 1
		green.Color = mathx.Color{G: 1, A: 1}
		green.Elevation = mathx.Pi / 2
		green.ID = ids.NodeID{Lo: 2}
		in := Input(lighting.LDR, red, green)
		a, b := g.Sprite(g.Lit), g.Sprite(g.Lit)
		a.Position.X = 8
		b.Position.X = 24
		a.Size.X = 12
		b.Size.X = 12
		a.VisibilityLayers = uint64(1) << 63
		b.VisibilityLayers = 1
		got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(a); v.Sprite(b) })
		if err != nil {
			return err
		}
		sa, sb := WhiteSurface(), WhiteSurface()
		sa.X = 8
		sb.X = 24
		sa.Width = 12
		sb.Width = 12
		sa.Lit = false
		sb.Lit = false
		sa.Color = [4]float64{.75, 0, 0, 1}
		sb.Color = [4]float64{0, .75, 0, 1}
		if err = report.Image("light-mask-bit63-and-bit0-remain-distinct", got, Reference(in, [4]float64{}, sa, sb), 2); err != nil {
			return err
		}
		report.Stats["light-mask-bit63-and-bit0-remain-distinct"] = g.Renderer.Stats()
		a.VisibilityLayers = 0
		b.VisibilityLayers = 0
		got, err = g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(a); v.Sprite(b) })
		if err != nil {
			return err
		}
		sa.Color = [4]float64{0, 0, 0, 1}
		sb.Color = sa.Color
		return report.Image("light-mask-zero-means-no-light", got, Reference(in, [4]float64{}, sa, sb), 1)
	})
	report.Case("light-budget-priority-influence-stable-ID", func() error {
		lights := make([]lighting.Light, 9)
		for i := range lights {
			l := Light(lighting.Directional)
			l.ID = ids.NodeID{Lo: uint64(i + 1)}
			l.Intensity = .08
			l.Elevation = mathx.Pi / 2
			l.Color = mathx.Color{G: 1, A: 1}
			lights[i] = l
		}
		lights[0].Color = mathx.Color{R: 1, A: 1}
		lights[8].Color = mathx.Color{B: 1, A: 1}
		in := Input(lighting.LDR, lights...)
		d := g.Sprite(g.Lit)
		got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		expected := Input(lighting.LDR, slices.Clone(lights[:8])...)
		if err = report.Image("light-budget-stable-ID", got, Reference(expected, [4]float64{}, WhiteSurface()), 2); err != nil {
			return err
		}
		if g.Renderer.Stats().DroppedLights != 1 {
			return fmt.Errorf("dropped light counter %+v", g.Renderer.Stats())
		}
		slices.Reverse(in.Lights)
		got, err = g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		if err = report.Image("light-budget-reversed-enumeration", got, Reference(expected, [4]float64{}, WhiteSurface()), 2); err != nil {
			return err
		}
		// Explicit priority wins before equal influence/ID.
		in.Lights[0].Priority = 1
		expected.Lights[7] = in.Lights[0]
		got, err = g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
		if err != nil {
			return err
		}
		return report.Image("light-budget-explicit-priority", got, Reference(expected, [4]float64{}, WhiteSurface()), 2)
	})
	report.Case("unlit-output-adds-no-lighting-passes-or-targets", func() error {
		in := ShadowInput(lighting.Point)
		d := g.Sprite(g.Unlit)
		// Other cases used this renderer's lighting outputs. Their bounded cache
		// can survive briefly; it must expire while Unlit itself issues no passes.
		for frame := 0; frame < 4; frame++ {
			if _, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) }); err != nil {
				return err
			}
			if s := g.Renderer.Stats(); s.ResolvePasses != 0 || s.ShadowDraws != 0 || s.LightingUBOBytes != 0 {
				return fmt.Errorf("unlit consumed lighting passes: %+v", s)
			}
		}
		s := g.Renderer.Stats()
		report.Stats["unlit-output-adds-no-lighting-passes-or-targets"] = s
		if s.ResolvePasses != 0 || s.ShadowDraws != 0 || s.LightingUBOBytes != 0 || s.LightingTargetBytes != 0 {
			return fmt.Errorf("unlit consumed lighting path: %+v", s)
		}
		return nil
	})
}
