package pipeline

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
)

type AssetInfo struct {
	ID       ids.AssetID
	Path     string
	Name     string
	Kind     asset.ArtifactKind
	Importer string
	Error    string
}

func (p *Pipeline) Assets() []AssetInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]AssetInfo, 0, len(p.byID))
	for _, r := range p.byID {
		info := AssetInfo{ID: r.id, Path: r.path, Name: assetName(r.path), Error: r.err}
		if r.importer != nil {
			info.Importer = r.importer.ID()
		}
		if e, ok := p.index.Lookup(r.id); ok {
			info.Kind = e.Kind
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func (p *Pipeline) Entry(id ids.AssetID) (asset.Entry, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	e, ok := p.index.Lookup(id)
	if !ok {
		return asset.Entry{}, false
	}
	return *e, true
}

// PrimaryArtifact returns a value snapshot while the pipeline index is locked.
// A thumbnail worker needs no access to the editor's mutable source-file map.
func (p *Pipeline) PrimaryArtifact(id ids.AssetID) (asset.Artifact, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	e, ok := p.index.Lookup(id)
	if !ok {
		return asset.Artifact{}, false
	}
	return e.Artifact(e.Kind)
}

func (p *Pipeline) RootDirs() []string {
	out := make([]string, len(p.roots))
	for i, r := range p.roots {
		out[i] = p.rel(r)
	}
	return out
}

func (p *Pipeline) Abs(rel string) string {
	return p.abs(rel)
}

func (p *Pipeline) Dirs() []string {
	var out []string
	for _, root := range p.roots {
		filepath.WalkDir(root, func(full string, d os.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if full != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			out = append(out, p.rel(full))
			return nil
		})
	}
	sort.Strings(out)
	return out
}

func cleanRel(rel string) string {
	return path.Clean(filepath.ToSlash(strings.TrimSpace(rel)))
}

func (p *Pipeline) inRoots(rel string) bool {
	for _, root := range p.RootDirs() {
		if rel == root || strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	return false
}

func (p *Pipeline) isRoot(rel string) bool {
	for _, root := range p.RootDirs() {
		if rel == root {
			return true
		}
	}
	return false
}

func badPath(rel, msg string) error {
	return errs.New(errs.VFSBadPath, msg, log.F("path", rel))
}

func exists(abs string) bool {
	_, err := os.Stat(abs)
	return err == nil
}

// checkSidecarDestination preserves an existing orphan or symlinked sidecar.
// As with the surrounding asset operations, the check is not an atomic CAS.
func (p *Pipeline) checkSidecarDestination(rel string) error {
	_, err := os.Lstat(p.abs(rel) + MetaSuffix)
	if err == nil {
		return badPath(rel, "destination metadata already exists")
	}
	if !os.IsNotExist(err) {
		return errs.Wrap(err, errs.VFSIO, "cannot check destination metadata").With(log.F("path", rel))
	}
	return nil
}

func (p *Pipeline) unique(rel string) string {
	ext := path.Ext(rel)
	base := strings.TrimSuffix(rel, ext)
	candidate := rel
	for n := 2; ; n++ {
		if _, taken := p.byPath[candidate]; !taken && !exists(p.abs(candidate)) {
			return candidate
		}
		candidate = base + " " + strconv.Itoa(n) + ext
	}
}

func (p *Pipeline) UniqueDir(rel string) string {
	rel = cleanRel(rel)
	candidate := rel
	for n := 2; exists(p.abs(candidate)); n++ {
		candidate = rel + " " + strconv.Itoa(n)
	}
	return candidate
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

func (p *Pipeline) Mkdir(rel string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.requireWritableSources(); err != nil {
		return err
	}
	rel = cleanRel(rel)
	if !p.inRoots(rel) {
		return badPath(rel, "folder must be inside assets or scenes")
	}
	if err := os.MkdirAll(p.abs(rel), 0o755); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot create the folder")
	}
	p.rev++
	return nil
}

func (p *Pipeline) Move(id ids.AssetID, newRel string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.requireWritableSources(); err != nil {
		return err
	}
	r, ok := p.byID[id]
	if !ok {
		return errs.New(errs.AssetNotFound, "asset is not known to the pipeline", log.F("asset", id.String()))
	}
	newRel = cleanRel(newRel)
	if newRel == r.path {
		return nil
	}
	if !p.inRoots(path.Dir(newRel)) {
		return badPath(newRel, "destination must be inside assets or scenes")
	}
	if path.Ext(newRel) != path.Ext(r.path) {
		return badPath(newRel, "renaming cannot change the file extension")
	}
	if _, taken := p.byPath[newRel]; taken || exists(p.abs(newRel)) {
		return badPath(newRel, "destination already exists")
	}
	if err := p.checkMetadataSnapshot(r); err != nil {
		return err
	}
	if err := p.checkSidecarDestination(newRel); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.abs(newRel)), 0o755); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot create the destination folder")
	}
	oldAbs, newAbs := p.abs(r.path), p.abs(newRel)
	if err := os.Rename(oldAbs, newAbs); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot move the asset")
	}
	if exists(oldAbs + MetaSuffix) {
		if err := os.Rename(oldAbs+MetaSuffix, newAbs+MetaSuffix); err != nil {
			os.Rename(newAbs, oldAbs)
			return errs.Wrap(err, errs.VFSIO, "cannot move the meta file")
		}
	}
	p.relocate(r, newRel)
	p.rev++
	return p.saveIndex()
}

func (p *Pipeline) relocate(r *record, newRel string) {
	delete(p.byPath, r.path)
	r.path = newRel
	p.byPath[newRel] = r
	if e, ok := p.index.Lookup(r.id); ok {
		e.Source = newRel
		e.Name = assetName(newRel)
	}
	if r.group != "" {
		p.stale[r.group] = true
		delete(p.groupKey, r.group)
	}
}

func (p *Pipeline) MoveDir(oldRel, newRel string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.requireWritableSources(); err != nil {
		return err
	}
	oldRel, newRel = cleanRel(oldRel), cleanRel(newRel)
	if oldRel == newRel {
		return nil
	}
	if p.isRoot(oldRel) || !p.inRoots(oldRel) || !p.inRoots(newRel) {
		return badPath(oldRel, "only folders inside assets or scenes can be moved")
	}
	if strings.HasPrefix(newRel, oldRel+"/") {
		return badPath(newRel, "a folder cannot be moved into itself")
	}
	if exists(p.abs(newRel)) {
		return badPath(newRel, "destination already exists")
	}
	var moved []*record
	for _, r := range p.byID {
		if strings.HasPrefix(r.path, oldRel+"/") {
			moved = append(moved, r)
		}
	}
	sort.Slice(moved, func(i, j int) bool { return moved[i].path < moved[j].path })
	for _, r := range moved {
		if err := p.checkMetadataSnapshot(r); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(p.abs(newRel)), 0o755); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot create the destination folder")
	}
	if err := os.Rename(p.abs(oldRel), p.abs(newRel)); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot move the folder")
	}
	for _, r := range moved {
		p.relocate(r, newRel+strings.TrimPrefix(r.path, oldRel))
	}
	p.rev++
	return p.saveIndex()
}

func (p *Pipeline) RemoveDir(rel string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.requireWritableSources(); err != nil {
		return err
	}
	rel = cleanRel(rel)
	if p.isRoot(rel) || !p.inRoots(rel) {
		return badPath(rel, "only folders inside assets or scenes can be removed")
	}
	if err := os.Remove(p.abs(rel)); err != nil {
		return errs.Wrap(err, errs.VFSIO, "folder is not empty or cannot be removed")
	}
	p.rev++
	return nil
}

func (p *Pipeline) Duplicate(id ids.AssetID) (ids.AssetID, *Batch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.requireWritableSources(); err != nil {
		return ids.AssetID{}, nil, err
	}
	r, ok := p.byID[id]
	if !ok {
		return ids.AssetID{}, nil, errs.New(errs.AssetNotFound, "asset is not known to the pipeline", log.F("asset", id.String()))
	}
	if err := p.checkMetadataSnapshot(r); err != nil {
		return ids.AssetID{}, nil, err
	}
	ext := path.Ext(r.path)
	dst := p.unique(strings.TrimSuffix(r.path, ext) + " copy" + ext)
	if err := p.checkSidecarDestination(dst); err != nil {
		return ids.AssetID{}, nil, err
	}
	if err := copyFile(p.abs(r.path), p.abs(dst)); err != nil {
		return ids.AssetID{}, nil, errs.Wrap(err, errs.VFSIO, "cannot copy the asset")
	}
	meta := &Meta{ID: ids.NewAssetID(), Importer: r.meta.Importer, Version: r.meta.Version, Settings: r.meta.Settings.Clone()}
	if err := os.WriteFile(p.abs(dst)+MetaSuffix, WriteMeta(meta), 0o644); err != nil {
		os.Remove(p.abs(dst))
		return ids.AssetID{}, nil, errs.Wrap(err, errs.VFSIO, "cannot write the meta file")
	}
	if _, _, err := p.scan(); err != nil {
		return ids.AssetID{}, nil, err
	}
	batch, err := p.importIDs(meta.ID)
	return meta.ID, batch, err
}

func (p *Pipeline) Delete(id ids.AssetID) (*Batch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.requireWritableSources(); err != nil {
		return nil, err
	}
	r, ok := p.byID[id]
	if !ok {
		return nil, errs.New(errs.AssetNotFound, "asset is not known to the pipeline", log.F("asset", id.String()))
	}
	if err := p.checkMetadataSnapshot(r); err != nil {
		return nil, err
	}
	full := p.abs(r.path)
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return nil, errs.Wrap(err, errs.VFSIO, "cannot delete the asset")
	}
	os.Remove(full + MetaSuffix)
	if _, _, err := p.scan(); err != nil {
		return nil, err
	}
	batch, err := p.importIDs()
	if batch == nil {
		batch = &Batch{Failed: map[ids.AssetID]error{}}
	}
	batch.Removed = append(batch.Removed, id)
	return batch, err
}

func (p *Pipeline) Reimport(list ...ids.AssetID) (*Batch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reimport(list...)
}

func (p *Pipeline) reimport(list ...ids.AssetID) (*Batch, error) {
	if err := p.requireNoPendingSourceRecovery(); err != nil {
		return nil, err
	}
	for _, id := range list {
		if r := p.byID[id]; r != nil {
			r.key = ""
			if r.group != "" {
				p.groupKey[r.group] = ""
			}
		}
	}
	return p.importIDs(list...)
}

// ReimportWithDependents imports the source first, then checks the existing
// keys of its direct dependents under the same pipeline lock. A failed or
// unstable source never triggers dependent publication. This is not a
// recursive dependency traversal or an atomic artifact transaction.
func (p *Pipeline) ReimportWithDependents(id ids.AssetID) (*Batch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	batch, err := p.reimport(id)
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		return batch, err
	}
	list := p.dependents(id)
	list = slices.DeleteFunc(list, func(dependent ids.AssetID) bool { return dependent == id })
	sort.Slice(list, func(i, j int) bool { return p.byID[list[i]].path < p.byID[list[j]].path })
	if len(list) == 0 {
		return batch, nil
	}
	dependentBatch, err := p.importIDs(list...)
	if dependentBatch != nil {
		batch.Imported = append(batch.Imported, dependentBatch.Imported...)
		batch.Removed = append(batch.Removed, dependentBatch.Removed...)
		batch.Retry = append(batch.Retry, dependentBatch.Retry...)
		for id, failure := range dependentBatch.Failed {
			batch.Failed[id] = failure
		}
	}
	return batch, err
}

func convertible(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".gif":
		return true
	}
	return false
}

func convertToPNG(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	img, _, err := image.Decode(in)
	if err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if err := png.Encode(out, img); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

func (p *Pipeline) ImportExternal(srcs []string, dstDir string) ([]ids.AssetID, *Batch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.requireWritableSources(); err != nil {
		return nil, nil, err
	}
	dstDir = cleanRel(dstDir)
	if !p.inRoots(dstDir) {
		return nil, nil, badPath(dstDir, "files can only be imported into assets or scenes")
	}
	if err := os.MkdirAll(p.abs(dstDir), 0o755); err != nil {
		return nil, nil, errs.Wrap(err, errs.VFSIO, "cannot create the destination folder")
	}
	_, pngOK := p.imps.ForPath("image.png")
	targets := map[string]bool{}
	var skipped []string
	for _, src := range srcs {
		base := filepath.Base(src)
		info, err := os.Stat(src)
		if err != nil || info.IsDir() {
			skipped = append(skipped, base)
			continue
		}
		ext := strings.ToLower(filepath.Ext(src))
		if convertible(ext) && pngOK {
			rel := p.unique(path.Join(dstDir, strings.TrimSuffix(base, filepath.Ext(base))+".png"))
			if err := convertToPNG(src, p.abs(rel)); err != nil {
				return nil, nil, errs.Wrap(err, errs.AssetImportFailed, "cannot convert the dropped image to png").With(log.F("src", src))
			}
			targets[rel] = true
			continue
		}
		if _, ok := p.imps.ForPath(src); !ok {
			skipped = append(skipped, base)
			continue
		}
		rel := p.unique(path.Join(dstDir, base))
		if err := copyFile(src, p.abs(rel)); err != nil {
			return nil, nil, errs.Wrap(err, errs.VFSIO, "cannot copy the dropped file").With(log.F("src", src))
		}
		targets[rel] = true
	}
	if len(targets) == 0 {
		return nil, nil, errs.New(errs.AssetImportFailed, "none of the dropped files can be imported",
			log.F("files", strings.Join(skipped, ", ")))
	}
	added, _, err := p.scan()
	if err != nil {
		return nil, nil, err
	}
	var created []ids.AssetID
	for _, id := range added {
		if r := p.byID[id]; r != nil && targets[r.path] {
			created = append(created, id)
		}
	}
	batch, err := p.importIDs(created...)
	return created, batch, err
}
