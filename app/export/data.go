package export

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"aego/asset"
	"aego/core/errs"
	"aego/core/log"
	"aego/engine"
	"aego/format/aetext"
	"aego/runtime/bundle"
)

const debugSection = "debug"

func exportSkip(k asset.ArtifactKind) bool {
	return k == asset.ArtPreset
}

func setField(blocks []*aetext.Node, head, name string, v aetext.Value) []*aetext.Node {
	for _, b := range blocks {
		if b.Head != head {
			continue
		}
		for i := range b.Fields {
			if b.Fields[i].Name == name {
				b.Fields[i].Value = v
				return blocks
			}
		}
		b.Fields = append(b.Fields, aetext.Field{Name: name, Value: v})
		return blocks
	}
	return append(blocks, &aetext.Node{Head: head, Fields: []aetext.Field{{Name: name, Value: v}}})
}

func (j *job) game() bundle.Game {
	p := j.p
	g := bundle.Game{
		Name: p.Name, Module: p.Module, Version: GameVersion(p), MainScene: p.MainScene,
		Window: bundle.Window{Width: p.Window.Width, Height: p.Window.Height, Title: p.Window.Title},
	}
	if p.Settings != nil {
		g.Defaults = p.Settings.Clone()
	}
	for _, b := range p.Extra {
		g.Blocks = append(g.Blocks, b.Clone())
	}
	if j.profile == bundle.Release {
		g.Blocks = setField(g.Blocks, debugSection, "enabled", aetext.Bool(false))
	}
	return g
}

func (j *job) data() error {
	art := filepath.Join(j.l.data, bundle.ArtifactsDir)
	if err := os.MkdirAll(art, 0o755); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot create the data directory")
	}
	idx := asset.NewIndex()
	sizes := map[string]*bundle.Size{}
	written := map[string]bool{}
	var names []string
	var total int64
	count := 0
	var failed error
	j.files = j.files[:0]
	j.snap.idx.Each(func(e *asset.Entry) {
		if failed != nil || e.Missing || e.Error != "" || exportSkip(e.Kind) {
			return
		}
		cp := *e
		cp.Artifacts = make([]asset.Artifact, 0, len(e.Artifacts))
		for _, a := range e.Artifacts {
			raw, err := j.snap.files.ReadFile(a.Path)
			if err != nil {
				failed = errs.Wrap(err, errs.AssetLoadFailed, "cannot read an artifact").With(log.F("asset", e.ID.String()), log.F("path", a.Path))
				return
			}
			sum := sha256.Sum256(raw)
			name := hex.EncodeToString(sum[:12]) + "." + string(a.Kind)
			kind := string(a.Kind)
			s := sizes[kind]
			if s == nil {
				s = &bundle.Size{Kind: kind}
				sizes[kind] = s
			}
			s.Count++
			if !written[name] {
				if err := os.WriteFile(filepath.Join(art, name), raw, 0o644); err != nil {
					failed = errs.Wrap(err, errs.VFSIO, "cannot write an artifact").With(log.F("path", name))
					return
				}
				written[name] = true
				names = append(names, name)
				j.files = append(j.files, webFile{Path: bundle.ArtifactsDir + "/" + name, Size: int64(len(raw))})
				s.Bytes += int64(len(raw))
				total += int64(len(raw))
			}
			count++
			cp.Artifacts = append(cp.Artifacts, asset.Artifact{
				Kind: a.Kind, Name: a.Name, Path: bundle.ArtifactPath(name),
				Hash: hex.EncodeToString(sum[:8]), Size: int64(len(raw)),
			})
		}
		idx.Put(cp)
	})
	if failed != nil {
		return failed
	}
	idx.Sort()
	raw, err := bundle.EncodeData(&bundle.Data{Engine: engine.Version, Profile: j.profile, Target: j.t.String(), Game: j.game(), Index: idx})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(j.l.data, bundle.ManifestFile), raw, 0o644); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write the data manifest")
	}
	j.manifestSize = int64(len(raw))
	slices.Sort(names)
	slices.SortFunc(j.files, func(a, b webFile) int { return strings.Compare(a.Path, b.Path) })
	h := sha256.New()
	h.Write([]byte(strings.Join(names, "\n")))
	h.Write(raw)
	j.rep.Hash = hex.EncodeToString(h.Sum(nil))
	j.rep.DataSize = total + int64(len(raw))
	j.rep.Artifacts = count
	j.rep.Data = j.rep.Data[:0]
	for _, s := range sizes {
		j.rep.Data = append(j.rep.Data, *s)
	}
	slices.SortFunc(j.rep.Data, func(a, b bundle.Size) int {
		switch {
		case a.Bytes > b.Bytes:
			return -1
		case a.Bytes < b.Bytes:
			return 1
		}
		return strings.Compare(a.Kind, b.Kind)
	})
	return nil
}

func (j *job) manifest() error {
	p := j.p
	x := &bundle.Export{
		Engine: engine.Version, Game: p.Name, GameVersion: GameVersion(p), Module: p.Module,
		Target: j.t.String(), Profile: j.profile, Binary: j.rep.Binary, BinarySize: j.rep.BinarySize, BinaryGzip: j.rep.BinaryGzip,
		StartScene: p.MainScene, Modules: j.rep.Modules, Libs: j.libs(), Data: j.rep.Data,
		DataSize: j.rep.DataSize, Artifacts: j.rep.Artifacts, Hash: j.rep.Hash,
		BuildMillis: time.Since(j.start).Milliseconds(), Time: time.Now().UTC().Format(time.RFC3339),
	}
	if err := os.WriteFile(filepath.Join(j.out, bundle.ExportFile), bundle.EncodeExport(x), 0o644); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write the export manifest")
	}
	return nil
}

func (j *job) sdlLicense() string {
	cands, _ := filepath.Glob(filepath.Join(j.opt.Engine, ".cache", "build", "sdl-*", "src", "LICENSE.txt"))
	cands = append(cands, filepath.Join(j.opt.Engine, "third_party", "sdl", "LICENSE.txt"))
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func (j *job) license(lib bundle.Lib, src, name string) bundle.Lib {
	if src == "" {
		return lib
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return lib
	}
	dst := filepath.Join(j.out, "licenses", name)
	if os.MkdirAll(filepath.Dir(dst), 0o755) == nil && os.WriteFile(dst, data, 0o644) == nil {
		lib.License = "licenses/" + name
	}
	return lib
}

func (j *job) libs() []bundle.Lib {
	if j.t.Web() {
		return []bundle.Lib{j.license(bundle.Lib{Name: "Go wasm_exec.js", Link: "script", File: webExec}, filepath.Join(j.goroot, "LICENSE"), "Go.txt")}
	}
	return []bundle.Lib{j.license(bundle.Lib{Name: "SDL3", Link: "static"}, j.sdlLicense(), "SDL3.txt")}
}
