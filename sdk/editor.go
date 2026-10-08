//go:build aego_editor

package sdk

import (
	"aego/editor/command"
	"aego/editor/context"
	"aego/editor/extension"
	"aego/editor/gizmo"
	"aego/editor/inspector"
	"aego/editor/templates"
	"aego/editor/workspace"
)

type (
	EditorRegistry = extension.Registry
	EditorModule   = extension.Module
	Editor         = context.Editor
	Panel          = workspace.Panel
	Template       = templates.Template
	CommandDesc    = command.Desc
	Inspector      = inspector.Custom
	InspectorFunc  = inspector.CustomFunc
	ComponentMeta  = inspector.Meta
	Gizmo          = gizmo.Gizmo
	GizmoHit       = gizmo.Hit
	GizmoDraw      = gizmo.DrawCtx
	GizmoHitTest   = gizmo.HitCtx
	GizmoEdit      = gizmo.EditCtx
)

func RegisterInspector[T any](r *EditorRegistry, c Inspector) error {
	return extension.RegisterInspector[T](r, c)
}

func DescribeComponent[T any](r *EditorRegistry, m ComponentMeta) error {
	return inspector.RegisterMeta[T](r.Inspectors(), r.Schema(), m)
}

func RegisterGizmo[T any](r *EditorRegistry, g Gizmo) error {
	return extension.RegisterGizmo[T](r, g)
}
