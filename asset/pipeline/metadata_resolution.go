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
	if p.shared || p.mode == MetaReadOnly {
		return errs.New(errs.AssetBadMeta, "automatic asset references need a writable metadata owner", log.F("path", r.path))
	}
	path := p.abs(r.path) + MetaSuffix
	before, err := os.ReadFile(path)
	if err != nil {
		return errs.Wrap(err, errs.AssetBadMeta, "cannot read metadata before resolving references")
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
	if err := project.WriteAtomic(path, WriteMeta(&copyMeta)); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot persist resolved asset references")
	}
	r.meta = &copyMeta
	ctx.Meta = r.meta
	p.metas[r.path+MetaSuffix] = true
	p.rev++
	return nil
}
