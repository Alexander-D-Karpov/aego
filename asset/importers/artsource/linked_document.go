package artsource

import (
	"fmt"
	"image"
	"strings"

	"aego/art"
	"aego/asset"
	texImporter "aego/asset/importers/texture"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/format/aetext"
)

// ImportTextureDocument copies unpacked source pixels into Art Core while
// retaining the original region IDs in a separate mapping. Callers must guard
// every source and metadata read while publishing the link.
func ImportTextureDocument(ctx *pipeline.ImportContext) (*art.Document, []TextureLinkFrame, error) {
	if ctx == nil || ctx.Meta == nil || ctx.ID.IsZero() || ctx.Meta.ID != ctx.ID {
		return nil, nil, fmt.Errorf("artsource: an indexed texture source is required")
	}
	if link, err := TextureLinkOf(ctx.Meta); err != nil || link != nil {
		if err == nil {
			err = fmt.Errorf("artsource: texture already has an authoring source")
		}
		return nil, nil, err
	}
	role, err := asset.TextureRoleOf(ctx.Path, ctx.Meta.Settings)
	if err != nil {
		return nil, nil, err
	}
	if role != asset.RoleColor {
		return nil, nil, fmt.Errorf("artsource: open the color sprite to edit its linked surface channels")
	}
	pixels, err := decodeEditableTexturePNG(ctx.Source)
	if err != nil {
		return nil, nil, err
	}
	frames := textureSourceFrames(ctx.Meta, pixels.Bounds())
	width, height := 0, 0
	seen := map[ids.AssetID]bool{}
	for i, f := range frames {
		r := sourceFrameRect(f)
		if f.ID.IsZero() || seen[f.ID] || r.Empty() || !r.In(pixels.Bounds()) {
			return nil, nil, fmt.Errorf("artsource: frame %s has invalid source bounds or identity", f.Name)
		}
		seen[f.ID] = true
		for _, previous := range frames[:i] {
			if !r.Intersect(sourceFrameRect(previous)).Empty() {
				return nil, nil, fmt.Errorf("artsource: overlapping source regions need an independent art copy before editing")
			}
		}
		width, height = max(width, r.Dx()), max(height, r.Dy())
	}
	d, err := art.New(width, height, art.RGBA, art.DefaultLimits())
	if err != nil {
		return nil, nil, err
	}
	keep := false
	defer func() {
		if !keep {
			d.Close()
		}
	}()
	info := d.Info()
	e, err := d.Begin(d.Revision(), "Create linked source art")
	if err != nil {
		return nil, nil, err
	}
	defer e.Cancel()
	layer, previous := info.Layers[0].ID, info.Frames[0].ID
	links := make([]TextureLinkFrame, 0, len(frames))
	surfaces := sourceSurfaceReader{ctx: ctx, pixels: map[string]*image.NRGBA{}, metas: map[string]*pipeline.Meta{}}
	for n, f := range frames {
		frame := previous
		if n != 0 {
			frame, err = e.AddFrame(previous, 100)
			if err != nil {
				return nil, nil, err
			}
		}
		previous = frame
		links = append(links, TextureLinkFrame{Asset: f.ID, Frame: frame})
		r := sourceFrameRect(f)
		if err := pasteSourcePixels(e, layer, frame, art.Albedo, pixels, r); err != nil {
			return nil, nil, err
		}
		pivot := f.EffectivePivot()
		if _, err := e.SetSlice(art.Slice{Name: "pivot", Frame: frame,
			Rect:  image.Rect(0, 0, r.Dx(), r.Dy()),
			Pivot: normalizedPivotToPixels(pivot, r.Dx(), r.Dy()),
		}); err != nil {
			return nil, nil, err
		}
		for _, ch := range []art.Channel{art.Normal, art.Height, art.Emission} {
			id, _, err := asset.TextureChannel(ctx.Meta, f.ID, string(ch))
			if err != nil {
				return nil, nil, err
			}
			if id.IsZero() {
				continue
			}
			surface, region, err := surfaces.read(id, ch)
			if err != nil {
				return nil, nil, fmt.Errorf("artsource: %s for frame %s: %w", ch, f.Name, err)
			}
			if region.Dx() != r.Dx() || region.Dy() != r.Dy() {
				return nil, nil, fmt.Errorf("artsource: %s frame size differs from its color sprite", ch)
			}
			if err := pasteSourcePixels(e, layer, frame, ch, surface, region); err != nil {
				return nil, nil, err
			}
		}
	}
	change, err := e.Prepare()
	if err != nil {
		return nil, nil, err
	}
	if change != nil {
		if err := change.Apply(); err != nil {
			return nil, nil, err
		}
	}
	keep = true
	return d, links, nil
}

func pasteSourcePixels(e *art.Edit, layer, frame art.ID, channel art.Channel, pixels *image.NRGBA, region image.Rectangle) error {
	cel, err := e.EnsureCel(layer, frame, channel)
	if err != nil {
		return err
	}
	clip := &art.Clipboard{Width: region.Dx(), Height: region.Dy(), Pixels: make([]art.Color, region.Dx()*region.Dy())}
	for y := 0; y < region.Dy(); y++ {
		for x := 0; x < region.Dx(); x++ {
			c := pixels.NRGBAAt(region.Min.X+x, region.Min.Y+y)
			clip.Pixels[y*region.Dx()+x] = art.Color{R: c.R, G: c.G, B: c.B, A: c.A}
		}
	}
	return e.Paste(cel, clip, image.Point{}, nil)
}

type sourceSurfaceReader struct {
	ctx    *pipeline.ImportContext
	pixels map[string]*image.NRGBA
	metas  map[string]*pipeline.Meta
}

func (r *sourceSurfaceReader) read(id ids.AssetID, channel art.Channel) (*image.NRGBA, image.Rectangle, error) {
	if r.ctx.Read == nil || r.ctx.SourcePath == nil {
		return nil, image.Rectangle{}, fmt.Errorf("surface source resolver is unavailable")
	}
	name, ok := r.ctx.SourcePath(id)
	if !ok {
		return nil, image.Rectangle{}, fmt.Errorf("surface source %s is unavailable", id.String())
	}
	name, _, _ = strings.Cut(name, pipeline.SubSep)
	key := name + "#" + string(channel)
	meta, pixels := r.metas[key], r.pixels[key]
	if meta == nil {
		encoded, err := r.ctx.Read(name + pipeline.MetaSuffix)
		if err != nil {
			return nil, image.Rectangle{}, err
		}
		meta, err = pipeline.ParseMeta(encoded, name+pipeline.MetaSuffix)
		if err != nil {
			return nil, image.Rectangle{}, err
		}
		if link, err := TextureLinkOf(meta); err != nil || link != nil {
			return nil, image.Rectangle{}, fmt.Errorf("surface already has a separate authoring source; edit it separately")
		}
		data, err := r.ctx.Read(name)
		if err != nil {
			return nil, image.Rectangle{}, err
		}
		pixels, err = decodeEditableTexturePNG(data)
		if err != nil {
			return nil, image.Rectangle{}, err
		}
		if channel == art.Normal {
			// Reuse the texture importer's normal convention conversion.
			localMeta := *meta
			localMeta.Settings = meta.Settings.Clone()
			localMeta.Settings["atlas"] = aetext.String("")
			localMeta.Settings["trim"] = aetext.Bool(false)
			local := *r.ctx
			local.ID, local.Path, local.Source, local.Meta = meta.ID, name, data, &localMeta
			prepared, err := texImporter.New().Import(&local)
			if err != nil {
				return nil, image.Rectangle{}, err
			}
			found := false
			for _, artifact := range prepared.Artifacts {
				if artifact.Kind != asset.ArtTexture {
					continue
				}
				texture, err := asset.DecodeTexture(artifact.Data)
				if err != nil || texture.Semantic != asset.TextureNormal {
					return nil, image.Rectangle{}, fmt.Errorf("normal source did not prepare a normal texture: %v", err)
				}
				pixels = &image.NRGBA{Pix: texture.Pixels, Stride: texture.Width * 4, Rect: image.Rect(0, 0, texture.Width, texture.Height)}
				found = true
			}
			if !found {
				return nil, image.Rectangle{}, fmt.Errorf("normal source did not produce prepared pixels")
			}
		}
		if channel == art.Emission {
			role, err := asset.TextureRoleOf(name, meta.Settings)
			if err != nil {
				return nil, image.Rectangle{}, err
			}
			srgb, err := asset.TextureSRGB(role, meta.Settings)
			if err != nil {
				return nil, image.Rectangle{}, err
			}
			artSRGB, err := asset.TextureSRGB(asset.RoleEmissionRGB, r.ctx.Meta.Settings)
			if err != nil || role != asset.RoleEmissionRGB || srgb != artSRGB {
				return nil, image.Rectangle{}, fmt.Errorf("emission mask or a different emission color profile needs an independent source; the linked art copy would change its color interpretation")
			}
		}
		r.metas[key], r.pixels[key] = meta, pixels
	}
	if id == meta.ID {
		return pixels, pixels.Bounds(), nil
	}
	frame, ok := meta.Frame(id)
	if !ok {
		return nil, image.Rectangle{}, fmt.Errorf("surface frame %s is missing from its source metadata", id.String())
	}
	region := sourceFrameRect(*frame)
	if region.Empty() || !region.In(pixels.Bounds()) {
		return nil, image.Rectangle{}, fmt.Errorf("surface frame has invalid bounds")
	}
	return pixels, region, nil
}
