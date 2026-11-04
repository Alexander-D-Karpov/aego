package export

import (
	"fmt"

	"aego/asset"
	"aego/core/ids"
	"aego/engine/problems"
)

type preparedTexture struct {
	semantic asset.TextureSemantic
	region   bool
	w, h     int
}

// Export validates prepared pixels and independent normal-region mappings
// without inventing GPU handles or loading the authoring importer in runtime.
func (r *resolver) textureInfo(id ids.AssetID) (preparedTexture, bool) {
	if id.IsZero() {
		return preparedTexture{}, true
	}
	if tex, ok := asset.BuiltinTexture(id); ok {
		semantic := asset.TextureColor
		if tex.NonColor {
			semantic = asset.TextureData
		}
		return preparedTexture{w: tex.Width, h: tex.Height, semantic: semantic}, true
	}
	e, ok := r.s.idx.Lookup(id)
	if !ok || e.Missing || e.Error != "" || r.s.files == nil {
		return preparedTexture{}, false
	}
	page := e
	var region *asset.RegionArtifact
	if e.Kind == asset.ArtSpriteRegion {
		data, ok := r.blob(id, e.Kind)
		if !ok {
			return preparedTexture{}, false
		}
		var err error
		region, err = asset.DecodeRegion(data)
		if err != nil || region.Page == id {
			return preparedTexture{}, false
		}
		page, ok = r.s.idx.Lookup(region.Page)
		if !ok || page.Missing || page.Error != "" {
			return preparedTexture{}, false
		}
	}
	if page.Kind != asset.ArtTexture && page.Kind != asset.ArtAtlasPage {
		return preparedTexture{}, false
	}
	data, ok := r.blob(page.ID, page.Kind)
	if !ok {
		return preparedTexture{}, false
	}
	tex, err := asset.DecodeTexture(data)
	if err != nil {
		return preparedTexture{}, false
	}
	info := preparedTexture{semantic: tex.Semantic, w: tex.Width, h: tex.Height}
	if region != nil {
		if region.PageW != tex.Width || region.PageH != tex.Height {
			return preparedTexture{}, false
		}
		info.region, info.w, info.h = true, region.W, region.H
		if region.SourceW != 0 {
			info.w, info.h = region.SourceW, region.SourceH
		}
	}
	return info, true
}

func (r *resolver) TextureMetadata(id ids.AssetID) (asset.TextureMetadata, bool) {
	info, ok := r.textureInfo(id)
	return asset.TextureMetadata{Width: info.w, Height: info.h, Semantic: info.semantic, Region: info.region}, ok && !id.IsZero()
}

// Compare the complete frozen dependency closure. Region bytes can remain
// identical while a normal atlas page or a height-derived image is reimported.
func (r *resolver) verifyCurrentTexture(id ids.AssetID) error {
	if r.s.current == nil {
		return nil
	}
	seen := map[ids.AssetID]bool{}
	pending := []ids.AssetID{id}
	for len(pending) != 0 {
		id = pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if id.IsZero() || seen[id] {
			continue
		}
		seen[id] = true
		if _, builtin := asset.LookupBuiltin(id); builtin {
			continue
		}
		if err := r.s.current.VerifyCurrent(id); err != nil {
			return err
		}
		frozen, haveFrozen := r.s.idx.Lookup(id)
		live, haveLive := r.s.current.Entry(id)
		if !haveFrozen || !haveLive || live.Missing {
			return fmt.Errorf("asset %s disappeared from the current import index", id)
		}
		if frozen.Kind != live.Kind || len(frozen.Artifacts) != len(live.Artifacts) {
			return fmt.Errorf("asset %s changed after the export snapshot; capture and export again", id)
		}
		for _, before := range frozen.Artifacts {
			after, exists := live.Artifact(before.Kind)
			if !exists || before.Hash != after.Hash || before.Size != after.Size {
				return fmt.Errorf("asset %s changed after the export snapshot; capture and export again", id)
			}
		}
		pending = append(pending, frozen.Deps...)
	}
	return nil
}

func checkNormalAssociations(s *Snapshot) []problems.Diagnostic {
	var out []problems.Diagnostic
	r := &resolver{s: s}
	s.idx.Each(func(e *asset.Entry) {
		if e.Missing || e.Error != "" {
			return
		}
		art, exists := e.Artifact(asset.ArtSpriteRegion)
		if !exists {
			return
		}
		data, err := s.files.ReadFile(art.Path)
		if err != nil {
			return // Snapshot capture already reports missing artifact bytes.
		}
		region, err := asset.DecodeRegion(data)
		if err != nil {
			out = append(out, diag(CodeMaterial, problems.Error, "invalid prepared region: "+err.Error(), e.Source))
			return
		}
		albedo, aok := r.textureInfo(e.ID)
		for _, channel := range []struct {
			name string
			id   ids.AssetID
		}{{"normal", region.Normal}, {"emission", region.Emission}} {
			if channel.id.IsZero() {
				continue
			}
			target, ok := r.textureInfo(channel.id)
			compatible := target.semantic == asset.TextureNormal
			if channel.name == "emission" {
				compatible = target.semantic == asset.TextureEmissionRGB || target.semantic == asset.TextureEmissionMask
			}
			switch {
			case !ok:
				out = append(out, diag(CodeMaterial, problems.Error, "paired "+channel.name+" reference is unresolved: "+channel.id.String(), e.Source))
			case !aok || !compatible || albedo.w != target.w || albedo.h != target.h:
				out = append(out, diag(CodeMaterial, problems.Error, "paired "+channel.name+" needs compatible semantics and the same logical canvas: "+channel.id.String(), e.Source))
			default:
				if err := r.verifyCurrentTexture(e.ID); err != nil {
					out = append(out, diag(CodeMaterial, problems.Error, channel.name+" association is not current: "+err.Error(), e.Source))
				}
			}
		}

	})
	return out
}
