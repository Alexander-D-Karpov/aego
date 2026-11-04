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
	"strings"

	"aego/core/ids"
)

// SourceEdit uses nil for a missing file; the exact prior bytes guard a delayed
// Apply, Undo or Redo against overwriting somebody else's work.
type SourceEdit struct {
	Path          string
	Before, After []byte
}

const maxSourceBytes = 16 << 20

func (p *Pipeline) NewSourceMeta(rel string) (*Meta, error) {
	imp, ok := p.imps.ForPath(rel)
	if !ok {
		return nil, fmt.Errorf("no importer for %s", rel)
	}
	return NewMeta(imp.ID(), imp.Version(), imp.Defaults()), nil
}

func (p *Pipeline) SourceBytes(rel string) ([]byte, error) {
	root, err := os.OpenRoot(p.project.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return p.sourceBytes(root, rel)
}

func (p *Pipeline) sourceBytes(root *os.Root, rel string) ([]byte, error) {
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
	b, err := io.ReadAll(io.LimitReader(f, maxSourceBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxSourceBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", rel, maxSourceBytes)
	}
	return b, nil
}

func sameSource(a, b []byte) bool { return (a == nil) == (b == nil) && bytes.Equal(a, b) }

func replaceSource(root *os.Root, rel string, data []byte) error {
	if data == nil {
		err := root.Remove(rel)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
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
	defer root.Remove(tmp)
	_, err = f.Write(data)
	if err = errors.Join(err, f.Close()); err != nil {
		return err
	}
	return root.Rename(tmp, rel)
}

func (p *Pipeline) ApplySources(edits []SourceEdit, back bool) (*Batch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.shared || p.mode == MetaReadOnly {
		return nil, errors.New("asset index is read-only")
	}
	root, err := os.OpenRoot(p.project.Root)
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
	for _, e := range edits {
		if seen[e.Path] {
			return nil, fmt.Errorf("duplicate source edit %s", e.Path)
		}
		seen[e.Path] = true
		was, _ := source(e)
		got, err := p.sourceBytes(root, e.Path)
		if err != nil {
			return nil, err
		}
		if !sameSource(was, got) {
			return nil, fmt.Errorf("%s changed since this edit was prepared", e.Path)
		}
	}
	var done []SourceEdit
	refresh := func() (*Batch, error) {
		if len(paths) == 0 {
			return &Batch{}, nil
		}
		b := p.applyChangesLocked(paths)
		if b == nil {
			// Deleting a source restored before it was indexed has no import work.
			for rel := range paths {
				if _, err := root.Stat(rel); err == nil {
					return nil, fmt.Errorf("could not refresh %s", rel)
				}
			}
			return &Batch{}, nil
		}
		for rel := range paths {
			if r := p.byPath[rel]; r != nil {
				if err := b.Failed[r.id]; err != nil {
					return nil, err
				}
				if slices.Contains(b.Retry, r.id) {
					return nil, fmt.Errorf("source changed during import: %s", rel)
				}
			} else if _, err := root.Stat(rel); err == nil {
				return nil, fmt.Errorf("%s was not indexed", rel)
			}
		}
		return b, nil
	}
	restore := func(cause error) error {
		for _, e := range slices.Backward(done) {
			was, _ := source(e)
			cause = errors.Join(cause, replaceSource(root, e.Path, was))
		}
		clear(paths)
		for _, e := range done {
			paths[strings.TrimSuffix(e.Path, MetaSuffix)] = ChangeWrite
		}
		_, e := refresh()
		return errors.Join(cause, e)
	}
	for _, e := range edits {
		was, want := source(e)
		if sameSource(was, want) {
			continue
		}
		rel := strings.TrimSuffix(e.Path, MetaSuffix)
		if err := replaceSource(root, e.Path, want); err != nil {
			return nil, restore(err)
		}
		done = append(done, e)
		if want == nil {
			paths[rel] = ChangeRemove
		} else {
			paths[rel] = ChangeWrite
		}
	}
	batch, err := refresh()
	if err != nil {
		return nil, restore(err)
	}
	// Reference discovery during import may canonicalize a sidecar. History
	// must compare against those actual published bytes on the next operation.
	for i := range edits {
		e := &edits[i]
		if !strings.HasSuffix(e.Path, MetaSuffix) {
			continue
		}
		body, readErr := p.sourceBytes(root, e.Path)
		if readErr != nil {
			return nil, restore(readErr)
		}
		if back {
			e.Before = body
		} else {
			e.After = body
		}
	}
	return batch, nil
}
