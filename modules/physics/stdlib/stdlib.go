package stdlib

import (
	"aego/contracts/physics2d"
	"aego/core/mathx"
	"aego/engine"
	"aego/modules/physics/systems"
	"aego/scene"
	"aego/script/bind"
	"aego/script/vm"
)

type Lib struct {
	contact bind.Packer[Contact]
	hits    []physics2d.RayHit
	nodes   []scene.NodeHandle
}

func (l *Lib) Contact(w []vm.Value) Contact {
	return l.contact.Get(w)
}

func (l *Lib) ContactSlots() int {
	return l.contact.Slots()
}

func simOf(c *bind.Ctx) *systems.Sim {
	if c.World == nil {
		return nil
	}
	s, _ := engine.ServiceOf[*systems.Sim](c.World, systems.ServiceID)
	return s
}

func need(c *bind.Ctx) *systems.Sim {
	s := simOf(c)
	if s == nil {
		c.Fail("physics is not running in this world, queries work only during Play")
	}
	return s
}

func fail(c *bind.Ctx, err error) {
	if err != nil {
		c.Fail(err.Error())
	}
}

func defaultFilter(c *bind.Ctx) physics2d.QueryFilter {
	return physics2d.QueryFilter{LayerMask: ^uint32(0), ExcludeNode: c.Node}
}

func toHit(h physics2d.RayHit, ok bool) RayHit {
	if !ok {
		return RayHit{}
	}
	return RayHit{Hit: true, Point: h.Point, Normal: h.Normal, Distance: h.Distance, Node: h.Node, Collider: h.ColliderNode}
}

func (l *Lib) hitBuf(n int) []physics2d.RayHit {
	if cap(l.hits) < n {
		l.hits = make([]physics2d.RayHit, n)
	}
	return l.hits[:n]
}

func (l *Lib) nodeBuf(n int) []scene.NodeHandle {
	if cap(l.nodes) < n {
		l.nodes = make([]scene.NodeHandle, n)
	}
	return l.nodes[:n]
}

func copyNodes(out bind.Out[scene.NodeHandle], src []scene.NodeHandle) int {
	for i, n := range src {
		out.Set(i, n)
	}
	return len(src)
}

func Install(r *bind.Registry) *Lib {
	m := r.Module(ModPhysics)
	bind.Struct[Contact](m, "Contact",
		bind.Doc("Contact describes a touch between two colliders from the receiver's side.\nNormal points from the other body into this one, RelativeVelocity is this body's velocity relative to the other at Point."))
	bind.Struct[RayHit](m, "RayHit",
		bind.Doc("RayHit is the result of a ray query. Hit is false when nothing was found; Node is the body, Collider the node of the shape that was hit."))
	pk, err := bind.NewPacker[Contact](r)
	if err != nil {
		r.Fail(err)
		return nil
	}
	l := &Lib{contact: pk}

	bind.Func3(m, "Raycast", func(c *bind.Ctx, origin, dir mathx.Vec2, maxDist float32) RayHit {
		s := need(c)
		if s == nil {
			return RayHit{}
		}
		return toHit(s.Raycast(origin, dir, maxDist, defaultFilter(c)))
	}, bind.Doc("Raycast returns the closest hit along dir within maxDist. The caller's own body is ignored, triggers are skipped."),
		bind.Params("origin", "dir", "maxDist"),
		bind.Example("hit := physics.Raycast(pos, Vec2{Y: 1}, 0.6)\nif hit.Hit {\n\tgrounded = true\n}"))

	bind.Func3(m, "Linecast", func(c *bind.Ctx, from, to mathx.Vec2, mask int) RayHit {
		s := need(c)
		d := to.Sub(from)
		if s == nil || d.LenSq() == 0 {
			return RayHit{}
		}
		f := defaultFilter(c)
		f.LayerMask = uint32(mask)
		return toHit(s.Raycast(from, d, d.Len(), f))
	}, bind.Doc("Linecast returns the first hit between from and to on the layers in mask; -1 means every layer."),
		bind.Params("from", "to", "mask"),
		bind.Example("hit := physics.Linecast(eye, target, physics.LayerMask(\"World\"))"))

	bind.Func3(m, "RaycastAll", func(c *bind.Ctx, from, to mathx.Vec2, out bind.Out[RayHit]) int {
		s := need(c)
		d := to.Sub(from)
		if s == nil || d.LenSq() == 0 || out.Cap() == 0 {
			return 0
		}
		buf := l.hitBuf(out.Cap())
		n := s.RaycastAll(from, d, d.Len(), defaultFilter(c), buf)
		for i := 0; i < n; i++ {
			out.Set(i, toHit(buf[i], true))
		}
		return n
	}, bind.Doc("RaycastAll fills out with hits between from and to, nearest first, and returns how many it wrote. It writes at most len(out)."),
		bind.Params("from", "to", "out"))

	bind.Func2(m, "OverlapPoint", func(c *bind.Ctx, p mathx.Vec2, out bind.Out[scene.NodeHandle]) int {
		s := need(c)
		if s == nil || out.Cap() == 0 {
			return 0
		}
		buf := l.nodeBuf(out.Cap())
		return copyNodes(out, buf[:s.OverlapPoint(p, defaultFilter(c), buf)])
	}, bind.Doc("OverlapPoint writes the bodies whose colliders contain p into out and returns the count."),
		bind.Params("point", "out"))

	bind.Func3(m, "OverlapCircle", func(c *bind.Ctx, center mathx.Vec2, radius float32, out bind.Out[scene.NodeHandle]) int {
		s := need(c)
		if s == nil || out.Cap() == 0 {
			return 0
		}
		buf := l.nodeBuf(out.Cap())
		return copyNodes(out, buf[:s.OverlapCircle(center, radius, defaultFilter(c), buf)])
	}, bind.Doc("OverlapCircle writes the bodies touching the circle into out and returns the count."),
		bind.Params("center", "radius", "out"))

	bind.Func3(m, "OverlapBox", func(c *bind.Ctx, center, size mathx.Vec2, out bind.Out[scene.NodeHandle]) int {
		s := need(c)
		if s == nil || out.Cap() == 0 {
			return 0
		}
		buf := l.nodeBuf(out.Cap())
		return copyNodes(out, buf[:s.OverlapBox(center, size.Scale(0.5), 0, defaultFilter(c), buf)])
	}, bind.Doc("OverlapBox writes the bodies touching the axis-aligned box into out and returns the count."),
		bind.Params("center", "size", "out"))

	bind.Func0(m, "Gravity", func(c *bind.Ctx) mathx.Vec2 {
		if s := need(c); s != nil {
			return s.World().Gravity()
		}
		return mathx.Vec2{}
	}, bind.Doc("Gravity returns the world gravity in units per second squared. +Y points down."))

	bind.Proc1(m, "SetGravity", func(c *bind.Ctx, g mathx.Vec2) {
		if s := need(c); s != nil {
			fail(c, s.World().SetGravity(g))
		}
	}, bind.Doc("SetGravity changes the world gravity for the rest of the Play session."), bind.Params("gravity"))

	bind.Func1(m, "Layer", func(c *bind.Ctx, name string) int {
		s := need(c)
		if s == nil {
			return -1
		}
		i, ok := s.LayerIndex(name)
		if !ok {
			c.Failf("unknown physics layer %q, declare it in Project Settings → Physics", name)
		}
		return i
	}, bind.Doc("Layer returns the index of a named physics layer."), bind.Params("name"))

	bind.Func1(m, "LayerMask", func(c *bind.Ctx, name string) int {
		s := need(c)
		if s == nil {
			return 0
		}
		i, ok := s.LayerIndex(name)
		if !ok {
			c.Failf("unknown physics layer %q, declare it in Project Settings → Physics", name)
			return 0
		}
		return 1 << i
	}, bind.Doc("LayerMask returns the mask bit of a named layer. Add masks of different layers to combine them."),
		bind.Params("name"), bind.Example("mask := physics.LayerMask(\"World\") + physics.LayerMask(\"Enemies\")"))
	return l
}
