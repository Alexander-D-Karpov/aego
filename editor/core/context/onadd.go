package context

import (
	"math"

	"aego/core/mathx"
	"aego/editor/core/shape"
	"aego/engine/module"
	"aego/scene"
	"aego/schema"
)

func (e *Editor) AddContext(s *scene.Scene, h scene.NodeHandle) module.AddContext {
	return module.AddContext{Scene: s, Node: h, Visual: func() (mathx.Rect, bool) {
		sh, ok := shape.Set{Reg: e.Shapes, Res: s.Resolver()}.Of(s, h)
		if !ok || sh.Kind != shape.Rect || sh.Order == math.MinInt32 {
			return mathx.Rect{}, false
		}
		return sh.Local, true
	}}
}

func (e *Editor) OnAdd(td *schema.TypeDescriptor) func(*scene.Scene, scene.NodeHandle) {
	if !module.HasOnAdd(td) {
		return nil
	}
	return func(s *scene.Scene, h scene.NodeHandle) {
		module.RunOnAdd(e.AddContext(s, h), td)
	}
}

func (e *Editor) RunOnAdd(s *scene.Scene, h scene.NodeHandle) {
	reg := s.Registry()
	var list []*schema.TypeDescriptor
	s.ComponentsOf(h, func(t schema.TypeID) {
		if td, ok := reg.ByID(t); ok && module.HasOnAdd(td) {
			list = append(list, td)
		}
	})
	for _, td := range list {
		module.RunOnAdd(e.AddContext(s, h), td)
	}
}
