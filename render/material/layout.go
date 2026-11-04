package material

import (
	"fmt"
	"strconv"
	"strings"

	"aego/gpu"
	"aego/render/shader"
)

func (c *Cache) validateLayout(sh *Shader, h gpu.ShaderHandle) error {
	inspector, ok := c.dev.(gpu.UniformBlockInspector)
	if !ok {
		return nil
	}
	if sh.Features&shader.FeatureLighting != 0 {
		if err := validateLightingLayout(inspector, h); err != nil {
			return err
		}
		if err := validateSurfaceLayout(inspector, h); err != nil {
			return err
		}
	}
	block, active, err := inspector.UniformBlockLayout(h, shader.SlotMaterial)
	if err != nil || !active {
		return err
	}
	if block.Size > sh.Schema.Size {
		return fmt.Errorf("SHD009: linked material block needs %d bytes, ABI allocates %d", block.Size, sh.Schema.Size)
	}
	for _, member := range block.Members {
		name := strings.TrimPrefix(member.Name, "ub2.")
		p, ok := sh.Schema.Param(name)
		if !ok || p.Instance || p.Kind == shader.KindTexture {
			return fmt.Errorf("SHD009: linked material block has unexpected member %s", member.Name)
		}
		if member.Offset != p.Offset || member.Type != p.Kind.GLSL() {
			return fmt.Errorf("SHD009: %s linked as %s at %d, ABI requires %s at %d", name, member.Type, member.Offset, p.Kind.GLSL(), p.Offset)
		}
	}
	return nil
}

func validateSurfaceLayout(inspector gpu.UniformBlockInspector, h gpu.ShaderHandle) error {
	block, active, err := inspector.UniformBlockLayout(h, shader.SlotSurface)
	if err != nil || !active {
		return err
	}
	if block.Size > shader.SurfaceSize {
		return fmt.Errorf("SHD009: surface block size %d exceeds std140 ABI %d", block.Size, shader.SurfaceSize)
	}
	for _, member := range block.Members {
		name := strings.TrimPrefix(member.Name, "ub5.")
		valid := false
		for _, field := range shader.SurfaceOffsets {
			if name == field.Name {
				valid = member.Type == "vec4" && member.Offset == field.Offset
				break
			}
		}
		if !valid {
			return fmt.Errorf("SHD009: unexpected surface member %s (%s offset %d)", member.Name, member.Type, member.Offset)
		}
	}
	return nil
}

func validateLightingLayout(inspector gpu.UniformBlockInspector, h gpu.ShaderHandle) error {
	block, active, err := inspector.UniformBlockLayout(h, shader.SlotLighting)
	if err != nil || !active {
		return err
	}
	if block.Size > shader.LightingSize {
		return fmt.Errorf("SHD009: lighting block size %d exceeds std140 ABI %d", block.Size, shader.LightingSize)
	}
	for _, member := range block.Members {
		name := strings.TrimPrefix(member.Name, "ub4.")
		valid := false
		for _, field := range shader.LightingOffsets {
			base := strings.TrimSuffix(field.Name, "[0]")
			if name == base || name == field.Name {
				valid = member.Offset == field.Offset && member.Type == "vec4"
				break
			}
			if tail, ok := strings.CutPrefix(name, base+"["); ok && strings.HasSuffix(tail, "]") {
				index, e := strconv.Atoi(strings.TrimSuffix(tail, "]"))
				valid = e == nil && index >= 0 && index < shader.LightingMaxLights && member.Offset == field.Offset+index*16 && member.Type == "vec4"
				break
			}
		}
		if !valid {
			return fmt.Errorf("SHD009: unexpected lighting member %s (%s offset %d)", member.Name, member.Type, member.Offset)
		}
	}
	return nil
}
