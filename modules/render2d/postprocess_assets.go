package render2d

import (
	"aego/asset"
	"aego/render/postfx"
	"aego/render/shader"
	"fmt"
)

// PreparePostProcess validates a private editor preview's authored phases/inputs as in a game frame; the scene loader prevents stale
// texture/material handles across reimport/restore, even for stable numeric stacks. LUTs are horizontal data strips, never atlas cells or sRGB images.
func PreparePostProcess(reg *postfx.Registry, dst *postfx.Plan, c *PostProcess, res asset.Resolver, gpuRequired bool) error {
	src, _ := res.(postfx.MaterialSource)
	if err := reg.BuildWithMaterials(dst, c.Enabled, c.Effects, postfx.Profile{}, src); err != nil {
		return err
	}
	return resolvePostAssets(dst, res, gpuRequired)
}

func resolvePostAssets(plan *postfx.Plan, res asset.Resolver, gpuRequired bool) error {
	for i := range plan.Passes {
		p := &plan.Passes[i]
		if !p.LUTID.IsZero() {
			meta, ok := res.(asset.TextureMetadataSource)
			if !ok {
				return fmt.Errorf("%s: no prepared LUT metadata", p.InstanceID)
			}
			m, ok := meta.TextureMetadata(p.LUTID)
			if !ok || m.Region || m.Semantic != asset.TextureData || (m.Height != 16 && m.Height != 32) || m.Width != m.Height*m.Height {
				return fmt.Errorf("%s: LUT must be a standalone linear 256x16 or 1024x32 data strip", p.InstanceID)
			}
			p.LUTSize = m.Height
			if gpuRequired {
				r, ok := res.Texture(p.LUTID)
				if !ok || r.Texture.IsZero() {
					return fmt.Errorf("%s: LUT texture is not ready", p.InstanceID)
				}
				p.LUT = r.Texture
			}
		}
		if !p.MaterialID.IsZero() {
			src, ok := res.(asset.MaterialSource)
			if !ok {
				return fmt.Errorf("%s: material source unavailable", p.InstanceID)
			}
			m, ok := src.Material(p.MaterialID)
			if !ok || m == nil || m.Shader() == nil || m.Shader().Domain != shader.DomainPostFX {
				return fmt.Errorf("%s: expected a prepared postfx-domain material", p.InstanceID)
			}
			p.Material = m
		}
	}
	return nil
}
