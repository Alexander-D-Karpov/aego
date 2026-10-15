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
			p, ok := s.GetByType(h, td.ID)
			if !ok {
				continue
			}
			for _, d := range td.Validate.(validateHook)(ValidateContext{Scene: s, Node: h, Type: td}, p) {
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
		}
	}
	return dst
}
