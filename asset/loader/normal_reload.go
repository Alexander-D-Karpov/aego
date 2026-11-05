package loader

import (
	"fmt"

	"aego/asset"
	"aego/core/ids"
)

type normalShape struct {
	w, h     int
	normal   ids.AssetID
	emission ids.AssetID
	semantic asset.TextureSemantic
}

// Albedo, normal and emission share a generation: incompatible partial batches or failed candidates leave all live channel GPU resources intact,
// even when a same-sized pixel update accompanies a rejected channel.
func (l *Loader) validateNormalSwap(entries []asset.Entry, prepared []staged) error {
	if len(l.normals) == 0 && len(l.emissions) == 0 {
		return nil
	}
	next := make(map[ids.AssetID]*asset.Entry, len(entries))
	pages := make(map[ids.AssetID]pageEntry)
	for i := range entries {
		next[entries[i].ID] = &entries[i]
	}
	for _, s := range prepared {
		if s.isTex {
			pages[s.id] = s.page
		}
	}
	entry := func(id ids.AssetID) (*asset.Entry, bool) {
		if e := next[id]; e != nil {
			return e, true
		}
		return l.entry(id)
	}
	pageInfo := func(id ids.AssetID) (pageEntry, bool, error) {
		if p, ok := pages[id]; ok {
			return p, true, nil
		}
		if p, ok := l.pages[id]; ok {
			return p, true, nil
		}
		e, ok := entry(id)
		if !ok {
			return pageEntry{}, false, nil
		}
		a, ok := e.Artifact(e.Kind)
		if !ok || (e.Kind != asset.ArtTexture && e.Kind != asset.ArtAtlasPage) {
			return pageEntry{}, false, nil
		}
		data, err := l.read(a)
		if err != nil {
			return pageEntry{}, false, err
		}
		t, err := asset.DecodeTexture(data)
		if err != nil {
			return pageEntry{}, false, err
		}
		return pageEntry{width: t.Width, height: t.Height, semantic: t.Semantic}, true, nil
	}
	shape := func(id ids.AssetID) (normalShape, bool, error) {
		e, exists := entry(id)
		if !exists {
			return normalShape{}, false, nil
		}
		if next[id] == nil {
			if cached, ok := l.region[id]; ok {
				p, ok, err := pageInfo(cached.page)
				if err != nil || !ok {
					return normalShape{}, false, err
				}
				if old, had := l.pages[cached.page]; had && (old.width != p.width || old.height != p.height) {
					return normalShape{}, false, fmt.Errorf("normal-related atlas page %s resized without a new region mapping for %s", cached.page, id)
				}
				size := cached.region.Size()
				return normalShape{w: int(size.X), h: int(size.Y), normal: l.normals[id], emission: l.emissions[id], semantic: p.semantic}, true, nil
			}
		}
		if a, ok := e.Artifact(asset.ArtSpriteRegion); ok {
			data, err := l.read(a)
			if err != nil {
				return normalShape{}, false, err
			}
			r, err := asset.DecodeRegion(data)
			if err != nil {
				return normalShape{}, false, err
			}
			p, ok, err := pageInfo(r.Page)
			if err != nil || !ok {
				return normalShape{}, false, err
			}
			if p.width != r.PageW || p.height != r.PageH {
				return normalShape{}, false, fmt.Errorf("normal-related region %s and page %s have different dimensions", id, r.Page)
			}
			w, h := r.W, r.H
			if r.SourceW != 0 {
				w, h = r.SourceW, r.SourceH
			}
			return normalShape{w: w, h: h, normal: r.Normal, emission: r.Emission, semantic: p.semantic}, true, nil
		}
		p, ok, err := pageInfo(id)
		if err != nil || !ok {
			return normalShape{}, false, err
		}
		return normalShape{w: p.width, h: p.height, semantic: p.semantic}, true, nil
	}

	active := make(map[ids.AssetID]bool, len(l.normals)+len(l.emissions))
	for id := range l.normals {
		active[id] = true
	}
	for id := range l.emissions {
		active[id] = true
	}
	rejected := func(id ids.AssetID) bool {
		if l.rejected[id] && next[id] == nil {
			return true
		}
		if e, ok := entry(id); ok {
			for _, dep := range e.Deps {
				if l.rejected[dep] && next[dep] == nil {
					return true
				}
			}
		}
		return false
	}
	for albedo := range active {
		aRegion := l.region[albedo]
		nID, eID := l.normals[albedo], l.emissions[albedo]
		nRegion, eRegion := l.region[nID], l.region[eID]
		if next[albedo] == nil && next[aRegion.page] == nil && next[nID] == nil && next[nRegion.page] == nil && next[eID] == nil && next[eRegion.page] == nil {
			continue
		}
		a, ok, err := shape(albedo)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if rejected(albedo) || rejected(a.normal) || rejected(a.emission) {
			return fmt.Errorf("surface %s has a failed current channel generation; retain the entire last-good surface until its sources are repaired", albedo)
		}
		for _, channel := range []struct {
			name string
			id   ids.AssetID
		}{{"normal", a.normal}, {"emission", a.emission}} {
			if channel.id.IsZero() {
				continue
			}
			target, ok, err := shape(channel.id)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			compatible := target.semantic == asset.TextureNormal
			if channel.name == "emission" {
				compatible = target.semantic == asset.TextureEmissionRGB || target.semantic == asset.TextureEmissionMask
			}
			if !compatible || a.w != target.w || a.h != target.h {
				return fmt.Errorf("%s %s is incompatible with albedo %s; publish matching role and logical canvases together", channel.name, channel.id, albedo)
			}
		}
	}
	return nil
}
