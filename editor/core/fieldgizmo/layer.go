package fieldgizmo

import (
	"aego/editor/core/context"
	"aego/scene/scriptcomp"
	"aego/schema"
)

type layer struct {
	ctx  *context.Editor
	list []field
}

func (l *layer) draw(v *context.VectorSink) {
	s := v.Scene
	if s == nil {
		return
	}
	root := s.Root()
	for _, h := range s.PreOrder() {
		if h == root || !s.IsAlive(h) || !s.EffectiveEnabled(h) {
			continue
		}
		l.list = l.list[:0]
		s.ComponentsOf(h, func(t schema.TypeID) {
			l.list = collect(s, h, t, t == scriptcomp.ScriptTypeID, l.list)
		})
		for _, f := range l.list {
			gm, ok := shapeOf(s, h, f, 1)
			if !ok {
				continue
			}
			switch f.fd.Hints.Gizmo {
			case schema.GizmoVector:
				v.Arrow(gm.base, gm.tip.Sub(gm.base), colField)
			case schema.GizmoPoint:
				v.Debug.Cross(gm.tip, v.Px*10, colField)
			case schema.GizmoRadius:
				v.Circle(gm.base, gm.r, colField)
			}
		}
	}
}
