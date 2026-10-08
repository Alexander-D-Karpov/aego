package extension

import (
	"fmt"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/context"
	"aego/editor/gizmo"
	"aego/editor/inspector"
	"aego/editor/workspace"
)

type safePanel struct {
	inner workspace.Panel
	ext   string
	reg   *Registry
}

func (s *safePanel) ID() string {
	return s.inner.ID()
}

func (s *safePanel) Title() string {
	return s.inner.Title()
}

func (s *safePanel) Scope() string {
	if sc, ok := s.inner.(workspace.Scoped); ok {
		return sc.Scope()
	}
	return s.inner.ID()
}

func (s *safePanel) Draw(ctx *context.Editor, r mathx.Rect) {
	defer func() {
		if rec := recover(); rec != nil {
			s.reg.Disable(s.ext, fmt.Sprint(rec))
			panic(rec)
		}
	}()
	s.inner.Draw(ctx, r)
}

type safeCustom struct {
	inner inspector.Custom
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
