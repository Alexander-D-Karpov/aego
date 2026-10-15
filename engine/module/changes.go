package module

import (
	"slices"

	"aego/core/ids"
	"aego/scene"
	"aego/schema"
)

type Change struct {
	Node    ids.NodeID
	Type    schema.TypeID
	Fields  []schema.FieldID
	Added   bool
	Removed bool
}

type ChangeSet struct {
	Scene *scene.Scene
	list  []Change
}

func (c *ChangeSet) Reset(s *scene.Scene) {
	c.Scene = s
	c.list = c.list[:0]
}

func (c *ChangeSet) at(node ids.NodeID, t schema.TypeID) *Change {
	for i := range c.list {
		if c.list[i].Node == node && c.list[i].Type == t {
			return &c.list[i]
		}
	}
	c.list = append(c.list, Change{Node: node, Type: t})
	return &c.list[len(c.list)-1]
}

func (c *ChangeSet) MarkField(node ids.NodeID, t schema.TypeID, f schema.FieldID) {
	ch := c.at(node, t)
	if !slices.Contains(ch.Fields, f) {
		ch.Fields = append(ch.Fields, f)
	}
}

func (c *ChangeSet) MarkAdded(node ids.NodeID, t schema.TypeID) {
	ch := c.at(node, t)
	if ch.Removed {
		ch.Removed = false
		return
	}
	ch.Added = true
}

func (c *ChangeSet) MarkRemoved(node ids.NodeID, t schema.TypeID) {
	ch := c.at(node, t)
	if ch.Added {
		ch.Added = false
		ch.Fields = ch.Fields[:0]
		return
	}
	ch.Removed = true
}

func (c *ChangeSet) Changes() []Change {
	return c.list
}

func (c *ChangeSet) Empty() bool {
	for i := range c.list {
		ch := &c.list[i]
		if ch.Added || ch.Removed || len(ch.Fields) > 0 {
			return false
		}
	}
	return true
}

type changedHook func(*ChangeSet)

func (b *ComponentBuilder[T]) OnChanged(fn func(*ChangeSet)) *ComponentBuilder[T] {
	d := b.edit()
	if d == nil {
		return b
	}
	switch {
	case fn == nil:
		b.fail("component " + d.FullName + " has a nil OnChanged")
	case d.OnChanged != nil:
		b.fail("component " + d.FullName + " declares OnChanged twice")
	default:
		d.OnChanged = changedHook(fn)
	}
	return b
}

func RunChanged(cs *ChangeSet) {
	if cs == nil || cs.Scene == nil || cs.Empty() {
		return
	}
	reg := cs.Scene.Registry()
	var seen []schema.TypeID
	for i := range cs.list {
		t := cs.list[i].Type
		if slices.Contains(seen, t) {
			continue
		}
		seen = append(seen, t)
		if td, ok := reg.ByID(t); ok {
			if hook, ok := td.OnChanged.(changedHook); ok {
				hook(cs)
			}
		}
	}
}
