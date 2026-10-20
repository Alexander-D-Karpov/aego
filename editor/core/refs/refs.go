package refs

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history"
	"aego/editor/core/prefabs"
	"aego/editor/core/scenediff"
	"aego/format/aetext"
	"aego/project"
	"aego/scene/prefab"
	"aego/scene/scriptcomp"
	"aego/schema"
)

const RecoveryDir = ".aego/recovery"

var (
	vAsset = aetext.AssetRef("").Kind
	vNode  = aetext.NodeRef("").Kind
)

type Ref struct {
	Asset   ids.AssetID
	Path    string
	Kind    asset.ArtifactKind
	Project bool
}

type Place struct {
	Node  ids.NodeID
	Path  string
	Owner string
	Field string
}

func (p Place) String() string {
	var parts []string
	for _, s := range []string{p.Path, p.Owner, p.Field} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " · ")
}

type Hit struct {
	Ref    Ref
	Open   bool
	Places []Place
	Err    error
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
	var out []Ref
	if p := s.ctx.Project; p != nil && !target.IsZero() && p.MainScene == target {
		out = append(out, Ref{Project: true, Path: project.FileName})
	}
	pl := s.ctx.Pipeline
	if pl == nil {
		return out
	}
	seen := map[string]bool{}
	for _, a := range pl.Assets() {
		if a.ID == target {
			continue
		}
		if e, ok := pl.Entry(a.ID); ok && slices.Contains(e.Deps, target) {
			out = append(out, Ref{Asset: a.ID, Path: a.Path, Kind: a.Kind})
			seen[a.Path] = true
		}
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
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Project != out[j].Project {
			return out[i].Project
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func textPlaces(doc *aetext.Node, kind aetext.ValueKind, target string) []Place {
	var out []Place
	var visit func(n *aetext.Node, pl Place)
	var value func(v aetext.Value, field string, pl Place)
	value = func(v aetext.Value, field string, pl Place) {
		switch {
		case v.Kind == kind && v.Str == target:
			pl.Field = field
			out = append(out, pl)
		case v.Kind == aetext.VList:
			for i, it := range v.List {
				value(it, field+"["+itoa(i)+"]", pl)
			}
		case v.Kind == aetext.VStruct:
			for _, f := range v.Obj {
				value(f.Value, field+"."+f.Name, pl)
			}
		}
	}
	visit = func(n *aetext.Node, pl Place) {
		switch n.Head {
		case "node":
			if id, err := ids.ParseNodeID(n.Label); err == nil {
				pl.Node = id
			}
			name := n.Label
			if v, ok := n.Attr("name"); ok {
				name = v.Str
			}
			pl.Path = path.Join(pl.Path, name)
			pl.Owner = ""
		case "component", "script":
			pl.Owner = n.Label
		case "override":
			if v, ok := n.Attr("prop"); ok {
				pl.Owner = v.Str
			}
		}
		for _, f := range n.Attrs {
			value(f.Value, f.Name, pl)
		}
		for _, f := range n.Fields {
			value(f.Value, f.Name, pl)
		}
		for _, b := range n.Blocks {
			visit(b, pl)
		}
	}
	visit(doc, Place{})
	return out
}

func itoa(i int) string {
	return strconv.Itoa(i)
}

func (s *Service) Find(target ids.AssetID) []Hit {
	var out []Hit
	for _, r := range s.Referrers(target) {
		h := Hit{Ref: r}
		switch {
		case r.Project:
			h.Places = []Place{{Field: "mainScene"}}
		default:
			if d := s.ctx.Docs.Find(r.Path); d != nil {
				h.Open = true
				h.Places = s.livePlaces(d, isAsset(target))
				break
			}
			data, err := os.ReadFile(s.ctx.Pipeline.Abs(r.Path))
			if err != nil {
				h.Err = err
				break
			}
			doc, err := aetext.Parse(data, r.Path)
			if err != nil {
				h.Err = err
				break
			}
			h.Places = textPlaces(doc, vAsset, target.String())
		}
		out = append(out, h)
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
	root    string
	abs     func(string) string
	changed []string
}

func token(id ids.AssetID) []byte {
	return []byte(scenediff.ValueText(aetext.AssetRef(id.String())))
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
	return len(edits), apply(d.History, "Replace References", func(tx *history.Transaction) error {
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
	if from == to {
		return nil, errs.New(errs.EditorCommand, "an asset cannot replace itself")
	}
	pl := s.ctx.Pipeline
	if pl == nil || s.ctx.Project == nil {
		return nil, errs.New(errs.EditorCommand, "the asset pipeline is not available")
	}
	if !to.IsZero() {
		fe, fok := pl.Entry(from)
		te, tok := pl.Entry(to)
		if !tok {
			return nil, errs.New(errs.EditorCommand, "the replacement is not an imported asset")
		}
		if fok && fe.Kind != te.Kind {
			return nil, errs.New(errs.EditorCommand, "the replacement is a "+string(te.Kind)+", the references expect a "+string(fe.Kind))
		}
	}
	j := &Job{From: from, To: to, root: s.ctx.Project.Root, abs: pl.Abs}
	j.Backup = filepath.ToSlash(filepath.Join(RecoveryDir, "refs-"+s.Now().Format("20060102-150405")))
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
	return j, nil
}

func (j *Job) Rewrite() error {
	from, to := token(j.From), token(j.To)
	backup := filepath.Join(j.root, filepath.FromSlash(j.Backup))
	for _, rel := range j.Files {
		abs := j.abs(rel)
		src, err := os.ReadFile(abs)
		if err != nil {
			return errs.Wrap(err, errs.EditorDocument, "cannot read "+rel)
		}
		if !bytes.Contains(src, from) {
			continue
		}
		bak := filepath.Join(backup, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(bak), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(bak, src, 0o644); err != nil {
			return errs.Wrap(err, errs.EditorDocument, "cannot back up "+rel).With(log.F("backup", j.Backup))
		}
		if err := document.WriteAtomic(abs, bytes.ReplaceAll(src, from, to)); err != nil {
			return errs.Wrap(err, errs.EditorDocument, "cannot write "+rel).With(log.F("backup", j.Backup))
		}
		j.changed = append(j.changed, rel)
	}
	return nil
}

func (j *Job) Changed() []string {
	return j.changed
}

func (s *Service) Finish(j *Job) error {
	pl := s.ctx.Pipeline
	var list []ids.AssetID
	for _, rel := range j.changed {
		if id, ok := pl.Lookup(rel); ok {
			list = append(list, id)
		}
	}
	if len(list) == 0 {
		return nil
	}
	batch, err := pl.Reimport(list...)
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

type Missing struct {
	ID      ids.AssetID
	Grave   pipeline.Grave
	Buried  bool
	Refs    []Ref
	Suggest ids.AssetID
	Path    string
}

func (m Missing) Label() string {
	if m.Buried && m.Grave.Path != "" {
		return m.Grave.Path
	}
	return m.ID.String()
}

func (s *Service) known(id ids.AssetID) bool {
	if id.IsZero() {
		return true
	}
	if _, ok := asset.LookupBuiltin(id); ok {
		return true
	}
	_, ok := s.ctx.Pipeline.PathOf(id)
	return ok
}

func (s *Service) MissingAssets() []Missing {
	pl := s.ctx.Pipeline
	if pl == nil {
		return nil
	}
	by := map[ids.AssetID]*Missing{}
	var order []ids.AssetID
	add := func(id ids.AssetID, r Ref) {
		m := by[id]
		if m == nil {
			m = &Missing{ID: id}
			by[id] = m
			order = append(order, id)
		}
		m.Refs = append(m.Refs, r)
	}
	if p := s.ctx.Project; p != nil && !s.known(p.MainScene) {
		add(p.MainScene, Ref{Project: true, Path: project.FileName})
	}
	assets := pl.Assets()
	for _, a := range assets {
		e, ok := pl.Entry(a.ID)
		if !ok {
			continue
		}
		for _, d := range e.Deps {
			if !s.known(d) {
				add(d, Ref{Asset: a.ID, Path: a.Path, Kind: a.Kind})
			}
		}
	}
	out := make([]Missing, 0, len(order))
	for _, id := range order {
		m := by[id]
		if g, ok := pl.Grave(id); ok {
			m.Grave, m.Buried = g, true
			base := strings.ToLower(path.Base(g.Path))
			for _, a := range assets {
				if a.Kind == g.Kind && strings.ToLower(path.Base(a.Path)) == base {
					m.Suggest, m.Path = a.ID, a.Path
					break
				}
			}
		}
		out = append(out, *m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Label() < out[j].Label() })
	return out
}

type MissingNode struct {
	Scene  string
	Node   ids.NodeID
	Path   string
	Owner  string
	Field  string
	Target ids.NodeID
	Open   bool
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
		data, err := os.ReadFile(pl.Abs(a.Path))
		if err != nil {
			continue
		}
		doc, err := aetext.Parse(data, a.Path)
		if err != nil {
			continue
		}
		out = append(out, textMissingNodes(doc, a.Path)...)
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
		out = append(out, MissingNode{Scene: d.Path, Node: id, Path: sc.Path(sc.Handle(id)), Owner: td.FullName, Field: fd.Name, Target: target, Open: true})
	})
	return out
}

func textMissingNodes(doc *aetext.Node, scene string) []MissingNode {
	defined := map[string]bool{}
	var collect func(n *aetext.Node)
	collect = func(n *aetext.Node) {
		if n.Head == "node" {
			defined[n.Label] = true
		}
		for _, b := range n.Blocks {
			collect(b)
		}
	}
	collect(doc)
	var refs []string
	var scan func(v aetext.Value)
	scan = func(v aetext.Value) {
		switch v.Kind {
		case vNode:
			refs = append(refs, v.Str)
		case aetext.VList:
			for _, it := range v.List {
				scan(it)
			}
		case aetext.VStruct:
			for _, f := range v.Obj {
				scan(f.Value)
			}
		}
	}
	var walk func(n *aetext.Node)
	walk = func(n *aetext.Node) {
		for _, f := range n.Fields {
			scan(f.Value)
		}
		for _, f := range n.Attrs {
			scan(f.Value)
		}
		for _, b := range n.Blocks {
			walk(b)
		}
	}
	walk(doc)
	var out []MissingNode
	seen := map[string]bool{}
	for _, id := range refs {
		if id == "" || defined[id] || seen[id] {
			continue
		}
		nid, err := ids.ParseNodeID(id)
		if err != nil || nid.IsZero() {
			continue
		}
		seen[id] = true
		for _, pl := range textPlaces(doc, vNode, id) {
			out = append(out, MissingNode{Scene: scene, Node: pl.Node, Path: pl.Path, Owner: pl.Owner, Field: pl.Field, Target: nid})
		}
	}
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
	return apply(d.History, "Clear Missing Node Reference", func(tx *history.Transaction) error {
		for _, e := range edits {
			if err := d.Props.Set(tx, []ids.NodeID{e.node}, e.t, e.f, e.v); err != nil {
				return err
			}
		}
		return nil
	})
}
