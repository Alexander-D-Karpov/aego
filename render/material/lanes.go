package material

import (
	"encoding/binary"
	"fmt"
	"math"

	"aego/gpu"
	"aego/render/shader"
)

func (c *Cache) BindDomain(p gpu.Pass, m *Material, domain shader.Domain) bool {
	return c.bindDomain(p, m, domain, false, false)
}

// BindSprite enables the optional original-local-UV stream for trimmed batches.
func (c *Cache) BindSprite(p gpu.Pass, m *Material, trim bool) bool {
	return c.bindDomain(p, m, shader.DomainSprite, trim, false)
}

func (c *Cache) BindSpriteAux(p gpu.Pass, m *Material, trim, aux bool) bool {
	return c.bindDomain(p, m, shader.DomainSprite, trim, aux)
}

func (c *Cache) BindSpriteCoverage(p gpu.Pass, m *Material, trim, aux, opaque bool) bool {
	if opaque && m != nil && m.shader != nil && m.shader.Domain == shader.DomainSprite && m.shader.OpaqueWithOpaqueAlbedo && m.blend == shader.BlendPremultiplied {
		return c.bindBlend(p, m, trim, aux, shader.BlendOpaque)
	}
	return c.BindSpriteAux(p, m, trim, aux)
}

func (c *Cache) bindDomain(p gpu.Pass, m *Material, domain shader.Domain, trim, aux bool) bool {
	if m == nil {
		return false
	}
	if m.shader.Domain != domain {
		key := domainFailure{m.shader, domain}
		if !c.domainFailed[key] {
			c.domainFailed[key] = true
			if c.OnError != nil {
				c.OnError(m.shader, m.variant, DomainMismatch{Actual: m.shader.Domain, Required: domain})
			}
		}
		return false
	}
	return c.bind(p, m, trim, aux)
}

// DomainMismatch rejects a draw, not a shader program. The same material
// can remain valid in its own domain (notably a full-screen PostFX pass).
type DomainMismatch struct{ Actual, Required shader.Domain }

func (e DomainMismatch) Error() string {
	return fmt.Sprintf("%s: %s shader cannot draw %s geometry", MAT004, e.Actual, e.Required)
}

type domainFailure struct {
	shader *Shader
	domain shader.Domain
}

// BindLanes is the single-instance path for Mesh2D. Sprites use the batched lane
// stream instead. The scratch buffer is retained, so steady-state draws allocate nothing.
func (c *Cache) BindLanes(p gpu.Pass, m *Material, lanes *[16]float32) bool {
	if m.Lanes() == 0 {
		return true
	}
	if c.laneBuffer.IsZero() {
		h, err := c.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferVertex, Size: shader.LaneStride, Dynamic: true, Label: "material.mesh-lanes"})
		if err != nil {
			return false
		}
		c.laneBuffer = h
	}
	if lanes == nil {
		lanes = m.DefaultLanes()
	}
	for i, v := range lanes {
		binary.LittleEndian.PutUint32(c.laneBytes[i*4:], math.Float32bits(v))
	}
	c.dev.UpdateBuffer(c.laneBuffer, 0, c.laneBytes[:])
	c.stats.LaneBytes += uint64(len(c.laneBytes))
	p.SetVertexBuffer(shader.LaneSlot, c.laneBuffer)
	return true
}
