package pipeline

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"os"
	"slices"
	"strings"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/project"
)

type MetaMode uint8

const (
	MetaEnsure MetaMode = iota
	MetaReadOnly
)

type MetaIssueKind uint8

const (
	MetaMissing MetaIssueKind = iota
	MetaOrphan
	MetaDuplicate
	MetaStale
)

var metaCodes = [...]string{"META001", "META002", "META003", "META004"}

type MetaIssue struct {
	Kind  MetaIssueKind
	Path  string
	Other string
	ID    ids.AssetID
}

func (i MetaIssue) Code() string {
	return metaCodes[i.Kind]
}

func (i MetaIssue) Message() string {
	switch i.Kind {
	case MetaMissing:
		return i.Path + " has no .meta file; its asset id exists only in memory"
	case MetaOrphan:
		return i.Path + " has no asset next to it"
	case MetaDuplicate:
		return i.Path + " uses the same asset id as " + i.Other + ", it was probably copied together with its .meta"
	}
	return "the asset index lists an asset for " + i.Path + " whose source or identity could not be matched"
}

func (i MetaIssue) Action() string {
	switch i.Kind {
	case MetaMissing:
		return "create " + i.Path + MetaSuffix
	case MetaOrphan:
		return "delete " + i.Path
	case MetaDuplicate:
		return "give " + i.Path + " a new asset id"
	}
	return "drop the indexed asset ID for " + i.Path
}

type walk struct {
	seen  map[string]bool
	files map[string]bool
	metas map[string]bool
}

func newWalk() *walk {
	return &walk{seen: map[string]bool{}, files: map[string]bool{}, metas: map[string]bool{}}
}

func derivedID(rel string) ids.AssetID {
	sum := sha256.Sum256([]byte("aego.meta/" + rel))
	return ids.AssetID{Hi: binary.BigEndian.Uint64(sum[0:8]), Lo: binary.BigEndian.Uint64(sum[8:16])}
}

func (p *Pipeline) SetMetaMode(m MetaMode) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.shared {
		p.mode = m
	}
}

func (p *Pipeline) MetaMode() MetaMode {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.mode
}

func (p *Pipeline) Shared() bool {
	return p.shared
}

func (p *Pipeline) MetaIssues() ([]MetaIssue, uint64) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.issues, p.issueRev
}

func (p *Pipeline) settle(r, other *record, added *[]ids.AssetID) {
	e, ok := p.index.Lookup(other.id)
	if !ok || e.Source != r.path || e.Source == other.path {
		r.dup, r.derived = other.path, true
		r.meta.ID = derivedID(r.path)
		return
	}
	old := other.id
	if other.group != "" {
		p.dropFromGroup(other.group, old)
	}
	delete(p.byID, old)
	other.id = derivedID(other.path)
	other.meta.ID = other.id
	other.dup, other.derived, other.key = r.path, true, ""
	p.byID[other.id] = other
	for i, id := range *added {
		if id == old {
			(*added)[i] = other.id
		}
	}
}

func (p *Pipeline) audit() {
	var out []MetaIssue
	for _, r := range p.byID {
		switch {
		case r.dup != "":
			out = append(out, MetaIssue{Kind: MetaDuplicate, Path: r.path, Other: r.dup, ID: r.id})
		case r.derived:
			out = append(out, MetaIssue{Kind: MetaMissing, Path: r.path, ID: r.id})
		}
	}
	for m := range p.metas {
		if !p.files[strings.TrimSuffix(m, MetaSuffix)] {
			out = append(out, MetaIssue{Kind: MetaOrphan, Path: m})
		}
	}
	p.index.Each(func(e *asset.Entry) {
		if e.Missing || strings.Contains(e.Source, ":") || p.byID[e.ID] != nil {
			return
		}
		// putSubs records the owning source as the first dependency. Some
		// importers derive sub-asset IDs without listing them in .meta, so a
		// fresh scan cannot know all of them until the owner has been imported.
		// After import, only its registered sub-assets qualify at the next audit.
		// A path match alone is insufficient: the source's identity might have
		// changed, or its metadata might no longer be readable.
		if p.files[e.Source] && len(e.Deps) > 0 {
			owner := p.byID[e.Deps[0]]
			if owner != nil && owner.path == e.Source && (owner.key == "" || p.subs[e.ID] == owner) {
				return
			}
		}
		out = append(out, MetaIssue{Kind: MetaStale, Path: e.Source, ID: e.ID})
	})
	slices.SortFunc(out, func(a, b MetaIssue) int {
		if a.Kind != b.Kind {
			return int(a.Kind) - int(b.Kind)
		}
		return strings.Compare(a.Path, b.Path)
	})
	if !slices.Equal(out, p.issues) {
		p.issues = out
		p.issueRev++
	}
}

func (p *Pipeline) Repair(list []MetaIssue) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.requireNoPendingSourceRecovery(); err != nil {
		return 0, err
	}
	if p.shared {
		return 0, errs.New(errs.AssetBadMeta, "another process holds the asset index, repair the meta files there")
	}
	n := 0
	var all []error
	for _, is := range list {
		var err error
		switch is.Kind {
		case MetaMissing, MetaDuplicate:
			r := p.byPath[is.Path]
			if r == nil || !r.derived {
				continue
			}
			candidate := WriteMeta(r.meta)
			if err = p.checkMetadataSnapshot(r); err == nil {
				var root *os.Root
				root, err = os.OpenRoot(p.root)
				if err == nil {
					if err = p.checkMetadataSnapshot(r); err == nil {
						err = replaceSource(root, r.path+MetaSuffix, candidate)
					}
					if closeErr := root.Close(); err == nil {
						err = closeErr
					}
				}
			}
			if err == nil {
				r.captureMetadata(candidate)
				r.derived, r.dup = false, ""
				p.metas[r.path+MetaSuffix] = true
			}
		case MetaOrphan:
			if err = os.Remove(p.abs(is.Path)); err == nil || os.IsNotExist(err) {
				err = nil
				delete(p.metas, is.Path)
			}
		case MetaStale:
			p.index.Delete(is.ID)
		}
		if err != nil {
			all = append(all, errs.Wrap(err, errs.VFSIO, "cannot repair "+is.Path).With(log.F("path", is.Path)))
			continue
		}
		n++
	}
	p.audit()
	p.rev++
	if err := p.saveIndex(); err != nil {
		all = append(all, err)
	}
	return n, errors.Join(all...)
}

func Audit(pr *project.Project, imps *ImporterRegistry, mode MetaMode) ([]MetaIssue, error) {
	p := newPipeline(pr.Root, imps, nil)
	p.mode, p.shared = mode, true
	if _, _, err := p.scan(); err != nil {
		return nil, err
	}
	return p.issues, nil
}
