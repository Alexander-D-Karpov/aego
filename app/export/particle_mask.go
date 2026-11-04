package export

import (
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
)

func (r *resolver) ParticleMask(id ids.AssetID, threshold uint8) ([]mathx.Vec2, bool) {
	return asset.PrepareParticleMask(id, threshold, func(id ids.AssetID, k asset.ArtifactKind) ([]byte, bool) {
		e, ok := r.s.idx.Lookup(id)
		if !ok || e.Missing || e.Error != "" {
			return nil, false
		}
		a, ok := e.Artifact(k)
		if !ok {
			return nil, false
		}
		data, err := r.s.files.ReadFile(a.Path)
		return data, err == nil
	})
}
