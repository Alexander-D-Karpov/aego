package commands

import (
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/history"
	"aego/scene"
	"aego/scene/mutate"
)

func node(t history.Target, id ids.NodeID) (*scene.Scene, scene.NodeHandle, error) {
	s := t.Scene()
	h := s.Handle(id)
	if h.IsZero() || !s.IsAlive(h) {
		return s, h, errs.New(errs.SceneDeadNode, "node is not in the scene", log.F("node", id.String()))
	}
	return s, h, nil
}

func parentOf(s *scene.Scene, id ids.NodeID) (scene.NodeHandle, error) {
	if id.IsZero() {
		return s.Root(), nil
	}
	h := s.Handle(id)
	if h.IsZero() || !s.IsAlive(h) {
		return h, errs.New(errs.SceneDeadNode, "parent is not in the scene", log.F("node", id.String()))
	}
	return h, nil
}

func ChildIndex(s *scene.Scene, h scene.NodeHandle) int {
	i, idx := 0, -1
	s.Children(s.Parent(h), func(c scene.NodeHandle) bool {
		if c == h {
			idx = i
			return false
		}
		i++
		return true
	})
	return idx
}

func Decompose(m mathx.Mat3) scene.Transform2D {
	return mutate.Decompose(m)
}

func Unlift(m mathx.Mat4) mathx.Mat3 {
	return mutate.Unlift(m)
}

func PreserveWorld(s *scene.Scene, h, parent scene.NodeHandle) (scene.Pose, error) {
	return mutate.PreserveWorld(s, h, parent)
}

type SetTransform struct {
	Node   ids.NodeID
	Before scene.Transform2D
	After  scene.Transform2D
}

func (c *SetTransform) Label() string {
	return "Transform"
}

func (c *SetTransform) Do(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	s.SetLocal(h, c.After)
	return nil
}

func (c *SetTransform) Undo(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	s.SetLocal(h, c.Before)
	return nil
}

func (c *SetTransform) Merge(next history.Command) bool {
	n, ok := next.(*SetTransform)
	if !ok || n.Node != c.Node {
		return false
	}
	c.After = n.After
	return true
}

type SetTransform3D struct {
	Node   ids.NodeID
	Before scene.Transform3D
	After  scene.Transform3D
}

func (c *SetTransform3D) Label() string {
	return "Transform"
}

func (c *SetTransform3D) Do(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	s.SetLocal3D(h, c.After)
	return nil
}

func (c *SetTransform3D) Undo(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	s.SetLocal3D(h, c.Before)
	return nil
}

func (c *SetTransform3D) Merge(next history.Command) bool {
	n, ok := next.(*SetTransform3D)
	if !ok || n.Node != c.Node {
		return false
	}
	c.After = n.After
	return true
}

type SetSpace struct {
	Node   ids.NodeID
	After  scene.Space
	before scene.Pose
}

func (c *SetSpace) Label() string {
	return "Change Space"
}

func (c *SetSpace) Do(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	c.before = s.Pose(h)
	return s.SetSpace(h, c.After)
}

func (c *SetSpace) Undo(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	if err := s.SetSpace(h, c.before.Space); err != nil {
		return err
	}
	s.SetPose(h, c.before)
	return nil
}

func (c *SetSpace) Merge(history.Command) bool {
	return false
}

type SetLayers struct {
	Node   ids.NodeID
	Before uint64
	After  uint64
}

func (c *SetLayers) Label() string {
	return "Set Layers"
}

func (c *SetLayers) Do(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	s.SetLayers(h, c.After)
	return nil
}

func (c *SetLayers) Undo(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	s.SetLayers(h, c.Before)
	return nil
}

func (c *SetLayers) Merge(next history.Command) bool {
	n, ok := next.(*SetLayers)
	if !ok || n.Node != c.Node {
		return false
	}
	c.After = n.After
	return true
}
