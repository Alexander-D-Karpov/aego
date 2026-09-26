package commands

import (
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/history"
	"aego/schema"
)

type FieldValue struct {
	Field schema.FieldID
	Value schema.Value
}

type SetProperty struct {
	Node   ids.NodeID
	Type   schema.TypeID
	Field  schema.FieldID
	Before schema.Value
	After  schema.Value
	Name   string
}

func (c *SetProperty) Label() string {
	if c.Name != "" {
		return "Set " + c.Name
	}
	return "Set Property"
}

func (c *SetProperty) Do(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	return s.SetProperty(h, c.Type, c.Field, c.After)
}

func (c *SetProperty) Undo(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	return s.SetProperty(h, c.Type, c.Field, c.Before)
}

func (c *SetProperty) Merge(next history.Command) bool {
	n, ok := next.(*SetProperty)
	if !ok || n.Node != c.Node || n.Type != c.Type || n.Field != c.Field {
		return false
	}
	c.After = n.After
	return true
}

type AddComponent struct {
	Node   ids.NodeID
	Type   schema.TypeID
	Name   string
	Values []FieldValue
}

func (c *AddComponent) Label() string {
	return "Add " + c.Name
}

func (c *AddComponent) Do(t history.Target) error {
	s, h, err := node(t, c.Node)
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
	p, err := s.AddByType(h, c.Type)
	if err != nil {
		return err
	}
	for _, fv := range c.Values {
		if fd, ok := td.FieldByID(fv.Field); ok {
			if err := fd.Set(p, fv.Value); err != nil {
				return err
			}
		}
	}
	s.Flush()
	return nil
}

func (c *AddComponent) Undo(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	s.RemoveByType(h, c.Type)
	s.Flush()
	return nil
}

func (c *AddComponent) Merge(history.Command) bool {
	return false
}

type RemoveComponent struct {
	Node     ids.NodeID
	Type     schema.TypeID
	Name     string
	snapshot []FieldValue
}

func (c *RemoveComponent) Label() string {
	return "Remove " + c.Name
}

func (c *RemoveComponent) Do(t history.Target) error {
	s, h, err := node(t, c.Node)
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
		c.snapshot = append(c.snapshot, FieldValue{Field: td.Fields[i].ID, Value: v})
	}
	s.RemoveByType(h, c.Type)
	s.Flush()
	return nil
}

func (c *RemoveComponent) Undo(t history.Target) error {
	s, h, err := node(t, c.Node)
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

func (c *RemoveComponent) Merge(history.Command) bool {
	return false
}
