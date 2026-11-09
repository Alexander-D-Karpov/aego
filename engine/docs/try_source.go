//go:build !aego_runtime

package docs

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/script/diag"
	"aego/script/parser"
	scriptsource "aego/script/source"
)

// TrySource is a build-time export of the same sources that produced the Try.
// It is never a replacement runtime format. The site packages these files with
// remoteexample's existing ZIP/manifest contract; the editor uses its ordinary
// project importer. No source bytes are read when the registry serves a Try.
type TrySource struct {
	EntryScene string
	Files      map[string][]byte
	Modules    []string
	AssetRefs  []string
}

type trySourceLocation struct {
	src   module.DocSource
	scene string
}

const (
	maxTrySourceFiles = 512
	maxTrySourceFile  = 16 << 20
	maxTrySourceBytes = 64 << 20
)

func trySourceID(loc trySourceLocation, name string) ids.AssetID {
	h := sha256.Sum256([]byte("aego/docs/source/v1/" + loc.src.Owner + "/" + loc.scene + "/" + name))
	id, _ := ids.ParseAssetID(fmt.Sprintf("%x", h[:16]))
	return id
}

func trySourceMeta(id ids.AssetID, importer string) []byte {
	return asset.MarshalSourceMeta(&asset.SourceMeta{ID: id, Importer: importer, Version: 1, Settings: asset.ImportSettings{}})
}

func readTrySource(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > maxTrySourceFile {
		return nil, fmt.Errorf("not a bounded regular source file: %s", name)
	}
	b, err := io.ReadAll(io.LimitReader(f, maxTrySourceFile+1))
	if err == nil && len(b) > maxTrySourceFile {
		err = fmt.Errorf("source file exceeds limit: %s", name)
	}
	return b, err
}

func (r *Registry) exportTrySource(e Entry, loc trySourceLocation) (*TrySource, error) {
	s := &TrySource{EntryScene: "scenes/main.aescene", Files: map[string][]byte{}}
	base := strings.TrimSuffix(loc.scene, TryExt)
	assetDir := base + ".assets"
	if _, err := fs.Stat(loc.src.FS, assetDir); err == nil {
		var total int
		err = fs.WalkDir(loc.src.FS, assetDir, func(name string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return fmt.Errorf("source symlink rejected: %s", name)
			}
			if d.IsDir() {
				if name != assetDir && strings.HasPrefix(d.Name(), ".") {
					return fs.SkipDir
				}
				return nil
			}
			data, err := readTrySource(loc.src.FS, name)
			if err != nil {
				return err
			}
			total += len(data)
			if len(s.Files) >= maxTrySourceFiles || total > maxTrySourceBytes {
				return fmt.Errorf("try source project exceeds file/byte budget")
			}
			s.Files[strings.TrimPrefix(name, assetDir+"/")] = data
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if data, ok := s.Files["project.aego"]; ok {
		doc, err := aetext.Parse(data, "project.aego")
		if err != nil || doc.Head != "project" {
			return nil, fmt.Errorf("invalid source project.aego")
		}
		if v, ok := doc.Field("native"); ok && v.Bool {
			return nil, fmt.Errorf("try example cannot include native project code")
		}
		ref, ok := doc.Field("mainScene")
		if !ok {
			return nil, fmt.Errorf("source project has no mainScene")
		}
		found := false
		for name, meta := range s.Files {
			if !strings.HasSuffix(name, ".aescene.meta") {
				continue
			}
			m, err := asset.ParseSourceMeta(meta, name)
			if err != nil {
				return nil, err
			}
			if m.ID.String() == ref.Str {
				s.EntryScene, found = strings.TrimSuffix(name, ".meta"), true
			}
		}
		if !found {
			return nil, fmt.Errorf("source project mainScene has no source/meta pair")
		}
	} else {
		id := trySourceID(loc, s.EntryScene)
		// The registry exports asset sources. Its packaging host creates the
		// project shell with project.Project.Encode; engine/docs must not import
		// the project/editor layer or carry a second project serializer.
		s.Files[s.EntryScene+".meta"] = trySourceMeta(id, "scene")
	}
	// The exact registry scene is authoritative, including subsequent inspector
	// fixes. Existing source AssetIDs are retained, not recreated by the export.
	s.Files[s.EntryScene] = []byte(e.Try)
	if e.TryScript != "" {
		var scripts []string
		matched := false
		for name, data := range s.Files {
			if strings.HasSuffix(name, ".aes") {
				scripts = append(scripts, name)
				matched = matched || bytes.Equal(data, []byte(e.TryScript))
			}
		}
		if !matched {
			pkg, err := trySourceScriptPackage("main.aes", []byte(e.TryScript))
			if err != nil {
				return nil, err
			}
			// The sandbox places scripts by their package clause. A downloaded
			// project must use that same directory, not the scripts root.
			name := path.Join("scripts", pkg, "main.aes")
			if len(scripts) == 1 {
				name = scripts[0]
			} else if len(scripts) > 1 {
				return nil, fmt.Errorf("try script differs from a multiple-script source project; synchronize the source")
			}
			s.Files[name] = []byte(e.TryScript)
			if _, ok := s.Files[name+".meta"]; !ok {
				s.Files[name+".meta"] = trySourceMeta(trySourceID(loc, name), "script")
			}
		}
	}
	for rawID, text := range e.TryPrefabs {
		id, err := ids.ParseAssetID(rawID)
		if err != nil {
			return nil, err
		}
		name := "prefabs/" + rawID + ".aeprefab"
		for candidate, data := range s.Files {
			if !strings.HasSuffix(candidate, ".aeprefab.meta") {
				continue
			}
			m, err := asset.ParseSourceMeta(data, candidate)
			if err != nil {
				return nil, err
			}
			if m.ID == id {
				name = strings.TrimSuffix(candidate, ".meta")
			}
		}
		s.Files[name] = []byte(text)
		if _, ok := s.Files[name+".meta"]; !ok {
			s.Files[name+".meta"] = trySourceMeta(id, "prefab")
		}
	}
	if err := r.checkTrySource(s); err != nil {
		return nil, err
	}
	return s, nil
}

var sourceImports = regexp.MustCompile(`"(aego/[a-z0-9_/]+)"`)

func trySourceScriptPackage(name string, data []byte) (string, error) {
	f, diagnostics := parser.ParseSource(scriptsource.NewManager(), name, data)
	if diag.HasErrors(diagnostics) || f == nil || f.Package == nil || f.Package.Name == "" {
		return "", fmt.Errorf("invalid example script package: %s", name)
	}
	return f.Package.Name, nil
}

func (r *Registry) checkTrySource(s *TrySource) error {
	available, refs, modules := map[string]string{}, map[string]bool{}, map[string]bool{}
	moduleName := "game"
	if data, ok := s.Files["project.aego"]; ok {
		doc, err := aetext.Parse(data, "project.aego")
		if err != nil {
			return err
		}
		if v, ok := doc.Field("module"); ok && v.Str != "" {
			moduleName = path.Base(v.Str)
		}
	}
	entry, err := asset.ParseSourceMeta(s.Files[s.EntryScene+".meta"], s.EntryScene+".meta")
	if err != nil {
		return fmt.Errorf("entry scene metadata: %w", err)
	}
	refs[entry.ID.String()] = true
	var addModule func(string)
	addModule = func(id string) {
		if id == "" || modules[id] {
			return
		}
		if m, ok := r.mods[id]; ok {
			modules[id] = true
			for _, dep := range m.Requires {
				addModule(dep)
			}
		}
	}
	addModule(EngineOwner)
	var value func(aetext.Value)
	value = func(v aetext.Value) {
		if v.Kind == aetext.VAsset && v.Str != "" && v.Str != (ids.AssetID{}).String() {
			refs[v.Str] = true
		}
		for _, item := range v.List {
			value(item)
		}
		for _, f := range v.Obj {
			value(f.Value)
		}
	}
	var node func(*aetext.Node)
	node = func(n *aetext.Node) {
		if n.Head == "component" {
			if i, ok := r.byID[n.Label]; ok {
				addModule(r.entries[i].Owner)
			}
		}
		for _, f := range n.Attrs {
			value(f.Value)
		}
		for _, f := range n.Fields {
			value(f.Value)
		}
		for _, child := range n.Blocks {
			node(child)
		}
	}
	var total int
	for name, data := range s.Files {
		total += len(data)
		if len(data) > maxTrySourceFile || total > maxTrySourceBytes || len(s.Files) > maxTrySourceFiles {
			return fmt.Errorf("try source exceeds resource budget")
		}
		if strings.HasSuffix(name, ".meta") {
			m, err := asset.ParseSourceMeta(data, name)
			if err != nil {
				return err
			}
			if _, ok := s.Files[strings.TrimSuffix(name, ".meta")]; !ok {
				return fmt.Errorf("metadata has no source: %s", name)
			}
			register := func(id ids.AssetID) error {
				key := id.String()
				if old, ok := available[key]; ok {
					return fmt.Errorf("duplicate source AssetID %s in %s and %s", key, old, name)
				}
				available[key] = name
				return nil
			}
			if err := register(m.ID); err != nil {
				return err
			}
			for _, sub := range m.Subs {
				if err := register(sub.ID); err != nil {
					return err
				}
			}
			// Import settings can reference auxiliary source assets, such as
			// normal/emission textures. They are dependencies, not just IDs
			// declared by this sidecar's source and sprite regions.
			for _, setting := range m.Settings {
				value(setting)
			}
			continue
		}
		switch path.Ext(name) {
		case ".aego", ".aescene", ".aeprefab", ".aeanim", ".aectrl", ".aedata", ".aetable", ".aemat", ".aepreset", ".aeanimator", ".aemixer":
			doc, err := aetext.Parse(data, name)
			if err != nil {
				return err
			}
			node(doc)
		case ".aes":
			pkg, err := trySourceScriptPackage(name, data)
			if err != nil {
				return err
			}
			dir := path.Dir(name)
			want := path.Base(dir)
			if dir == "scripts" {
				want = moduleName
			}
			if !strings.HasPrefix(name, "scripts/") || pkg != want {
				return fmt.Errorf("script package %s does not match its project directory: %s", pkg, name)
			}
			for _, match := range sourceImports.FindAllSubmatch(data, -1) {
				if i, ok := r.byID[string(match[1])]; ok {
					addModule(r.entries[i].Owner)
				}
			}
		}
	}
	refList := make([]string, 0, len(refs))
	for raw := range refs {
		refList = append(refList, raw)
	}
	slices.Sort(refList)
	for _, raw := range refList {
		id, err := ids.ParseAssetID(raw)
		if err != nil {
			return err
		}
		if _, ok := available[raw]; !ok && !asset.IsBuiltin(id) {
			return fmt.Errorf("referenced AssetID has no source in the example: %s", raw)
		}
	}
	moduleList := make([]string, 0, len(modules))
	for id := range modules {
		moduleList = append(moduleList, id)
	}
	slices.Sort(moduleList)
	// Revalidation must be deterministic and cannot accumulate stale refs or
	// publish a partially checked dependency list on failure.
	s.AssetRefs, s.Modules = refList, moduleList
	return nil
}
