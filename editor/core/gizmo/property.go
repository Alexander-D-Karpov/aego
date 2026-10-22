package gizmo

import (
	"aego/core/ids"
	"aego/editor/core/props"
	"aego/schema"
)

func SetProperty(e *EditCtx, t schema.TypeID, fd *schema.FieldDescriptor, v schema.Value) {
	if e.Err != nil || e.Tx == nil || fd == nil || e.Scene.Handle(e.Node).IsZero() {
		return
	}
	if err := props.New(e.Scene, nil).Set(e.Tx, []ids.NodeID{e.Node}, t, fd.ID, v); err != nil {
		e.Err = err
	}
}
