package context

import (
	"fmt"
	"strings"

	"aego/asset"
	"aego/asset/importers/preset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/document"
	"aego/format/aetext"
	"aego/scene/mutate"
)

func (e *Editor) assetRequest(id ids.AssetID) (document.AssetRequest, error) {
	if e.Docs == nil {
		return document.AssetRequest{}, fmt.Errorf("asset documents require the project asset index")
	}
	if b, ok := asset.LookupBuiltin(id); ok {
		return document.AssetRequest{Asset: id, Selected: id, Path: b.Name, Kind: b.Kind, ReadOnly: true}, nil
	}
	if e.Pipeline == nil {
		return document.AssetRequest{}, fmt.Errorf("asset index is unavailable")
	}
	entry, known := e.Pipeline.Entry(id)
	rel, source := e.Pipeline.PathOf(id)
	if !known && !source {
		if working, ok := e.workingPreviewAsset(id); ok {
			return working, nil
		}
		if src, ok := e.Thumbs.(interface {
			SourceFile(ids.AssetID) (pipeline.AssetInfo, bool)
		}); ok {
			if info, ok := src.SourceFile(id); ok {
				return document.AssetRequest{Asset: id, Selected: id, Path: info.Path, Kind: info.Kind}, nil
			}
		}
		return document.AssetRequest{}, fmt.Errorf("asset %s is not in the project", id.String())
	}
	r := document.AssetRequest{Asset: id, Selected: id, Path: rel, Kind: entry.Kind, ReadOnly: !source || asset.IsBuiltin(id)}
	if !source {
		r.Path = entry.Name
	}
	if parent, sub, ok := strings.Cut(rel, pipeline.SubSep); ok {
		if owner, ok := e.Pipeline.Lookup(parent); ok {
			r.Asset, r.Path, r.Subasset = owner, parent, sub
		}
	}
	if meta, ok := e.Pipeline.MetaOf(r.Asset); ok {
		if role, err := asset.TextureRoleOf(r.Path, meta.Settings); meta.Importer == "texture" && err == nil {
			r.Role = string(role)
		} else {
			r.Role = meta.Settings.String("role", meta.Settings.String("semantic", ""))
		}
	}
	if known && entry.Kind == asset.ArtPreset {
		if art, ok := entry.Artifact(asset.ArtPreset); ok {
			if b, err := e.Pipeline.Artifacts().ReadFile(art.Path); err == nil {
				if n, err := aetext.Parse(b, r.Path); err == nil {
					r.Role, _, _ = preset.Type(n)
				}
			}
		}
	}
	return r, nil
}

// TextureAuthoringEnv resolves source dimensions, roles and frame rectangles before packing; dirty metadata governs views and mutations, and runtime atlas coordinates never become source data.
func (e *Editor) TextureAuthoringEnv(id ids.AssetID) (mutate.TextureEnv, error) {
	if e.Pipeline == nil {
		return mutate.TextureEnv{}, fmt.Errorf("asset index is unavailable")
	}
	overlay := func(name string) ([]byte, bool) {
		if e.Docs != nil {
			if d := e.Docs.Script(name); d != nil && d.Dirty() {
				return d.Code.Bytes(), true
			}
		}
		return nil, false
	}
	info, ok := e.Pipeline.TextureSourceInfo(id, overlay)
	if !ok {
		return mutate.TextureEnv{}, fmt.Errorf("image source %s is unavailable", id.String())
	}
	return mutate.TextureEnv{Width: info.SourceWidth, Height: info.SourceHeight, Resolve: func(other ids.AssetID) (asset.TextureSourceInfo, bool) {
		return e.Pipeline.TextureSourceInfo(other, overlay)
	}}, nil
}

// DocumentForAsset resolves the same stable identity used by OpenAsset without
// activating a panel. Inspector and structured metadata mutations use this.
func (e *Editor) DocumentForAsset(id ids.AssetID) (*document.AssetDocument, error) {
	r, err := e.assetRequest(id)
	if err != nil {
		return nil, err
	}
	return e.Docs.AcquireAsset(r)
}

func (e *Editor) OpenAsset(id ids.AssetID) (*document.AssetDocument, error) {
	r, err := e.assetRequest(id)
	if err != nil {
		return nil, err
	}
	d, err := e.Docs.OpenAsset(r)
	if err == nil && d.Handler == "asset.readonly" {
		if e.FallbackAsset != nil {
			err = e.FallbackAsset(d)
		} else if e.InspectAsset != nil {
			e.InspectAsset(id)
		}
	}
	return d, err
}

func (e *Editor) OpenAssetPath(rel string) (*document.AssetDocument, error) {
	if e.Pipeline == nil {
		return nil, fmt.Errorf("asset index is unavailable")
	}
	id, ok := e.Pipeline.Lookup(document.Clean(rel))
	if !ok {
		if src, ok := e.Thumbs.(interface {
			SourceID(string) (ids.AssetID, bool)
		}); ok {
			if id, ok := src.SourceID(document.Clean(rel)); ok {
				return e.OpenAsset(id)
			}
		}
		return nil, fmt.Errorf("asset source %s is not in the project", rel)
	}
	return e.OpenAsset(id)
}
