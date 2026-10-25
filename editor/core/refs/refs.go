package refs

import (
	"time"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history"
	"aego/editor/core/prefabs"
	"aego/engine/i18n"
	"aego/project/refactor"
	prefs "aego/project/refs"
	"aego/scene/prefab"
	"aego/scene/scriptcomp"
	"aego/schema"
	"slices"
)

const RecoveryDir = refactor.RecoveryDir

type (
	Ref     = prefs.Ref
	Place   = prefs.Place
	Missing = prefs.Missing
)

type Hit struct {
	prefs.Hit
	Open bool
}

type MissingNode struct {
	prefs.MissingNode
	Open bool
}

type PrefabInfo interface {
	Info(d *document.SceneDocument, node ids.NodeID) (*prefabs.Info, bool)
}

type Service struct {
	ctx     *context.Editor
	Prefabs PrefabInfo
	Now     func() time.Time
}

func New(ctx *context.Editor) *Service {
	return &Service{ctx: ctx, Now: time.Now}
}

func (s *Service) source() prefs.Source {
	if s.ctx.Pipeline == nil {
		return nil
	}
	return s.ctx.Pipeline
}

func leafAny(v schema.Value, leaf func(schema.Value) bool) bool {
	switch v.Kind {
	case schema.KindAssetRef, schema.KindNodeRef:
		return leaf(v)
	case schema.KindList:
		for _, it := range v.List {
			if leafAny(it, leaf) {
				return true
			}
		}
	case schema.KindStruct:
		if v.Obj != nil {
			for _, f := range v.Obj.Fields {
				if leafAny(f, leaf) {
					return true
				}
			}
		}
	}
	return false
}

func rewrite(v schema.Value, leaf func(schema.Value) (schema.Value, bool)) (schema.Value, bool) {
	switch v.Kind {
	case schema.KindAssetRef, schema.KindNodeRef:
		return leaf(v)
	case schema.KindList:
		var out []schema.Value
		for i, it := range v.List {
			nv, ok := rewrite(it, leaf)
			if !ok {
				continue
			}
			if out == nil {
				out = slices.Clone(v.List)
			}
			out[i] = nv
		}
		if out != nil {
			v.List = out
			return v, true
		}
	case schema.KindStruct:
		if v.Obj == nil {
			return v, false
		}
		var obj *schema.Object
		for i, f := range v.Obj.Fields {
			nv, ok := rewrite(f, leaf)
			if !ok {
				continue
			}
			if obj == nil {
				obj = &schema.Object{Type: v.Obj.Type, Fields: slices.Clone(v.Obj.Fields)}
			}
			obj.Fields[i] = nv
		}
		if obj != nil {
			v.Obj = obj
			return v, true
		}
	}
	return v, false
}

func isAsset(id ids.AssetID) func(schema.Value) bool {
	return func(v schema.Value) bool { return v.Kind == schema.KindAssetRef && v.Asset == id }
}

func swapAsset(from, to ids.AssetID) func(schema.Value) (schema.Value, bool) {
	return func(v schema.Value) (schema.Value, bool) {
		if v.Kind == schema.KindAssetRef && v.Asset == from {
			return schema.Asset(to), true
		}
		return v, false
	}
}

type visitFn func(id ids.NodeID, td *schema.TypeDescriptor, fd *schema.FieldDescriptor, owner string, v schema.Value)

func (s *Service) walk(d *document.SceneDocument, leaf func(schema.Value) bool, overrides bool, fn visitFn) {
	sc := d.Scene()
	reg := sc.Registry()
	root := sc.Root()
	unknown, hasUnknown := reg.ByName("engine.Unknown")
	for _, h := range sc.PreOrder() {
		if h == root || !sc.IsAlive(h) {
			continue
		}
		id := sc.ID(h)
		var info *prefabs.Info
		if overrides && s.Prefabs != nil {
			if inf, ok := s.Prefabs.Info(d, id); ok && !inf.Missing {
				info = inf
			}
		}
		visit := func(td *schema.TypeDescriptor, owner string) {
			for i := range td.Fields {
				fd := &td.Fields[i]
				v, err := d.Props.Get(id, td.ID, fd.ID)
				if err != nil || !leafAny(v, leaf) {
					continue
				}
				if info != nil && !info.Added(id) && !info.Overridden(id, owner, fd.Name) {
					continue
				}
				fn(id, td, fd, owner, v)
			}
		}
		sc.ComponentsOf(h, func(t schema.TypeID) {
			if (hasUnknown && t == unknown.ID) || t == scriptcomp.ScriptTypeID {
				return
			}
			if td, ok := reg.ByID(t); ok {
				visit(td, td.FullName)
			}
		})
		if scs, ok := scriptcomp.ScriptsOf(sc, h); ok {
			for i := range scs.Attachments {
				name := scs.Attachments[i].TypeName
				if td, ok := reg.ByName(name); ok && td.Kind == schema.TypeScript {
					visit(td, prefab.ScriptOwner(name))
				}
			}
		}
	}
}

func (s *Service) livePlaces(d *document.SceneDocument, leaf func(schema.Value) bool) []Place {
	var out []Place
	sc := d.Scene()
	s.walk(d, leaf, false, func(id ids.NodeID, td *schema.TypeDescriptor, fd *schema.FieldDescriptor, _ string, _ schema.Value) {
		out = append(out, Place{Node: id, Path: sc.Path(sc.Handle(id)), Owner: td.FullName, Field: fd.Name})
	})
	return out
}

func (s *Service) Referrers(target ids.AssetID) []Ref {
	src := s.source()
	out := prefs.Referrers(s.ctx.Project, src, target)
	if src == nil {
		return out
	}
	pl := s.ctx.Pipeline
	seen := map[string]bool{}
	for _, r := range out {
		seen[r.Path] = true
	}
	for _, d := range s.ctx.Docs.Docs() {
		if seen[d.Path] || !d.Dirty() || d.IsPlay() || len(s.livePlaces(d, isAsset(target))) == 0 {
			continue
		}
		r := Ref{Path: d.Path, Kind: asset.ArtScene}
		if d.IsPrefab() {
			r.Kind = asset.ArtPrefab
		}
		if id, ok := pl.Lookup(d.Path); ok {
			r.Asset = id
		}
		out = append(out, r)
	}
	prefs.Sort(out)
	return out
}

func (s *Service) Find(target ids.AssetID) []Hit {
	var out []Hit
	for _, r := range s.Referrers(target) {
		if !r.Project {
			if d := s.ctx.Docs.Find(r.Path); d != nil {
				out = append(out, Hit{Hit: prefs.Hit{Ref: r, Places: s.livePlaces(d, isAsset(target))}, Open: true})
				continue
			}
		}
		out = append(out, Hit{Hit: prefs.FileHit(s.source(), r, target)})
	}
	return out
}

type Job struct {
	From    ids.AssetID
	To      ids.AssetID
	Docs    []string
	Files   []string
	Backup  string
	Project bool
	plan    *refactor.Replacement
	root    string
	abs     func(string) string
	changed []string
}

func (s *Service) replaceLive(d *document.SceneDocument, from, to ids.AssetID) (int, error) {
	type edit struct {
		node ids.NodeID
		t    schema.TypeID
		f    schema.FieldID
		v    schema.Value
	}
	var edits []edit
	s.walk(d, isAsset(from), true, func(id ids.NodeID, td *schema.TypeDescriptor, fd *schema.FieldDescriptor, _ string, v schema.Value) {
		if fd.Flags.Has(schema.FlagReadOnly) {
			return
		}
		if nv, ok := rewrite(v, swapAsset(from, to)); ok {
			edits = append(edits, edit{node: id, t: td.ID, f: fd.ID, v: nv})
		}
	})
	if len(edits) == 0 {
		return 0, nil
	}
	return len(edits), apply(d.History, i18n.Tr("editor.history.replace_references", "Replace References"), func(tx *history.Transaction) error {
		for _, e := range edits {
			if err := d.Props.Set(tx, []ids.NodeID{e.node}, e.t, e.f, e.v); err != nil {
				return err
			}
		}
		return nil
	})
}

func apply(h *history.History, label string, fn func(*history.Transaction) error) error {
	tx := h.Begin(label)
	if err := fn(tx); err != nil {
		tx.Abort()
		return err
	}
	tx.Commit()
	return nil
}

func (s *Service) Begin(from, to ids.AssetID) (*Job, error) {
	pl := s.ctx.Pipeline
	if pl == nil || s.ctx.Project == nil {
		return nil, errs.New(errs.EditorCommand, "the asset pipeline is not available")
	}
	if err := refactor.CheckReplace(pl, from, to); err != nil {
		return nil, err
	}
	j := &Job{From: from, To: to, root: s.ctx.Project.Root, abs: pl.Abs, Backup: refactor.BackupDir(s.Now())}
	for _, r := range s.Referrers(from) {
		switch {
		case r.Project:
			j.Project = true
			if !to.IsZero() {
				if rel, ok := pl.PathOf(to); ok {
					assetops.SetStartup(s.ctx, to, rel)
				}
			}
		case s.ctx.Docs.Find(r.Path) != nil:
			d := s.ctx.Docs.Find(r.Path)
			if d.IsPlay() {
				return nil, errs.New(errs.EditorCommand, "stop Play before replacing references")
			}
			if _, err := s.replaceLive(d, from, to); err != nil {
				return nil, errs.Wrap(err, errs.EditorCommand, "cannot update "+r.Path)
			}
			j.Docs = append(j.Docs, r.Path)
		default:
			j.Files = append(j.Files, r.Path)
		}
	}
	j.plan = &refactor.Replacement{From: from, To: to, Project: j.Project, Files: j.Files}
	return j, nil
}

func (j *Job) Rewrite() error {
	changed, err := j.plan.Write(j.root, j.abs, j.Backup)
	j.changed = changed
	return err
}

func (j *Job) Changed() []string {
	return j.changed
}

func (s *Service) Finish(j *Job) error {
	batch, err := refactor.Reimport(s.ctx.Pipeline, j.changed)
	s.ctx.ApplyBatch(batch)
	return err
}

func (s *Service) Replace(from, to ids.AssetID) (*Job, error) {
	j, err := s.Begin(from, to)
	if err != nil {
		return nil, err
	}
	if err := j.Rewrite(); err != nil {
		return j, err
	}
	return j, s.Finish(j)
}

func (s *Service) MissingAssets() []Missing {
	src := s.source()
	if src == nil {
		return nil
	}
	return prefs.MissingAssets(s.ctx.Project, src)
}

func (s *Service) MissingNodes() []MissingNode {
	pl := s.ctx.Pipeline
	if pl == nil {
		return nil
	}
	var out []MissingNode
	for _, a := range pl.Assets() {
		if a.Kind != asset.ArtScene && a.Kind != asset.ArtPrefab {
			continue
		}
		if d := s.ctx.Docs.Find(a.Path); d != nil {
			out = append(out, s.liveMissingNodes(d)...)
			continue
		}
		for _, m := range prefs.FileMissingNodes(pl, a.Path) {
			out = append(out, MissingNode{MissingNode: m})
		}
	}
	return out
}

func (s *Service) liveMissingNodes(d *document.SceneDocument) []MissingNode {
	sc := d.Scene()
	dead := func(v schema.Value) bool {
		if v.Kind != schema.KindNodeRef || v.Node.IsZero() {
			return false
		}
		h := sc.Handle(v.Node)
		return h.IsZero() || !sc.IsAlive(h)
	}
	var out []MissingNode
	s.walk(d, dead, false, func(id ids.NodeID, td *schema.TypeDescriptor, fd *schema.FieldDescriptor, _ string, v schema.Value) {
		var target ids.NodeID
		leafAny(v, func(x schema.Value) bool {
			if dead(x) {
				target = x.Node
				return true
			}
			return false
		})
		m := prefs.MissingNode{Scene: d.Path, Node: id, Path: sc.Path(sc.Handle(id)), Owner: td.FullName, Field: fd.Name, Target: target}
		out = append(out, MissingNode{MissingNode: m, Open: true})
	})
	return out
}

func (s *Service) ClearNode(m MissingNode) error {
	d, err := s.ctx.Docs.Open(m.Scene)
	if err != nil {
		return err
	}
	is := func(v schema.Value) bool { return v.Kind == schema.KindNodeRef && v.Node == m.Target }
	type edit struct {
		node ids.NodeID
		t    schema.TypeID
		f    schema.FieldID
		v    schema.Value
	}
	var edits []edit
	s.walk(d, is, true, func(id ids.NodeID, td *schema.TypeDescriptor, fd *schema.FieldDescriptor, _ string, v schema.Value) {
		if fd.Flags.Has(schema.FlagReadOnly) {
			return
		}
		nv, ok := rewrite(v, func(x schema.Value) (schema.Value, bool) {
			if is(x) {
				return schema.Node(ids.NodeID{}), true
			}
			return x, false
		})
		if ok {
			edits = append(edits, edit{node: id, t: td.ID, f: fd.ID, v: nv})
		}
	})
	if len(edits) == 0 {
		return nil
	}
	return apply(d.History, i18n.Tr("editor.history.clear_missing_node_reference", "Clear Missing Node Reference"), func(tx *history.Transaction) error {
		for _, e := range edits {
			if err := d.Props.Set(tx, []ids.NodeID{e.node}, e.t, e.f, e.v); err != nil {
				return err
			}
		}
		return nil
	})
}
