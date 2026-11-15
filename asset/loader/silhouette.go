package loader

import (
	"aego/asset"
	"aego/asset/silhouette"
	"aego/core/ids"
	"errors"
)

// SpriteShadow reads prepared artifacts, never source PNGs or GPU readbacks.
// The cache is invalidated on loader-generation changes, including atlas repacks.
func (l *Loader) SpriteShadow(id ids.AssetID) (silhouette.Shape, bool) {
	if l.shadowFrames == nil {
		l.shadowFrames = make(map[ids.AssetID]silhouette.Shape)
	}
	if s, ok := l.shadowFrames[id]; ok {
		return s, s.W > 0
	}
	s, err := l.readShadowFrame(id)
	if err != nil {
		s = silhouette.Shape{}
	}
	l.shadowFrames[id] = s
	return s, err == nil
}
func (l *Loader) readShadowFrame(id ids.AssetID) (silhouette.Shape, error) {
	read := func(id ids.AssetID, k asset.ArtifactKind) ([]byte, error) {
		e, ok := l.entry(id)
		if !ok {
			return nil, errors.New("missing shadow asset")
		}
		a, ok := e.Artifact(k)
		if !ok {
			return nil, errors.New("missing shadow artifact")
		}
		return l.read(a)
	}
	var reg *asset.RegionArtifact
	page := id
	if e, ok := l.entry(id); ok && (e.Kind == asset.ArtSpriteRegion || e.Kind == asset.ArtTexture && hasSpriteRegion(e)) {
		b, err := read(id, asset.ArtSpriteRegion)
		if err != nil {
			return silhouette.Shape{}, err
		}
		reg, err = asset.DecodeRegion(b)
		if err != nil {
			return silhouette.Shape{}, err
		}
		page = reg.Page
	}
	var t asset.TextureArtifact
	if d, ok := asset.BuiltinTexture(page); ok {
		t.Width, t.Height, t.Pixels = d.Width, d.Height, d.Pixels
	} else {
		e, ok := l.entry(page)
		if !ok || (e.Kind != asset.ArtTexture && e.Kind != asset.ArtAtlasPage) {
			return silhouette.Shape{}, errors.New("missing shadow texture")
		}
		b, err := read(page, e.Kind)
		if err != nil {
			return silhouette.Shape{}, err
		}
		d, err := asset.DecodeTexture(b)
		if err != nil {
			return silhouette.Shape{}, err
		}
		t = *d
	}
	cut := silhouette.Rect{W: t.Width, H: t.Height}
	cw, ch, dx, dy := cut.W, cut.H, 0, 0
	if reg != nil {
		if reg.PageW != t.Width || reg.PageH != t.Height {
			return silhouette.Shape{}, errors.New("shadow atlas revision mismatch")
		}
		cut = silhouette.Rect{X: reg.X, Y: reg.Y, W: reg.W, H: reg.H}
		cw, ch = cut.W, cut.H
		if reg.SourceW > 0 && reg.SourceH > 0 {
			cw, ch, dx, dy = reg.SourceW, reg.SourceH, reg.TrimX, reg.TrimY
		}
	}
	return silhouette.Trace(t.Pixels, t.Width, t.Height, cut, cw, ch, dx, dy)
}

func hasSpriteRegion(e *asset.Entry) bool {
	_, ok := e.Artifact(asset.ArtSpriteRegion)
	return ok
}
