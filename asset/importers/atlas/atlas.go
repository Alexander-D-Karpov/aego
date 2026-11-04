package atlas

import (
	"sort"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/format/aetext"
	"aego/gpu"
	rtex "aego/render/texture"
)

const (
	GroupVersion = 4
	MaxPage      = 2048
	MinPage      = 64
)

type placement struct {
	pivot                          [2]float32
	hasPivot                       bool
	sourceW, sourceH, trimX, trimY int
	id                             ids.AssetID
	normal                         ids.AssetID
	emission                       ids.AssetID
	deps                           []ids.AssetID
	data                           *rtex.Data
	x, y                           int
	w, h                           int
	filter                         gpu.FilterMode
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

func (a *Importer) CacheSalt(ctx *pipeline.ImportContext) ([]byte, error) {
	if imp, ok := a.Importer.(pipeline.CacheSalter); ok {
		return imp.CacheSalt(ctx)
	}
	return nil, nil
}

func (a *Importer) SourceDependencies() bool {
	imp, ok := a.Importer.(pipeline.SourceDependencySalter)
	return ok && imp.SourceDependencies()
}

func (a *Importer) ResolveMetadata(ctx *pipeline.ImportContext) (pipeline.Settings, error) {
	if imp, ok := a.Importer.(pipeline.MetadataResolver); ok {
		return imp.ResolveMetadata(ctx)
	}
	return nil, nil
}

func (a *Importer) Finalize(ctx *pipeline.GroupContext, members []pipeline.GroupMember) (map[ids.AssetID]*pipeline.Result, []pipeline.Produced, error) {
	padding, extrude := settingsOf(members)
	semantic := asset.TextureColor
	var filter gpu.FilterMode
	first := true
	for _, m := range members {
		if m.Result == nil || m.Result.Raw == nil {
			continue
		}
		if first {
			semantic, filter, first = m.Result.RawSemantic, gpu.FilterMode(m.Result.RawFilter), false
		}
		if semantic != m.Result.RawSemantic {
			return nil, nil, errs.New(errs.AssetImportFailed, "atlas group mixes incompatible texture semantics")
		}
		if gpu.FilterMode(m.Result.RawFilter) != filter || m.Settings.Int("padding", 2) != padding || m.Settings.Bool("extrude", true) != extrude {
			return nil, nil, errs.New(errs.AssetImportFailed, "atlas group requires matching filter, padding and extrusion settings")
		}
		if m.Settings.Bool("mips", false) {
			return nil, nil, errs.New(errs.AssetImportFailed, "atlas mipmaps require per-region mip isolation; use mips=false")
		}
	}
	placements, width, height, err := pack(members, padding)
	if err != nil {
		return nil, nil, err
	}
	if len(placements) == 0 {
		return nil, nil, nil
	}

	format := placements[0].data.Format
	if semantic == asset.TextureColor {
		format = gpu.TexSRGBA8
	}
	for _, p := range placements {
		if semantic != asset.TextureColor && p.data.Format != format {
			return nil, nil, errs.New(errs.AssetImportFailed, "atlas group mixes linear and sRGB pixel formats")
		}
	}
	page := rtex.New(width, height, format)
	page.NonColor = semantic != asset.TextureColor
	page.Premultiplied = !page.NonColor
	if semantic == asset.TextureNormal {
		for i := 0; i < len(page.Pixels); i += 4 {
			page.Pixels[i], page.Pixels[i+1], page.Pixels[i+2] = 128, 128, 255
		}
	}
	for i := range placements {
		p := &placements[i]
		blit(page, p.data, p.x, p.y)
		if extrude {
			extrudeEdges(page, p.x, p.y, p.w, p.h, padding)
		}
		filter = p.filter
	}

	pageData := asset.EncodeTexture(&asset.TextureArtifact{
		Width:    width,
		Height:   height,
		Format:   page.Format,
		Semantic: semantic,
		Filter:   filter,
		Pixels:   page.Pixels,
	})

	perMember := make(map[ids.AssetID]*pipeline.Result, len(placements))
	for i := range placements {
		p := &placements[i]
		region := asset.EncodeRegion(&asset.RegionArtifact{
			SourceW: p.sourceW, SourceH: p.sourceH, TrimX: p.trimX, TrimY: p.trimY,
			Page: ctx.PageID, Pivot: p.pivot, HasPivot: p.hasPivot,
			Normal:   p.normal,
			Emission: p.emission,
			X:        p.x,
			Y:        p.y,
			W:        p.w,
			H:        p.h,
			PageW:    width,
			PageH:    height,
			Filter:   p.filter,
		})
		perMember[p.id] = &pipeline.Result{
			Kind:      asset.ArtSpriteRegion,
			Deps:      p.deps,
			Artifacts: []pipeline.Produced{{Kind: asset.ArtSpriteRegion, Data: region}},
		}
	}

	if ctx.Log != nil {
		ctx.Log.Debug("atlas packed",
			log.F("group", ctx.Group),
			log.F("members", len(placements)),
			log.F("w", width), log.F("h", height))
	}

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
		if (d.Format != gpu.TexRGBA8 && d.Format != gpu.TexSRGBA8) || d.NonColor != (m.Result.RawSemantic != asset.TextureColor) || (d.NonColor && m.Result.RawSemantic != asset.TextureEmissionRGB && d.Format != gpu.TexRGBA8) {
			return nil, 0, 0, errs.New(errs.AssetImportFailed, "atlas requires RGBA8 pixels with consistent color/data semantics")
		}
		for _, key := range []string{"rotate", "rotation", "allow_rotation"} {
			if v, ok := m.Settings[key]; ok && !((v.Kind == aetext.VBool && !v.Bool) || (v.Kind == aetext.VInt && v.Int == 0)) {
				return nil, 0, 0, errs.New(errs.AssetImportFailed, "atlas rotation is not supported", log.F("setting", key))
			}
		}
		sw, sh, tx, ty := 0, 0, 0, 0
		if m.Settings.Bool("trim", false) {
			if d.Width > 65535 || d.Height > 65535 {
				return nil, 0, 0, errs.New(errs.AssetImportFailed, "trim source dimensions exceed sprite ABI")
			}
			sw, sh = d.Width, d.Height
			d, tx, ty = trim(d)
		}
		items = append(items, placement{
			sourceW: sw, sourceH: sh, trimX: tx, trimY: ty,
			id:     m.ID,
			normal: m.Result.RawNormal, pivot: m.Result.RawPivot, hasPivot: m.Result.RawHasPivot,
			emission: m.Result.RawEmission,
			deps:     m.Result.Deps,
			data:     d,
			w:        d.Width,
			h:        d.Height,
			filter:   gpu.FilterMode(m.Result.RawFilter),
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
		if src.Format == dst.Format {
			copy(dst.Pixels[di:di+src.Width*4], src.Pixels[si:si+src.Width*4])
			continue
		}
		for x := 0; x < src.Width; x++ {
			for c := 0; c < 3; c++ {
				v := float32(src.Pixels[si+x*4+c]) / 255
				if src.Format == gpu.TexSRGBA8 {
					v = mathx.SRGBToLinear(v)
				}
				if dst.Format == gpu.TexSRGBA8 {
					v = mathx.LinearToSRGB(v)
				}
				dst.Pixels[di+x*4+c] = byte(mathx.Clamp(v, 0, 1)*255 + .5)
			}
			dst.Pixels[di+x*4+3] = src.Pixels[si+x*4+3]
		}
	}
}

func extrudeEdges(d *rtex.Data, x, y, w, h, padding int) {
	for dy := y - padding; dy < y+h+padding; dy++ {
		for dx := x - padding; dx < x+w+padding; dx++ {
			if dx < x || dx >= x+w || dy < y || dy >= y+h {
				copyPixel(d, dx, dy, min(max(dx, x), x+w-1), min(max(dy, y), y+h-1))
			}
		}
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

// trim crops only storage/geometry. Original canvas dimensions and offset live
// in the region artifact, so anchors, explicit Size and local UV stay stable.
func trim(src *rtex.Data) (*rtex.Data, int, int) {
	x0, y0, x1, y1 := src.Width, src.Height, 0, 0
	for y := 0; y < src.Height; y++ {
		for x := 0; x < src.Width; x++ {
			if src.Pixels[(y*src.Width+x)*4+3] != 0 {
				x0 = min(x0, x)
				y0 = min(y0, y)
				x1 = max(x1, x+1)
				y1 = max(y1, y+1)
			}
		}
	}
	if x1 == 0 || y1 == 0 {
		x0, y0, x1, y1 = 0, 0, 1, 1
	}
	if x0 == 0 && y0 == 0 && x1 == src.Width && y1 == src.Height {
		return src, 0, 0
	}
	dst := rtex.New(x1-x0, y1-y0, src.Format)
	dst.Premultiplied = src.Premultiplied
	dst.NonColor = src.NonColor
	for y := 0; y < dst.Height; y++ {
		a, b := y*dst.Width*4, ((y+y0)*src.Width+x0)*4
		copy(dst.Pixels[a:a+dst.Width*4], src.Pixels[b:b+dst.Width*4])
	}
	return dst, x0, y0
}
