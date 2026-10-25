package prefabs

import (
	"os"
	"slices"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history"
	"aego/editor/core/history/commands"
	"aego/engine/i18n"
	"aego/project/refactor"
	"aego/scene"
	"aego/scene/prefab"
	"aego/scene/sceneio"
)

type fileChange struct {
	id     ids.AssetID
	rel    string
	before []byte
	after  []byte
}

type source struct {
	doc    document.ID
	node   ids.NodeID
	before []byte
	after  []byte
}

type target struct {
	doc    *document.SceneDocument
	node   ids.NodeID
	parent ids.NodeID
	index  int
	block  []byte
}

type group struct {
	svc     *Service
	label   string
	files   []fileChange
	src     *source
	applied bool
}

type groupCmd struct {
	g *group
}

func (c *groupCmd) Label() string {
	return c.g.label
}

func (c *groupCmd) Do(history.Target) error {
	if c.g.applied {
		return nil
	}
	return c.g.run(true)
}

func (c *groupCmd) Undo(history.Target) error {
	if !c.g.applied {
		return nil
	}
	return c.g.run(false)
}

func (c *groupCmd) Merge(history.Command) bool {
	return false
}

func readFile(abs string) ([]byte, error) {
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, errs.Wrap(err, errs.EditorDocument, "cannot read the prefab")
	}
	return data, nil
}

func (s *Service) files(plan *refactor.ApplyPlan) ([]fileChange, error) {
	if s.ctx.Pipeline == nil {
		return nil, errs.New(errs.EditorCommand, "the asset pipeline is not available")
	}
	edits, err := plan.Edits(s.ctx.Types.Schema(), s.ctx.Pipeline)
	if err != nil {
		return nil, err
	}
	out := make([]fileChange, len(edits))
	for i, e := range edits {
		out[i] = fileChange{id: e.ID, rel: e.Path, before: e.Before, after: e.After}
	}
	return out, nil
}

func (s *Service) reimport(list []ids.AssetID) error {
	if len(list) == 0 {
		return nil
	}
	s.writing = true
	defer func() { s.writing = false }()
	batch, err := s.ctx.Pipeline.Reimport(list...)
	s.ctx.ApplyBatch(batch)
	if err != nil {
		return err
	}
	for _, id := range list {
		if ferr, bad := batch.Failed[id]; bad {
			return ferr
		}
	}
	return nil
}

func (s *Service) dependsOn(lib *sceneio.Library, id ids.AssetID, changed map[ids.AssetID]bool) bool {
	seen := map[ids.AssetID]bool{}
	var visit func(ids.AssetID) bool
	var walk func(*prefab.Node) bool
	visit = func(id ids.AssetID) bool {
		if changed[id] {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		p, ok := lib.Prefab(id)
		return ok && walk(p)
	}
	walk = func(n *prefab.Node) bool {
		if n.Instance != nil {
			if visit(n.Instance.Prefab) {
				return true
			}
			for _, a := range n.Instance.Overrides.AddNodes {
				if walk(a.Node) {
					return true
				}
			}
			return false
		}
		for _, c := range n.Children {
			if walk(c) {
				return true
			}
		}
		return false
	}
	return visit(id)
}

func covered(sc *scene.Scene, h scene.NodeHandle, taken []scene.NodeHandle) bool {
	for p := sc.Parent(h); !p.IsZero(); p = sc.Parent(p) {
		if slices.Contains(taken, p) {
			return true
		}
	}
	return false
}

func (s *Service) affected(lib *sceneio.Library, changed map[ids.AssetID]bool, blocks bool) []target {
	var out []target
	for _, d := range s.ctx.Docs.Docs() {
		sc := d.Scene()
		var taken []scene.NodeHandle
		for _, h := range sc.PreOrder() {
			pi, ok := sceneio.InstanceOf(sc, h)
			if !ok || !pi.Owner || !sc.IsAlive(h) || covered(sc, h, taken) || !s.dependsOn(lib, pi.Prefab, changed) {
				continue
			}
			taken = append(taken, h)
			t := target{doc: d, node: sc.ID(h), parent: sc.ID(sc.Parent(h)), index: commands.ChildIndex(sc, h)}
			if blocks {
				t.block = sceneio.SaveSubtreeWith(sc, h, lib)
			}
			out = append(out, t)
		}
	}
	return out
}

func (g *group) changed() map[ids.AssetID]bool {
	out := make(map[ids.AssetID]bool, len(g.files))
	for _, f := range g.files {
		out[f.id] = true
	}
	return out
}

func (g *group) ids() []ids.AssetID {
	out := make([]ids.AssetID, len(g.files))
	for i, f := range g.files {
		out[i] = f.id
	}
	return out
}

func (g *group) write(forward bool) error {
	pl := g.svc.ctx.Pipeline
	for i, f := range g.files {
		data := f.after
		if !forward {
			data = f.before
		}
		if err := document.WriteAtomic(pl.Abs(f.rel), data); err != nil {
			for _, w := range g.files[:i] {
				back := w.before
				if !forward {
					back = w.after
				}
				document.WriteAtomic(pl.Abs(w.rel), back)
			}
			g.svc.reimport(g.ids())
			return errs.Wrap(err, errs.EditorDocument, "cannot write "+f.rel)
		}
	}
	if err := g.svc.reimport(g.ids()); err != nil {
		for _, f := range g.files {
			back := f.before
			if !forward {
				back = f.after
			}
			document.WriteAtomic(pl.Abs(f.rel), back)
		}
		g.svc.reimport(g.ids())
		return err
	}
	return nil
}

func (g *group) run(forward bool) error {
	svc := g.svc
	from := svc.lib()
	plan := svc.affected(from, g.changed(), true)
	if err := g.write(forward); err != nil {
		return err
	}
	to := svc.lib()
	var done []target
	for _, t := range plan {
		data := t.block
		if g.src != nil && t.doc.ID == g.src.doc && t.node == g.src.node {
			data = g.src.after
			if !forward {
				data = g.src.before
			}
		}
		if _, err := commands.ReplaceNode(t.doc.Scene(), t.node, t.parent, t.index, data, to); err != nil {
			g.write(!forward)
			back := svc.lib()
			for _, r := range done {
				commands.ReplaceNode(r.doc.Scene(), r.node, r.parent, r.index, r.block, back)
			}
			return err
		}
		done = append(done, t)
	}
	g.applied = forward
	return nil
}

func (s *Service) commit(label string, files []fileChange, src *source) error {
	if len(files) == 0 {
		s.ctx.Notify(context.LevelInfo, i18n.Note("editor.prefabs.the_prefab_already_matches", "The prefab already matches"))
		return nil
	}
	g := &group{svc: s, label: label, files: files, src: src}
	var hosts []*document.SceneDocument
	for _, t := range s.affected(s.lib(), g.changed(), false) {
		if !slices.Contains(hosts, t.doc) {
			hosts = append(hosts, t.doc)
		}
	}
	if src != nil {
		if d := s.ctx.Docs.Get(src.doc); d != nil && !slices.Contains(hosts, d) {
			hosts = append([]*document.SceneDocument{d}, hosts...)
		}
	}
	for _, d := range hosts {
		if reason, locked := d.History.Locked(); locked {
			return errs.New(errs.EditorHistory, reason)
		}
	}
	if len(hosts) == 0 {
		return g.run(true)
	}
	for _, d := range hosts {
		if err := d.History.Exec(&groupCmd{g: g}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) External(b *pipeline.Batch) func() {
	if s.writing || s.ctx.Docs.InPlay() {
		return nil
	}
	changed := map[ids.AssetID]bool{}
	for _, e := range b.Imported {
		if e.Kind == asset.ArtPrefab {
			changed[e.ID] = true
		}
	}
	if len(changed) == 0 {
		return nil
	}
	plan := s.affected(s.lib(), changed, true)
	if len(plan) == 0 {
		return nil
	}
	return func() {
		lib := s.lib()
		for _, t := range plan {
			if _, err := commands.ReplaceNode(t.doc.Scene(), t.node, t.parent, t.index, t.block, lib); err != nil {
				s.ctx.Notify(context.LevelError, i18n.Note("editor.prefabs.cannot_refresh_the_prefab_instances", "cannot refresh the prefab instances in {path}: {error}", i18n.Str("path", t.doc.Path), i18n.Str("error", err.Error())))
			}
		}
		clear(s.infos)
	}
}
