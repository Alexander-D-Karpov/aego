package module

import (
	"unsafe"

	"aego/engine/problems"
	"aego/scene"
	"aego/schema"
)

type ValidateContext struct {
	Scene *scene.Scene
	Node  scene.NodeHandle
	Type  *schema.TypeDescriptor
}

type validateHook func(ValidateContext, unsafe.Pointer) []problems.Diagnostic

func (b *ComponentBuilder[T]) Validate(fn func(ValidateContext, *T) []problems.Diagnostic) *ComponentBuilder[T] {
	d := b.edit()
	if d == nil {
		return b
	}
	switch {
	case fn == nil:
		b.fail("component " + d.FullName + " has a nil Validate")
	case d.Validate != nil:
		b.fail("component " + d.FullName + " declares Validate twice")
	default:
		d.Validate = validateHook(func(ctx ValidateContext, p unsafe.Pointer) []problems.Diagnostic { return fn(ctx, (*T)(p)) })
	}
	return b
}

func ValidateScene(s *scene.Scene, dst []problems.Diagnostic) []problems.Diagnostic {
	var list []*schema.TypeDescriptor
	for _, td := range s.Registry().Components() {
		if _, ok := td.Validate.(validateHook); ok {
			list = append(list, td)
		}
	}
	if len(list) == 0 {
		return dst
	}
	s.UpdateTransforms()
	root := s.Root()
	for _, h := range s.PreOrder() {
		if h == root || !s.IsAlive(h) {
			continue
		}
		for _, td := range list {
			dst = appendComponentDiagnostics(s, h, td, dst)
		}
	}
	return dst
}

// ValidateComponent runs the registered validator against a specific node in
// its actual scene context, e.g. after applying a component preset in a dry copy.
func ValidateComponent(s *scene.Scene, h scene.NodeHandle, td *schema.TypeDescriptor) []problems.Diagnostic {
	s.UpdateTransforms()
	return appendComponentDiagnostics(s, h, td, nil)
}

func appendComponentDiagnostics(s *scene.Scene, h scene.NodeHandle, td *schema.TypeDescriptor, dst []problems.Diagnostic) []problems.Diagnostic {
	check, ok := td.Validate.(validateHook)
	if !ok {
		return dst
	}
	p, ok := s.GetByType(h, td.ID)
	if !ok {
		return dst
	}
	for _, d := range check(ValidateContext{Scene: s, Node: h, Type: td}, p) {
		if d.Owner == "" {
			d.Owner = td.Owner
		}
		if d.Subject.Kind == problems.SubjectNone {
			d.Subject.Kind = problems.SubjectField
		}
		if d.Subject.Node.IsZero() {
			d.Subject.Node = s.ID(h)
		}
		if d.Subject.Field == "" {
			d.Subject.Field = td.FullName
		}
		d.Subject.Scene = s
		dst = append(dst, d)
	}
	return dst
}
