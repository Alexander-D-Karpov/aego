package loader

import (
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
)

type maskResult struct {
	id        ids.AssetID
	threshold uint8
	points    []mathx.Vec2
	ok        bool
}
type maskCache struct {
	next  int
	slots [4]maskResult
}

func (l *Loader) ParticleMask(id ids.AssetID, threshold uint8) ([]mathx.Vec2, bool) {
	for _, s := range l.masks.slots {
		if !id.IsZero() && s.id == id && s.threshold == threshold {
			return s.points, s.ok
		}
	}
	list, ok := asset.PrepareParticleMask(id, threshold, func(id ids.AssetID, k asset.ArtifactKind) ([]byte, bool) {
		e, ok := l.entry(id)
		if !ok {
			return nil, false
		}
		a, ok := e.Artifact(k)
		if !ok {
			return nil, false
		}
		d, err := l.read(a)
		return d, err == nil
	})
	l.masks.slots[l.masks.next] = maskResult{id, threshold, list, ok}
	l.masks.next = (l.masks.next + 1) % len(l.masks.slots)
	return list, ok
}
