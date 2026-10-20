//go:build aego_editor

package mutate_test

import (
	"unsafe"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/history"
	"aego/editor/core/history/commands"
	"aego/engine/module"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

func legacyNode(t history.Target, id ids.NodeID) (*scene.Scene, scene.NodeHandle, error) {
	s := t.Scene()
	h := s.Handle(id)
	if h.IsZero() || !s.IsAlive(h) {
		return s, h, errs.New(errs.SceneDeadNode, "node is not in the scene", log.F("node", id.String()))
	}
	return s, h, nil
}

func legacyParent(s *scene.Scene, id ids.NodeID) (scene.NodeHandle, error) {
	if id.IsZero() {
		return s.Root(), nil
	}
	h := s.Handle(id)
	if h.IsZero() || !s.IsAlive(h) {
		return h, errs.New(errs.SceneDeadNode, "parent is not in the scene", log.F("node", id.String()))
	}
	return h, nil
}

type legacyCreate struct {
	Parent   ids.NodeID
	Index    int
	Fragment []byte
	Name     string
	roots    []ids.NodeID
}

func (c *legacyCreate) Label() string {
	if c.Name != "" {
		return "Create " + c.Name
	}
	return "Create"
}

func (c *legacyCreate) Roots() []ids.NodeID {
	return c.roots
}

func (c *legacyCreate) Do(t history.Target) error {
	s := t.Scene()
	p, err := legacyParent(s, c.Parent)
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

func (c *legacyCreate) Undo(t history.Target) error {
	s := t.Scene()
	for _, id := range c.roots {
		if h := s.Handle(id); !h.IsZero() {
			s.Destroy(h)
		}
	}
	s.Flush()
	return nil
}

func (c *legacyCreate) Merge(history.Command) bool {
	return false
}

type legacyDelete struct {
	Node     ids.NodeID
	parent   ids.NodeID
	index    int
	fragment []byte
}

func (c *legacyDelete) Label() string {
	return "Delete"
}

func (c *legacyDelete) Do(t history.Target) error {
	s, h, err := legacyNode(t, c.Node)
	if err != nil {
		return err
	}
	if h == s.Root() {
		return errs.New(errs.SceneBadReparent, "the root node cannot be deleted")
	}
	c.parent = s.ID(s.Parent(h))
	c.index = commands.ChildIndex(s, h)
	c.fragment = sceneio.SaveSubtree(s, h, false)
	s.Destroy(h)
	s.Flush()
	return nil
}

func (c *legacyDelete) Undo(t history.Target) error {
	s := t.Scene()
	p, err := legacyParent(s, c.parent)
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

func (c *legacyDelete) Merge(history.Command) bool {
	return false
}

type legacyReparent struct {
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

func (c *legacyReparent) Label() string {
	return "Reparent"
}

func (c *legacyReparent) Do(t history.Target) error {
	s, h, err := legacyNode(t, c.Node)
	if err != nil {
		return err
	}
	np, err := legacyParent(s, c.NewParent)
	if err != nil {
		return err
	}
	if !c.recorded {
		c.oldParent = s.ID(s.Parent(h))
		c.oldIndex = commands.ChildIndex(s, h)
		c.oldPose = s.Pose(h)
		c.newPose = c.oldPose
		if c.KeepWorld {
			pose, err := commands.PreserveWorld(s, h, np)
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

func (c *legacyReparent) Undo(t history.Target) error {
	s, h, err := legacyNode(t, c.Node)
	if err != nil {
		return err
	}
	op, err := legacyParent(s, c.oldParent)
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

func (c *legacyReparent) Merge(history.Command) bool {
	return false
}

type legacyAddComponent struct {
	Node   ids.NodeID
	Type   schema.TypeID
	Name   string
	Values []commands.FieldValue
	OnAdd  func(*scene.Scene, scene.NodeHandle)
}

func (c *legacyAddComponent) Label() string {
	return "Add " + c.Name
}

func (c *legacyAddComponent) Do(t history.Target) error {
	s, h, err := legacyNode(t, c.Node)
	if err != nil {
		return err
	}
	if _, ok := s.GetByType(h, c.Type); ok {
		return errs.New(errs.EditorProps, "node already has this component", log.F("type", c.Name))
	}
	td, ok := s.Registry().ByID(c.Type)
	if !ok {
		return errs.New(errs.SchemaUnknownType, "component type is not registered", log.F("type", c.Name))
	}
	if !td.Space.Allows(s.Space(h).Kind()) {
		return errs.New(errs.SceneSpaceMismatch, "component does not fit the node space",
			log.F("type", c.Name), log.F("space", s.Space(h).String()))
	}
	p, err := s.AddByType(h, c.Type)
	if err != nil {
		return err
	}
	if td.Default != nil && td.Copy != nil {
		td.Copy(p, td.Default())
	}
	for _, fv := range c.Values {
		if fd, ok := td.FieldByID(fv.Field); ok {
			if err := fd.Set(p, fv.Value); err != nil {
				return err
			}
		}
	}
	if c.OnAdd != nil {
		c.OnAdd(s, h)
		c.OnAdd = nil
		c.Values = legacySnapshot(td, p)
	}
	s.Flush()
	return nil
}

func legacySnapshot(td *schema.TypeDescriptor, p unsafe.Pointer) []commands.FieldValue {
	out := make([]commands.FieldValue, 0, len(td.Fields))
	for i := range td.Fields {
		var v schema.Value
		td.Fields[i].Get(p, &v)
		out = append(out, commands.FieldValue{Field: td.Fields[i].ID, Value: v})
	}
	return out
}

func (c *legacyAddComponent) Undo(t history.Target) error {
	s, h, err := legacyNode(t, c.Node)
	if err != nil {
		return err
	}
	if td, ok := s.Registry().ByID(c.Type); ok {
		module.RunOnRemoving(module.AddContext{Scene: s, Node: h}, td)
	}
	s.RemoveByType(h, c.Type)
	s.Flush()
	return nil
}

func (c *legacyAddComponent) Merge(history.Command) bool {
	return false
}

type legacyRemoveComponent struct {
	Node     ids.NodeID
	Type     schema.TypeID
	Name     string
	snapshot []commands.FieldValue
}

func (c *legacyRemoveComponent) Label() string {
	return "Remove " + c.Name
}

func (c *legacyRemoveComponent) Do(t history.Target) error {
	s, h, err := legacyNode(t, c.Node)
	if err != nil {
		return err
	}
	p, ok := s.GetByType(h, c.Type)
	if !ok {
		return errs.New(errs.SceneNoComponent, "node does not have this component", log.F("type", c.Name))
	}
	td, ok := s.Registry().ByID(c.Type)
	if !ok {
		return errs.New(errs.SchemaUnknownType, "component type is not registered", log.F("type", c.Name))
	}
	c.snapshot = c.snapshot[:0]
	for i := range td.Fields {
		var v schema.Value
		td.Fields[i].Get(p, &v)
		c.snapshot = append(c.snapshot, commands.FieldValue{Field: td.Fields[i].ID, Value: v})
	}
	module.RunOnRemoving(module.AddContext{Scene: s, Node: h}, td)
	s.RemoveByType(h, c.Type)
	s.Flush()
	return nil
}

func (c *legacyRemoveComponent) Undo(t history.Target) error {
	s, h, err := legacyNode(t, c.Node)
	if err != nil {
		return err
	}
	td, ok := s.Registry().ByID(c.Type)
	if !ok {
		return errs.New(errs.SchemaUnknownType, "component type is not registered", log.F("type", c.Name))
	}
	p, err := s.AddByType(h, c.Type)
	if err != nil {
		return err
	}
	for _, fv := range c.snapshot {
		if fd, ok := td.FieldByID(fv.Field); ok {
			if err := fd.Set(p, fv.Value); err != nil {
				return err
			}
		}
	}
	s.Flush()
	return nil
}

func (c *legacyRemoveComponent) Merge(history.Command) bool {
	return false
}

func (c *legacyAddComponent) Record(cs *module.ChangeSet, undo bool) {
	if undo {
		cs.MarkRemoved(c.Node, c.Type)
		return
	}
	cs.MarkAdded(c.Node, c.Type)
}

func (c *legacyRemoveComponent) Record(cs *module.ChangeSet, undo bool) {
	if undo {
		cs.MarkAdded(c.Node, c.Type)
		return
	}
	cs.MarkRemoved(c.Node, c.Type)
}

type legacyRename struct {
	Node   ids.NodeID
	Before string
	After  string
}

func (c *legacyRename) Label() string {
	return "Rename"
}

func (c *legacyRename) Do(t history.Target) error {
	s, h, err := legacyNode(t, c.Node)
	if err != nil {
		return err
	}
	c.After = s.SetName(h, c.After)
	return nil
}

func (c *legacyRename) Undo(t history.Target) error {
	s, h, err := legacyNode(t, c.Node)
	if err != nil {
		return err
	}
	s.SetName(h, c.Before)
	return nil
}

func (c *legacyRename) Merge(history.Command) bool {
	return false
}

type legacySetProperty struct {
	Node   ids.NodeID
	Type   schema.TypeID
	Field  schema.FieldID
	Before schema.Value
	After  schema.Value
	Name   string
}

func (c *legacySetProperty) Label() string {
	if c.Name != "" {
		return "Set " + c.Name
	}
	return "Set Property"
}

func (c *legacySetProperty) Do(t history.Target) error {
	s, h, err := legacyNode(t, c.Node)
	if err != nil {
		return err
	}
	return s.SetProperty(h, c.Type, c.Field, c.After)
}

func (c *legacySetProperty) Undo(t history.Target) error {
	s, h, err := legacyNode(t, c.Node)
	if err != nil {
		return err
	}
	return s.SetProperty(h, c.Type, c.Field, c.Before)
}

func (c *legacySetProperty) Merge(next history.Command) bool {
	n, ok := next.(*legacySetProperty)
	if !ok || n.Node != c.Node || n.Type != c.Type || n.Field != c.Field {
		return false
	}
	c.After = n.After
	return true
}
