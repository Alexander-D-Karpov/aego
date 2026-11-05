package loader

import (
	"aego/asset"
	"aego/core/ids"
	"aego/render/texture"
)

// EmissionFor reads prepared AERG associations, not source names or .meta; its warmed path uses Texture's region/page caches without allocations.
// Missing references retain their ID for editor diagnostics and repair.
func (l *Loader) EmissionFor(albedo ids.AssetID) (ids.AssetID, texture.Emission, bool) {
	if l.failed[albedo] {
		return l.emissions[albedo], texture.Emission{}, false
	}
	id, cached := l.emissions[albedo]
	if !cached {
		e, exists := l.entry(albedo)
		if !exists {
			return ids.AssetID{}, texture.Emission{}, false
		}
		if art, found := e.Artifact(asset.ArtSpriteRegion); found {
			data, err := l.read(art)
			if err != nil {
				l.fail(albedo, "cannot read emission association", err)
				return ids.AssetID{}, texture.Emission{}, false
			}
			region, err := asset.DecodeRegion(data)
			if err != nil {
				l.fail(albedo, "malformed emission association", err)
				return ids.AssetID{}, texture.Emission{}, false
			}
			id = region.Emission
		}
		l.emissions[albedo] = id
	}
	if id.IsZero() || id == albedo || l.failed[id] {
		return id, texture.Emission{}, false
	}
	region, ok := l.Texture(id)
	if !ok {
		return id, texture.Emission{}, false
	}
	page := l.pages[l.region[id].page]
	if emissionMode(page.semantic) == texture.EmissionNone {
		l.fail(id, "emission association must resolve to an imported emission RGB or mask texture", nil)
		return id, texture.Emission{}, false
	}
	albedoRegion, ok := l.Texture(albedo)
	if !ok || albedoRegion.Size() != region.Size() {
		l.fail(id, "emission source canvas does not match the albedo source canvas", nil)
		return id, texture.Emission{}, false
	}
	return id, texture.Emission{Region: region, Mode: emissionMode(page.semantic)}, true
}
