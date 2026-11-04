package pipeline

import (
	"bytes"
	"image/png"
	"io"
	"os"
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/engine/res"
)

// TextureSourceInfo resolves source metadata (including a dirty editor overlay)
// for semantic mutations. Runtime code must use TextureMetadataSource instead.
func (p *Pipeline) TextureSourceInfo(id ids.AssetID, overlay func(string) ([]byte, bool)) (asset.TextureSourceInfo, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	name, ok := p.pathOf(id)
	if !ok && overlay != nil {
		// Newly sliced frames already have stable IDs in dirty metadata, before
		// Save adds them to the persistent import index.
		for _, record := range p.byID {
			if record.importer.ID() != "texture" {
				continue
			}
			encoded, present := overlay(record.path + MetaSuffix)
			if !present {
				continue
			}
			meta, err := ParseMeta(encoded, record.path+MetaSuffix)
			if err != nil || meta.ID != record.id {
				continue
			}
			if frame, exists := meta.Frame(id); exists {
				if ok {
					return asset.TextureSourceInfo{}, false
				}
				name, ok = record.path+SubSep+frame.Name, true
			}
		}
	}
	if !ok {
		return asset.TextureSourceInfo{}, false
	}
	source, _, _ := strings.Cut(name, SubSep)
	read := func(name string) ([]byte, error) {
		if overlay != nil {
			if b, ok := overlay(name); ok {
				return b, nil
			}
		}
		return p.read(name)
	}
	// Schema and channel pickers need dimensions, not an entire source image.
	// Keep the disk read streaming and bounded; dirty image bytes already live
	// in the caller's overlay and likewise never need a second full copy.
	var sourceReader io.Reader
	if overlay != nil {
		if data, present := overlay(source); present {
			sourceReader = bytes.NewReader(data)
		}
	}
	if sourceReader == nil {
		var file io.ReadCloser
		var err error
		if embedded, ok := strings.CutPrefix(source, "engine://"); ok {
			file, err = res.FS().Open(embedded)
		} else {
			file, err = os.Open(p.Abs(source))
		}
		if err != nil {
			return asset.TextureSourceInfo{}, false
		}
		defer file.Close()
		sourceReader = file
	}
	config, err := png.DecodeConfig(io.LimitReader(sourceReader, 32<<20))
	if err != nil {
		return asset.TextureSourceInfo{}, false
	}
	encoded, err := read(source + MetaSuffix)
	if err != nil {
		return asset.TextureSourceInfo{}, false
	}
	meta, err := ParseMeta(encoded, source+MetaSuffix)
	if err != nil {
		return asset.TextureSourceInfo{}, false
	}
	return asset.TextureSource(meta, source, config.Width, config.Height, id)
}
