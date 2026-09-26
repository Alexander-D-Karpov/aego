package atlas

import (
	"sort"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/gpu"
	rtex "aego/render/texture"
)

const (
	GroupVersion = 1
	MaxPage      = 2048
	MinPage      = 64
)

type placement struct {
	id     ids.AssetID
	data   *rtex.Data
	x, y   int
	w, h   int
	filter gpu.FilterMode
}

type shelf struct {
	y      int
	height int
	cursor int
}

type Importer struct {
	pipeline.Importer
}

func Wrap(base pipeline.Importer) *Importer {
	return &Importer{Importer: base}
}

func (*Importer) GroupVersion() int {
	return GroupVersion
}

func (a *Importer) Finalize(ctx *pipeline.GroupContext, members []pipeline.GroupMember) (map[ids.AssetID]*pipeline.Result, []pipeline.Produced, error) {
	padding, extrude := settingsOf(members)
	placements, width, height, err := pack(members, padding)
	if err != nil {
		return nil, nil, err
	}
	if len(placements) == 0 {
		return nil, nil, nil
	}

	page := rtex.New(width, height, gpu.TexSRGBA8)
	page.Premultiplied = true
	filter := gpu.FilterNearest
	for i := range placements {
		p := &placements[i]
		blit(page, p.data, p.x, p.y)
		if extrude {
			extrudeEdges(page, p.x, p.y, p.w, p.h, padding)
		}
		filter = p.filter
	}

	pageData := asset.EncodeTexture(&asset.TextureArtifact{
		Width:  width,
		Height: height,
		Format: gpu.TexSRGBA8,
		Filter: filter,
		Pixels: page.Pixels,
	})

	perMember := make(map[ids.AssetID]*pipeline.Result, len(placements))
	for i := range placements {
		p := &placements[i]
		region := asset.EncodeRegion(&asset.RegionArtifact{
			Page:   ctx.PageID,
			X:      p.x,
			Y:      p.y,
			W:      p.w,
			H:      p.h,
			PageW:  width,
			PageH:  height,
			Filter: p.filter,
		})
		perMember[p.id] = &pipeline.Result{
			Kind:      asset.ArtSpriteRegion,
			Artifacts: []pipeline.Produced{{Kind: asset.ArtSpriteRegion, Data: region}},
		}
	}

	ctx.Log.Debug("atlas packed",
		log.F("group", ctx.Group),
		log.F("members", len(placements)),
		log.F("w", width), log.F("h", height))

	return perMember, []pipeline.Produced{{Kind: asset.ArtAtlasPage, Data: pageData}}, nil
}

func settingsOf(members []pipeline.GroupMember) (padding int, extrude bool) {
	padding, extrude = 2, true
	for _, m := range members {
		if m.Settings == nil {
			continue
		}
		padding = m.Settings.Int("padding", padding)
		extrude = m.Settings.Bool("extrude", extrude)
		break
	}
	if padding < 0 {
		padding = 0
	}
	return padding, extrude
}

func pack(members []pipeline.GroupMember, padding int) ([]placement, int, int, error) {
	items := make([]placement, 0, len(members))
	for _, m := range members {
		if m.Result == nil || m.Result.Raw == nil {
			continue
		}
		d := m.Result.Raw
		items = append(items, placement{
			id:     m.ID,
			data:   d,
			w:      d.Width,
			h:      d.Height,
			filter: gpu.FilterMode(m.Result.RawFilter),
		})
	}
	if len(items) == 0 {
		return nil, 0, 0, nil
	}
	sort.Slice(items, func(a, b int) bool {
		if items[a].h != items[b].h {
			return items[a].h > items[b].h
		}
		if items[a].w != items[b].w {
			return items[a].w > items[b].w
		}
		return items[a].id.String() < items[b].id.String()
	})

	width := MinPage
	for _, it := range items {
		for width < it.w+padding*2 && width < MaxPage {
			width *= 2
		}
	}
	for {
		placed, height, ok := packAt(items, width, padding)
		if ok {
			return placed, width, height, nil
		}
		if width >= MaxPage {
			return nil, 0, 0, errs.New(errs.AssetImportFailed,
				"atlas group does not fit into a single page",
				log.F("members", len(items)), log.F("max", MaxPage))
		}
		width *= 2
	}
}

func packAt(items []placement, width, padding int) ([]placement, int, bool) {
	out := make([]placement, len(items))
	copy(out, items)
	var shelves []shelf
	maxY := 0
	for i := range out {
		w := out[i].w + padding*2
		h := out[i].h + padding*2
		if w > width {
			return nil, 0, false
		}
		placed := false
		for s := range shelves {
			if shelves[s].height >= h && shelves[s].cursor+w <= width {
				out[i].x = shelves[s].cursor + padding
				out[i].y = shelves[s].y + padding
				shelves[s].cursor += w
				placed = true
				break
			}
		}
		if placed {
			continue
		}
		if maxY+h > MaxPage {
			return nil, 0, false
		}
		shelves = append(shelves, shelf{y: maxY, height: h, cursor: w})
		out[i].x = padding
		out[i].y = maxY + padding
		maxY += h
	}
	height := MinPage
	for height < maxY {
		height *= 2
	}
	if height > MaxPage {
		return nil, 0, false
	}
	return out, height, true
}

func blit(dst, src *rtex.Data, ox, oy int) {
	for y := 0; y < src.Height; y++ {
		di := ((oy+y)*dst.Width + ox) * 4
		si := y * src.Width * 4
		copy(dst.Pixels[di:di+src.Width*4], src.Pixels[si:si+src.Width*4])
	}
}

func extrudeEdges(d *rtex.Data, x, y, w, h, padding int) {
	for p := 1; p <= padding; p++ {
		for i := 0; i < w; i++ {
			copyPixel(d, x+i, y-p, x+i, y)
			copyPixel(d, x+i, y+h-1+p, x+i, y+h-1)
		}
		for i := 0; i < h; i++ {
			copyPixel(d, x-p, y+i, x, y+i)
			copyPixel(d, x+w-1+p, y+i, x+w-1, y+i)
		}
		copyPixel(d, x-p, y-p, x, y)
		copyPixel(d, x+w-1+p, y-p, x+w-1, y)
		copyPixel(d, x-p, y+h-1+p, x, y+h-1)
		copyPixel(d, x+w-1+p, y+h-1+p, x+w-1, y+h-1)
	}
}

func copyPixel(d *rtex.Data, dx, dy, sx, sy int) {
	if dx < 0 || dy < 0 || dx >= d.Width || dy >= d.Height {
		return
	}
	di := (dy*d.Width + dx) * 4
	si := (sy*d.Width + sx) * 4
	copy(d.Pixels[di:di+4], d.Pixels[si:si+4])
}
