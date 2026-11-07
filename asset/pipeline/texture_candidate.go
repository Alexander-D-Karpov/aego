package pipeline

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/vfs"
)

// PreviewMetadata builds an Undo/Redo surface generation for saved metadata using existing importers and group finalizers; artifacts live in a private VFS.
// Source, persistent import keys, index, sidecars and disk cache stay unchanged; callers must check document revision and device/context generation before publishing.
func (p *Pipeline) PreviewMetadata(id ids.AssetID, metadata []byte, overlay func(string) ([]byte, bool)) (*Batch, vfs.FS, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	owner := p.byID[id]
	if owner == nil {
		owner = p.subs[id]
	}
	if owner == nil || !workingSourceCompatible(owner.importer) {
		return nil, nil, errs.New(errs.AssetBadImporter, "working metadata candidates require a registered working-source importer")
	}
	target, err := ParseMeta(metadata, owner.path+MetaSuffix)
	if err != nil {
		return nil, nil, err
	}
	if target.ID != owner.id || target.Importer != owner.importer.ID() {
		return nil, nil, errs.New(errs.AssetBadMeta, "working metadata candidate cannot change source identity or importer")
	}
	snapshots := map[string][]byte{owner.path + MetaSuffix: slices.Clone(metadata)}
	fromDisk := map[string]bool{}
	fromOverlay := map[string]bool{}
	working := map[ids.AssetID]bool{owner.id: true}
	read := func(name string) ([]byte, error) {
		if b, ok := snapshots[name]; ok {
			return b, nil
		}
		if overlay != nil {
			if b, ok := overlay(name); ok {
				snapshots[name] = slices.Clone(b)
				fromOverlay[name] = true
				return snapshots[name], nil
			}
		}
		b, err := p.read(name)
		if err == nil {
			snapshots[name] = slices.Clone(b)
			fromDisk[name] = true
		}
		return b, err
	}
	files := &candidateFiles{data: map[string][]byte{}, fallback: p.artifacts}
	candidate := newPipeline(p.project, p.imps, nil)
	candidate.artifacts = files
	candidate.log = p.log
	candidate.index = p.index.Clone()
	for group, page := range p.pages {
		candidate.pages[group] = page
	}
	failures := map[ids.AssetID]error{}
	list := make([]ids.AssetID, 0, len(p.byID))
	for source, r := range p.byID {
		if !workingSourceCompatible(r.importer) {
			continue
		}
		copyRecord := *r
		copyRecord.subs = slices.Clone(r.subs)
		copyRecord.deps = slices.Clone(r.deps)
		encoded, readErr := read(r.path + MetaSuffix)
		m := r.meta
		if readErr == nil {
			m, readErr = ParseMeta(encoded, r.path+MetaSuffix)
		}
		if readErr == nil && (m.ID != source || m.Importer != r.importer.ID()) {
			readErr = errs.New(errs.AssetBadMeta, "candidate metadata changes source identity/importer")
		}
		if readErr != nil {
			failures[source] = readErr
			m, _ = ParseMeta(WriteMeta(r.meta), r.path+MetaSuffix)
		}
		for key, value := range r.importer.Defaults() {
			if _, ok := m.Settings[key]; !ok {
				m.Settings[key] = value
			}
		}
		// Explicit overlays also refresh saved bytes when a working owner closes.
		if fromOverlay[r.path+MetaSuffix] {
			working[source] = true
		}
		copyRecord.meta = m
		candidate.byID[source] = &copyRecord
		candidate.byPath[r.path] = &copyRecord
		refs := make([]subRef, 0, len(m.Subs))
		for _, frame := range m.Subs {
			refs = append(refs, subRef{id: frame.ID, name: frame.Name})
		}
		for _, ref := range refs {
			candidate.subs[ref.id] = &copyRecord
		}
		list = append(list, source)
	}
	slices.SortFunc(list, func(a, b ids.AssetID) int { return strings.Compare(candidate.byID[a].path, candidate.byID[b].path) })
	sourcePath := func(ref ids.AssetID) (string, bool) {
		if r := candidate.byID[ref]; r != nil {
			return r.path, true
		}
		if r := candidate.subs[ref]; r != nil {
			for _, f := range r.meta.Subs {
				if f.ID == ref {
					return r.path + SubSep + f.Name, true
				}
			}
		}
		return "", false
	}
	lookup := func(name string) (ids.AssetID, bool) {
		source, frame, sub := strings.Cut(name, SubSep)
		r := candidate.byPath[source]
		if r == nil {
			return ids.AssetID{}, false
		}
		if !sub {
			return r.id, true
		}
		for _, f := range r.meta.Subs {
			if f.Name == frame {
				return f.ID, true
			}
		}
		return ids.AssetID{}, false
	}
	results := map[ids.AssetID]*Result{}
	for _, source := range list {
		if failures[source] != nil {
			continue
		}
		r := candidate.byID[source]
		data, err := read(r.path)
		if fromOverlay[r.path] {
			working[source] = true
		}
		if err != nil {
			failures[source] = err
			continue
		}
		ctx := &ImportContext{ID: source, Path: r.path, Source: data, Meta: r.meta, Read: read, Lookup: lookup, SourcePath: sourcePath, Log: p.log}
		if resolver, ok := r.importer.(MetadataResolver); ok {
			settings, err := resolver.ResolveMetadata(ctx)
			if err != nil {
				failures[source] = err
				continue
			}
			if settings != nil {
				r.meta.Settings = settings
			}
		}
		result, err := r.importer.Import(ctx)
		if err != nil {
			failures[source] = err
			continue
		}
		if result == nil {
			failures[source] = fmt.Errorf("working-source importer returned no candidate")
			continue
		}
		results[source] = result
	}
	sourceOwner := func(ref ids.AssetID) ids.AssetID {
		if candidate.byID[ref] != nil {
			return ref
		}
		if r := candidate.subs[ref]; r != nil {
			return r.id
		}
		if r := p.subs[ref]; r != nil {
			return r.id
		}
		return ids.AssetID{}
	}
	selected := working
	groups := map[string]bool{}
	// Close both sides of surface relationships and every affected atlas group.
	// Unrelated invalid images do not prevent a valid working generation.
	for changed := true; changed; {
		changed = false
		mark := func(source ids.AssetID) {
			if !source.IsZero() && !selected[source] {
				selected[source] = true
				changed = true
			}
		}
		for _, source := range list {
			old := p.byID[source]
			res := results[source]
			deps := slices.Clone(old.deps)
			group := ""
			if res != nil {
				deps = append(deps, res.Deps...)
				group = res.Group
			}
			if selected[source] {
				if old.group != "" && !groups[old.group] {
					groups[old.group] = true
					changed = true
				}
				if group != "" && !groups[group] {
					groups[group] = true
					changed = true
				}
				for _, dep := range deps {
					mark(sourceOwner(dep))
				}
			}
			if (old.group != "" && groups[old.group]) || (group != "" && groups[group]) {
				mark(source)
			}
			for _, dep := range deps {
				if selected[sourceOwner(dep)] {
					mark(source)
					break
				}
			}
		}
	}
	for _, source := range list {
		if selected[source] && failures[source] != nil {
			return nil, nil, fmt.Errorf("%s: %w", candidate.byID[source].path, failures[source])
		}
	}
	batch := &Batch{Failed: map[ids.AssetID]error{}}
	for _, source := range list {
		if !selected[source] {
			continue
		}
		r, res := candidate.byID[source], results[source]
		if res.Group != "" {
			candidate.addToGroup(res.Group, source, res)
			r.group = res.Group
			groups[res.Group] = true
			continue
		}
		r.group = ""
		entry, err := candidate.writeArtifacts(r, res)
		if err != nil {
			return nil, nil, err
		}
		candidate.index.Put(entry)
		batch.Imported = append(batch.Imported, entry)
		if len(res.Subs) > 0 || len(r.subs) > 0 {
			candidate.putSubs(r, res.Subs, batch)
		}
	}
	groupNames := make([]string, 0, len(groups))
	for name := range groups {
		groupNames = append(groupNames, name)
	}
	slices.Sort(groupNames)
	for _, group := range groupNames {
		oldPage, hadPage := p.pages[group]
		entries, failed, err := candidate.finalizeGroup(group)
		if err != nil {
			return nil, nil, err
		}
		for _, err := range failed {
			return nil, nil, err
		}
		batch.Imported = append(batch.Imported, entries...)
		if hadPage && len(candidate.groups[group]) == 0 {
			batch.Removed = append(batch.Removed, oldPage)
		}
	}
	if len(batch.Failed) > 0 {
		for _, err := range batch.Failed {
			return nil, nil, err
		}
	}
	// A file edited during background preparation must not publish stale pixels.
	for name, old := range snapshots {
		if !fromDisk[name] {
			continue
		}
		source, _, _ := strings.Cut(strings.TrimSuffix(name, MetaSuffix), SubSep)
		r := candidate.byPath[source]
		if r == nil || !selected[r.id] {
			continue
		}
		now, err := p.read(name)
		if err != nil || !bytes.Equal(now, old) {
			return nil, nil, errs.New(errs.AssetSourceUnstable, "surface source changed while preparing the working candidate")
		}
	}
	files.sealed = true
	return batch, files, nil
}

// candidateFiles is a private artifact overlay, not a second asset pipeline.
// It exists only as long as a loader retains the published working generation.
type candidateFiles struct {
	data     map[string][]byte
	fallback vfs.FS
	sealed   bool
}
type candidateFile struct{ *bytes.Reader }

func (candidateFile) Close() error { return nil }
func (f *candidateFiles) ReadFile(name string) ([]byte, error) {
	if b, ok := f.data[name]; ok {
		return b, nil
	}
	return f.fallback.ReadFile(name)
}
func (f *candidateFiles) Open(name string) (vfs.File, error) {
	if b, ok := f.data[name]; ok {
		return candidateFile{bytes.NewReader(b)}, nil
	}
	return f.fallback.Open(name)
}
func (f *candidateFiles) Exists(name string) bool {
	_, ok := f.data[name]
	return ok || f.fallback.Exists(name)
}
func (f *candidateFiles) WriteFile(name string, data []byte) error {
	if f.sealed {
		return errs.New(errs.VFSReadOnly, "working surface artifacts are immutable")
	}
	f.data[name] = slices.Clone(data)
	return nil
}
