package gizmo

import (
	"aego/core/errs"
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
	td, ok := e.Scene.Registry().ByID(t)
	if !ok {
		e.Err = errs.New(errs.SchemaUnknownType, "component type is not registered")
		return
	}
	nv, err := props.Normalize(fd, v)
	if err != nil {
		e.Err = err
		return
	}
	before, err := e.Scene.GetProperty(h, t, fd.ID)
	if err != nil {
		e.Err = err
		return
	}
	if props.Equal(before, nv) {
		return
	}
	if err := e.Tx.Exec(commands.SetField(e.Scene, e.Node, td, fd, nv, props.Label(fd))); err != nil {
		e.Err = err
	}
}
