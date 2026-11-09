// Package gpupreview presents immutable Art Core snapshots with Aego's existing
// runtime renderer, lighting and CPU particles. It owns no authoring state,
// commands, history or file-system access.
package gpupreview

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"aego/art"
	"aego/asset"
	"aego/core/ids"
	"aego/gpu"
	"aego/render/texture"
	"aego/vfs"
	"aego/vfs/memfs"
)

const MaxSnapshotBytes = 64 << 20
const maxPreparedBytes = 64 << 20

type prepared struct {
	info    art.Info
	entries []asset.Entry
	files   vfs.FS
	frames  map[art.ID]map[art.Channel]ids.AssetID
}

func previewID(document string, frame art.ID, channel art.Channel, kind string) ids.AssetID {
	h := sha256.Sum256([]byte("aego.art.preview.v1\x00" + document + "\x00" + string(frame) + "\x00" + string(channel) + "\x00" + kind))
	return ids.AssetID{Hi: binary.BigEndian.Uint64(h[:8]), Lo: binary.BigEndian.Uint64(h[8:16])}
}

// surfaceChannels indexes cels once, preserving the renderer's channel order and
// counting each frame/channel only once even when several layers contain it.
// Info comes from a validated Art document; enforce the existing expanded-pixel
// budget before rendering or allocating any frame-sized surface.
func surfaceChannels(i art.Info) (map[art.ID][]art.Channel, error) {
	const (
		normal = 1 << iota
		height
		emission
	)
	present := make(map[art.ID]uint8, len(i.Frames))
	for _, c := range i.Cels {
		switch c.Channel {
		case art.Normal:
			present[c.Frame] |= normal
		case art.Height:
			present[c.Frame] |= height
		case art.Emission:
			present[c.Frame] |= emission
		}
	}
	channels := make(map[art.ID][]art.Channel, len(i.Frames))
	surfaces := 0
	for _, f := range i.Frames {
		channels[f.ID] = []art.Channel{art.Albedo}
		for _, ch := range []struct {
			channel art.Channel
			mask    uint8
		}{{art.Normal, normal}, {art.Height, height}, {art.Emission, emission}} {
			if present[f.ID]&ch.mask != 0 {
				channels[f.ID] = append(channels[f.ID], ch.channel)
			}
		}
		surfaces += len(channels[f.ID])
	}
	bytes := int64(i.Canvas.Width) * int64(i.Canvas.Height) * 4 * int64(surfaces)
	if bytes > maxPreparedBytes {
		return nil, fmt.Errorf("preview: prepared surface pixels exceed the 64 MiB budget")
	}
	return channels, nil
}

func prepare(data []byte) (*prepared, error) {
	if len(data) == 0 || len(data) > MaxSnapshotBytes {
		return nil, fmt.Errorf("preview: document snapshot exceeds the 64 MiB limit")
	}
	d, err := art.Decode(data, art.DefaultLimits())
	if err != nil {
		return nil, err
	}
	// Rendered textures and Info are detached copies. Once preparation ends,
	// the decoded authoring document and its tiled pixels are no longer needed.
	defer d.Close()
	i := d.Info()
	channels, err := surfaceChannels(i)
	if err != nil {
		return nil, err
	}
	p := &prepared{info: i, frames: make(map[art.ID]map[art.Channel]ids.AssetID)}
	files := make(map[string][]byte)
	put := func(id ids.AssetID, kind asset.ArtifactKind, data []byte, deps []ids.AssetID) {
		h := sha256.Sum256(data)
		name := id.String() + "." + string(kind)
		files[name] = data
		p.entries = append(p.entries, asset.Entry{ID: id, Kind: kind, Name: name, Deps: deps,
			Artifacts: []asset.Artifact{{Kind: kind, Path: "res://" + name, Hash: hex.EncodeToString(h[:]), Size: int64(len(data))}}})
	}
	for _, f := range i.Frames {
		p.frames[f.ID] = make(map[art.Channel]ids.AssetID)
		for _, ch := range channels[f.ID] {
			p.frames[f.ID][ch] = previewID(i.ID, f.ID, ch, "region")
		}
	}
	for _, f := range i.Frames {
		for _, ch := range channels[f.ID] {
			im, e := d.Render(f.ID, art.RenderOptions{Channel: ch})
			if e != nil {
				return nil, e
			}
			format, semantic := gpu.TexSRGBA8, asset.TextureColor
			switch ch {
			case art.Normal:
				format, semantic = gpu.TexRGBA8, asset.TextureNormal
			case art.Height:
				format, semantic = gpu.TexRGBA8, asset.TextureHeight
			case art.Emission:
				semantic = asset.TextureEmissionRGB
			}
			pixels := &texture.Data{Width: i.Canvas.Width, Height: i.Canvas.Height, Format: format, Pixels: im.Pix, NonColor: semantic != asset.TextureColor}
			// Identical color preparation to the PNG importer and runtime loader.
			pixels.Premultiply()
			page := previewID(i.ID, f.ID, ch, "texture")
			put(page, asset.ArtTexture, asset.EncodeTexture(&asset.TextureArtifact{Width: pixels.Width, Height: pixels.Height,
				Format: format, Filter: gpu.FilterNearest, Semantic: semantic, Pixels: pixels.Pixels}), nil)
			r := asset.RegionArtifact{Page: page, W: pixels.Width, H: pixels.Height, PageW: pixels.Width, PageH: pixels.Height, Filter: gpu.FilterNearest}
			deps := []ids.AssetID{page}
			if pivot, ok := i.FramePivot(f.ID); ok {
				r.HasPivot = true
				r.Pivot = [2]float32{float32(pivot.X) / float32(pixels.Width), float32(pivot.Y) / float32(pixels.Height)}
			}
			if ch == art.Albedo {
				r.Normal, r.Emission = p.frames[f.ID][art.Normal], p.frames[f.ID][art.Emission]
				for _, id := range []ids.AssetID{r.Normal, r.Emission} {
					if !id.IsZero() {
						deps = append(deps, id)
					}
				}
			}
			put(p.frames[f.ID][ch], asset.ArtSpriteRegion, asset.EncodeRegion(&r), deps)
		}
	}
	p.files, err = vfs.New(vfs.Mount{Scheme: "res", Source: memfs.New(files), ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return p, nil
}
