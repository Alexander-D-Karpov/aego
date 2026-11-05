package export

import (
	"bytes"
	"os"

	"aego/asset"
	"aego/core/ids"
	"aego/engine/problems"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/texture"
)

const (
	CodeShader      = "EXP013"
	CodeMaterial    = "EXP014"
	CodeDirtySource = "EXP015"
)

// Export checks current source against the exact artifact being shipped, even before file watching or after a failed import; last-good is an editor rendering policy.
func checkMaterialSources(s *Snapshot) []problems.Diagnostic {
	var out []problems.Diagnostic
	for _, rel := range s.DirtySources {
		out = append(out, diag(CodeDirtySource, problems.Error, "save and validate the modified asset source or import metadata before export", rel))
	}
	if s.current == nil {
		return out
	}
	s.idx.Each(func(e *asset.Entry) {
		if e.Missing || (e.Kind != asset.ArtShader && e.Kind != asset.ArtMaterial) {
			return
		}
		src, err := os.ReadFile(s.current.Abs(e.Source))
		if err != nil {
			out = append(out, diag(CodeShader, problems.Error, "cannot read current source: "+err.Error(), e.Source))
			return
		}
		result, err := s.current.Preview(e.Source, src, nil)
		if err != nil {
			out = append(out, diag(CodeShader, problems.Error, "current source is invalid; last-good cannot be exported: "+err.Error(), e.Source))
			return
		}
		for _, prepared := range result.Artifacts {
			if prepared.Kind != e.Kind {
				continue
			}
			artifact, ok := e.Artifact(e.Kind)
			if !ok {
				continue
			}
			cached, err := s.files.ReadFile(artifact.Path)
			if err != nil || !bytes.Equal(cached, prepared.Data) {
				out = append(out, diag(CodeShader, problems.Error, "current shader/material source differs from the export snapshot; reimport and export again", e.Source))
			}
		}
	})
	return out
}

func checkMaterialArtifacts(s *Snapshot) []problems.Diagnostic {
	var out []problems.Diagnostic
	r := &resolver{s: s}
	s.idx.Each(func(e *asset.Entry) {
		if e.Missing || (e.Kind != asset.ArtShader && e.Kind != asset.ArtMaterial) {
			return
		}
		data, ok := r.blob(e.ID, e.Kind)
		if !ok {
			out = append(out, diag(CodeMaterial, problems.Error, "prepared artifact is missing", e.Source))
			return
		}
		if e.Kind == asset.ArtShader {
			a, err := asset.DecodeShader(data)
			if err != nil {
				out = append(out, diag(CodeShader, problems.Error, err.Error(), e.Source))
				return
			}
			if a.Validation&shader.ReferenceChecked == 0 {
				out = append(out, diag(CodeShader, problems.Error, "portable export requires reference validation; install glslangValidator and reimport the shader", e.Source))
			}
			for _, variant := range a.Variants {
				for _, source := range variant.Sources {
					if source == "" {
						out = append(out, diag(CodeShader, problems.Error, "a variant has no prepared source for one of its target stages", e.Source))
						return
					}
				}
			}
			return
		}
		doc, err := asset.DecodeMaterial(data, e.Source)
		if err != nil {
			out = append(out, diag(CodeMaterial, problems.Error, err.Error(), e.Source))
			return
		}
		sh, ok := r.Shader(doc.Shader)
		if !ok {
			out = append(out, diag(CodeMaterial, problems.Error, "the material's shader artifact is missing or invalid", e.Source))
			return
		}
		for _, p := range doc.Validate(sh) {
			severity := problems.Error
			if p.Warn {
				severity = problems.Warning
			}
			out = append(out, diag(CodeMaterial, severity, p.Code+": "+p.Msg, e.Source))
		}
		for name, id := range doc.Textures {
			p, declared := sh.Schema.Param(name)
			texture, exists := r.textureInfo(id)
			if !exists || texture.region && (!declared || (!p.Normal && !p.Emission)) {
				out = append(out, diag(CodeMaterial, problems.Error, "texture "+name+" is missing or has an incompatible region binding: "+id.String(), e.Source))
				continue
			}
			if !id.IsZero() && declared && p.Normal && texture.semantic != asset.TextureNormal {
				out = append(out, diag(CodeMaterial, problems.Error, "texture "+name+" requires linear normal-map semantics: "+id.String(), e.Source))
				continue
			}
			if !id.IsZero() && declared && p.Emission && texture.semantic != asset.TextureEmissionRGB && texture.semantic != asset.TextureEmissionMask {
				out = append(out, diag(CodeMaterial, problems.Error, "texture "+name+" requires emission RGB or mask semantics: "+id.String(), e.Source))
				continue
			}
			if err := r.verifyCurrentTexture(id); err != nil {
				out = append(out, diag(CodeMaterial, problems.Error, "texture "+name+" is not current: "+err.Error(), e.Source))
			}
		}
	})
	return out
}

func (r *resolver) Shader(id ids.AssetID) (*material.Shader, bool) {
	data, ok := r.blob(id, asset.ArtShader)
	if !ok {
		data, ok = asset.BuiltinShader(id)
	}
	if !ok {
		return nil, false
	}
	a, err := asset.DecodeShader(data)
	if err != nil {
		return nil, false
	}
	return a.Runtime(id, id.String()), true
}

func (r *resolver) Material(id ids.AssetID) (*material.Material, bool) {
	data, ok := r.blob(id, asset.ArtMaterial)
	if !ok {
		data, ok = asset.BuiltinMaterial(id)
	}
	if !ok {
		return nil, false
	}
	doc, err := asset.DecodeMaterial(data, id.String())
	if err != nil {
		return nil, false
	}
	sh, ok := r.Shader(doc.Shader)
	if !ok {
		return nil, false
	}
	m, _ := doc.Build(id.String(), sh, func(tid ids.AssetID) (material.Tex, bool) {
		t, ok := r.textureInfo(tid)
		mode := texture.EmissionNone
		switch t.semantic {
		case asset.TextureEmissionRGB:
			mode = texture.EmissionRGB
		case asset.TextureEmissionMask:
			mode = texture.EmissionMask
		}
		return material.Tex{RegionOnly: t.region, NormalMap: t.semantic == asset.TextureNormal, EmissionMode: mode}, ok
	})
	if m != nil {
		m.ID = id
	}
	return m, m != nil
}
