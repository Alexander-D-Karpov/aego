package pipeline

import (
	"bytes"
	"os"

	"aego/core/errs"
	"aego/core/log"
	"aego/project"
)

// resolveMetadata runs under the import lock; importers cannot write metadata or mutate the record.
// Stable references are persisted before publishing artifacts that use them.
func (p *Pipeline) resolveMetadata(r *record, ctx *ImportContext) error {
	resolver, ok := r.importer.(MetadataResolver)
	if !ok {
		return nil
	}
	copyMeta := *r.meta
	copyMeta.Settings = r.meta.Settings.Clone()
	copyMeta.Subs = append([]SubAsset(nil), r.meta.Subs...)
	copyCtx := *ctx
	copyCtx.Meta = &copyMeta
	settings, err := resolver.ResolveMetadata(&copyCtx)
	if err != nil || settings == nil {
		return err
	}
	if bytes.Equal(canonicalSettings(settings), canonicalSettings(r.meta.Settings)) {
		return nil
	}
	if p.sourceTransactionReadOnlyMetas[r.path+MetaSuffix] {
		return errs.New(errs.AssetSourceUnstable, "metadata needs changes but is a read-only source guard", log.F("path", r.path))
	}
	if p.shared || p.mode == MetaReadOnly {
		return errs.New(errs.AssetBadMeta, "automatic asset references need a writable metadata owner", log.F("path", r.path))
	}
	transactionPath := r.path + MetaSuffix
	if p.sourceTransactionMetas != nil {
		if _, guarded := p.sourceTransactionMetas[transactionPath]; !guarded {
			// Dependencies and cache-salted sources are also reimported during
			// ApplySources. Their sidecars are not automatically part of the
			// caller's history or rollback: never silently normalize one here.
			return errs.New(errs.AssetBadMeta, "dependent metadata needs an explicit source transaction guard before references can be resolved", log.F("path", transactionPath))
		}
	}
	if err := p.checkMetadataSnapshot(r); err != nil {
		return err
	}
	path := p.abs(r.path) + MetaSuffix
	before, err := os.ReadFile(path)
	if err != nil {
		return errs.Wrap(err, errs.AssetBadMeta, "cannot read metadata before resolving references")
	}
	if expected, guarded := p.sourceTransactionMetas[transactionPath]; guarded && !sameSource(before, expected) {
		return errs.New(errs.AssetSourceUnstable, "metadata changed during source transaction; resolve before retrying", log.F("path", r.path))
	}
	disk, err := ParseMeta(before, r.path+MetaSuffix)
	if err != nil {
		return err
	}
	if disk.Importer == "" {
		disk.Importer = r.importer.ID()
	}
	for k, v := range r.importer.Defaults() {
		if _, exists := disk.Settings[k]; !exists {
			disk.Settings[k] = v
		}
	}
	if !bytes.Equal(WriteMeta(disk), WriteMeta(r.meta)) {
		return errs.New(errs.AssetSourceUnstable, "metadata changed during reference resolution; rescan before retrying", log.F("path", r.path))
	}
	copyMeta.Settings = settings.Clone()
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		return errs.New(errs.AssetSourceUnstable, "metadata is still being written", log.F("path", r.path))
	}
	published := WriteMeta(&copyMeta)
	if p.sourceJournal != nil {
		checked, err := p.sourceBytes(p.sourceJournalRoot, transactionPath)
		if err != nil || !sameSource(checked, before) {
			return errs.New(errs.AssetSourceUnstable, "metadata changed before reference publication", log.F("path", r.path))
		}
		if err := p.sourceJournal.authorize(transactionPath, published); err != nil {
			return errs.Wrap(err, errs.VFSIO, "cannot protect resolved asset references")
		}
		if err := p.checkMetadataSnapshot(r); err != nil {
			return err
		}
		if err := replaceSource(p.sourceJournalRoot, transactionPath, published); err != nil {
			return errs.Wrap(err, errs.VFSIO, "cannot persist resolved asset references")
		}
	} else {
		if err := p.checkMetadataSnapshot(r); err != nil {
			return err
		}
		if err := project.WriteAtomic(path, published); err != nil {
			return errs.Wrap(err, errs.VFSIO, "cannot persist resolved asset references")
		}
	}
	r.captureMetadata(published)
	if _, guarded := p.sourceTransactionMetas[transactionPath]; guarded {
		p.sourceTransactionMetas[transactionPath] = published
	}
	if p.sourceJournal != nil && p.sourceTransactionCheckpoint != nil {
		p.sourceTransactionCheckpoint("metadata", transactionPath)
	}
	r.meta = &copyMeta
	ctx.Meta = r.meta
	p.metas[r.path+MetaSuffix] = true
	p.rev++
	return nil
}
