package render

import (
	"math"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render/lighting"
	"aego/render/shader"
	"aego/render/sprite"
)

// Only byte-verified stock Lit is eligible: opaque normalized albedo without emission bounds RGB by tint times incident energy.
// Shadow, normal, cookie, cone and distance attenuation only reduce it; unknown or additive draws contribute their full geometry.
func (f *ViewFrame) belowBloomThreshold(d *sprite.Draw) bool {
	m, in := d.Material, f.lighting
	if m == nil || !m.Shader().StockLit || m.Blend() != shader.BlendPremultiplied || !d.Lighting || d.Tint.A != 1 ||
		in == nil || in.Debug != lighting.DebugLighting || in.Disabled || d.LightSet == 0 {
		return false
	}
	lanes := d.Lanes
	if lanes == nil {
		lanes = m.DefaultLanes()
	}
	// Runtime submissions may supply arbitrary lanes and a very ill-conditioned
	// affine basis. Keep non-finite normals/emission out of the interval proof.
	for _, v := range lanes {
		if !(v >= -65504 && v <= 65504) {
			return false
		}
	}
	basis := d.Basis()
	scale := max(mathx.Abs(basis[0]), mathx.Abs(basis[1]), mathx.Abs(basis[2]), mathx.Abs(basis[3]))
	det := mathx.Abs(basis[0]*basis[3] - basis[1]*basis[2])
	if !(scale >= .001 && scale <= 1024 && det > scale*scale/16) {
		return false // the inverse-transpose normal transform must be well-conditioned
	}
	p, ok := m.Shader().Schema.Param("EmissionIntensity")
	if !ok || !p.Instance || lanes[p.Lane*4+p.Comp] != 0 {
		return false
	}
	proof, ok := f.r.dev.(interface {
		gpu.TextureDescriber
		OpaqueRegion(gpu.TextureHandle, float32, float32, float32, float32) bool
	})
	r := d.Region
	if !ok || !proof.OpaqueRegion(r.Texture, r.UVMin.X, r.UVMin.Y, r.UVMax.X, r.UVMax.Y) {
		return false
	}
	desc, ok := proof.TextureInfo(r.Texture)
	if !ok || desc.Format != gpu.TexRGBA8 && desc.Format != gpu.TexSRGBA8 {
		return false
	}
	for _, h := range [...]gpu.TextureHandle{d.Normal.Texture, d.Emission.Region.Texture} {
		if h.IsZero() {
			continue
		}
		// Even zero emission times infinity would produce NaN with an
		// unbounded source. The normal must also be finite before normalization.
		info, known := proof.TextureInfo(h)
		if !known || info.Format != gpu.TexRGBA8 && info.Format != gpu.TexSRGBA8 && info.Format != gpu.TexR8 {
			return false
		}
	}
	env := lighting.DefaultEnvironment()
	if in.HasEnvironment {
		env = in.Environment
	}
	// Invalid inputs are rejected by prepareLighting. Qualify a finite,
	// nonnegative subset here as well, before the GPU scene is prepared.
	if !boundedEnergy(env.Ambient, env.AmbientIntensity) {
		return false
	}
	red := env.Ambient.R * env.AmbientIntensity
	green := env.Ambient.G * env.AmbientIntensity
	blue := env.Ambient.B * env.AmbientIntensity
	set := f.sets[d.LightSet-1]
	for _, index := range set.Indices[:set.Count] {
		l := &in.Lights[index]
		if !boundedEnergy(l.Color, l.Intensity) {
			return false
		}
		// Normal directions are uploaded normalized (zero maps to +X). Leave
		// very large inputs and degenerate local attenuation to the full kernel.
		if !(mathx.Abs(l.Direction.X) <= 2 && mathx.Abs(l.Direction.Y) <= 2) {
			return false
		}
		if l.Type != lighting.Directional && !(l.Range >= .001 && l.Range <= 65504 &&
			l.Height >= 0 && l.Height <= 65504 && l.Falloff > 0 && l.Falloff <= 64 &&
			mathx.Abs(l.Position.X) <= 1<<20 && mathx.Abs(l.Position.Y) <= 1<<20) {
			return false
		}
		red += l.Color.R * l.Intensity
		green += l.Color.G * l.Intensity
		blue += l.Color.B * l.Intensity
	}
	tint := d.Tint.PackRGBA8()
	luma := (.2126*float32(byte(tint))*red + .7152*float32(byte(tint>>8))*green + .0722*float32(byte(tint>>16))*blue) / 255
	// Cover shader normalization, interpolation, one RT storage rounding
	// (binary16 or UNORM8), and extraction filtering with generous headroom.
	return luma >= 0 && luma*1.02+2.0/255 < f.brightThreshold
}

func boundedEnergy(color mathx.Color, intensity float32) bool {
	return color.R >= 0 && color.R <= 65504 && color.G >= 0 && color.G <= 65504 &&
		color.B >= 0 && color.B <= 65504 && intensity >= 0 && intensity <= 65504
}

// A handful of world rectangles keeps distant, short platforms from forcing a
// tall backdrop's entire width through the HDR resolve. Unions only overdraw.
type sceneGroups struct {
	box   [4]mathx.AABB2
	count int
}

func (g *sceneGroups) add(b mathx.AABB2) {
	size := b.Size()
	best := 0
	waste := float32(math.Inf(1))
	for i, a := range g.box[:g.count] {
		ab, as := a.Union(b).Size(), a.Size()
		delta := ab.X*ab.Y - as.X*as.Y - size.X*size.Y
		if delta < waste {
			waste, best = delta, i
		}
	}
	if g.count < len(g.box) && waste > 0 {
		g.box[g.count] = b
		g.count++
	} else {
		g.box[best] = g.box[best].Union(b)
	}
}

func (f *ViewFrame) drawSparsePost(p gpu.Pass, pipe, copyPipe gpu.PipelineHandle, near, scene, dst mathx.Rect) {
	var cuts [4]mathx.Rect
	count := 0
	for _, b := range f.sceneGroups.box[:f.sceneGroups.count] {
		cut, ok := f.projectBounds(b, dst)
		if !ok {
			cuts[0], count = scene, 1
			break
		}
		// The full Bloom draw will cover this region by itself.
		if near.X <= cut.X && near.Y <= cut.Y && near.X+near.W >= cut.X+cut.W && near.Y+near.H >= cut.Y+cut.H {
			continue
		}
		cuts[count] = cut
		count++
	}
	if count != 0 {
		p.SetPipeline(copyPipe)
		for _, area := range cuts[:count] {
			p.SetScissor(area, true)
			p.Draw(0, 4)
		}
		p.SetPipeline(pipe)
	}
	p.SetScissor(near, true)
	p.Draw(0, 4)
}
