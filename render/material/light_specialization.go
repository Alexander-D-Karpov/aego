package material

import (
	"fmt"
	"slices"
	"strings"

	"aego/gpu"
	"aego/render/shader"
)

type LightSignature struct {
	Count int
	// Each low two-bit type matches forward2d's ABI. Bit two denotes an active shadow layer.
	Lights [8]uint8
}
type specKey struct {
	sh     *Shader
	v      int
	lights LightSignature
}

// BindSpriteLightSet specializes only opt-in typed light-call shaders; its bounded cache falls back to the general forward program
// for older assets, unsupported layouts or failed optional compilation.
func (c *Cache) BindSpriteLightSet(p gpu.Pass, m *Material, trim, aux, opaque bool, sig LightSignature) bool {
	if m == nil || m.shader == nil || c.lost() {
		return false
	}
	if !c.es || !sig.valid() || m.shader.Domain != shader.DomainSprite || m.shader.Features&shader.FeatureLighting == 0 {
		return c.BindSpriteCoverage(p, m, trim, aux, opaque)
	}
	blend := m.blend
	if opaque && m.shader.OpaqueWithOpaqueAlbedo && blend == shader.BlendPremultiplied {
		blend = shader.BlendOpaque
	}
	if c.spec == nil {
		c.spec = make(map[specKey]*Shader)
		c.lightFirstUse = make(map[*Shader]uint64)
	}
	first, seen := c.lightFirstUse[m.shader]
	if !seen {
		first = c.frame
		c.lightFirstUse[m.shader] = first
	}
	initializing := first == c.frame
	key := specKey{m.shader, m.variant, sig}
	sh, known := c.spec[key]
	if !known && initializing && len(c.spec) < 24 {
		sh = specializeLightShader(m.shader, m.variant, sig)
		c.spec[key] = sh
	}
	if sh == nil {
		return c.BindSpriteCoverage(p, m, trim, aux, opaque)
	}
	// Prepare the ordinary layout during the same initialization. A moving
	// receiver can gain a new kind of light later without compiling a program.
	layout := pipeKey{sh, m.variant, blend, trim, aux}
	pipe, ready := c.pipes[layout]
	if !ready {
		if !initializing {
			return c.BindSpriteCoverage(p, m, trim, aux, opaque)
		}
		if _, ok := c.pipelineLayout(m.shader, m.variant, blend, trim, aux); !ok {
			return false
		}
		var ok bool
		pipe, ok = c.pipelineLayout(sh, m.variant, blend, trim, aux)
		if !ok {
			c.drop(sh)
			c.spec[key] = nil
			return c.BindSpriteCoverage(p, m, trim, aux, opaque)
		}
	}
	if !c.upload(m) {
		return false
	}
	c.Track(m)
	c.used[m.shader] = c.frame
	p.SetPipeline(pipe)
	c.BindPostParameters(p, m)
	return true
}

func (sig LightSignature) valid() bool {
	if sig.Count < 0 || sig.Count > len(sig.Lights) {
		return false
	}
	for i, entry := range sig.Lights {
		if entry > 7 || (i >= sig.Count && entry != 0) {
			return false
		}
	}
	return true
}

func specializeLightShader(parent *Shader, variant int, signature LightSignature) *Shader {
	esFragment := SourceIndex(true, true)
	source := parent.Variants[variant].Sources[esFragment]
	header, body, ok := strings.Cut(source, "\n")
	if !ok || !strings.HasPrefix(header, "#version ") || !strings.Contains(body, "#ifdef AEGO_TYPED_LIGHT_CALLS\n") {
		return nil
	}
	var out strings.Builder
	out.Grow(len(source) + signature.Count*64 + 40)
	out.WriteString(header)
	out.WriteString("\n#define AEGO_TYPED_LIGHT_CALLS ")
	for i, entry := range signature.Lights[:signature.Count] {
		fmt.Fprintf(&out, "aego_apply_light(%d,%d,%t,normal,receiveShadows,result);", i, entry&3, entry&4 != 0)
	}
	out.WriteByte('\n')
	out.WriteString(body)
	next := *parent
	next.lightParent = parent
	next.Variants = slices.Clone(next.Variants)
	next.Variants[variant].Sources[esFragment] = out.String()
	return &next
}
