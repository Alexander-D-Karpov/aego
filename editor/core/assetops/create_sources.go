package assetops

import (
	"bytes"
	"fmt"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/context"
)

// CreateSources is a creation-only capability. Every imported source must have
// explicit metadata, so callers can prepare stable cross-asset references
// before publication. Validation, import rollback and crash recovery remain in
// the existing Pipeline.ApplySources implementation.
func (o *Ops) CreateSources(ctx *context.Editor, edits []pipeline.SourceEdit) ([]ids.AssetID, error) {
	if ctx == nil || ctx.Pipeline == nil || len(edits) == 0 || len(edits) > 128 {
		return nil, fmt.Errorf("asset creation requires 1–128 source files and a project pipeline")
	}
	step := FileOp{createdSources: make([]pipeline.SourceEdit, len(edits)), createdAssets: map[string]ids.AssetID{}}
	byPath := make(map[string]int, len(edits))
	var total int64
	for i, edit := range edits {
		if edit.Before != nil || edit.After == nil || edit.Auxiliary {
			return nil, fmt.Errorf("%s is not a new imported source or metadata file", edit.Path)
		}
		if _, duplicate := byPath[edit.Path]; duplicate {
			return nil, fmt.Errorf("duplicate source creation %s", edit.Path)
		}
		total += int64(len(edit.After))
		if total > 64<<20 {
			return nil, fmt.Errorf("asset creation exceeds the 64 MiB history budget")
		}
		byPath[edit.Path] = i
		step.createdSources[i] = pipeline.SourceEdit{Path: edit.Path, After: bytes.Clone(edit.After)}
	}
	var created []ids.AssetID
	seenIDs := map[ids.AssetID]bool{}
	for _, edit := range step.createdSources {
		if strings.HasSuffix(edit.Path, pipeline.MetaSuffix) {
			if _, ok := byPath[strings.TrimSuffix(edit.Path, pipeline.MetaSuffix)]; !ok {
				return nil, fmt.Errorf("metadata %s has no source in this creation", edit.Path)
			}
			continue
		}
		defaults, err := ctx.Pipeline.NewSourceMeta(edit.Path)
		if err != nil {
			return nil, err
		}
		mi, ok := byPath[edit.Path+pipeline.MetaSuffix]
		if !ok {
			return nil, fmt.Errorf("%s requires explicit metadata for atomic creation", edit.Path)
		}
		meta, err := pipeline.ParseMeta(step.createdSources[mi].After, edit.Path+pipeline.MetaSuffix)
		if err != nil {
			return nil, err
		}
		if meta.Importer != defaults.Importer || meta.Version != defaults.Version || meta.ID.IsZero() || asset.IsBuiltin(meta.ID) || seenIDs[meta.ID] {
			return nil, fmt.Errorf("%s has incompatible metadata or a repeated asset identity", edit.Path)
		}
		if _, exists := ctx.Pipeline.Entry(meta.ID); exists {
			return nil, fmt.Errorf("%s uses an existing asset identity", edit.Path)
		}
		seenIDs[meta.ID] = true
		step.createdAssets[edit.Path] = meta.ID
		created = append(created, meta.ID)
	}
	if len(created) == 0 {
		return nil, fmt.Errorf("asset creation has no imported source")
	}
	if err := step.applyCreatedSources(ctx, false); err != nil {
		return nil, err
	}
	// ApplySources updates metadata entries to their actual imported bytes. Keep
	// those exact bytes in the single history entry for the next Undo/Redo guard.
	o.record([]FileOp{step})
	return created, nil
}

func (f FileOp) checkCreatedSources(ctx *context.Editor, back bool) error {
	for _, edit := range f.createdSources {
		if ctx.Docs != nil {
			if script := ctx.Docs.Script(edit.Path); script != nil && (script.Dirty() || script.Conflict) {
				return fmt.Errorf("cannot change created files while %s has unsaved edits or a conflict", edit.Path)
			}
			for _, doc := range ctx.Docs.AssetDocuments() {
				if doc.Path == edit.Path || doc.Path+pipeline.MetaSuffix == edit.Path {
					pending := doc.Binding.PendingRecovery != nil && doc.Binding.PendingRecovery()
					if doc.Dirty() || pending || (doc.Source != nil && doc.Source.Conflict) || (doc.Metadata != nil && doc.Metadata.Conflict) {
						return fmt.Errorf("cannot change created files while %s has unsaved edits, pending recovery or a conflict", doc.Path)
					}
				}
			}
		}
		want := edit.Before
		if back {
			want = edit.After
		}
		got, err := ctx.Pipeline.SourceBytes(edit.Path)
		if err != nil {
			return err
		}
		if (got == nil) != (want == nil) || !bytes.Equal(got, want) {
			return fmt.Errorf("%s changed since asset creation", edit.Path)
		}
	}
	if back {
		for rel, id := range f.createdAssets {
			if current, ok := ctx.Pipeline.Lookup(rel); !ok || current != id {
				return fmt.Errorf("cannot undo creation: the asset identity changed for %s", rel)
			}
		}
	}
	return nil
}

func (f FileOp) applyCreatedSources(ctx *context.Editor, back bool) error {
	if err := f.checkCreatedSources(ctx, back); err != nil {
		return err
	}
	batch, err := ctx.Pipeline.ApplySources(f.createdSources, back)
	if err != nil {
		return err
	}
	ctx.ApplyBatch(batch)
	if ctx.Docs != nil {
		for rel, id := range f.createdAssets {
			if back {
				// Dirty documents were rejected before any publication. Close only
				// these clean views after the coherent delete has committed.
				if err := ctx.Docs.CloseAsset(id, false); err != nil {
					// Publication is already committed. Keep any view that acquired
					// an edit in a host callback, and keep this undo entry coherent.
					ctx.Notify(context.LevelWarn, err.Error())
				}
				if script := ctx.Docs.Script(rel); script != nil {
					script.OnDisk = false
				}
			} else if script := ctx.Docs.Script(rel); script != nil {
				ctx.Docs.KeepScript(script)
			}
		}
	}
	return nil
}
