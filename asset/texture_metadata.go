package asset

import "aego/core/ids"

// ReadTextureMetadata inspects prepared texture or region bytes. It doesn't
// require a GPU and uses the same import semantics for games and authoring tools.
func ReadTextureMetadata(id ids.AssetID, entry func(ids.AssetID) (*Entry, bool), read func(Artifact) ([]byte, error)) (TextureMetadata, bool) {
	if tex, ok := BuiltinTexture(id); ok {
		semantic := TextureColor
		if tex.NonColor {
			semantic = TextureData
		}
		return TextureMetadata{Width: tex.Width, Height: tex.Height, Semantic: semantic}, true
	}
	e, ok := entry(id)
	if !ok {
		return TextureMetadata{}, false
	}
	page := e
	var region *RegionArtifact
	art, hasRegion := e.Artifact(ArtSpriteRegion)
	if e.Kind == ArtSpriteRegion && !hasRegion {
		return TextureMetadata{}, false
	}
	if hasRegion && (e.Kind == ArtSpriteRegion || e.Kind == ArtTexture) {
		data, err := read(art)
		if err != nil {
			return TextureMetadata{}, false
		}
		region, err = DecodeRegion(data)
		if err != nil || (e.Kind == ArtSpriteRegion && region.Page == id) {
			return TextureMetadata{}, false
		}
		if region.Page != id {
			page, ok = entry(region.Page)
			if !ok {
				return TextureMetadata{}, false
			}
		}
	}
	if page.Kind != ArtTexture && page.Kind != ArtAtlasPage {
		return TextureMetadata{}, false
	}
	art, found := page.Artifact(page.Kind)
	if !found {
		return TextureMetadata{}, false
	}
	data, err := read(art)
	if err != nil {
		return TextureMetadata{}, false
	}
	tex, err := DecodeTexture(data)
	if err != nil {
		return TextureMetadata{}, false
	}
	info := TextureMetadata{Semantic: tex.Semantic, Width: tex.Width, Height: tex.Height}
	if region != nil {
		if region.PageW != tex.Width || region.PageH != tex.Height {
			return TextureMetadata{}, false
		}
		info.Region = e.Kind == ArtSpriteRegion || region.Page != id || region.X != 0 || region.Y != 0 || region.W != tex.Width || region.H != tex.Height ||
			(region.SourceW > 0 && (region.SourceW != region.W || region.SourceH != region.H || region.TrimX != 0 || region.TrimY != 0))
		info.Width, info.Height = region.W, region.H
		if region.SourceW != 0 {
			info.Width, info.Height = region.SourceW, region.SourceH
		}
	}
	return info, true
}
