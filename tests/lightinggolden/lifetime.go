package lightinggolden

import (
	"fmt"
	"slices"
	"time"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render"
	"aego/render/lighting"
	"aego/render/material"
	"aego/render/sprite"
	"aego/vfs"
)

// RichScene exercises every persistent lighting resource category. It is also
// used immediately before/after a real browser context loss.
func RichScene(g *Rig) (lighting.Input, sprite.Draw, Surface) {
	in := ShadowInput(lighting.Sprite)
	in.Lights[0].Cookie = g.Cookie
	in.Lights[0].Position = mathx.Vec2{X: 6, Y: 16}
	if g.Device.Caps().RGBA16FRenderable {
		in.Environment.ColorMode = lighting.HDR
	}
	in.Environment.Ambient = mathx.Color{R: .04, G: .06, B: .1, A: 1}
	in.Environment.AmbientIntensity = 1
	d := g.Sprite(g.Lit)
	d.Normal = g.Normal
	d.Tint = mathx.Color{R: 1, G: .7, B: .4, A: 128.0 / 255}
	s := WhiteSurface()
	s.Color = PMA(d.Tint)
	s.Normal = NormalReference([3]byte{191, 223, 223}, s.Basis, false, false)
	return in, d, s
}

func (g *Rig) WaitReload(in *lighting.Input, d sprite.Draw) error {
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := g.Draw(in, func(v *render.ViewFrame) { v.Sprite(d) }); err != nil {
			return err
		}
		if g.Renderer.Materials().Stats().Pending == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("lighting shader completion timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

func sameResources(a, b gpu.ResourceStats) bool {
	return a.Buffers == b.Buffers && a.Textures == b.Textures && a.Samplers == b.Samplers && a.Shaders == b.Shaders && a.Pipelines == b.Pipelines && a.PendingCompiles == b.PendingCompiles && a.Framebuffers == b.Framebuffers && a.TextureStorageBytes == b.TextureStorageBytes
}

func RunLifetime(dev gpu.Device, files vfs.FS, report *Report) {
	reporter, ok := dev.(gpu.ResourceReporter)
	if !ok {
		report.Failures = append(report.Failures, "lighting-lifetime: real backend has no GPU resource reporter")
		return
	}
	report.Case("lighting-reload-resize-close-resource-lifetime", func() error {
		before := reporter.ResourceStats()
		for session := 0; session < 3; session++ {
			g, err := New(dev, dev.Caps(), files)
			if err != nil {
				return err
			}
			err = func() error {
				defer g.Close()
				in, d, s := RichScene(g)
				first, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
				if err != nil {
					return err
				}
				if session == 0 {
					if err = report.Image("lifetime-rich-scene", first, Reference(in, [4]float64{}, s), 2); err != nil {
						return err
					}
				}
				want := slices.Clone(first)
				baseline := reporter.ResourceStats()
				for generation := 0; generation < 8; generation++ {
					old := g.Lit.Shader()
					next := CloneShader(old, fmt.Sprintf("lighting-reload-%d-%d", session, generation), false)
					g.Renderer.Materials().QueueReload(old, next, []*material.Material{g.Lit}, nil)
					if err = g.WaitReload(&in, d); err != nil {
						return err
					}
					got, err := g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
					if err != nil {
						return err
					}
					if g.Lit.Shader() != next || g.Lit.Failed() {
						return fmt.Errorf("successful Lit reload did not commit")
					}
					if err = Compare(got, want, 0); err != nil {
						return fmt.Errorf("reload changed image: %w", err)
					}
					if after := reporter.ResourceStats(); !sameResources(after, baseline) {
						return fmt.Errorf("reload resource growth: before=%+v after=%+v", baseline, after)
					}
					broken := CloneShader(next, "lighting-broken", true)
					errorsBefore := len(g.Errors)
					g.AllowErrors = true
					g.Renderer.Materials().QueueReload(next, broken, []*material.Material{g.Lit}, nil)
					if err = g.WaitReload(&in, d); err != nil {
						return err
					}
					g.AllowErrors = false
					if g.Lit.Shader() != next || !g.Lit.Failed() || len(g.Errors) == errorsBefore {
						return fmt.Errorf("broken Lit did not retain last-good plus compiler diagnostic")
					}
					got, err = g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) })
					if err != nil {
						return err
					}
					if err = Compare(got, want, 0); err != nil {
						return fmt.Errorf("broken Lit changed last-good: %w", err)
					}
					if after := reporter.ResourceStats(); !sameResources(after, baseline) {
						return fmt.Errorf("failed reload leaked: before=%+v after=%+v", baseline, after)
					}
				}
				for size := 36; size <= 52; size += 4 {
					h, err := dev.CreateTexture(gpu.TextureDesc{Width: size, Height: size, Format: gpu.TexRGBA8, Label: "lighting-test.resize"})
					if err != nil {
						return err
					}
					v := g.View()
					v.Target = h
					v.TargetW, v.TargetH = size, size
					err = g.Frame(v, &in, func(v *render.ViewFrame) { v.Sprite(d) })
					dev.DestroyTexture(h)
					if err != nil {
						return err
					}
					if _, err = g.Draw(&in, func(v *render.ViewFrame) { v.Sprite(d) }); err != nil {
						return err
					}
					if after := reporter.ResourceStats(); !sameResources(after, baseline) {
						return fmt.Errorf("resize resource growth: before=%+v after=%+v", baseline, after)
					}
				}
				return nil
			}()
			if err != nil {
				return err
			}
			if after := reporter.ResourceStats(); !sameResources(after, before) {
				return fmt.Errorf("session %d Close leaked: before=%+v after=%+v", session, before, after)
			}
		}
		return nil
	})
}
