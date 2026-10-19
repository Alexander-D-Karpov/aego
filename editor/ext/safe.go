package ext

import (
	"aego/editor/ext/inspect"
	"fmt"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/gizmo"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
)

type safePanel struct {
	p   workspace.Panel
	ext string
	reg *Registry
}

func (s *safePanel) ID() string {
	return s.p.ID()
}

func (s *safePanel) Title() string {
	return s.p.Title()
}

func (s *safePanel) Scope() string {
	if sc, ok := s.p.(workspace.Scoped); ok {
		return sc.Scope()
	}
	return s.p.ID()
}

func (s *safePanel) Draw(f *services.Frame, r mathx.Rect) {
	defer func() {
		if rec := recover(); rec != nil {
			s.reg.Disable(s.ext, fmt.Sprint(rec))
			panic(rec)
		}
	}()
	s.p.Draw(f, r)
}

type safeCustom struct {
	inner inspect.Custom
	ext   string
	reg   *Registry
}

func (s *safeCustom) Draw(ctx *context.Editor, nodes []ids.NodeID, defaultFields func()) {
	defer func() {
		if rec := recover(); rec != nil {
			s.reg.Disable(s.ext, fmt.Sprint(rec))
			panic(rec)
		}
	}()
	s.inner.Draw(ctx, nodes, defaultFields)
}

type safeGizmo struct {
	inner gizmo.Gizmo
	ext   string
	reg   *Registry
}

func (s *safeGizmo) guard() {
	if rec := recover(); rec != nil {
		s.reg.Disable(s.ext, fmt.Sprint(rec))
	}
}

func (s *safeGizmo) Draw(c *gizmo.DrawCtx) {
	defer s.guard()
	s.inner.Draw(c)
}

func (s *safeGizmo) HitTest(c *gizmo.HitCtx) (h gizmo.Hit, ok bool) {
	defer func() {
		if rec := recover(); rec != nil {
			s.reg.Disable(s.ext, fmt.Sprint(rec))
			h, ok = gizmo.Hit{}, false
		}
	}()
	return s.inner.HitTest(c)
}

func (s *safeGizmo) Begin(e *gizmo.EditCtx, h gizmo.Hit) {
	defer s.guard()
	s.inner.Begin(e, h)
}

func (s *safeGizmo) Update(e *gizmo.EditCtx) {
	defer s.guard()
	s.inner.Update(e)
}

func (s *safeGizmo) End(e *gizmo.EditCtx) {
	defer s.guard()
	s.inner.End(e)
}

func (s *safePanel) Icon() string {
	if ic, ok := s.p.(workspace.Iconed); ok {
		return ic.Icon()
	}
	return ""
}

func (s *safePanel) Dirty() bool {
	if d, ok := s.p.(workspace.Dirtier); ok {
		return d.Dirty()
	}
	return false
}
