package commands

import (
	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/history"
	"aego/scene"
	"aego/scene/sceneio"
)

type Create struct {
	Parent   ids.NodeID
	Index    int
	Fragment []byte
	Name     string
	roots    []ids.NodeID
}

func (c *Create) Label() string {
	if c.Name != "" {
		return "Create " + c.Name
	}
	return "Create"
}

func (c *Create) Roots() []ids.NodeID {
	return c.roots
}

func (c *Create) Do(t history.Target) error {
	s := t.Scene()
	p, err := parentOf(s, c.Parent)
	if err != nil {
		return err
	}
	created, _, err := sceneio.LoadInto(s, p, c.Fragment, "fragment", sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		return err
	}
	c.roots = c.roots[:0]
	var tops []scene.NodeHandle
	for _, h := range created {
		if s.Parent(h) == p {
			tops = append(tops, h)
			c.roots = append(c.roots, s.ID(h))
		}
	}
	if c.Index >= 0 {
		for i, h := range tops {
			if err := s.Reparent(h, p, c.Index+i); err != nil {
				return err
			}
		}
		s.Flush()
	}
	return nil
}

func (c *Create) Undo(t history.Target) error {
	s := t.Scene()
	for _, id := range c.roots {
		if h := s.Handle(id); !h.IsZero() {
			s.Destroy(h)
		}
	}
	s.Flush()
	return nil
}

func (c *Create) Merge(history.Command) bool {
	return false
}

type Delete struct {
	Node     ids.NodeID
	parent   ids.NodeID
	index    int
	fragment []byte
}

func (c *Delete) Label() string {
	return "Delete"
}

func (c *Delete) Do(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	if h == s.Root() {
		return errs.New(errs.SceneBadReparent, "the root node cannot be deleted")
	}
	c.parent = s.ID(s.Parent(h))
	c.index = ChildIndex(s, h)
	c.fragment = sceneio.SaveSubtree(s, h, false)
	s.Destroy(h)
	s.Flush()
	return nil
}

func (c *Delete) Undo(t history.Target) error {
	s := t.Scene()
	p, err := parentOf(s, c.parent)
	if err != nil {
		return err
	}
	created, _, err := sceneio.LoadInto(s, p, c.fragment, "deleted", sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		return err
	}
	if len(created) > 0 && c.index >= 0 {
		if err := s.Reparent(created[0], p, c.index); err != nil {
			return err
		}
		s.Flush()
	}
	return nil
}

func (c *Delete) Merge(history.Command) bool {
	return false
}

type Reparent struct {
	Node      ids.NodeID
	NewParent ids.NodeID
	NewIndex  int
	KeepWorld bool
	recorded  bool
	oldParent ids.NodeID
	oldIndex  int
	oldPose   scene.Pose
	newPose   scene.Pose
}

func NewReorder(s *scene.Scene, h scene.NodeHandle, index int) *Reparent {
	return &Reparent{Node: s.ID(h), NewParent: s.ID(s.Parent(h)), NewIndex: index}
}

func (c *Reparent) Label() string {
	return "Reparent"
}

func (c *Reparent) Do(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	np, err := parentOf(s, c.NewParent)
	if err != nil {
		return err
	}
	if !c.recorded {
		c.oldParent = s.ID(s.Parent(h))
		c.oldIndex = ChildIndex(s, h)
		c.oldPose = s.Pose(h)
		c.newPose = c.oldPose
		if c.KeepWorld {
			pose, err := PreserveWorld(s, h, np)
			if err != nil {
				return err
			}
			c.newPose = pose
		}
		c.recorded = true
	}
	if err := s.Reparent(h, np, c.NewIndex); err != nil {
		return err
	}
	s.Flush()
	s.SetPose(h, c.newPose)
	return nil
}

func (c *Reparent) Undo(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	op, err := parentOf(s, c.oldParent)
	if err != nil {
		return err
	}
	if err := s.Reparent(h, op, c.oldIndex); err != nil {
		return err
	}
	s.Flush()
	s.SetPose(h, c.oldPose)
	return nil
}

func (c *Reparent) Merge(history.Command) bool {
	return false
}
