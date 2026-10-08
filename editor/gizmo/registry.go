package gizmo

import (
	"aego/core/errs"
	"aego/scene"
	"aego/schema"
)

type Registry struct {
	byType map[schema.TypeID]Gizmo
	order  []schema.TypeID
}

func NewRegistry() *Registry {
	return &Registry{byType: map[schema.TypeID]Gizmo{}}
}

func (r *Registry) Register(t schema.TypeID, g Gizmo) error {
	if g == nil {
		return errs.New(errs.EditorPanel, "gizmo is nil")
	}
	if _, dup := r.byType[t]; dup {
		return errs.New(errs.EditorPanel, "a gizmo is already registered for this component")
	}
	r.byType[t] = g
	r.order = append(r.order, t)
	return nil
}

func (r *Registry) Unregister(t schema.TypeID) {
	if _, ok := r.byType[t]; !ok {
		return
	}
	delete(r.byType, t)
	for i, o := range r.order {
		if o == t {
			r.order = append(r.order[:i], r.order[i+1:]...)
			return
		}
	}
}

func (r *Registry) Each(s *scene.Scene, h scene.NodeHandle, fn func(Gizmo)) {
	if s == nil || h.IsZero() || !s.IsAlive(h) {
		return
	}
	for _, t := range r.order {
		if _, ok := s.GetByType(h, t); ok {
			fn(r.byType[t])
		}
	}
}
