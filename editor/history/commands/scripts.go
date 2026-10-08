package commands

import (
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/history"
	"aego/scene"
	"aego/scene/components"
	"aego/schema"
)

func shortName(full string) string {
	return full[strings.LastIndexByte(full, '.')+1:]
}

func attachments(s *scene.Scene, h scene.NodeHandle) (*components.ScriptComponent, bool) {
	return components.ScriptsOf(s, h)
}

func notAttached(name string) error {
	return errs.New(errs.EditorProps, "script is not attached", log.F("script", name))
}

type AddScript struct {
	Node     ids.NodeID
	TypeName string
	Props    schema.PropertyBag
	created  bool
}

func (c *AddScript) Label() string {
	return "Add " + shortName(c.TypeName)
}

func (c *AddScript) Do(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	sc, ok := attachments(s, h)
	c.created = !ok
	if !ok {
		sc = scene.Add[components.ScriptComponent](s, h)
	}
	if sc.Index(c.TypeName) >= 0 {
		return errs.New(errs.EditorProps, "script is already attached", log.F("script", c.TypeName))
	}
	sc.Attachments = append(sc.Attachments, components.Attachment{TypeName: c.TypeName, Enabled: true, Props: c.Props.Clone()})
	s.Flush()
	return nil
}

func (c *AddScript) Undo(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	if sc, ok := attachments(s, h); ok {
		if i := sc.Index(c.TypeName); i >= 0 {
			sc.Attachments = append(sc.Attachments[:i], sc.Attachments[i+1:]...)
		}
	}
	if c.created {
		scene.Remove[components.ScriptComponent](s, h)
	}
	s.Flush()
	return nil
}

func (c *AddScript) Merge(history.Command) bool {
	return false
}

type RemoveScript struct {
	Node     ids.NodeID
	TypeName string
	index    int
	saved    components.Attachment
	dropped  bool
}

func (c *RemoveScript) Label() string {
	return "Remove " + shortName(c.TypeName)
}

func (c *RemoveScript) Do(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	sc, ok := attachments(s, h)
	if !ok {
		return notAttached(c.TypeName)
	}
	i := sc.Index(c.TypeName)
	if i < 0 {
		return notAttached(c.TypeName)
	}
	c.index, c.saved = i, sc.Attachments[i].Clone()
	sc.Attachments = append(sc.Attachments[:i], sc.Attachments[i+1:]...)
	c.dropped = len(sc.Attachments) == 0
	if c.dropped {
		scene.Remove[components.ScriptComponent](s, h)
	}
	s.Flush()
	return nil
}

func (c *RemoveScript) Undo(t history.Target) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	sc, ok := attachments(s, h)
	if !ok {
		sc = scene.Add[components.ScriptComponent](s, h)
	}
	i := min(c.index, len(sc.Attachments))
	sc.Attachments = append(sc.Attachments, components.Attachment{})
	copy(sc.Attachments[i+1:], sc.Attachments[i:])
	sc.Attachments[i] = c.saved.Clone()
	s.Flush()
	return nil
}

func (c *RemoveScript) Merge(history.Command) bool {
	return false
}

type SetScriptEnabled struct {
	Node     ids.NodeID
	TypeName string
	Before   bool
	After    bool
}

func (c *SetScriptEnabled) Label() string {
	return "Toggle " + shortName(c.TypeName)
}

func (c *SetScriptEnabled) set(t history.Target, v bool) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	sc, ok := attachments(s, h)
	if !ok {
		return notAttached(c.TypeName)
	}
	i := sc.Index(c.TypeName)
	if i < 0 {
		return notAttached(c.TypeName)
	}
	sc.Attachments[i].Enabled = v
	return nil
}

func (c *SetScriptEnabled) Do(t history.Target) error {
	return c.set(t, c.After)
}

func (c *SetScriptEnabled) Undo(t history.Target) error {
	return c.set(t, c.Before)
}

func (c *SetScriptEnabled) Merge(history.Command) bool {
	return false
}

type ReorderScript struct {
	Node ids.NodeID
	From int
	To   int
}

func (c *ReorderScript) Label() string {
	return "Reorder Scripts"
}

func (c *ReorderScript) move(t history.Target, from, to int) error {
	s, h, err := node(t, c.Node)
	if err != nil {
		return err
	}
	sc, ok := attachments(s, h)
	if !ok || from < 0 || to < 0 || from >= len(sc.Attachments) || to >= len(sc.Attachments) {
		return errs.New(errs.EditorProps, "script index is out of range")
	}
	a := sc.Attachments[from]
	sc.Attachments = append(sc.Attachments[:from], sc.Attachments[from+1:]...)
	sc.Attachments = append(sc.Attachments, components.Attachment{})
	copy(sc.Attachments[to+1:], sc.Attachments[to:])
	sc.Attachments[to] = a
	return nil
}

func (c *ReorderScript) Do(t history.Target) error {
	return c.move(t, c.From, c.To)
}

func (c *ReorderScript) Undo(t history.Target) error {
	return c.move(t, c.To, c.From)
}

func (c *ReorderScript) Merge(history.Command) bool {
	return false
}
