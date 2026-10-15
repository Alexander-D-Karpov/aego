package sceneview

import (
	"aego/core/ids"
	"aego/editor/core/context"
	"aego/editor/core/gizmo"
	"aego/editor/core/props"
	"aego/editor/core/shape"
	"aego/scene"
)

type host3 struct {
	p *Panel
	s *scene.Scene
}

func (p *Panel) host3() Host3 {
	var s *scene.Scene
	if p.cur != nil {
		s = p.cur.scene
	}
	return host3{p: p, s: s}
}

func (h host3) Scene() *scene.Scene {
	return h.s
}

func (h host3) Shapes() shape.Set {
	return h.p.shapes()
}

func (h host3) Selected() []ids.NodeID {
	return h.p.d.Selection.Selected()
}

func (h host3) Select(nodes []ids.NodeID, additive bool) {
	if additive {
		h.p.d.Selection.AddToSelection(nodes...)
		return
	}
	h.p.d.Selection.Select(nodes...)
}

func (h host3) Commit3D(label string, edits []props.Transform3DEdit) {
	d := h.p.d.Docs.Doc()
	if d == nil || len(edits) == 0 {
		return
	}
	if err := d.Props.ApplyTransforms3D(nil, label, edits); err != nil {
		h.p.d.Notices.Notify(context.LevelError, err.Error())
	}
}

func (h host3) Tool3() gizmo.Op3 {
	switch h.p.d.Tools.Tool {
	case context.ToolRotate:
		return gizmo.Rotate3
	case context.ToolScale:
		return gizmo.Scale3
	}
	return gizmo.Translate3
}

func (h host3) ShowGizmo() bool {
	return h.p.d.Tools.Tool != context.ToolSelect
}

func (h host3) LocalAxes() bool {
	return false
}

func (h host3) Snap3() gizmo.Snap3 {
	set := h.p.d.Tools
	return gizmo.Snap3{
		Enabled: set.Snap != h.p.ui().Primary(),
		Move:    set.SnapMove,
		Angle:   set.SnapRotate,
		Scale:   set.SnapScale,
	}
}
