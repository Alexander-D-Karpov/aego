package render2d

import (
	"aego/engine/module"
	"aego/engine/problems"
	"aego/render/shader"
	"fmt"
)

func validateSpriteMaterial(ctx module.ValidateContext, s *SpriteRenderer) []problems.Diagnostic {
	if s.Material.IsZero() {
		return nil
	}
	m, err := NodeMaterial(ctx.Scene, ctx.Node)
	if err != nil {
		return []problems.Diagnostic{{Code: "MAT004", Severity: problems.Warning, Message: err.Error(), Help: "Assign an existing material asset; the unresolved AssetID is preserved."}}
	}
	if m.Shader().Domain != shader.DomainSprite {
		return []problems.Diagnostic{{Code: "MAT006", Severity: problems.Error, Message: "SpriteRenderer needs a sprite-domain material", BlocksPlay: true}}
	}
	return nil
}
func validateMeshMaterial(ctx module.ValidateContext, mr *Mesh2D) []problems.Diagnostic {
	if mr.Material.IsZero() {
		return nil
	}
	m, err := NodeMaterial(ctx.Scene, ctx.Node)
	if err != nil {
		return []problems.Diagnostic{{Code: "MAT004", Severity: problems.Warning, Message: err.Error()}}
	}
	if m.Shader().Domain != shader.DomainMesh2D {
		return []problems.Diagnostic{{Code: "MAT006", Severity: problems.Error, Message: "Mesh2D needs a mesh2d-domain material", BlocksPlay: true}}
	}
	return nil
}
func validateOverrides(ctx module.ValidateContext, o *MaterialOverride) []problems.Diagnostic {
	m, err := NodeMaterial(ctx.Scene, ctx.Node)
	if err != nil {
		return []problems.Diagnostic{{Code: "MAT004", Severity: problems.Warning, Message: err.Error()}}
	}
	var out []problems.Diagnostic
	seen := map[string]bool{}
	for _, v := range o.Values {
		p, known := m.Shader().Schema.Param(v.Name)
		if !known {
			out = append(out, problems.Diagnostic{Code: "MAT001", Severity: problems.Warning, Message: "Unknown material parameter " + v.Name + " is preserved"})
			continue
		}
		if seen[p.Name] {
			out = append(out, problems.Diagnostic{Code: "MAT002", Severity: problems.Error, Message: "Duplicate material override " + p.Name, BlocksPlay: true})
			continue
		}
		seen[p.Name] = true
		if !p.Instance || p.Kind != v.Kind {
			out = append(out, problems.Diagnostic{Code: "MAT002", Severity: problems.Error, Message: "Explicit material override " + v.Name + " no longer matches the shader type or instance policy", BlocksPlay: true})
		}
	}
	if len(seen) > MaxMaterialOverrides {
		out = append(out, problems.Diagnostic{Code: "MAT002", Severity: problems.Error, Message: fmt.Sprintf("MaterialOverride has %d active values; maximum is %d", len(seen), MaxMaterialOverrides), BlocksPlay: true})
	}
	return out
}
