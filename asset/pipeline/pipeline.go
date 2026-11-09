package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/engine/res"
	"aego/project"
	"aego/vfs"
)

const PipelineVersion = 1

const ServiceID = "asset.pipeline"

type Batch struct {
	Imported []asset.Entry
	Failed   map[ids.AssetID]error
	Removed  []ids.AssetID
	Retry    []ids.AssetID
}

func (b *Batch) Empty() bool {
	return len(b.Imported) == 0 && len(b.Failed) == 0 && len(b.Removed) == 0
}

type subRef struct {
	id   ids.AssetID
	name string
}

type record struct {
	subs     []subRef
	id       ids.AssetID
	path     string
	meta     *Meta
	importer Importer
	key      string
	group    string
	deps     []ids.AssetID
	err      string
	derived  bool
	dup      string
}

type Pipeline struct {
	// Paths belong to the opened pipeline, not to the mutable Project value
	// that Project Settings can replace while preview workers are running.
	root         string
	cacheDir     string
	imps         *ImporterRegistry
	log          *log.ChannelLogger
	watcher      Watcher
	index        *asset.Index
	artifacts    vfs.FS
	byID         map[ids.AssetID]*record
	byPath       map[string]*record
	subs         map[ids.AssetID]*record
	groups       map[string][]ids.AssetID
	groupKey     map[string]string
	groupResults map[string]map[ids.AssetID]*Result
	pages        map[string]ids.AssetID
	stale        map[string]bool
	roots        []string
	mode         MetaMode
	shared       bool
	unlock       func()
	files        map[string]bool
	metas        map[string]bool
	issues       []MetaIssue
	issueRev     uint64
	mu           sync.RWMutex
	pmu          sync.Mutex
	rev          uint64
	graves       []Grave
	gravesDirty  bool
	// Non-nil only while ApplySources holds mu. Resolver writes must be chained
	// from these exact sidecar bytes and update them after publication.
	sourceTransactionMetas         map[string][]byte
	sourceTransactionReadOnlyMetas map[string]bool
	sourceJournal                  *sourceJournal
	sourceJournalRoot              *os.Root
	// A synchronous seam for deterministic interruption tests; nil in normal use.
	sourceTransactionCheckpoint func(stage, rel string)
}

func newPipeline(root string, imps *ImporterRegistry, lg *log.Logger) *Pipeline {
	if lg == nil {
		lg = log.Nop()
	}
	p := &project.Project{Root: root}
	return &Pipeline{
		root:         root,
		cacheDir:     p.CacheDir(),
		imps:         imps,
		log:          lg.With(log.Asset),
		watcher:      PollWatcher{},
		index:        asset.NewIndex(),
		byID:         map[ids.AssetID]*record{},
		byPath:       map[string]*record{},
		subs:         map[ids.AssetID]*record{},
		groups:       map[string][]ids.AssetID{},
		groupKey:     map[string]string{},
		groupResults: map[string]map[ids.AssetID]*Result{},
		pages:        map[string]ids.AssetID{},
		stale:        map[string]bool{},
		files:        map[string]bool{},
		metas:        map[string]bool{},
		roots:        []string{p.AssetsDir(), p.ScenesDir(), p.ScriptsDir(), p.PrefabsDir()},
	}
}

// Option changes how Open treats the project.
type Option func(*options)

type options struct{ expectShared bool }

// ExpectShared is for a process the editor starts on its own project: the editor holding the index is normal there, so it is not a warning.
func ExpectShared() Option { return func(o *options) { o.expectShared = true } }

func Open(p *project.Project, imps *ImporterRegistry, lg *log.Logger, opts ...Option) (*Pipeline, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if p == nil {
		return nil, errs.New(errs.AssetNoProject, "pipeline needs a project")
	}
	cacheDir := p.CacheDir()
	if err := os.MkdirAll(filepath.Join(cacheDir, "artifacts"), 0o755); err != nil {
		return nil, errs.Wrap(err, errs.VFSIO, "cannot create the artifact cache")
	}
	files, err := vfs.New(vfs.Mount{Scheme: "cache", Root: cacheDir})
	if err != nil {
		return nil, err
	}
	pl := newPipeline(p.Root, imps, lg)
	pl.artifacts = files
	unlock, owner := acquireIndexLock(cacheDir)
	pl.unlock = unlock
	if !owner {
		pl.shared, pl.mode = true, MetaReadOnly
		say := pl.log.Warn
		if o.expectShared {
			say = pl.log.Debug
		}
		say("another process holds the asset index, .meta files and the index are read-only here", log.F("lock", filepath.Join(cacheDir, indexLock)))
	}
	if pending, err := sourceJournalPresent(pl.root); err != nil {
		unlock()
		return nil, err
	} else if pending {
		unlock()
		return nil, sourceRecoveryRequired()
	}
	if data, err := os.ReadFile(pl.indexPath()); err == nil {
		if idx, err := asset.ReadIndex(data, "index.aetext"); err == nil {
			pl.index = idx
		} else {
			pl.log.Warn("asset index is unreadable, rebuilding", log.F("err", err.Error()))
		}
	}
	pl.loadGraves()
	return pl, nil
}

func (p *Pipeline) SetWatcher(w Watcher) {
	if w != nil {
		p.watcher = w
	}
}

func (p *Pipeline) Index() *asset.Index {
	return p.index
}

func (p *Pipeline) Artifacts() vfs.FS {
	return p.artifacts
}

// ReadSnapshot prevents import commits while copying index/artifacts; its index is private and its VFS expires after the callback.
// The callback must not reenter the pipeline.
func (p *Pipeline) ReadSnapshot(read func(*asset.Index, vfs.FS)) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	read(p.index.Clone(), p.artifacts)
}

func (p *Pipeline) Revision() uint64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.rev
}

func (p *Pipeline) indexPath() string {
	return filepath.Join(p.cacheDir, "index.aetext")
}

func (p *Pipeline) saveIndex() error {
	if p.shared {
		return nil
	}
	// Do not truncate the last usable index if writing a newer generation fails.
	// The asset index lock is still required; this replacement is not a CAS.
	if err := project.WriteAtomic(p.indexPath(), asset.WriteIndex(p.index)); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write the asset index")
	}
	return nil
}

func (p *Pipeline) Lookup(path string) (ids.AssetID, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lookup(path)
}

// read gives an importer another source file: engine:// from the engine's embedded files, the rest from the project.
func (p *Pipeline) read(path string) ([]byte, error) {
	if rest, ok := strings.CutPrefix(path, "engine://"); ok {
		return fs.ReadFile(res.FS(), rest)
	}
	return os.ReadFile(p.Abs(path))
}

func (p *Pipeline) lookup(path string) (ids.AssetID, bool) {
	path = filepath.ToSlash(path)
	if sheet, name, ok := strings.Cut(path, SubSep); ok {
		if r, ok := p.byPath[sheet]; ok {
			for _, s := range r.subs {
				if s.name == name {
					return s.id, true
				}
			}
		}
		return ids.AssetID{}, false
	}
	r, ok := p.byPath[path]
	if !ok {
		return ids.AssetID{}, false
	}
	return r.id, true
}

func (p *Pipeline) PathOf(id ids.AssetID) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.pathOf(id)
}

func (p *Pipeline) pathOf(id ids.AssetID) (string, bool) {
	r, ok := p.byID[id]
	if !ok {
		if r, ok := p.subs[id]; ok {
			for _, s := range r.subs {
				if s.id == id {
					return r.path + SubSep + s.name, true
				}
			}
		}
		return "", false
	}
	return r.path, true
}

// SubSep joins a sheet path and a cell name: "assets/hero.png#hero_000".
const SubSep = "#"

func (p *Pipeline) setSubRefs(r *record, subs []subRef) {
	for _, s := range r.subs {
		delete(p.subs, s.id)
	}
	r.subs = subs
	for _, s := range subs {
		p.subs[s.id] = r
	}
}

// putSubs writes the sub-assets of an import and buries those that are gone.
func (p *Pipeline) putSubs(r *record, subs []SubResult, batch *Batch) {
	keep := make([]subRef, 0, len(subs))
	for _, sr := range subs {
		deps := append([]ids.AssetID{r.id}, sr.Deps...)
		entry, err := p.writeEntry(sr.ID, sr.Kind, assetName(r.path)+"/"+sr.Name, r.path, deps, sr.Artifacts)
		if err != nil {
			batch.Failed[sr.ID] = err
			continue
		}
		p.index.Put(entry)
		batch.Imported = append(batch.Imported, entry)
		keep = append(keep, subRef{id: sr.ID, name: sr.Name})
	}
	for _, old := range r.subs {
		if !slices.ContainsFunc(keep, func(s subRef) bool { return s.id == old.id }) {
			p.bury(old.id, r.path+SubSep+old.name)
			p.index.Delete(old.id)
			batch.Removed = append(batch.Removed, old.id)
			p.gravesDirty = true
		}
	}
	p.setSubRefs(r, keep)
}

func (p *Pipeline) Dependents(id ids.AssetID) []ids.AssetID {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.dependents(id)
}

func (p *Pipeline) dependents(id ids.AssetID) []ids.AssetID {
	var out []ids.AssetID
	for _, r := range p.byID {
		for _, d := range r.deps {
			if d == id {
				out = append(out, r.id)
				break
			}
		}
	}
	return out
}

func (p *Pipeline) Scan() (added, removed []ids.AssetID, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.scan()
}

func (p *Pipeline) scan() (added, removed []ids.AssetID, err error) {
	if err := p.requireNoPendingSourceRecovery(); err != nil {
		return nil, nil, err
	}
	w := newWalk()
	for _, root := range p.roots {
		if err := p.scanRoot(root, w, &added); err != nil {
			return nil, nil, err
		}
	}
	for path, r := range p.byPath {
		if w.seen[path] {
			continue
		}
		p.bury(r.id, path)
		for _, sr := range r.subs {
			p.bury(sr.id, path+SubSep+sr.name)
			if e, ok := p.index.Lookup(sr.id); ok {
				e.Missing = true
			}
			removed = append(removed, sr.id)
		}
		p.setSubRefs(r, nil)
		delete(p.byPath, path)
		delete(p.byID, r.id)
		if r.group != "" {
			p.dropFromGroup(r.group, r.id)
		}
		removed = append(removed, r.id)
		if e, ok := p.index.Lookup(r.id); ok {
			e.Missing = true
		}
		p.log.Warn("asset source disappeared", log.F("path", path))
	}
	p.files, p.metas = w.files, w.metas
	p.audit()
	if len(removed) > 0 {
		if err := p.saveGraves(); err != nil {
			p.log.Warn("cannot write the asset graveyard", log.F("err", err.Error()))
		}
	}
	if len(added) > 0 || len(removed) > 0 {
		p.rev++
	}
	return added, removed, nil
}

func (p *Pipeline) scanRoot(root string, w *walk, added *[]ids.AssetID) error {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (name == ".aego" || name[0] == '.') {
				return filepath.SkipDir
			}
			return nil
		}
		rel := p.rel(path)
		if strings.HasSuffix(path, MetaSuffix) {
			w.metas[rel] = true
			return nil
		}
		w.files[rel] = true
		imp, ok := p.imps.ForPath(path)
		if !ok {
			return nil
		}
		w.seen[rel] = true
		if existing, ok := p.byPath[rel]; ok {
			existing.importer = imp
			return nil
		}
		meta, created, err := p.loadOrCreateMeta(path, imp)
		if err != nil {
			p.log.Error("cannot read the meta file", log.F("path", rel), log.F("err", err.Error()))
			return nil
		}
		r := &record{path: rel, meta: meta, importer: imp}
		switch {
		case created && p.sourceTransactionMetas != nil:
			// A scan invoked by ApplySources must not create a sidecar for a
			// newly discovered source outside the caller's history. Edited
			// new sources already have their guarded sidecar before this scan;
			// otherwise leave this source for the next ordinary scan.
			return nil
		case created && p.mode == MetaReadOnly:
			r.derived = true
			meta.ID = derivedID(rel)
		case created:
			if err := os.WriteFile(path+MetaSuffix, WriteMeta(meta), 0o644); err != nil {
				p.log.Error("cannot write the meta file", log.F("path", rel), log.F("err", err.Error()))
				r.derived = true
				meta.ID = derivedID(rel)
			} else {
				w.metas[rel+MetaSuffix] = true
			}
		default:
			if other, clash := p.byID[meta.ID]; clash {
				p.settle(r, other, added)
			}
		}
		r.id = meta.ID
		p.byPath[rel] = r
		p.byID[r.id] = r
		refs := make([]subRef, 0, len(meta.Subs))
		for _, sa := range meta.Subs {
			refs = append(refs, subRef{id: sa.ID, name: sa.Name})
		}
		p.setSubRefs(r, refs)
		*added = append(*added, r.id)
		return nil
	})
}

func (p *Pipeline) rel(path string) string {
	r, err := filepath.Rel(p.root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(r)
}

func (p *Pipeline) abs(rel string) string {
	return filepath.Join(p.root, filepath.FromSlash(rel))
}

func (p *Pipeline) loadOrCreateMeta(path string, imp Importer) (*Meta, bool, error) {
	metaPath := path + MetaSuffix
	data, err := os.ReadFile(metaPath)
	if err == nil {
		m, err := ParseMeta(data, p.rel(metaPath))
		if err != nil {
			return nil, false, err
		}
		if m.Importer == "" {
			m.Importer = imp.ID()
		}
		for key, def := range imp.Defaults() {
			if _, ok := m.Settings[key]; !ok {
				m.Settings[key] = def
			}
		}
		return m, false, nil
	}
	if !os.IsNotExist(err) {
		return nil, false, err
	}
	return NewMeta(imp.ID(), imp.Version(), imp.Defaults()), true, nil
}

func (p *Pipeline) ImportAll() (*Batch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	all := make([]ids.AssetID, 0, len(p.byID))
	for id := range p.byID {
		all = append(all, id)
	}
	sort.Slice(all, func(i, j int) bool {
		return p.byID[all[i]].path < p.byID[all[j]].path
	})
	return p.importIDs(all...)
}

func (p *Pipeline) Import(list ...ids.AssetID) (*Batch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.importIDs(list...)
}

func (p *Pipeline) importIDs(list ...ids.AssetID) (*Batch, error) {
	if err := p.requireNoPendingSourceRecovery(); err != nil {
		return nil, err
	}
	batch := &Batch{Failed: map[ids.AssetID]error{}}
	groupsTouched := map[string]bool{}
	for g := range p.stale {
		groupsTouched[g] = true
	}
	clear(p.stale)

	for _, id := range list {
		r, ok := p.byID[id]
		if !ok {
			continue
		}
		res, key, err := p.importOne(r)
		if err != nil {
			if errs.Has(err, errs.AssetSourceUnstable) {
				batch.Retry = append(batch.Retry, id)
				continue
			}
			batch.Failed[id] = err
			r.err = err.Error()
			p.log.Error("import failed", log.F("path", r.path), log.F("err", err.Error()))
			continue
		}
		r.err = ""
		if res == nil {
			continue
		}
		if r.group != "" && r.group != res.Group {
			oldGroup := r.group
			p.dropFromGroup(oldGroup, id)
			groupsTouched[oldGroup] = true
		}
		r.key = key
		r.group = res.Group
		r.deps = res.Deps
		if res.Group != "" {
			p.addToGroup(res.Group, id, res)
			groupsTouched[res.Group] = true
			continue
		}
		entry, err := p.writeArtifacts(r, res)
		if err != nil {
			batch.Failed[id] = err
			r.err = err.Error()
			continue
		}
		p.index.Put(entry)
		batch.Imported = append(batch.Imported, entry)
		if len(res.Subs) > 0 || len(r.subs) > 0 {
			p.putSubs(r, res.Subs, batch)
		}
	}
	if p.gravesDirty {
		if err := p.saveGraves(); err != nil {
			p.log.Warn("cannot write the asset graveyard", log.F("err", err.Error()))
		}
	}

	for group := range groupsTouched {
		oldPage, hadPage := p.pages[group]
		entries, failed, err := p.finalizeGroup(group)
		if err != nil {
			p.log.Error("import group failed", log.F("group", group), log.F("err", err.Error()))
			for _, id := range p.groups[group] {
				batch.Failed[id] = err
				if r := p.byID[id]; r != nil {
					r.err = err.Error()
				}
			}
			continue
		}
		if hadPage && len(p.groups[group]) == 0 {
			batch.Removed = append(batch.Removed, oldPage)
		}
		for id, ferr := range failed {
			batch.Failed[id] = ferr
			if r := p.byID[id]; r != nil {
				r.err = ferr.Error()
				p.log.Error("import failed", log.F("path", r.path), log.F("err", ferr.Error()))
			}
		}
		batch.Imported = append(batch.Imported, entries...)
	}

	if len(batch.Imported) > 0 || len(batch.Failed) > 0 {
		p.rev++
	}
	p.index.Sort()
	if err := p.saveIndex(); err != nil {
		return batch, err
	}
	return batch, nil
}

func (p *Pipeline) importOne(r *record) (*Result, string, error) {
	full := p.abs(r.path)
	before, err := os.Stat(full)
	if err != nil {
		return nil, "", errs.Wrap(err, errs.VFSIO, "cannot stat the asset source")
	}
	source, err := os.ReadFile(full)
	if err != nil {
		return nil, "", errs.Wrap(err, errs.VFSIO, "cannot read the asset source")
	}
	after, err := os.Stat(full)
	if err != nil {
		return nil, "", errs.Wrap(err, errs.VFSIO, "cannot stat the asset source")
	}
	if len(source) == 0 ||
		int64(len(source)) != after.Size() ||
		before.Size() != after.Size() ||
		!before.ModTime().Equal(after.ModTime()) {
		return nil, "", errs.New(errs.AssetSourceUnstable, "asset source is still being written",
			log.F("path", r.path))
	}
	ctx := &ImportContext{
		ID:         r.id,
		Path:       r.path,
		Source:     source,
		Meta:       r.meta,
		Log:        p.log,
		Lookup:     p.lookup,
		Read:       p.read,
		SourcePath: p.pathOf,
	}
	if err := p.resolveMetadata(r, ctx); err != nil {
		return nil, "", err
	}
	var salt []byte
	if imp, ok := r.importer.(CacheSalter); ok {
		salt, err = imp.CacheSalt(ctx)
		if err != nil {
			return nil, "", err
		}
	}
	key := p.cacheKey(r, source, r.deps, salt)
	if key == r.key && r.group == "" {
		if e, ok := p.index.Lookup(r.id); ok && !e.Missing {
			return nil, key, nil
		}
	}
	res, err := r.importer.Import(ctx)
	if err != nil {
		return nil, "", err
	}
	if res == nil {
		return nil, "", errs.New(errs.AssetImportFailed, "importer returned no result", log.F("path", r.path))
	}
	return res, p.cacheKey(r, source, res.Deps, salt), nil
}

func (p *Pipeline) cacheKey(r *record, source []byte, deps []ids.AssetID, salts ...[]byte) string {
	h := sha256.New()
	h.Write(source)
	h.Write(canonicalSettings(r.meta.Settings))
	if len(r.meta.Subs) > 0 {
		h.Write(canonicalSubs(r.meta.Subs))
	}
	writeUint(h, uint64(r.importer.Version()))
	writeUint(h, PipelineVersion)
	h.Write([]byte(r.importer.ID()))
	for _, salt := range salts {
		writeUint(h, uint64(len(salt)))
		h.Write(salt)
	}
	sourceDeps := false
	if salter, ok := r.importer.(SourceDependencySalter); ok {
		sourceDeps = salter.SourceDependencies()
	}
	for _, dep := range deps {
		if sourceDeps {
			h.Write([]byte(dep.String()))
			continue
		}
		if e, ok := p.index.Lookup(dep); ok {
			for _, a := range e.Artifacts {
				h.Write([]byte(a.Hash))
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeUint(h interface{ Write([]byte) (int, error) }, v uint64) {
	var b [8]byte
	for i := 0; i < 8; i++ {
		b[i] = byte(v >> (i * 8))
	}
	h.Write(b[:])
}

func (p *Pipeline) addToGroup(group string, id ids.AssetID, res *Result) {
	members := p.groups[group]
	for _, existing := range members {
		if existing == id {
			p.groupResults[group][id] = res
			return
		}
	}
	p.groups[group] = append(members, id)
	if p.groupResults == nil {
		p.groupResults = map[string]map[ids.AssetID]*Result{}
	}
	if p.groupResults[group] == nil {
		p.groupResults[group] = map[ids.AssetID]*Result{}
	}
	p.groupResults[group][id] = res
}

func (p *Pipeline) writeArtifacts(r *record, res *Result) (asset.Entry, error) {
	return p.writeEntry(r.id, res.Kind, assetName(r.path), r.path, res.Deps, res.Artifacts)
}

func (p *Pipeline) writeEntry(id ids.AssetID, kind asset.ArtifactKind, aname, source string, deps []ids.AssetID, arts []Produced) (asset.Entry, error) {
	entry := asset.Entry{
		ID:     id,
		Kind:   kind,
		Name:   aname,
		Source: source,
		Deps:   deps,
	}
	for _, a := range arts {
		name := a.Name
		if name == "" {
			name = id.String() + artifactExt(a.Kind)
		}
		path := "cache://artifacts/" + name
		if err := p.artifacts.WriteFile(path, a.Data); err != nil {
			return asset.Entry{}, err
		}
		sum := sha256.Sum256(a.Data)
		entry.Artifacts = append(entry.Artifacts, asset.Artifact{
			Kind: a.Kind,
			Path: path,
			Hash: hex.EncodeToString(sum[:8]),
			Size: int64(len(a.Data)),
		})
	}
	return entry, nil
}

func artifactExt(kind asset.ArtifactKind) string {
	switch kind {
	case asset.ArtTexture, asset.ArtAtlasPage:
		return ".aetex"
	case asset.ArtSpriteRegion:
		return ".aereg"
	case asset.ArtScene:
		return ".aescene"
	case asset.ArtPrefab:
		return ".aeprefab"
	case asset.ArtMesh:
		return ".aems"
	case asset.ArtScript:
		return ".aesb"
	case asset.ArtData:
		return ".aedata"
	case asset.ArtTable:
		return ".aetable"
	case asset.ArtAudio:
		return ".aeau"
	case asset.ArtAnim:
		return ".aean"
	case asset.ArtCtrl:
		return ".aect"
	}
	return ".bin"
}

func assetName(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	dir := filepath.ToSlash(filepath.Dir(path))
	if dir == "." {
		return name
	}
	return dir + "/" + name
}

const maxRetries = 5

func (p *Pipeline) Watch(quiet time.Duration, fn func(*Batch)) (func(), error) {
	if quiet <= 0 {
		quiet = 150 * time.Millisecond
	}
	interval := quiet / 2
	if interval < 50*time.Millisecond {
		interval = 50 * time.Millisecond
	}
	pending := map[string]ChangeKind{}
	attempts := map[string]int{}
	var timer *time.Timer

	var schedule func()
	fire := func() {
		p.pmu.Lock()
		paths := make(map[string]ChangeKind, len(pending))
		for k, v := range pending {
			paths[k] = v
		}
		pending = map[string]ChangeKind{}
		p.pmu.Unlock()

		batch, err := p.applyChanges(paths)
		if err != nil {
			p.log.Error("import batch failed", log.F("err", err.Error()))
		}
		if batch == nil {
			return
		}
		if len(batch.Retry) > 0 {
			p.pmu.Lock()
			for _, id := range batch.Retry {
				path, ok := p.PathOf(id)
				if !ok || attempts[path] >= maxRetries {
					continue
				}
				attempts[path]++
				pending[path] = ChangeWrite
			}
			p.pmu.Unlock()
			schedule()
		}
		p.pmu.Lock()
		for _, e := range batch.Imported {
			delete(attempts, e.Source)
		}
		p.pmu.Unlock()
		if !batch.Empty() {
			fn(batch)
		}
	}
	schedule = func() {
		p.pmu.Lock()
		defer p.pmu.Unlock()
		if len(pending) == 0 {
			return
		}
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(quiet, fire)
	}

	stop, err := p.watcher.Start(p.root, []string{".aego", ".git", "exports", "native"}, interval, func(changes []Change) {
		p.pmu.Lock()
		for _, c := range changes {
			pending[strings.TrimSuffix(c.Path, MetaSuffix)] = c.Kind
		}
		p.pmu.Unlock()
		schedule()
	})
	if err != nil {
		return nil, err
	}
	return func() {
		stop()
		p.pmu.Lock()
		if timer != nil {
			timer.Stop()
		}
		p.pmu.Unlock()
	}, nil
}

func (p *Pipeline) applyChanges(paths map[string]ChangeKind) (*Batch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.applyChangesLocked(paths)
}

func (p *Pipeline) applyChangesLocked(paths map[string]ChangeKind) (*Batch, error) {
	// Remember dependencies before scan removes a source from byPath. Removing
	// an include must invalidate consumers just as writing an include does.
	dirty := map[ids.AssetID]bool{}
	for path := range paths {
		if r, ok := p.byPath[path]; ok {
			for _, dep := range p.dependents(r.id) {
				dirty[dep] = true
			}
		}
	}
	_, removed, err := p.scan()
	if err != nil {
		return nil, err
	}
	// Missing supplemental inputs have no prior AssetID dependency, so recheck keys on file events to repair failures.
	// Unchanged keys still avoid compilation and artifact writes.
	for id, r := range p.byID {
		if _, ok := r.importer.(CacheSalter); ok && len(paths) > 0 {
			dirty[id] = true
		}
	}
	for path, kind := range paths {
		r, ok := p.byPath[path]
		if !ok {
			continue
		}
		if kind == ChangeRemove {
			removed = append(removed, r.id)
			continue
		}
		dirty[r.id] = true
		for _, dep := range p.dependents(r.id) {
			dirty[dep] = true
		}
		if r.group != "" {
			for _, member := range p.groups[r.group] {
				dirty[member] = true
			}
		}
	}
	if len(dirty) == 0 && len(removed) == 0 && len(p.stale) == 0 {
		return nil, nil
	}
	metadataFailed := map[ids.AssetID]error{}
	for name := range paths {
		if r := p.byPath[name]; r != nil {
			if err := p.reloadMetadata(r); err != nil {
				metadataFailed[r.id] = err
				r.err = err.Error()
			}
		}
	}
	list := make([]ids.AssetID, 0, len(dirty))
	for id := range dirty {
		if metadataFailed[id] != nil {
			continue
		}

		if _, exists := p.byID[id]; exists {
			list = append(list, id)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return p.byID[list[i]].path < p.byID[list[j]].path
	})
	batch, err := p.importIDs(list...)
	if batch != nil {
		for id, err := range metadataFailed {
			batch.Failed[id] = err
		}
		batch.Removed = append(batch.Removed, removed...)
	}
	return batch, err
}

func (p *Pipeline) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.saveIndex()
	if p.unlock != nil {
		p.unlock()
		p.unlock = nil
	}
}
