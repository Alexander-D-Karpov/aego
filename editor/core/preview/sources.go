package preview

import (
	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/project"
	"crypto/sha256"
	"encoding/binary"
	"io/fs"
	"maps"
	"os"
	"path"
	"slices"
	"strings"
)

const SourceKind asset.ArtifactKind = "editor.source"

type sourceFile struct {
	info           pipeline.AssetInfo
	modified, size int64
}

// Editor source IDs are for navigation only. They never enter the import index,
// scene references, export manifests or the project sidecar files.
func (s *Service) RefreshSources() bool {
	if s.d.Pipeline == nil || s.d.Docs == nil {
		return false
	}
	r, err := os.OpenRoot(s.d.Pipeline.Abs("."))
	if err != nil {
		return false
	}
	defer r.Close()
	entries := map[ids.AssetID]sourceFile{}
	add := func(name string) {
		if h := s.d.Docs.Handlers().Resolve(SourceKind, name, ""); h.ID == "asset.readonly" {
			return
		}
		st, err := r.Lstat(name)
		if err != nil || !st.Mode().IsRegular() {
			return
		}
		raw := sha256.Sum256([]byte("aego.editor.source:" + name))
		id := ids.AssetID{Hi: binary.BigEndian.Uint64(raw[:8]), Lo: binary.BigEndian.Uint64(raw[8:16])}
		entries[id] = sourceFile{pipeline.AssetInfo{ID: id, Path: name, Name: path.Base(name), Kind: SourceKind}, st.ModTime().UnixNano(), st.Size()}
	}
	add(project.FileName)
	if st, e := r.Lstat("native"); e == nil && st.IsDir() {
		visited := 0
		_ = fs.WalkDir(r.FS(), "native", func(name string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			visited++
			if visited > 4096 || len(entries) > 512 {
				return fs.SkipAll
			}
			base := d.Name()
			if d.IsDir() && (strings.HasPrefix(base, ".") || base == "vendor" || base == "node_modules" || base == "generated") {
				return fs.SkipDir
			}
			if d.Type().IsRegular() && strings.HasSuffix(base, ".go") {
				add(name)
			}
			return nil
		})
	}
	if s.sources != nil && maps.Equal(s.sources, entries) {
		return false
	}
	for id, old := range s.sources {
		if next, ok := entries[id]; !ok || next != old {
			s.Forget(id)
		}
	}
	s.sources = entries
	return true
}
func (s *Service) SourceFiles() []pipeline.AssetInfo {
	if s.sources == nil {
		s.RefreshSources()
	}
	v := make([]pipeline.AssetInfo, 0, len(s.sources))
	for _, a := range s.sources {
		v = append(v, a.info)
	}
	slices.SortFunc(v, func(a, b pipeline.AssetInfo) int { return strings.Compare(a.Path, b.Path) })
	return v
}
func (s *Service) SourceFile(id ids.AssetID) (pipeline.AssetInfo, bool) {
	if s.sources == nil {
		s.RefreshSources()
	}
	v, ok := s.sources[id]
	return v.info, ok
}
func (s *Service) SourceID(rel string) (ids.AssetID, bool) {
	if s.sources == nil {
		s.RefreshSources()
	}
	for id, a := range s.sources {
		if a.info.Path == rel {
			return id, true
		}
	}
	return ids.AssetID{}, false
}
