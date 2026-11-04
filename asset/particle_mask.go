package asset

import (
	"aego/core/ids"
	"aego/core/mathx"
)

type ParticleMaskSource interface {
	ParticleMask(ids.AssetID, uint8) ([]mathx.Vec2, bool)
}

// PrepareParticleMask reads prepared pixels only. Positions are centered on the
// original logical canvas/pivot; atlas padding and packed page coordinates never escape.
func PrepareParticleMask(id ids.AssetID, threshold uint8, read func(ids.AssetID, ArtifactKind) ([]byte, bool)) ([]mathx.Vec2, bool) {
	var r *RegionArtifact
	page := id
	if data, ok := read(id, ArtSpriteRegion); ok {
		var err error
		r, err = DecodeRegion(data)
		if err != nil {
			return nil, false
		}
		page = r.Page
	}
	var t *TextureArtifact
	if b, ok := BuiltinTexture(page); ok {
		t = &TextureArtifact{Width: b.Width, Height: b.Height, Pixels: b.Pixels}
	} else {
		data, ok := read(page, ArtTexture)
		if !ok {
			data, ok = read(page, ArtAtlasPage)
		}
		if !ok {
			return nil, false
		}
		var err error
		t, err = DecodeTexture(data)
		if err != nil {
			return nil, false
		}
	}
	switch t.Semantic {
	case TextureColor, TextureData, TextureMask, TextureHeight:
	default:
		return nil, false
	}
	x, y, w, h := 0, 0, t.Width, t.Height
	lw, lh, dx, dy := w, h, 0, 0
	pivot := [2]float32{.5, .5}
	if r != nil {
		if r.PageW != t.Width || r.PageH != t.Height {
			return nil, false
		}
		x, y, w, h = r.X, r.Y, r.W, r.H
		lw, lh = w, h
		if r.SourceW != 0 {
			lw, lh, dx, dy = r.SourceW, r.SourceH, r.TrimX, r.TrimY
		}
		if r.HasPivot {
			pivot = r.Pivot
		}
	}
	if w <= 0 || h <= 0 || w > (1<<22)/h {
		return nil, false
	}
	n := 0
	for j := 0; j < h; j++ {
		row := t.Pixels[((y+j)*t.Width+x)*4:]
		for i := 0; i < w; i++ {
			a := row[i*4+3]
			if a != 0 && a >= threshold {
				n++
			}
		}
	}
	out := make([]mathx.Vec2, 0, n)
	for j := 0; j < h; j++ {
		row := t.Pixels[((y+j)*t.Width+x)*4:]
		for i := 0; i < w; i++ {
			a := row[i*4+3]
			if a != 0 && a >= threshold {
				out = append(out, mathx.V2((float32(i+dx)+.5)/float32(lw)-pivot[0], (float32(j+dy)+.5)/float32(lh)-pivot[1]))
			}
		}
	}
	return out, true
}
