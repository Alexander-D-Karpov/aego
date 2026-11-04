package loader

import (
	"aego/asset"
	"aego/core/ids"
	"aego/render/texture"
)

// NormalFor reads only prepared AERG associations, never source names or .meta.
// Its warmed path uses the same region/page caches as Texture and allocates no
// memory. Missing references keep their ID for editor diagnostics and repair.
func (l *Loader) NormalFor(albedo ids.AssetID) (ids.AssetID, texture.Region, bool) {
	if l.failed[albedo] {
		return l.normals[albedo], texture.Region{}, false
	}
	id, cached := l.normals[albedo]
	if !cached {
		e, exists := l.entry(albedo)
		if !exists {
			return ids.AssetID{}, texture.Region{}, false
		}
		if art, found := e.Artifact(asset.ArtSpriteRegion); found {
			data, err := l.read(art)
			if err != nil {
				l.fail(albedo, "cannot read normal association", err)
				return ids.AssetID{}, texture.Region{}, false
			}
			region, err := asset.DecodeRegion(data)
			if err != nil {
				l.fail(albedo, "malformed normal association", err)
				return ids.AssetID{}, texture.Region{}, false
			}
			id = region.Normal
		}
		l.normals[albedo] = id
	}
	if id.IsZero() || id == albedo || l.failed[id] {
		return id, texture.Region{}, false
	}
	region, ok := l.Texture(id)
	if !ok {
		return id, texture.Region{}, false
	}
	page := l.pages[l.region[id].page]
	if page.semantic != asset.TextureNormal {
		l.fail(id, "normal association must resolve to an imported linear normal texture", nil)
		return id, texture.Region{}, false
	}
	albedoRegion, ok := l.Texture(albedo)
	if !ok || albedoRegion.Size() != region.Size() {
		l.fail(id, "normal source canvas does not match the albedo source canvas", nil)
		return id, texture.Region{}, false
	}
	return id, region, true
}
