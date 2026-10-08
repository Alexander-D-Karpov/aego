package sceneview

import (
	"aego/asset"
	"aego/core/ids"
	"aego/editor/context"
	"aego/editor/gizmo"
	"aego/editor/props"
	"aego/scene"
)

type host3 struct {
	ctx *context.Editor
	s   *scene.Scene
}

func (p *Panel) host3() Host3 {
	var s *scene.Scene
	if p.cur != nil {
		s = p.cur.scene
	}
	return host3{ctx: p.ctx, s: s}
}

func (h host3) Scene() *scene.Scene {
	return h.s
}

func (h host3) Resolver() asset.Resolver {
	return resolver(h.ctx)
}

func (h host3) Selected() []ids.NodeID {
	return h.ctx.Selected()
}

func (h host3) Select(nodes []ids.NodeID, additive bool) {
	if additive {
		h.ctx.AddToSelection(nodes...)
		return
	}
	h.ctx.Select(nodes...)
}

func (h host3) Commit3D(label string, edits []props.Transform3DEdit) {
	d := h.ctx.Doc()
	if d == nil || len(edits) == 0 {
		return
	}
	if err := d.Props.ApplyTransforms3D(nil, label, edits); err != nil {
		h.ctx.Notify(context.LevelError, err.Error())
	}
}

func (h host3) Tool3() gizmo.Op3 {
	switch h.ctx.Settings.Tool {
	case ToolRotate:
		return gizmo.Rotate3
	case ToolScale:
		return gizmo.Scale3
	}
	return gizmo.Translate3
}

func (h host3) ShowGizmo() bool {
	return h.ctx.Settings.Tool != 0
}

func (h host3) LocalAxes() bool {
	return false
}

func (h host3) Snap3() gizmo.Snap3 {
	set := h.ctx.Settings
	return gizmo.Snap3{
		Enabled: set.Snap != h.ctx.UI.Primary(),
		Move:    set.SnapMove,
		Angle:   set.SnapRotate,
		Scale:   set.SnapScale,
	}
}
