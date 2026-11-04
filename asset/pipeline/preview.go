package pipeline

import (
	"path/filepath"
	"strings"

	"aego/core/errs"
	"aego/core/log"
)

// Preview imports an in-memory source with the project's registered importer,
// without changing source files, the index, .meta files or cached artifacts.
// overlay supplies unsaved editor include buffers; false falls back to the VFS.
func (p *Pipeline) Preview(rel string, source []byte, overlay func(string) ([]byte, bool)) (*Result, error) {
	return p.preview(rel, source, nil, overlay)
}

// PreviewWithMetadata imports one private authoring candidate; metadata and source
// are immutable snapshots and never enter the global index or GPU loader.
func (p *Pipeline) PreviewWithMetadata(rel string, source, metadata []byte, overlay func(string) ([]byte, bool)) (*Result, error) {
	return p.preview(rel, source, metadata, overlay)
}

func (p *Pipeline) preview(rel string, source, metadata []byte, overlay func(string) ([]byte, bool)) (*Result, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	rel = filepath.ToSlash(filepath.Clean(rel))
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, "../") {
		return nil, errs.New(errs.VFSBadPath, "preview needs a path inside the project")
	}
	imp, ok := p.imps.ForPath(rel)
	if !ok {
		return nil, errs.New(errs.AssetBadImporter, "no importer for this source", log.F("path", rel))
	}
	ctx := &ImportContext{Path: rel, Source: source, Log: p.log, Lookup: p.lookup, SourcePath: p.pathOf}
	if r, exists := p.byPath[rel]; exists {
		meta := *r.meta
		meta.Settings = r.meta.Settings.Clone()
		meta.Subs = append([]SubAsset(nil), r.meta.Subs...)
		ctx.ID, ctx.Meta = r.id, &meta
	} else {
		ctx.Meta = &Meta{Importer: imp.ID(), Settings: imp.Defaults()}
	}
	if metadata == nil && overlay != nil {
		if current, ok := overlay(rel + MetaSuffix); ok {
			metadata = current
		}
	}
	if metadata != nil {
		m, err := ParseMeta(metadata, rel+MetaSuffix)
		if err != nil {
			return nil, err
		}
		if m.Importer != "" && m.Importer != imp.ID() {
			return nil, errs.New(errs.AssetBadMeta, "preview metadata importer does not match source")
		}
		if !ctx.ID.IsZero() && m.ID != ctx.ID {
			return nil, errs.New(errs.AssetBadMeta, "preview metadata cannot change the source AssetID")
		}
		m.Importer = imp.ID()
		for key, value := range imp.Defaults() {
			if _, ok := m.Settings[key]; !ok {
				m.Settings[key] = value
			}
		}
		ctx.ID, ctx.Meta = m.ID, m
	}
	ctx.Read = func(path string) ([]byte, error) {
		if overlay != nil {
			if src, exists := overlay(path); exists {
				return src, nil
			}
		}
		return p.read(path)
	}
	if resolver, ok := imp.(MetadataResolver); ok {
		settings, err := resolver.ResolveMetadata(ctx)
		if err != nil {
			return nil, err
		}
		if settings != nil {
			ctx.Meta.Settings = settings
		}
	}
	return imp.Import(ctx)
}
