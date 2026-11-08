package pipeline

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"aego/core/ids"
	"aego/core/log"
)

// SourceEdit uses nil for a missing file; the exact prior bytes guard a delayed
// Apply, Undo or Redo against overwriting somebody else's work.
type SourceEdit struct {
	Path          string
	Before, After []byte
	// ReadOnlyGuard forbids pipeline metadata normalization as well as an
	// explicit write. Before and After must be identical, including nilness.
	ReadOnlyGuard bool
}

const (
	maxSourceBytes         = 16 << 20
	maxImporterSourceBytes = 64 << 20
)

func (p *Pipeline) sourceEditLimit(rel string) int64 {
	limit := int64(maxSourceBytes)
	if p.imps == nil || strings.HasSuffix(rel, MetaSuffix) {
		return limit
	}
	if imp, ok := p.imps.ForPath(rel); ok {
		if bounded, ok := imp.(SourceEditLimiter); ok {
			requested := bounded.SourceEditLimit()
			if requested > 0 {
				limit = min(requested, int64(maxImporterSourceBytes))
			}
		}
	}
	return limit
}

func (p *Pipeline) NewSourceMeta(rel string) (*Meta, error) {
	imp, ok := p.imps.ForPath(rel)
	if !ok {
		return nil, fmt.Errorf("no importer for %s", rel)
	}
	return NewMeta(imp.ID(), imp.Version(), imp.Defaults()), nil
}

func (p *Pipeline) SourceBytes(rel string) ([]byte, error) {
	root, err := os.OpenRoot(p.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return p.sourceBytes(root, rel)
}

func (p *Pipeline) sourceBytes(root *os.Root, rel string) ([]byte, error) {
	return p.sourceBytesWithLimit(root, rel, p.sourceEditLimit(rel))
}

func recoverySourceLimit(rel string) int64 {
	if strings.HasSuffix(rel, MetaSuffix) {
		return maxSourceBytes
	}
	return maxImporterSourceBytes
}

func (p *Pipeline) recoverySourceBytes(root *os.Root, rel string) ([]byte, error) {
	return p.sourceBytesWithLimit(root, rel, recoverySourceLimit(rel))
}

func (p *Pipeline) sourceBytesWithLimit(root *os.Root, rel string, limit int64) ([]byte, error) {
	if !fs.ValidPath(rel) || !filepath.IsLocal(rel) || strings.ContainsAny(rel, "\\:#") || !p.inRoots(rel) {
		return nil, fmt.Errorf("not a project source path: %s", rel)
	}
	current := ""
	for _, part := range strings.Split(rel, "/") {
		if strings.HasPrefix(part, ".") {
			return nil, fmt.Errorf("hidden asset path: %s", rel)
		}
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refusing symlink in asset path: %s", rel)
		}
		if current == rel && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file", rel)
		}
	}
	f, err := root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", rel, limit)
	}
	return b, nil
}

func sameSource(a, b []byte) bool { return (a == nil) == (b == nil) && bytes.Equal(a, b) }

// removeSourceTemp checks the last observed identity, metadata and bytes before
// cleanup. The final remove is not an atomic compare-and-delete.
func removeSourceTemp(root *os.Root, rel string, owned os.FileInfo, written []byte) error {
	current, err := root.Lstat(rel)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if owned == nil || !current.Mode().IsRegular() || !os.SameFile(owned, current) || owned.Mode() != current.Mode() || owned.Size() != current.Size() || !owned.ModTime().Equal(current.ModTime()) {
		return fmt.Errorf("temporary source file changed; refusing to remove %s", rel)
	}
	data, err := readJournalFile(root, rel, int64(len(written)))
	if err != nil || !bytes.Equal(data, written) {
		return fmt.Errorf("temporary source file content changed or cannot be checked; refusing to remove %s", rel)
	}
	current, err = root.Lstat(rel)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(owned, current) || owned.Mode() != current.Mode() || owned.Size() != current.Size() || !owned.ModTime().Equal(current.ModTime()) {
		return fmt.Errorf("temporary source file changed; refusing to remove %s", rel)
	}
	return root.Remove(rel)
}

func replaceSource(root *os.Root, rel string, data []byte) (result error) {
	if data == nil {
		err := root.Remove(rel)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		return syncSourceDirs(root, path.Dir(rel))
	}
	dir := path.Dir(rel)
	if err := root.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := path.Join(dir, ".aego-"+ids.NewAssetID().String()+".tmp")
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	var owned os.FileInfo
	written := 0
	published := false
	defer func() {
		if !published {
			result = errors.Join(result, removeSourceTemp(root, tmp, owned, data[:written]))
		}
	}()
	written, writeErr := f.Write(data)
	syncErr := f.Sync()
	owned, statErr := f.Stat()
	if err = errors.Join(writeErr, syncErr, statErr, f.Close()); err != nil {
		return err
	}
	if err := root.Rename(tmp, rel); err != nil {
		return err
	}
	published = true
	return syncSourceDirs(root, dir)
}

func (p *Pipeline) ApplySources(edits []SourceEdit, back bool) (*Batch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.shared || p.mode == MetaReadOnly {
		return nil, errors.New("asset index is read-only")
	}
	if err := p.requireNoPendingSourceRecovery(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(p.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	source := func(e SourceEdit) ([]byte, []byte) {
		if back {
			return e.After, e.Before
		}
		return e.Before, e.After
	}
	paths := map[string]ChangeKind{}
	seen := map[string]bool{}
	explicit := make(map[string]SourceEdit, len(edits))
	for _, e := range edits {
		if seen[e.Path] {
			return nil, fmt.Errorf("duplicate source edit %s", e.Path)
		}
		seen[e.Path] = true
		explicit[e.Path] = e
		if e.ReadOnlyGuard && !sameSource(e.Before, e.After) {
			return nil, fmt.Errorf("read-only source guard %s cannot request a write", e.Path)
		}
		// Rollback uses the same bounded reader as the preflight guard. Never
		// publish a value that cannot subsequently be checked and restored.
		limit := p.sourceEditLimit(e.Path)
		if int64(len(e.Before)) > limit || int64(len(e.After)) > limit {
			return nil, fmt.Errorf("%s exceeds %d bytes", e.Path, limit)
		}
		was, _ := source(e)
		got, err := p.sourceBytes(root, e.Path)
		if err != nil {
			return nil, err
		}
		if !sameSource(was, got) {
			return nil, fmt.Errorf("%s changed since this edit was prepared", e.Path)
		}
	}
	// Source importers may resolve and persist references in an existing
	// sidecar even when a caller only edits the source. Include such sidecars
	// as implicit guards so a failed import can undo our normalization and an
	// external metadata edit cannot be silently accepted or overwritten. A new
	// imported source gets its sidecar in this transaction rather than leaving
	// an untracked sidecar behind if its first import fails.
	// Clone the slice: callers own their transaction and history entries.
	callerEdits := edits
	edits = slices.Clone(edits)
	for _, e := range callerEdits {
		was, want := source(e)
		metaPath := e.Path + MetaSuffix
		if strings.HasSuffix(e.Path, MetaSuffix) || sameSource(was, want) {
			continue
		}
		if metaEdit, supplied := explicit[metaPath]; supplied {
			_, metaWant := source(metaEdit)
			if was == nil && want != nil && metaWant == nil {
				if _, imported := p.imps.ForPath(e.Path); imported {
					return nil, fmt.Errorf("%s cannot remain absent when creating an imported source", metaPath)
				}
			}
			continue
		}
		if seen[metaPath] {
			continue
		}
		meta, err := p.sourceBytes(root, metaPath)
		if err != nil {
			return nil, err
		}
		if meta != nil {
			edits = append(edits, SourceEdit{Path: metaPath, Before: meta, After: meta})
			seen[metaPath] = true
		} else if was == nil && want != nil {
			if imp, ok := p.imps.ForPath(e.Path); ok {
				created := WriteMeta(NewMeta(imp.ID(), imp.Version(), imp.Defaults()))
				implicit := SourceEdit{Path: metaPath}
				if back {
					implicit.Before = created
				} else {
					implicit.After = created
				}
				edits = append(edits, implicit)
				seen[metaPath] = true
			}
		}
	}
	changed := false
	for _, e := range edits {
		was, want := source(e)
		changed = changed || !sameSource(was, want)
	}
	if !changed {
		return &Batch{}, nil
	}
	journal, err := startSourceJournal(root, edits, back)
	if err != nil {
		return nil, err
	}
	journal.checkpoint = p.sourceTransactionCheckpoint
	p.sourceJournal, p.sourceJournalRoot = journal, root
	defer func() {
		p.sourceJournal, p.sourceJournalRoot = nil, nil
	}()
	if p.sourceTransactionCheckpoint != nil {
		p.sourceTransactionCheckpoint("prepared", "")
	}
	// Import may update the shared index before a later member of the batch
	// fails. Keep the previous generation so conflicts can fail closed without
	// reimporting an externally changed file as part of rollback.
	indexBefore := p.index.Clone()
	p.sourceTransactionMetas = make(map[string][]byte)
	p.sourceTransactionReadOnlyMetas = make(map[string]bool)
	for _, e := range edits {
		if strings.HasSuffix(e.Path, MetaSuffix) {
			_, want := source(e)
			p.sourceTransactionMetas[e.Path] = want
			p.sourceTransactionReadOnlyMetas[e.Path] = e.ReadOnlyGuard
		}
	}
	defer func() {
		p.sourceTransactionMetas = nil
		p.sourceTransactionReadOnlyMetas = nil
	}()
	var done []SourceEdit
	refresh := func() (*Batch, error) {
		if len(paths) == 0 {
			return &Batch{}, nil
		}
		b, err := p.applyChangesLocked(paths)
		if err != nil {
			return nil, fmt.Errorf("source transaction import: %w", err)
		}
		if b == nil {
			// Deleting a source restored before it was indexed has no import work.
			for rel := range paths {
				if _, err := root.Stat(rel); err == nil {
					return nil, fmt.Errorf("could not refresh %s", rel)
				}
			}
			return &Batch{}, nil
		}
		// A batch includes dependent imports and sub-assets as well as the
		// explicitly edited sources. Accepting only the main AssetID failures
		// can publish an incomplete generation while reporting success.
		failed := make([]ids.AssetID, 0, len(b.Failed))
		for id := range b.Failed {
			failed = append(failed, id)
		}
		sort.Slice(failed, func(i, j int) bool { return failed[i].String() < failed[j].String() })
		var problems []error
		for _, id := range failed {
			if failure := b.Failed[id]; failure != nil {
				problems = append(problems, fmt.Errorf("asset %s import failed: %w", id, failure))
			} else {
				problems = append(problems, fmt.Errorf("asset %s import failed", id))
			}
		}
		for _, id := range b.Retry {
			problems = append(problems, fmt.Errorf("asset %s needs retry after an unstable import", id))
		}
		if len(problems) != 0 {
			return nil, errors.Join(problems...)
		}
		for rel := range paths {
			if p.byPath[rel] == nil {
				if _, err := root.Stat(rel); err != nil {
					continue
				}
				return nil, fmt.Errorf("%s was not indexed", rel)
			}
		}
		return b, nil
	}
	restore := func(cause error, conflict bool) error {
		published := make(map[string]bool, len(done))
		for _, e := range done {
			published[e.Path] = true
		}
		var restored []string
		for _, e := range slices.Backward(edits) {
			if err := journal.checkDirectory(); err != nil {
				conflict = true
				cause = errors.Join(cause, err)
				break
			}
			was, want := source(e)
			wrote := published[e.Path]
			expected := was
			if wrote || sameSource(was, want) {
				expected = want
				if resolved, tracked := p.sourceTransactionMetas[e.Path]; tracked {
					expected = resolved
				}
				// Even an unchanged metadata guard can be canonicalized by the
				// resolver. That is our write, not an external modification.
				wrote = wrote || !sameSource(expected, was)
			}
			got, err := p.sourceBytes(root, e.Path)
			if err != nil {
				conflict = true
				cause = errors.Join(cause, fmt.Errorf("%s changed or cannot be checked before rollback: %w", e.Path, err))
				continue
			}
			if !sameSource(got, expected) {
				conflict = true
				cause = errors.Join(cause, fmt.Errorf("%s changed during this edit; refusing to overwrite it during rollback", e.Path))
				continue
			}
			if !wrote || sameSource(got, was) {
				continue
			}
			if err := replaceSource(root, e.Path, was); err != nil {
				conflict = true
				cause = errors.Join(cause, fmt.Errorf("cannot roll back %s: %w", e.Path, err))
			} else {
				restored = append(restored, e.Path)
				if _, tracked := p.sourceTransactionMetas[e.Path]; tracked {
					p.sourceTransactionMetas[e.Path] = was
				}
			}
		}
		if conflict {
			// A scan/import here can recreate a deleted sidecar or follow a path
			// replaced by a symlink. Preserve the last index generation instead;
			// strict export must reject the touched records until they are resolved
			// and reimported. This is not crash recovery.
			*p.index = *indexBefore
			for _, e := range edits {
				rel := strings.TrimSuffix(e.Path, MetaSuffix)
				if r := p.byPath[rel]; r != nil {
					r.key = ""
					r.err = "source changed during an edit; resolve the conflict and reimport"
					if r.group != "" {
						p.groupKey[r.group] = ""
					}
				}
			}
			p.rev++
			return errors.Join(cause, p.saveIndex())
		}
		clear(paths)
		for _, name := range restored {
			paths[strings.TrimSuffix(name, MetaSuffix)] = ChangeWrite
		}
		_, e := refresh()
		if e != nil {
			return errors.Join(cause, e)
		}
		for _, edit := range edits {
			was, _ := source(edit)
			got, err := p.sourceBytes(root, edit.Path)
			if err != nil || !sameSource(was, got) {
				return errors.Join(cause, fmt.Errorf("%s was not fully restored; source recovery is still available", edit.Path))
			}
		}
		return errors.Join(cause, journal.finish("restored"))
	}
	for _, e := range edits {
		if err := journal.checkDirectory(); err != nil {
			return nil, restore(err, true)
		}
		was, want := source(e)
		got, err := p.sourceBytes(root, e.Path)
		if err != nil {
			return nil, restore(fmt.Errorf("%s changed or cannot be checked before publication: %w", e.Path, err), true)
		}
		if !sameSource(was, got) {
			return nil, restore(fmt.Errorf("%s changed while this edit was being prepared", e.Path), true)
		}
		if sameSource(was, want) {
			continue
		}
		if err := journal.checkDirectory(); err != nil {
			return nil, restore(err, true)
		}
		rel := strings.TrimSuffix(e.Path, MetaSuffix)
		if err := replaceSource(root, e.Path, want); err != nil {
			return nil, restore(err, false)
		}
		done = append(done, e)
		if p.sourceTransactionCheckpoint != nil {
			p.sourceTransactionCheckpoint("published", e.Path)
		}
		if want == nil {
			paths[rel] = ChangeRemove
		} else {
			paths[rel] = ChangeWrite
		}
	}
	batch, err := refresh()
	if err != nil {
		return nil, restore(err, false)
	}
	publishedMetas := make(map[int][]byte)
	for i, e := range edits {
		_, want := source(e)
		if expected, tracked := p.sourceTransactionMetas[e.Path]; tracked {
			want = expected
		}
		got, readErr := p.sourceBytes(root, e.Path)
		if readErr != nil {
			return nil, restore(fmt.Errorf("%s changed or cannot be checked after import: %w", e.Path, readErr), true)
		}
		if !sameSource(want, got) {
			return nil, restore(fmt.Errorf("%s changed during import", e.Path), true)
		}
		if strings.HasSuffix(e.Path, MetaSuffix) {
			publishedMetas[i] = got
		}
	}
	// Reference discovery during import may canonicalize a sidecar. History
	// must compare against those actual published bytes on the next operation.
	for i, body := range publishedMetas {
		if i >= len(callerEdits) {
			continue // Internal guards are not additional caller history entries.
		}
		e := &callerEdits[i]
		if back {
			e.Before = body
		} else {
			e.After = body
		}
	}
	if err := journal.finish("committed"); err != nil {
		if journal.state == "committed" {
			p.log.Warn("source files were committed but recovery cleanup needs attention", log.F("err", err.Error()))
		} else {
			return batch, fmt.Errorf("source files were published but their completion record failed; inspect source recovery before continuing: %w", err)
		}
	}
	return batch, nil
}
