package commands

import (
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/history"
	"aego/scene"
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
	sx := mathx.Sqrt(m[0]*m[0] + m[1]*m[1])
	sy := mathx.Sqrt(m[3]*m[3] + m[4]*m[4])
	if m[0]*m[4]-m[1]*m[3] < 0 {
		sy = -sy
	}
	return scene.Transform2D{
		Position: mathx.Vec2{X: m[6], Y: m[7]},
		Rotation: mathx.Atan2(m[1], m[0]),
		Scale:    mathx.Vec2{X: sx, Y: sy},
	}
}

func PreserveWorld(s *scene.Scene, h, parent scene.NodeHandle) (scene.Transform2D, error) {
	s.UpdateTransforms()
	pw := s.World(parent)
	if mathx.Abs(pw[0]*pw[4]-pw[1]*pw[3]) < 1e-8 {
		return scene.Transform2D{}, errs.New(errs.SceneBadReparent,
			"new parent has a degenerate transform, the child world transform cannot be preserved",
			log.F("parent", s.Name(parent)))
	}
	return Decompose(pw.Inverse().Mul(s.World(h))), nil
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

type Rename struct {
	Node   ids.NodeID
	Before string
	After  string
}

func (c *Rename) Label() string {
	return "Rename"
}

func (c *Rename) Do(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	c.After = s.SetName(h, c.After)
	return nil
}

func (c *Rename) Undo(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	s.SetName(h, c.Before)
	return nil
}

func (c *Rename) Merge(history.Command) bool {
	return false
}

type SetEnabled struct {
	Node   ids.NodeID
	Before bool
	After  bool
}

func (c *SetEnabled) Label() string {
	return "Toggle Enabled"
}

func (c *SetEnabled) Do(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	s.SetEnabled(h, c.After)
	return nil
}

func (c *SetEnabled) Undo(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	s.SetEnabled(h, c.Before)
	return nil
}

func (c *SetEnabled) Merge(history.Command) bool {
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
