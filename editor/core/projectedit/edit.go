// Package projectedit changes a module section of project.aego as a History step (D342).
package projectedit

import (
	"slices"

	"aego/editor/core/history"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/project"
)

// Edit is a History step that replaces one module section of project.aego and the live value; Undo puts the old block back (D342).
type Edit struct {
	Name     string
	Project  *project.Project
	Sections *module.Sections
	ID       string
	After    *aetext.Node
	Changed  func()
	before   *aetext.Node
	done     bool
}

func (e *Edit) Label() string {
	return e.Name
}

func (e *Edit) heads() []string {
	if d, ok := e.Sections.Desc(e.ID); ok {
		return d.Heads()
	}
	return []string{e.ID}
}

func (e *Edit) current() *aetext.Node {
	for _, h := range e.heads() {
		if b := e.Project.Block(h); b != nil {
			return b.Clone()
		}
	}
	return nil
}

func (e *Edit) Do(history.Target) error {
	if !e.done {
		e.before, e.done = e.current(), true
	}
	return e.put(e.After)
}

func (e *Edit) Undo(history.Target) error {
	return e.put(e.before)
}

func (e *Edit) Merge(history.Command) bool {
	return false
}

func (e *Edit) put(n *aetext.Node) error {
	v, err := e.Sections.Decode(e.ID, n)
	if err != nil {
		return err
	}
	if n != nil {
		n = n.Clone()
	}
	old := slices.Clone(e.Project.Extra)
	e.Project.ReplaceBlock(n, e.heads()...)
	if err := e.Project.Save(); err != nil {
		e.Project.Extra = old
		return err
	}
	if err := e.Sections.Set(e.ID, v); err != nil {
		return err
	}
	if e.Changed != nil {
		e.Changed()
	}
	return nil
}

var _ history.Command = (*Edit)(nil)
