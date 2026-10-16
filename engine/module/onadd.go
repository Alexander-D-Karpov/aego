package module

import (
	"unsafe"

	"aego/core/mathx"
	"aego/scene"
	"aego/schema"
)

type AddContext struct {
	Scene  *scene.Scene
	Node   scene.NodeHandle
	Visual func() (mathx.Rect, bool)
}

func (c AddContext) VisualRect() (mathx.Rect, bool) {
	if c.Visual == nil {
		return mathx.Rect{}, false
	}
	r, ok := c.Visual()
	return r, ok && r.W > 0 && r.H > 0
}

type addHook func(AddContext, unsafe.Pointer)

func (b *ComponentBuilder[T]) OnAdd(fn func(AddContext, *T)) *ComponentBuilder[T] {
	d := b.edit()
	if d == nil {
		return b
	}
	switch {
	case fn == nil:
		b.fail("component " + d.FullName + " has a nil OnAdd")
	case d.OnAdd != nil:
		b.fail("component " + d.FullName + " declares OnAdd twice")
	default:
		d.OnAdd = addHook(func(ctx AddContext, p unsafe.Pointer) { fn(ctx, (*T)(p)) })
	}
	return b
}

func HasOnAdd(td *schema.TypeDescriptor) bool {
	_, ok := td.OnAdd.(addHook)
	return ok
}

func RunOnAdd(ctx AddContext, td *schema.TypeDescriptor) bool {
	hook, ok := td.OnAdd.(addHook)
	if !ok || ctx.Scene == nil {
		return false
	}
	p, ok := ctx.Scene.GetByType(ctx.Node, td.ID)
	if !ok {
		return false
	}
	hook(ctx, p)
	return true
}

type removeHook func(AddContext, unsafe.Pointer)

func (b *ComponentBuilder[T]) OnRemoving(fn func(AddContext, *T)) *ComponentBuilder[T] {
	d := b.edit()
	if d == nil {
		return b
	}
	switch {
	case fn == nil:
		b.fail("component " + d.FullName + " has a nil OnRemoving")
	case d.OnRemoving != nil:
		b.fail("component " + d.FullName + " declares OnRemoving twice")
	default:
		d.OnRemoving = removeHook(func(ctx AddContext, p unsafe.Pointer) { fn(ctx, (*T)(p)) })
	}
	return b
}

func RunOnRemoving(ctx AddContext, td *schema.TypeDescriptor) bool {
	hook, ok := td.OnRemoving.(removeHook)
	if !ok || ctx.Scene == nil {
		return false
	}
	p, ok := ctx.Scene.GetByType(ctx.Node, td.ID)
	if !ok {
		return false
	}
	hook(ctx, p)
	return true
}
