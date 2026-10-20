package commands

import (
	"aego/core/ids"
	"aego/editor/core/history"
	"aego/engine/module"
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

func (c *SetProperty) Record(cs *module.ChangeSet, _ bool) {
	cs.MarkField(c.Node, c.Type, c.Field)
}
