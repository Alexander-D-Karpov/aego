package pipeline

import (
	"fmt"

	"aego/core/ids"
)

// VerifyCurrent checks source and settings against the last successful import key without changing state.
// Strict export uses it when last-good artifacts may outlive invalid source; generated atlas pages require all source members to be current.
func (p *Pipeline) VerifyCurrent(id ids.AssetID) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if err := p.requireNoPendingSourceRecovery(); err != nil {
		return fmt.Errorf("cannot verify an asset while source recovery is pending: %w", err)
	}
	if r := p.byID[id]; r != nil {
		return p.verifyCurrent(r)
	}
	if r := p.subs[id]; r != nil {
		return p.verifyCurrent(r)
	}
	for group, page := range p.pages {
		if page != id {
			continue
		}
		members := p.groups[group]
		if len(members) == 0 {
			return fmt.Errorf("atlas %s has no current source members", group)
		}
		for _, member := range members {
			r := p.byID[member]
			if r == nil {
				return fmt.Errorf("atlas %s has a missing source member %s", group, member)
			}
			if err := p.verifyCurrent(r); err != nil {
				return err
			}
		}
		if p.groupKey[group] != p.groupCacheKey(group, members) {
			return fmt.Errorf("atlas %s needs reimport", group)
		}
		return nil
	}
	return fmt.Errorf("asset %s has no current import record", id)
}

func (p *Pipeline) verifyCurrent(r *record) error {
	if r.err != "" {
		return fmt.Errorf("%s: latest import failed: %s", r.path, r.err)
	}
	if r.importer == nil || r.key == "" {
		return fmt.Errorf("%s has no successful current import", r.path)
	}
	source, err := p.read(r.path)
	if err != nil {
		return fmt.Errorf("%s: cannot read current source: %w", r.path, err)
	}
	metaSource, err := p.read(r.path + MetaSuffix)
	if err != nil {
		return fmt.Errorf("%s: cannot read current metadata: %w", r.path, err)
	}
	meta, err := ParseMeta(metaSource, r.path+MetaSuffix)
	if err != nil {
		return err
	}
	if meta.Importer == "" {
		meta.Importer = r.importer.ID()
	}
	for key, value := range r.importer.Defaults() {
		if _, exists := meta.Settings[key]; !exists {
			meta.Settings[key] = value
		}
	}
	if meta.ID != r.id || meta.Importer != r.importer.ID() {
		return fmt.Errorf("%s: asset identity or importer changed; rescan and reimport", r.path)
	}
	current := *r
	current.meta = meta
	var salt []byte
	if imp, ok := r.importer.(CacheSalter); ok {
		salt, err = imp.CacheSalt(&ImportContext{ID: r.id, Path: r.path, Source: source, Meta: meta, Read: p.read, Lookup: p.lookup, SourcePath: p.pathOf, Log: p.log})
		if err != nil {
			return err
		}
	}
	if p.cacheKey(&current, source, r.deps, salt) != r.key {
		return fmt.Errorf("%s: current source or import settings changed; reimport before export", r.path)
	}
	return nil
}
