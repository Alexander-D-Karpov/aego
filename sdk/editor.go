//go:build aego_editor

package sdk

import (
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/gizmo"
	"aego/editor/core/services"
	"aego/editor/core/shape"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/editor/ext/inspect"
)

type (
	EditorRegistry = ext.Registry
	Editor         = context.Editor
	Panel          = workspace.Panel
	CommandDesc    = command.Desc
	Inspector      = inspect.Custom
	InspectorFunc  = inspect.CustomFunc
	Gizmo          = gizmo.Gizmo
	GizmoHit       = gizmo.Hit
	GizmoDraw      = gizmo.DrawCtx
	GizmoHitTest   = gizmo.HitCtx
	GizmoEdit      = gizmo.EditCtx
	Shape          = shape.Shape
	VectorSink     = context.VectorSink
	ShapeOverlay   = shape.OverlayCtx
	Frame          = services.Frame
	Navigation     = services.Navigation
	Notices        = services.Notices
	PanelDesc      = workspace.Desc
	PanelSlot      = workspace.Slot
)

const (
	SlotNone    = workspace.SlotNone
	SlotLeft    = workspace.SlotLeft
	SlotBrowser = workspace.SlotBrowser
	SlotCenter  = workspace.SlotCenter
	SlotBottom  = workspace.SlotBottom
	SlotRight   = workspace.SlotRight

	PanelHierarchy = workspace.Hierarchy
	PanelScene     = workspace.Scene
	PanelGame      = workspace.Game
	PanelInspector = workspace.Inspector
	PanelAssets    = workspace.Assets
	PanelConsole   = workspace.Console
)

func EditorHook(r *Registry, fn func(*EditorRegistry) error) {
	r.r.Editor(fn)
}
