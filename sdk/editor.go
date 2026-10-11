//go:build aego_editor

package sdk

import (
	"aego/editor/command"
	"aego/editor/context"
	"aego/editor/extension"
	"aego/editor/gizmo"
	"aego/editor/inspector"
	"aego/editor/shape"
	"aego/editor/workspace"
)

type (
	EditorRegistry = extension.Registry
	Editor         = context.Editor
	Panel          = workspace.Panel
	CommandDesc    = command.Desc
	Inspector      = inspector.Custom
	InspectorFunc  = inspector.CustomFunc
	Gizmo          = gizmo.Gizmo
	GizmoHit       = gizmo.Hit
	GizmoDraw      = gizmo.DrawCtx
	GizmoHitTest   = gizmo.HitCtx
	GizmoEdit      = gizmo.EditCtx
	Shape          = shape.Shape
	ShapeOverlay   = shape.OverlayCtx
)

func EditorHook(r *Registry, fn func(*EditorRegistry) error) {
	r.r.Editor(fn)
}
