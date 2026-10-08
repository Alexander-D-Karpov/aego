package scene

import (
	"unsafe"

	"aego/core/errs"
	"aego/schema"
)

type Overlay interface {
	Bag(s *Scene, h NodeHandle, td *schema.TypeDescriptor) (unsafe.Pointer, bool)
}

func (t *Types) SetOverlay(o Overlay) {
	t.overlay = o
}

func (s *Scene) target(h NodeHandle, t schema.TypeID) (unsafe.Pointer, *schema.TypeDescriptor, error) {
	if st, ok := s.storesByID[t]; ok {
		if p, ok := st.pointer(h); ok {
			return p, st.descriptor(), nil
		}
		return nil, nil, errs.New(errs.SceneNoComponent, "node does not have this component")
	}
	if td, ok := s.registry.ByID(t); ok && td.Kind == schema.TypeScript && s.types.overlay != nil {
		if p, ok := s.types.overlay.Bag(s, h, td); ok {
			return p, td, nil
		}
	}
	return nil, nil, errs.New(errs.SceneNoComponent, "node does not have this component")
}
