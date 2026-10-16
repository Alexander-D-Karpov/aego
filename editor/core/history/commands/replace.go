package commands

import (
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/history"
	"aego/scene"
	"aego/scene/sceneio"
)

type Replace struct {
	Node   ids.NodeID
	Before []byte
	After  []byte
	Name   string
	Post   func(s *scene.Scene)
	parent ids.NodeID
	index  int
	ready  bool
}

func (c *Replace) Label() string {
	if c.Name != "" {
		return c.Name
	}
	return "Replace"
}

func (c *Replace) Do(t history.Target) error {
	s := t.Scene()
	if err := c.swap(s, c.After); err != nil {
		return err
	}
	if c.Post != nil {
		c.Post(s)
	}
	return nil
}

func (c *Replace) Undo(t history.Target) error {
	return c.swap(t.Scene(), c.Before)
}

func (c *Replace) Merge(history.Command) bool {
	return false
}

func (c *Replace) swap(s *scene.Scene, data []byte) error {
	if !c.ready {
		h := s.Handle(c.Node)
		if h.IsZero() || !s.IsAlive(h) {
			return errs.New(errs.SceneDeadNode, "node is not in the scene", log.F("node", c.Node.String()))
		}
		c.parent, c.index, c.ready = s.ID(s.Parent(h)), ChildIndex(s, h), true
	}
	_, err := ReplaceNode(s, c.Node, c.parent, c.index, data, nil)
	return err
}

func ReplaceNode(s *scene.Scene, node, parent ids.NodeID, index int, data []byte, lib *sceneio.Library) (scene.NodeHandle, error) {
	if h := s.Handle(node); !h.IsZero() && s.IsAlive(h) {
		s.Destroy(h)
	}
	s.Flush()
	p, err := parentOf(s, parent)
	if err != nil {
		return scene.NodeHandle{}, err
	}
	created, _, err := sceneio.LoadInto(s, p, data, "replace", sceneio.LoadOptions{KeepIDs: true, Prefabs: lib})
	if err != nil {
		return scene.NodeHandle{}, err
	}
	for _, h := range created {
		if s.Parent(h) != p {
			continue
		}
		if index >= 0 {
			if err := s.Reparent(h, p, index); err != nil {
				return h, err
			}
			s.Flush()
		}
		return h, nil
	}
	return scene.NodeHandle{}, errs.New(errs.EditorCommand, "replacement has no root node")
}
