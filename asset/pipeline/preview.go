package pipeline

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aego/core/errs"
	"aego/core/log"
)

// Preview imports in-memory source with the registered importer without changing files, index or cached artifacts; overlay supplies unsaved includes, falling back to the VFS on false.
func (p *Pipeline) Preview(rel string, source []byte, overlay func(string) ([]byte, bool)) (*Result, error) {
	return p.preview(context.Background(), rel, source, nil, overlay)
}

// PreviewWithMetadata imports one private authoring candidate; metadata and source
// are immutable snapshots and never enter the global index or GPU loader.
func (p *Pipeline) PreviewWithMetadata(rel string, source, metadata []byte, overlay func(string) ([]byte, bool)) (*Result, error) {
	return p.preview(context.Background(), rel, source, metadata, overlay)
}

// PreviewWithMetadataContext permits cancellation of a guarded private
// preview. Cancellation is cooperative and cannot interrupt a mutex wait,
// parser, importer, or decoder call already in progress.
func (p *Pipeline) PreviewWithMetadataContext(ctx context.Context, rel string, source, metadata []byte, overlay func(string) ([]byte, bool)) (*Result, error) {
	return p.preview(ctx, rel, source, metadata, overlay)
}

func (p *Pipeline) preview(request context.Context, rel string, source, metadata []byte, overlay func(string) ([]byte, bool)) (*Result, error) {
	if request == nil {
		return nil, fmt.Errorf("preview needs a context")
	}
	if err := request.Err(); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if err := request.Err(); err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, "../") {
		return nil, errs.New(errs.VFSBadPath, "preview needs a path inside the project")
	}
	imp, ok := p.imps.ForPath(rel)
	if !ok {
		return nil, errs.New(errs.AssetBadImporter, "no importer for this source", log.F("path", rel))
	}
	ctx := &ImportContext{Path: rel, Source: source, Log: p.log, Lookup: p.lookup, SourcePath: p.pathOf}
	var cached *record
	if r, exists := p.byPath[rel]; exists {
		if err := p.checkMetadataSnapshot(r); err != nil {
			return nil, err
		}
		cached = r
		meta := *r.meta
		meta.Settings = r.meta.Settings.Clone()
		meta.Subs = append([]SubAsset(nil), r.meta.Subs...)
		ctx.ID, ctx.Meta = r.id, &meta
	} else {
		ctx.Meta = &Meta{Importer: imp.ID(), Settings: imp.Defaults()}
	}
	ctx.Read = func(path string) ([]byte, error) {
		if overlay != nil {
			if src, exists := overlay(path); exists {
				return src, nil
			}
		}
		return p.read(path)
	}
	if metadata == nil && overlay != nil {
		if current, ok := overlay(rel + MetaSuffix); ok {
			metadata = current
		}
	}
	if metadata != nil {
		if previewer, ok := imp.(GuardedMetadataPreviewer); ok {
			guarded, err := previewer.GuardedMetadataCandidate(rel, metadata)
			if err != nil {
				return nil, err
			}
			if err := request.Err(); err != nil {
				return nil, err
			}
			if guarded {
				return p.previewGuardedMetadata(request, previewer, ctx, cached, metadata)
			}
		}
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
	if resolver, ok := imp.(MetadataResolver); ok {
		settings, err := resolver.ResolveMetadata(ctx)
		if err != nil {
			return nil, err
		}
		if settings != nil {
			ctx.Meta.Settings = settings
		}
	}
	result, err := imp.Import(ctx)
	if err != nil {
		return nil, err
	}
	if err := request.Err(); err != nil {
		return nil, err
	}
	if cached != nil {
		if err := p.checkMetadataSnapshot(cached); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// previewGuardedMetadata runs only for registered opt-in importers and never
// changes an index or file. Its source and metadata checks are best-effort,
// not CAS or a substitute for the editor's asynchronous generation guard.
func (p *Pipeline) previewGuardedMetadata(request context.Context, previewer GuardedMetadataPreviewer, ctx *ImportContext, cached *record, metadata []byte) (*Result, error) {
	if cached == nil || ctx.ID.IsZero() || p.unlock == nil || int64(len(ctx.Source)) > MaxSourceFingerprintBytes {
		return nil, errs.New(errs.AssetBadMeta, "guarded metadata preview needs an indexed bounded source", log.F("path", ctx.Path))
	}
	if err := p.requireNoPendingSourceRecovery(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(p.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	openedRoot, err := root.Stat(".")
	if err != nil {
		return nil, fmt.Errorf("cannot identify guarded preview project root: %w", err)
	}
	checkRoot := func() error {
		current, err := os.Stat(p.root)
		if err != nil {
			return fmt.Errorf("cannot verify guarded preview project root: %w", err)
		}
		if !openedRoot.IsDir() || !current.IsDir() || !os.SameFile(openedRoot, current) {
			return fmt.Errorf("project root changed during guarded preview: %s", p.root)
		}
		return nil
	}
	if err := checkRoot(); err != nil {
		return nil, err
	}
	hash := sha256.New()
	for rest := ctx.Source; len(rest) > 0; {
		if err := request.Err(); err != nil {
			return nil, err
		}
		n := min(len(rest), 64<<10)
		_, _ = hash.Write(rest[:n])
		rest = rest[n:]
	}
	want := SourceFingerprint{Exists: true, Size: int64(len(ctx.Source))}
	copy(want.SHA256[:], hash.Sum(nil))
	checkSource := func() error {
		got, err := p.fingerprintSource(request, root, ctx.Path, int64(len(ctx.Source)), nil)
		if err != nil {
			return fmt.Errorf("cannot verify source for guarded preview %s: %w", ctx.Path, err)
		}
		if got != want {
			return fmt.Errorf("source changed before or during guarded preview: %s", ctx.Path)
		}
		return nil
	}
	if err := checkSource(); err != nil {
		return nil, err
	}
	// The source, metadata and renderer are immutable snapshots. Do not hand
	// the unlocked worker callbacks into the mutable pipeline index.
	input := &ImportContext{ID: ctx.ID, Path: ctx.Path, Source: ctx.Source, Log: ctx.Log}
	meta, key, metaHash := cached.meta, cached.key, cached.metaHash
	present, tracked := cached.metaPresent, cached.metaTracked
	result, err := func() (*Result, error) {
		p.mu.RUnlock()
		defer p.mu.RLock()
		return previewer.PreviewGuardedMetadata(request, input, metadata)
	}()
	if err != nil {
		return nil, err
	}
	if err := request.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errs.New(errs.AssetImportFailed, "guarded preview returned no result", log.F("path", ctx.Path))
	}
	if p.unlock == nil || p.byID[ctx.ID] != cached || p.byPath[ctx.Path] != cached ||
		cached.meta != meta || cached.key != key || cached.metaHash != metaHash ||
		cached.metaPresent != present || cached.metaTracked != tracked {
		return nil, errs.New(errs.AssetSourceUnstable, "asset changed during guarded preview", log.F("path", ctx.Path))
	}
	if err := p.requireNoPendingSourceRecovery(); err != nil {
		return nil, err
	}
	if err := checkSource(); err != nil {
		return nil, err
	}
	if err := p.checkMetadataSnapshot(cached); err != nil {
		return nil, err
	}
	if err := checkRoot(); err != nil {
		return nil, err
	}
	if err := request.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
