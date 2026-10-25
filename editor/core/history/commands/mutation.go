package commands

import (
	"aego/core/ids"
	"aego/editor/core/history"
	"aego/engine/i18n"
	"aego/engine/module"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/schema"
)

type Mutation struct {
	Name string
	Env  mutate.Env
	Tx   mutate.Tx
	res  *mutate.Result
	inv  mutate.Tx
	redo mutate.Tx
	done bool
}

func EnvFor(s *scene.Scene) mutate.Env {
	return mutate.Env{
		Types:    s.Types(),
		Resolver: s.Resolver(),
		OnRemove: func(td *schema.TypeDescriptor, s *scene.Scene, h scene.NodeHandle) {
			module.RunOnRemoving(module.AddContext{Scene: s, Node: h}, td)
		},
	}
}

func NewMutation(name string, env mutate.Env, ops ...mutate.Op) *Mutation {
	return &Mutation{Name: name, Env: env, Tx: mutate.Tx{Label: name, Ops: ops}}
}

func Ref(id ids.NodeID) string {
	if id.IsZero() {
		return mutate.RootName
	}
	return "#" + id.String()
}

func SetField(s *scene.Scene, node ids.NodeID, td *schema.TypeDescriptor, fd *schema.FieldDescriptor, v schema.Value, name string) *Mutation {
	op := mutate.SetValue(Ref(node), td.FullName+"."+fd.Name, sceneio.WriteValue(s.Registry(), fd, v)).Runtime()
	return NewMutation(i18n.Tr("editor.history.set", "Set {name}", i18n.Str("name", name)), EnvFor(s), op)
}

// Result is what the last Do changed, nil before it ran.
func (c *Mutation) Result() *mutate.Result {
	return c.res
}

func (c *Mutation) Label() string {
	return c.Name
}

func (c *Mutation) doc(t history.Target) *mutate.Document {
	s := t.Scene()
	env := c.Env
	if env.Types == nil {
		env.Types = s.Types()
	}
	return mutate.Wrap(s, "", false, env)
}

func (c *Mutation) Do(t history.Target) error {
	tx := c.Tx
	if c.done {
		tx = c.redo
	}
	res, err := c.doc(t).Apply(tx)
	if err != nil {
		return err
	}
	c.res, c.inv, c.redo, c.done = res, res.Inverse, res.Redo, true
	return nil
}

func (c *Mutation) Undo(t history.Target) error {
	_, err := c.doc(t).Apply(c.inv)
	return err
}

func (c *Mutation) field() (string, string, bool) {
	if c.res == nil || len(c.redo.Ops) != 1 || len(c.res.Changes) != 1 {
		return "", "", false
	}
	op := c.redo.Ops[0]
	if op.Kind != mutate.KindSet || c.res.Changes[0].Type == 0 {
		return "", "", false
	}
	return op.Node, op.Path, true
}

func (c *Mutation) Merge(next history.Command) bool {
	n, ok := next.(*Mutation)
	if !ok {
		return false
	}
	node, path, ok := c.field()
	if !ok {
		return false
	}
	nn, np, ok := n.field()
	if !ok || nn != node || np != path {
		return false
	}
	c.redo = n.redo
	c.res.Changes[0].After = n.res.Changes[0].After
	return true
}

func (c *Mutation) Record(cs *module.ChangeSet, undo bool) {
	if c.res == nil {
		return
	}
	for _, ch := range c.res.Changes {
		switch ch.Op {
		case mutate.KindSet, mutate.KindReset:
			if ch.Type != 0 {
				cs.MarkField(ch.NodeID, ch.Type, ch.Field)
			}
		case mutate.KindAddComp:
			if undo {
				cs.MarkRemoved(ch.NodeID, ch.Type)
			} else {
				cs.MarkAdded(ch.NodeID, ch.Type)
			}
		case mutate.KindRemoveComp:
			if undo {
				cs.MarkAdded(ch.NodeID, ch.Type)
			} else {
				cs.MarkRemoved(ch.NodeID, ch.Type)
			}
		}
	}
}

var _ history.Command = (*Mutation)(nil)
