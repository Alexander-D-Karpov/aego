package prefabs

import (
	"errors"
	"maps"

	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/prefab"
	"aego/scene/sceneio"
)

type Level = prefab.Level

const (
	LevelProperty  = prefab.LevelProperty
	LevelComponent = prefab.LevelComponent
	LevelNode      = prefab.LevelNode
	LevelAll       = prefab.LevelAll
	LevelOrphan    = prefab.LevelOrphan
)

type Change struct {
	Node    ids.NodeID
	Key     prefab.Key
	Level   Level
	Prop    prefab.PropKey
	Nearest bool
}

func (c Change) split(o prefab.Overrides, key, rootKey prefab.Key) (sel, rest prefab.Overrides) {
	return prefab.Pick{Level: c.Level, Key: key, Prop: c.Prop}.Split(o, rootKey)
}

func (s *Service) locate(d *document.SceneDocument, c Change) (scene.NodeHandle, *sceneio.PrefabInstance, prefab.Key, error) {
	if d == nil {
		return scene.NodeHandle{}, nil, "", errs.New(errs.EditorDocument, "no scene is open")
	}
	sc := d.Scene()
	h := sc.Handle(c.Node)
	if h.IsZero() || !sc.IsAlive(h) {
		return scene.NodeHandle{}, nil, "", errs.New(errs.SceneDeadNode, "node is not in the scene")
	}
	root, pi, ok := sceneio.OwnerRoot(sc, h)
	if !ok {
		return scene.NodeHandle{}, nil, "", errs.New(errs.EditorCommand, sc.Name(h)+" is not part of a prefab instance")
	}
	if pi.Missing {
		return scene.NodeHandle{}, nil, "", errs.New(errs.EditorCommand, "the prefab of "+sc.Name(root)+" is missing")
	}
	key := c.Key
	if key == "" {
		if k, ok := pi.Keys[c.Node]; ok {
			key = k
		} else {
			key = prefab.LocalKey(c.Node)
		}
	}
	return root, pi, key, nil
}

func (s *Service) env(sc *scene.Scene) mutate.Env {
	env := commands.EnvFor(sc)
	env.Library = s.lib()
	return env
}

func (s *Service) Revert(d *document.SceneDocument, c Change) error {
	if err := s.editable(); err != nil {
		return err
	}
	_, _, key, err := s.locate(d, c)
	if err != nil {
		return err
	}
	sc := d.Scene()
	op := mutate.RevertPrefab(commands.Ref(c.Node), c.Level.String())
	op.Key, op.Owner, op.Field = string(key), c.Prop.Owner, c.Prop.Field
	label := "Revert"
	if c.Level == LevelOrphan {
		label = "Remove Unused Override"
	}
	err = d.History.Exec(commands.NewMutation(label, s.env(sc), op))
	if errors.Is(err, mutate.ErrNothing) {
		return nil
	}
	return err
}

func (s *Service) Unpack(d *document.SceneDocument, node ids.NodeID, deep bool) error {
	if err := s.editable(); err != nil {
		return err
	}
	if d == nil {
		return nil
	}
	sc := d.Scene()
	if _, _, ok := sceneio.OwnerRoot(sc, sc.Handle(node)); !ok {
		return errs.New(errs.EditorCommand, "the node is not part of a prefab instance")
	}
	label := "Unpack Prefab"
	if deep {
		label = "Unpack Prefab Completely"
	}
	return d.History.Exec(commands.NewMutation(label, s.env(sc), mutate.Unpack(commands.Ref(node), deep)))
}

func addPrefix(base *prefab.Node, parent prefab.Key) string {
	if n := base.Find(parent); n != nil && n != base && n.Origin != nil {
		return string(parent)
	}
	return string(parent.Instance())
}

func (s *Service) Apply(d *document.SceneDocument, c Change) error {
	if err := s.editable(); err != nil {
		return err
	}
	if c.Level == LevelOrphan {
		return errs.New(errs.EditorCommand, "unused overrides cannot be applied")
	}
	root, pi, key, err := s.locate(d, c)
	if err != nil {
		return err
	}
	sc := d.Scene()
	lib := s.lib()
	inst, err := sceneio.Overrides(sc, root, lib)
	if err != nil {
		return err
	}
	sel, rest := c.split(inst.Overrides, key, pi.Keys[sc.ID(root)])
	if sel.Empty() {
		s.ctx.Notify(context.LevelInfo, "Nothing to apply")
		return nil
	}
	p, ok := lib.Prefab(pi.Prefab)
	if !ok {
		return errs.New(errs.EditorCommand, "the prefab is not loaded")
	}
	base, _, err := prefab.Expand(lib, p, nil)
	if err != nil {
		return err
	}
	b := newBaker(lib)
	if err := b.bake(pi.Prefab, sel, c.Nearest); err != nil {
		return err
	}
	files, err := s.files(b.edits)
	if err != nil {
		return err
	}
	keep := maps.Clone(inst.IDs)
	for _, a := range sel.AddNodes {
		prefix := addPrefix(base, a.Parent)
		a.Node.Walk(func(n, _ *prefab.Node) {
			if n.Instance != nil {
				return
			}
			k := prefab.LocalKey(n.Local)
			if prefix != "" {
				k = prefab.Key(prefix) + "/" + k
			}
			keep[k] = n.Local
		})
	}
	src := &source{
		doc:    d.ID,
		node:   sc.ID(root),
		before: sceneio.SaveSubtreeWith(sc, root, lib),
		after:  sceneio.EncodeInstance(sc.Registry(), sc.ID(root), &prefab.Instance{Prefab: pi.Prefab, Overrides: rest, IDs: keep}),
	}
	target := s.stem(pi.Prefab)
	if c.Nearest {
		if id, ok := b.Nearest(pi.Prefab, key); ok {
			target = s.stem(id)
		}
	}
	return s.commit("Apply to "+target, files, src)
}
