package prefabs

import (
	"path"
	"strconv"
	"strings"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/assetops"
	"aego/editor/core/clipboard"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/prefab"
	"aego/scene/sceneio"
)

const DefaultDir = "prefabs"

type Service struct {
	ctx   *context.Editor
	infos map[infoKey]*Info
	fixed *sceneio.Library
}

func New(ctx *context.Editor) *Service {
	return &Service{ctx: ctx, infos: map[infoKey]*Info{}}
}

func (s *Service) SetLibrary(lib *sceneio.Library) {
	s.fixed = lib
	clear(s.infos)
}

func (s *Service) lib() *sceneio.Library {
	if s.fixed != nil {
		return s.fixed
	}
	var res asset.Resolver
	if s.ctx.Loader != nil {
		res = s.ctx.Loader
	}
	return sceneio.NewLibrary(s.ctx.Types.Schema(), res)
}

func (s *Service) editable() error {
	if s.ctx.Docs.InPlay() {
		return errs.New(errs.EditorCommand, "prefabs cannot be changed during Play")
	}
	if s.ctx.Pipeline == nil {
		return errs.New(errs.EditorCommand, "the asset pipeline is not available")
	}
	return nil
}

func (s *Service) pathOf(id ids.AssetID) (string, bool) {
	if s.ctx.Pipeline == nil {
		return "", false
	}
	return s.ctx.Pipeline.PathOf(id)
}

func (s *Service) stem(id ids.AssetID) string {
	rel, ok := s.pathOf(id)
	if !ok {
		return id.Short()
	}
	return strings.TrimSuffix(path.Base(rel), path.Ext(rel))
}

func fileSafe(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, strings.TrimSpace(name))
	name = strings.TrimLeft(name, ".")
	if name == "" {
		return "Prefab"
	}
	return name
}

func (s *Service) Open(id ids.AssetID) error {
	rel, ok := s.pathOf(id)
	if !ok {
		return errs.New(errs.EditorDocument, "prefab "+id.Short()+" is not in the project")
	}
	var from document.ID
	if d := s.ctx.Docs.Edit(); d != nil {
		from = d.ID
	}
	_, err := s.ctx.Docs.OpenFrom(rel, from)
	return err
}

func (s *Service) SelectAsset(id ids.AssetID) {
	if rel, ok := s.pathOf(id); ok {
		s.ctx.Open(context.AssetAt(rel))
	}
}

func (s *Service) FindInstances(id ids.AssetID) {
	lib := s.lib()
	changed := map[ids.AssetID]bool{id: true}
	active := s.ctx.Doc()
	var here []ids.NodeID
	elsewhere := 0
	for _, d := range s.ctx.Docs.Docs() {
		sc := d.Scene()
		for _, h := range sc.PreOrder() {
			pi, ok := sceneio.InstanceOf(sc, h)
			if !ok || !pi.Owner || !sc.IsAlive(h) || !s.dependsOn(lib, pi.Prefab, changed) {
				continue
			}
			if active != nil && d.ID == active.ID {
				here = append(here, sc.ID(h))
			} else {
				elsewhere++
			}
		}
	}
	if len(here) > 0 {
		s.ctx.Select(here...)
	}
	msg := strconv.Itoa(len(here)) + " instance(s) of " + s.stem(id) + " in this scene"
	if elsewhere > 0 {
		msg += ", " + strconv.Itoa(elsewhere) + " in other open scenes"
	}
	var files []string
	for _, dep := range s.ctx.Pipeline.Dependents(id) {
		if rel, ok := s.ctx.Pipeline.PathOf(dep); ok {
			files = append(files, rel)
		}
	}
	if len(files) > 0 {
		msg += "; used by " + strings.Join(files, ", ")
	}
	s.ctx.Notify(context.LevelInfo, msg)
}

func (s *Service) Space(id ids.AssetID) (scene.Space, bool) {
	lib := s.lib()
	p, ok := lib.Prefab(id)
	if !ok {
		return scene.Space2D, false
	}
	t, _, err := prefab.Expand(lib, p, nil)
	if err != nil {
		return scene.Space2D, false
	}
	if t.Space3D {
		return scene.Space3D, true
	}
	return scene.Space2D, true
}

func (s *Service) Build(sc *scene.Scene, parent scene.NodeHandle, id ids.AssetID) (scene.NodeHandle, error) {
	if parent.IsZero() {
		parent = sc.Root()
	}
	data := sceneio.EncodeInstance(sc.Registry(), ids.NewNodeID(), &prefab.Instance{Prefab: id})
	created, _, err := sceneio.LoadInto(sc, parent, data, "prefab", sceneio.LoadOptions{KeepIDs: true, Prefabs: s.lib()})
	if err != nil {
		return scene.NodeHandle{}, err
	}
	for _, h := range created {
		if sc.Parent(h) == parent {
			return h, nil
		}
	}
	return scene.NodeHandle{}, errs.New(errs.EditorCommand, "prefab has no root node", log.F("prefab", id.String()))
}

func (s *Service) CreateFrom(d *document.SceneDocument, nodes []ids.NodeID, dir string) error {
	if err := s.editable(); err != nil {
		return err
	}
	if d == nil {
		return nil
	}
	if dir == "" {
		dir = DefaultDir
	}
	sc := d.Scene()
	tops := clipboard.TopLevel(sc, nodes)
	if len(tops) == 0 {
		return nil
	}
	if err := assetops.EnsureDir(s.ctx, dir); err != nil {
		return err
	}
	tx := d.History.Begin("Create Prefab")
	for _, h := range tops {
		if pi, ok := sceneio.InstanceOf(sc, h); ok && pi.Owner {
			tx.Abort()
			return errs.New(errs.EditorCommand, sc.Name(h)+" is already a prefab instance, use Create Variant")
		}
		if root, _, ok := sceneio.OwnerRoot(sc, h); ok && root != h {
			tx.Abort()
			return errs.New(errs.EditorCommand, sc.Name(h)+" belongs to a prefab instance, unpack it first")
		}
		rel := assetops.UniqueFile(s.ctx, dir, fileSafe(sc.Name(h)), document.PrefabExt)
		before := sceneio.SaveSubtree(sc, h, true)
		id, err := assetops.CreateFile(s.ctx, rel, before)
		if err != nil {
			tx.Abort()
			return err
		}
		after, err := s.linkBlock(sc, h, id)
		if err != nil {
			tx.Abort()
			return err
		}
		if err := tx.Exec(commands.NewMutation("Create Prefab", s.env(sc), mutate.Replace(commands.Ref(sc.ID(h)), string(after)))); err != nil {
			tx.Abort()
			return err
		}
		s.ctx.Notify(context.LevelInfo, "Created "+rel)
	}
	tx.Commit()
	return nil
}

func (s *Service) linkBlock(sc *scene.Scene, h scene.NodeHandle, id ids.AssetID) ([]byte, error) {
	lib := s.lib()
	p, ok := lib.Prefab(id)
	if !ok {
		return nil, errs.New(errs.EditorCommand, "the new prefab did not import")
	}
	tree, _, err := prefab.Expand(lib, p, nil)
	if err != nil {
		return nil, err
	}
	inst := &prefab.Instance{Prefab: id, IDs: map[prefab.Key]ids.NodeID{}}
	tree.Walk(func(n, _ *prefab.Node) {
		if sid, ok := sceneIDOf(sc, n.Key); ok {
			inst.IDs[n.Key] = sid
		}
	})
	return sceneio.EncodeInstance(sc.Registry(), sc.ID(h), inst), nil
}

func sceneIDOf(sc *scene.Scene, k prefab.Key) (ids.NodeID, bool) {
	head, rest, nested := strings.Cut(string(k), "/")
	first, err := ids.ParseNodeID(head)
	if err != nil {
		return ids.NodeID{}, false
	}
	if !nested {
		return first, !sc.Handle(first).IsZero()
	}
	if pi, ok := sceneio.InstanceOf(sc, sc.Handle(first)); ok {
		for sid, kk := range pi.Keys {
			if string(kk) == rest {
				return sid, true
			}
		}
	}
	if local, ok := k.Local(); ok && !sc.Handle(local).IsZero() {
		return local, true
	}
	return ids.NodeID{}, false
}

func (s *Service) CreateVariant(id ids.AssetID) error {
	if err := s.editable(); err != nil {
		return err
	}
	rel, ok := s.pathOf(id)
	if !ok {
		return errs.New(errs.EditorDocument, "prefab "+id.Short()+" is not in the project")
	}
	stem := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	dst := assetops.UniqueFile(s.ctx, path.Dir(rel), stem+" Variant", document.PrefabExt)
	root := &prefab.Node{Local: ids.NewNodeID(), Instance: &prefab.Instance{Prefab: id}}
	if _, err := assetops.CreateFile(s.ctx, dst, sceneio.EncodePrefab(s.ctx.Types.Schema(), root)); err != nil {
		return err
	}
	_, err := s.ctx.Docs.Open(dst)
	return err
}

func (s *Service) SaveDocument(d *document.SceneDocument) error {
	id, ok := s.ctx.Pipeline.Lookup(d.Path)
	if !ok || !d.OnDisk || s.ctx.Docs.InPlay() {
		return s.ctx.Docs.Save(d.ID)
	}
	before, err := readFile(s.ctx.Pipeline.Abs(d.Path))
	if err != nil {
		return err
	}
	f := fileChange{id: id, rel: d.Path, before: before, after: sceneio.SavePrefab(d.Scene())}
	if err := s.commit("Save "+path.Base(d.Path), []fileChange{f}, nil); err != nil {
		return err
	}
	return s.ctx.Docs.Save(d.ID)
}
