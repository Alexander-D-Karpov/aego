package art

import (
	"image"
	"slices"
)

const previewCacheBytes int64 = 32 << 20
const previewCacheEntries = 4

type previewKey struct {
	frame   ID
	options RenderOptions
}
type previewEntry struct {
	key         previewKey
	canvas      Canvas
	palette     []Swatch
	layers      []Layer
	cels        []Cel
	images      map[ID]*pixels
	image       *image.NRGBA
	draftDamage image.Rectangle
	bytes       int64
}
type previewCache struct {
	entries []*previewEntry
	// Diagnostic counters are deliberately private, not a second document API.
	composites       uint64
	compositedPixels uint64
}

func celArea(c Cel, p *pixels) image.Rectangle {
	return image.Rect(c.X, c.Y, c.X+p.Width, c.Y+p.Height)
}
func (p *previewEntry) damage(s *state, cels []Cel) image.Rectangle {
	bounds := image.Rect(0, 0, s.Canvas.Width, s.Canvas.Height)
	if p.canvas != s.Canvas || !slices.Equal(p.layers, s.Layers) || !slices.Equal(p.palette, s.Palette) {
		return bounds
	}
	old := make(map[ID]Cel, len(p.cels))
	for _, c := range p.cels {
		old[c.ID] = c
	}
	var damage image.Rectangle
	for _, c := range cels {
		was, ok := old[c.ID]
		delete(old, c.ID)
		im := s.Images[c.Image]
		if !ok {
			damage = damage.Union(celArea(c, im))
			continue
		}
		before := p.images[was.Image]
		if c != was || before.Width != im.Width || before.Height != im.Height {
			damage = damage.Union(celArea(was, before)).Union(celArea(c, im))
			continue
		}
		for key, block := range before.Tiles {
			if block != im.Tiles[key] {
				damage = damage.Union(image.Rect(c.X+key.X*TileSize, c.Y+key.Y*TileSize, c.X+(key.X+1)*TileSize, c.Y+(key.Y+1)*TileSize))
			}
		}
		for key, block := range im.Tiles {
			if block != before.Tiles[key] {
				damage = damage.Union(image.Rect(c.X+key.X*TileSize, c.Y+key.Y*TileSize, c.X+(key.X+1)*TileSize, c.Y+(key.Y+1)*TileSize))
			}
		}
	}
	for _, c := range old {
		damage = damage.Union(celArea(c, p.images[c.Image]))
	}
	return damage.Intersect(bounds)
}

// render retains only bounded per-frame presentation data and tile references,
// never a whole document state or history. Pixel composition remains the same
// renderRegion used by export. Returned images belong to callers: onion skin or
// UI code cannot mutate a cached frame or canonical pixels.
func (cache *previewCache) render(s *state, frame ID, opts RenderOptions, draftDamage image.Rectangle) (*image.NRGBA, error) {
	opts = options(opts)
	key := previewKey{frame, opts}
	bounds := image.Rect(0, 0, s.Canvas.Width, s.Canvas.Height)
	var cels []Cel
	images := make(map[ID]*pixels)
	for _, c := range s.Cels {
		if c.Frame == frame && c.Channel == opts.Channel {
			cels = append(cels, c)
			images[c.Image] = s.Images[c.Image]
		}
	}
	cost := int64(bounds.Dx()*bounds.Dy()*4 + len(cels)*128 + len(s.Layers)*192 + len(s.Palette)*64)
	for _, im := range images {
		cost += int64(128 + len(im.Tiles)*(TileSize*TileSize*4+64))
	}
	var entry *previewEntry
	for i, item := range cache.entries {
		if item.key == key {
			entry = item
			cache.entries = append(cache.entries[:i], cache.entries[i+1:]...)
			break
		}
	}
	if cost > previewCacheBytes {
		im := image.NewNRGBA(bounds)
		err := renderRegion(s, frame, im, bounds, opts)
		return im, err
	}
	damage := bounds
	if entry != nil && entry.image.Bounds() == bounds {
		damage = entry.damage(s, cels).Union(entry.draftDamage).Union(draftDamage).Intersect(bounds)
	} else {
		entry = &previewEntry{image: image.NewNRGBA(bounds)}
	}
	if !damage.Empty() {
		if err := renderRegion(s, frame, entry.image, damage, opts); err != nil {
			return nil, err
		}
		cache.composites++
		cache.compositedPixels += uint64(damage.Dx() * damage.Dy())
	} else if _, ok := s.frame(frame); !ok { // A deleted frame may never hit a stale cache.
		return nil, ErrConflict
	}
	entry.key, entry.canvas, entry.palette, entry.layers = key, s.Canvas, slices.Clone(s.Palette), slices.Clone(s.Layers)
	entry.cels, entry.images, entry.draftDamage, entry.bytes = cels, make(map[ID]*pixels, len(images)), draftDamage, cost
	for id, im := range images {
		entry.images[id] = im.clone()
	}
	// A draft may modify an already-owned tile in place. Explicit draft damage
	// (including the prior shape extent) invalidates it even if pointers match.
	cache.entries = append(cache.entries, entry)
	for {
		var total int64
		for _, v := range cache.entries {
			total += v.bytes
		}
		if len(cache.entries) <= previewCacheEntries && total <= previewCacheBytes {
			break
		}
		cache.entries[0] = nil
		cache.entries = cache.entries[1:]
	}
	out := image.NewNRGBA(bounds)
	copy(out.Pix, entry.image.Pix)
	return out, nil
}

func (s *Session) previewImage(frame ID, opts RenderOptions) (*image.NRGBA, error) {
	d := s.Document
	d.mu.Lock()
	closed, model := d.closed, d.current
	d.mu.Unlock()
	if closed {
		s.preview = previewCache{}
		return nil, ErrClosed
	}
	var damage image.Rectangle
	if s.draft != nil && frame == s.Frame {
		if err := s.draft.check(); err != nil {
			return nil, err
		}
		model = s.draft.work
		damage = s.draft.DirtyRect()
		if s.draft.structural {
			damage = image.Rect(0, 0, model.Canvas.Width, model.Canvas.Height)
		}
	}
	return s.preview.render(model, frame, opts, damage)
}

// ClearPreview releases optional CPU presentation caches without touching the
// document, the active gesture or history. Context-loss callers may safely use it.
func (s *Session) ClearPreview() { s.preview = previewCache{} }
