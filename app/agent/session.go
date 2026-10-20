package agent

import (
	"bytes"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	"aego/app"
	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/engine/module"
	scriptsystems "aego/modules/scripting/systems"
	"aego/project"
	"aego/project/refactor"
	"aego/project/refs"
	"aego/render/mesh"
	"aego/render/texture"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/script/bind"
	"aego/script/reload"
	"aego/script/runtime"
	"aego/script/workspace"
	"aego/sdk"
)

const (
	SceneExt  = ".aescene"
	PrefabExt = ".aeprefab"
)

type RunRequest struct {
	Scene  string `json:"scene,omitempty"`
	Frames int    `json:"frames"`
	Seed   uint64 `json:"seed"`
}

type RunReport struct {
	Frames int      `json:"frames"`
	Hash   string   `json:"hash"`
	Log    []string `json:"log,omitempty"`
	Error  string   `json:"error,omitempty"`
}

type Runner func(p *project.Project, r RunRequest) (RunReport, error)

type Options struct {
	Mods []sdk.Module
	Run  Runner
}

type Session struct {
	Project   *project.Project
	Types     *scene.Types
	Modules   *module.Manifest
	Pipeline  *pipeline.Pipeline
	Library   *sceneio.Library
	opt       Options
	kit       *Kit
	libErrs   []error
	scriptErr error
}

type Kit struct {
	Project *project.Project
	Types   *scene.Types
	Modules *module.Manifest
	imps    *pipeline.ImporterRegistry
	ws      *workspace.Workspace
	binds   *bind.Registry
	opt     Options
}

func NewKit(p *project.Project, opt Options) (*Kit, error) {
	types, err := assembly.NewTypes()
	if err != nil {
		return nil, err
	}
	reg := bind.New()
	man, err := builtin.Default().Load(types, reg, assembly.Options{}, sdk.AdaptAll(opt.Mods)...)
	if err != nil {
		return nil, err
	}
	ws, err := workspace.Open(workspace.Config{Root: p.Root, Module: p.Module, Scripts: p.ScriptsDir()})
	if err != nil {
		return nil, err
	}
	imps, err := app.DevImporters(ws, man)
	if err != nil {
		return nil, err
	}
	if err := types.Schema().Freeze(); err != nil {
		return nil, err
	}
	if err := scriptsystems.Freeze(reg, types.Schema()); err != nil {
		return nil, err
	}
	ws.SetBindings(reg.Table())
	return &Kit{Project: p, Types: types, Modules: man, imps: imps, ws: ws, binds: reg, opt: opt}, nil
}

func (k *Kit) Open() (*Session, error) {
	pl, err := pipeline.Open(k.Project, k.imps, log.Nop())
	if err != nil {
		return nil, err
	}
	s := &Session{Project: k.Project, Types: k.Types, Modules: k.Modules, Pipeline: pl, opt: k.opt, kit: k}
	if _, _, err := pl.Scan(); err != nil {
		s.Close()
		return nil, err
	}
	if _, err := pl.ImportAll(); err != nil {
		s.Close()
		return nil, err
	}
	if g := k.ws.Committed(); g.Modules != nil {
		prog := reload.NewProgram(g.ID, g.Modules, g.Hashes)
		if err := runtime.Publish(k.Types.Schema(), prog.Ordered(), k.binds.Surface()); err != nil {
			s.scriptErr = err
		}
	}
	s.reload()
	return s, nil
}

func Open(p *project.Project, opt Options) (*Session, error) {
	k, err := NewKit(p, opt)
	if err != nil {
		return nil, err
	}
	return k.Open()
}

func (k *Kit) Rel(p string) (string, error) {
	if p == "" {
		return "", errors.New("a file path is needed")
	}
	if filepath.IsAbs(p) {
		r, err := filepath.Rel(k.Project.Root, p)
		if err != nil {
			return "", errs.Wrap(err, errs.VFSBadPath, "the path is outside the project")
		}
		p = r
	}
	p = path.Clean(filepath.ToSlash(p))
	if p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return "", errs.New(errs.VFSBadPath, "the path is outside the project", log.F("path", p))
	}
	return p, nil
}

func (s *Session) Close() {
	if s.Pipeline != nil {
		s.Pipeline.Close()
		s.Pipeline = nil
	}
}

func (s *Session) release(fn func() error) error {
	s.Pipeline.Close()
	err := fn()
	pl, oerr := pipeline.Open(s.Project, s.kit.imps, log.Nop())
	if oerr != nil {
		return oerr
	}
	s.Pipeline = pl
	if _, _, serr := pl.Scan(); serr != nil && err == nil {
		err = serr
	}
	s.reload()
	return err
}

func (s *Session) reload() {
	s.Library, s.libErrs = refactor.PrefabLibrary(s.Types.Schema(), s.Pipeline, sources{s.Pipeline})
}

type sources struct {
	pl *pipeline.Pipeline
}

func (r sources) read(id ids.AssetID) ([]byte, bool) {
	rel, ok := r.pl.PathOf(id)
	if !ok {
		return nil, false
	}
	b, err := os.ReadFile(r.pl.Abs(rel))
	return b, err == nil
}

func (r sources) Texture(ids.AssetID) (texture.Region, bool) {
	return texture.Region{}, false
}

func (r sources) Mesh(ids.AssetID) (*mesh.Mesh, bool) {
	return nil, false
}

func (r sources) Scene(id ids.AssetID) ([]byte, bool) {
	return r.read(id)
}

func (r sources) Prefab(id ids.AssetID) ([]byte, bool) {
	return r.read(id)
}

func (r sources) Generation() uint32 {
	return 0
}

func (r sources) Subscribe(func(asset.Reloaded)) func() {
	return func() {}
}

func (s *Session) env() mutate.Env {
	pl := s.Pipeline
	return mutate.Env{
		Types:    s.Types,
		Resolver: sources{pl},
		Library:  s.Library,
		Assets:   func(id ids.AssetID) bool { return refs.Known(pl, id) },
		Lookup:   pl.Lookup,
		OnAdd: func(td *schema.TypeDescriptor, sc *scene.Scene, h scene.NodeHandle) {
			module.RunOnAdd(module.AddContext{Scene: sc, Node: h}, td)
		},
		OnRemove: func(td *schema.TypeDescriptor, sc *scene.Scene, h scene.NodeHandle) {
			module.RunOnRemoving(module.AddContext{Scene: sc, Node: h}, td)
		},
	}
}

func (s *Session) rel(p string) (string, error) {
	return s.kit.Rel(p)
}

func (s *Session) read(rel string) ([]byte, error) {
	b, err := os.ReadFile(s.Pipeline.Abs(rel))
	if err != nil {
		return nil, errs.Wrap(err, errs.VFSIO, "cannot read "+rel)
	}
	return b, nil
}

func (s *Session) exists(rel string) bool {
	_, err := os.Stat(s.Pipeline.Abs(rel))
	return err == nil
}

func (s *Session) asset(text string) (ids.AssetID, error) {
	if id, err := ids.ParseAssetID(text); err == nil {
		return id, nil
	}
	rel, err := s.rel(text)
	if err != nil {
		return ids.AssetID{}, err
	}
	if id, ok := s.Pipeline.Lookup(rel); ok {
		return id, nil
	}
	return ids.AssetID{}, errs.New(errs.AssetNotFound, "no asset with this id or path", log.F("asset", text))
}

func (s *Session) label(id ids.AssetID) string {
	if rel, ok := s.Pipeline.PathOf(id); ok {
		return rel
	}
	return id.String()
}

func (s *Session) writable() error {
	if s.Pipeline.Shared() {
		return errs.New(errs.VFSIO, "another process, usually the editor, holds the asset index; create assets there or close it first")
	}
	return nil
}

func (s *Session) write(edits []refactor.Edit) ([]string, error) {
	if len(edits) == 0 {
		return nil, nil
	}
	if err := refactor.WriteAll(s.Pipeline.Abs, edits); err != nil {
		return nil, err
	}
	rels := make([]string, len(edits))
	prefab := false
	for i, e := range edits {
		rels[i] = e.Path
		prefab = prefab || path.Ext(e.Path) == PrefabExt
	}
	_, err := refactor.Reimport(s.Pipeline, rels)
	if prefab {
		s.reload()
	}
	return rels, err
}

func (s *Session) save(rel string, before, after []byte) ([]string, error) {
	if bytes.Equal(before, after) {
		return nil, nil
	}
	return s.write([]refactor.Edit{{Path: rel, Before: before, After: after}})
}

func (s *Session) create(rel string, data []byte) (ids.AssetID, error) {
	if err := s.writable(); err != nil {
		return ids.AssetID{}, err
	}
	if s.exists(rel) {
		return ids.AssetID{}, errs.New(errs.VFSBadPath, rel+" already exists")
	}
	abs := s.Pipeline.Abs(rel)
	if err := project.WriteAtomic(abs, data); err != nil {
		return ids.AssetID{}, errs.Wrap(err, errs.VFSIO, "cannot write "+rel)
	}
	undo := func() {
		os.Remove(abs)
		os.Remove(abs + pipeline.MetaSuffix)
		s.Pipeline.Scan()
	}
	if _, _, err := s.Pipeline.Scan(); err != nil {
		undo()
		return ids.AssetID{}, err
	}
	id, ok := s.Pipeline.Lookup(rel)
	if !ok {
		undo()
		return ids.AssetID{}, errs.New(errs.VFSBadPath, rel+" has no importer or is outside assets, scenes, scripts and prefabs")
	}
	batch, err := s.Pipeline.Import(id)
	if err == nil && batch != nil {
		if ferr, bad := batch.Failed[id]; bad {
			err = ferr
		}
	}
	if err != nil {
		undo()
		return ids.AssetID{}, err
	}
	return id, nil
}
