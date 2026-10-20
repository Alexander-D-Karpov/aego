package prefabs

import (
	"errors"

	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/project/refactor"
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

type Change = refactor.PrefabChange

func (s *Service) locate(d *document.SceneDocument, c Change) (scene.NodeHandle, *sceneio.PrefabInstance, prefab.Key, error) {
	if d == nil {
		return scene.NodeHandle{}, nil, "", errs.New(errs.EditorDocument, "no scene is open")
	}
	return refactor.Locate(d.Scene(), c.Node, c.Key)
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

func (s *Service) Apply(d *document.SceneDocument, c Change) error {
	if err := s.editable(); err != nil {
		return err
	}
	if d == nil {
		return errs.New(errs.EditorDocument, "no scene is open")
	}
	plan, err := refactor.PlanApply(d.Scene(), s.lib(), c)
	if errors.Is(err, mutate.ErrNothing) {
		s.ctx.Notify(context.LevelInfo, "Nothing to apply")
		return nil
	}
	if err != nil {
		return err
	}
	files, err := s.files(plan)
	if err != nil {
		return err
	}
	src := &source{doc: d.ID, node: plan.Root, before: plan.Before, after: plan.After}
	return s.commit("Apply to "+s.stem(plan.Target), files, src)
}
