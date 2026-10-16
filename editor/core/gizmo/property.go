package gizmo

import (
	"aego/editor/core/history/commands"
	"aego/editor/core/props"
	"aego/schema"
)

func SetProperty(e *EditCtx, t schema.TypeID, fd *schema.FieldDescriptor, v schema.Value) {
	if e.Err != nil || e.Tx == nil || fd == nil {
		return
	}
	h := e.Scene.Handle(e.Node)
	if h.IsZero() {
		return
	}
	before, err := e.Scene.GetProperty(h, t, fd.ID)
	if err != nil {
		e.Err = err
		return
	}
	if props.Equal(before, v) {
		return
	}
	if err := e.Tx.Exec(&commands.SetProperty{Node: e.Node, Type: t, Field: fd.ID, Before: before, After: v, Name: props.Label(fd)}); err != nil {
		e.Err = err
	}
}
