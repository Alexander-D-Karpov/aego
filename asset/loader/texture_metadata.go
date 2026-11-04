package loader

import (
	"aego/asset"
	"aego/core/ids"
	"aego/gpu"
)

// TextureMetadata inspects the loaded generation's prepared artifacts. It
// creates no GPU resources and never consults source paths or import settings.
func (l *Loader) TextureMetadata(id ids.AssetID) (asset.TextureMetadata, bool) {
	if device, ok := l.dev.(gpu.ContextState); ok {
		if generation := device.ContextGeneration(); generation != l.metadataDeviceGen {
			clear(l.textureMetadata)
			l.metadataDeviceGen = generation
		}
	}
	if info, ok := l.textureMetadata[id]; ok {
		return info, true
	}
	info, ok := asset.ReadTextureMetadata(id, l.entry, l.read)
	if ok {
		if l.textureMetadata == nil {
			l.textureMetadata = make(map[ids.AssetID]asset.TextureMetadata)
		}
		l.textureMetadata[id] = info
	}
	return info, ok
}
