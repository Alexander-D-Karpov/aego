package pipeline

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"slices"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/project"
)

func (p *Pipeline) MetaOf(id ids.AssetID) (Meta, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	r := p.byID[id]
	if r == nil || r.meta == nil {
		return Meta{}, false
	}
	m := *r.meta
	m.Settings = r.meta.Settings.Clone()
	m.Subs = slices.Clone(r.meta.Subs)
	return m, true
}

// SetSubs writes the cells of a sliced sheet into its meta; the next import turns them into sprite assets (D339).
func (p *Pipeline) SetSubs(id ids.AssetID, subs []SubAsset) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.requireNoPendingSourceRecovery(); err != nil {
		return err
	}
	if p.shared {
		return errs.New(errs.AssetBadMeta, "another process holds the asset index, slice the sheet there")
	}
	r := p.byID[id]
	if r == nil || r.meta == nil {
		return errs.New(errs.AssetNotFound, "the asset is not in the project", log.F("asset", id.String()))
	}
	if err := p.checkMetadataSnapshot(r); err != nil {
		return err
	}
	m := *r.meta
	m.Subs = slices.Clone(subs)
	candidate := WriteMeta(&m)
	if err := p.checkMetadataSnapshot(r); err != nil {
		return err
	}
	if err := project.WriteAtomic(p.abs(r.path)+MetaSuffix, candidate); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write "+r.path+MetaSuffix).With(log.F("path", r.path))
	}
	r.captureMetadata(candidate)
	r.meta, r.derived, r.dup, r.key = &m, false, "", ""
	p.metas[r.path+MetaSuffix] = true
	p.rev++
	return nil
}

func (p *Pipeline) SetSettings(id ids.AssetID, s Settings) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.requireNoPendingSourceRecovery(); err != nil {
		return err
	}
	if p.shared {
		return errs.New(errs.AssetBadMeta, "another process holds the asset index, change the import settings there")
	}
	r := p.byID[id]
	if r == nil || r.meta == nil {
		return errs.New(errs.AssetNotFound, "the asset is not in the project", log.F("asset", id.String()))
	}
	if err := p.checkMetadataSnapshot(r); err != nil {
		return err
	}
	m := *r.meta
	m.Settings = s.Clone()
	m.Subs = slices.Clone(r.meta.Subs)
	candidate := WriteMeta(&m)
	if err := p.checkMetadataSnapshot(r); err != nil {
		return err
	}
	if err := project.WriteAtomic(p.abs(r.path)+MetaSuffix, candidate); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write "+r.path+MetaSuffix).With(log.F("path", r.path))
	}
	r.captureMetadata(candidate)
	r.meta, r.derived, r.dup, r.key = &m, false, "", ""
	p.metas[r.path+MetaSuffix] = true
	p.rev++
	return nil
}

func (r *record) captureMetadata(data []byte) {
	r.metaPresent, r.metaTracked = data != nil, true
	r.metaHash = sha256.Sum256(data)
}

// checkMetadataSnapshot refuses to use or overwrite a cached record when its
// disk metadata no longer describes the same source generation. Missing and
// duplicate metadata retain their existing derived-ID behavior. This is a
// best-effort preimage check, not a CAS against other or older processes.
// The caller holds p.mu for reading or writing.
func (p *Pipeline) checkMetadataSnapshot(r *record) error {
	if r == nil || r.meta == nil || r.importer == nil || !r.metaTracked {
		return errs.New(errs.AssetBadMeta, "asset has no metadata snapshot")
	}
	root, err := os.OpenRoot(p.root)
	if err != nil {
		return errs.Wrap(err, errs.AssetBadMeta, "cannot check current metadata").With(log.F("path", r.path))
	}
	defer root.Close()
	data, err := p.sourceBytes(root, r.path+MetaSuffix)
	if err != nil {
		return errs.Wrap(err, errs.AssetBadMeta, "cannot check current metadata").With(log.F("path", r.path))
	}
	if data == nil {
		if !r.metaPresent && r.derived && r.dup == "" {
			return nil
		}
		return errs.New(errs.AssetSourceUnstable, "metadata disappeared after this asset was indexed; rescan before editing", log.F("path", r.path))
	}
	if !r.metaPresent {
		return errs.New(errs.AssetSourceUnstable, "metadata appeared after this asset was indexed; rescan before editing", log.F("path", r.path))
	}
	if sha256.Sum256(data) != r.metaHash {
		// Unsupported or corrupt metadata is a failure, not merely a retry.
		if _, err := ParseMeta(data, r.path+MetaSuffix); err != nil {
			return err
		}
		return errs.New(errs.AssetSourceUnstable, "metadata changed on disk; reload it before importing or editing", log.F("path", r.path))
	}
	if r.meta.ID != r.id || r.meta.Importer != r.importer.ID() {
		return errs.New(errs.AssetBadMeta, "cached metadata identity or importer is inconsistent", log.F("path", r.path))
	}
	return nil
}

// ReloadMetadata reconciles a sidecar already atomically saved by the common
// document/file transaction. It never writes the sidecar or invents frame IDs.
func (p *Pipeline) ReloadMetadata(id ids.AssetID) (*Batch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.requireNoPendingSourceRecovery(); err != nil {
		return nil, err
	}
	if p.shared {
		return nil, errs.New(errs.AssetBadMeta, "another process holds the asset index")
	}
	r := p.byID[id]
	if r == nil || r.meta == nil {
		return nil, errs.New(errs.AssetNotFound, "metadata source is not in this project")
	}
	if err := p.reloadMetadata(r); err != nil {
		return nil, err
	}
	list := []ids.AssetID{id}
	list = append(list, p.dependents(id)...)
	// CacheSalters include indirect frame dependencies and private channel metadata.
	for source, record := range p.byID {
		if source != id {
			if _, ok := record.importer.(CacheSalter); ok && !slices.Contains(list, source) {
				list = append(list, source)
			}
		}
	}
	slices.SortFunc(list, func(a, b ids.AssetID) int { return strings.Compare(p.byID[a].path, p.byID[b].path) })
	return p.importIDs(list...)
}

func (p *Pipeline) reloadMetadata(r *record) error {
	data, err := p.read(r.path + MetaSuffix)
	if err != nil {
		return err
	}
	return p.reloadMetadataData(r, data, nil)
}

// RefreshMetadataSnapshot adopts an explicitly reloaded, exact disk snapshot
// into the in-memory index only. It does not import, write metadata, publish
// artifacts or mark them current. External changes remain best-effort checks,
// not CAS. Callers must guard the owning document, preserve dirty work and not
// mutate expected while this call captures it.
func (p *Pipeline) RefreshMetadataSnapshot(id ids.AssetID, expected []byte) error {
	if expected == nil || len(expected) > maxSourceBytes {
		return errs.New(errs.AssetBadMeta, "metadata refresh needs a bounded disk snapshot")
	}
	expected = bytes.Clone(expected)
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.requireNoPendingSourceRecovery(); err != nil {
		return err
	}
	r := p.byID[id]
	if r == nil || r.meta == nil || r.importer == nil {
		return errs.New(errs.AssetNotFound, "metadata source is not in this project")
	}
	root, err := os.OpenRoot(p.root)
	if err != nil {
		return err
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil {
		return err
	}
	if p.rootInfo == nil || !os.SameFile(p.rootInfo, opened) {
		return fmt.Errorf("project root changed since the pipeline was opened")
	}
	check := func() error {
		if err := p.requireNoPendingSourceRecovery(); err != nil {
			return err
		}
		current, err := os.Stat(p.root)
		if err != nil || !opened.IsDir() || !current.IsDir() || !os.SameFile(opened, current) {
			return fmt.Errorf("project root changed while refreshing metadata")
		}
		data, err := p.sourceBytes(root, r.path+MetaSuffix)
		if err != nil {
			return err
		}
		if data == nil || !bytes.Equal(data, expected) {
			return errs.New(errs.AssetSourceUnstable, "metadata changed while refreshing its snapshot", log.F("path", r.path))
		}
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	return p.reloadMetadataData(r, expected, check)
}

func (p *Pipeline) reloadMetadataData(r *record, data []byte, check func() error) error {
	m, err := ParseMeta(data, r.path+MetaSuffix)
	if err != nil {
		return err
	}
	if m.ID != r.id {
		return errs.New(errs.AssetBadMeta, "saving metadata cannot replace the source AssetID")
	}
	if m.Importer != "" && m.Importer != r.importer.ID() {
		return errs.New(errs.AssetBadMeta, "metadata importer does not match the source")
	}
	m.Importer = r.importer.ID()
	for key, value := range r.importer.Defaults() {
		if _, ok := m.Settings[key]; !ok {
			m.Settings[key] = value
		}
	}
	seen := map[ids.AssetID]bool{m.ID: true}
	for _, sub := range m.Subs {
		if sub.ID.IsZero() || seen[sub.ID] {
			return errs.New(errs.AssetBadMeta, "frame AssetIDs must be distinct and nonzero")
		}
		if other, ok := p.pathOf(sub.ID); ok && other != r.path+SubSep+sub.Name {
			own := false
			for _, old := range r.meta.Subs {
				own = own || old.ID == sub.ID
			}
			if !own {
				return errs.New(errs.AssetBadMeta, "frame AssetID belongs to another source")
			}
		}
		seen[sub.ID] = true
	}
	if check != nil {
		if err := check(); err != nil {
			return err
		}
	}
	if bytes.Equal(WriteMeta(r.meta), WriteMeta(m)) {
		r.captureMetadata(data)
		return nil
	}
	r.meta, r.key, r.derived = m, "", false
	r.captureMetadata(data)
	if r.group != "" {
		p.groupKey[r.group] = ""
	}
	p.rev++
	return nil
}
