package stdlib

import (
	"strings"

	"aego/contracts/physics2d"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
)

func copyOut(out bind.Out[scene.NodeHandle], src []scene.NodeHandle) int {
	for i, h := range src {
		out.Set(i, h)
	}
	return len(src)
}

func componentNamed(s *scene.Scene, name string) (schema.TypeID, bool) {
	reg := s.Registry()
	if td, ok := reg.ByName(name); ok && td.Kind == schema.TypeComponent {
		return td.ID, true
	}
	var found schema.TypeID
	n := 0
	for _, td := range reg.Components() {
		if td.FullName[strings.LastIndexByte(td.FullName, '.')+1:] == name {
			found, n = td.ID, n+1
		}
	}
	return found, n == 1
}

func installQueries(m *bind.Module) {
	var buf []scene.NodeHandle
	grow := func(n int) []scene.NodeHandle {
		if cap(buf) < n {
			buf = make([]scene.NodeHandle, n)
		}
		return buf[:n]
	}
	bind.Func2(m, "WithTag", func(c *bind.Ctx, tag string, out bind.Out[scene.NodeHandle]) int {
		s, ok := sceneOf(c)
		if !ok || out.Cap() == 0 {
			return 0
		}
		b := grow(out.Cap())
		return copyOut(out, b[:s.WithTag(tag, b)])
	}, bind.Doc("WithTag writes enabled nodes with the tag into out and returns the count; Enemy also finds Enemy.Flying. Nodes come in the order they got the tag."),
		bind.Params("tag", "out"), bind.Example("n := scene.WithTag(\"Enemy\", enemies)"))
	bind.Func2(m, "WithComponent", func(c *bind.Ctx, name string, out bind.Out[scene.NodeHandle]) int {
		s, ok := sceneOf(c)
		if !ok || out.Cap() == 0 {
			return 0
		}
		t, ok := componentNamed(s, name)
		if !ok {
			c.Failf("unknown or ambiguous component %q", name)
			return 0
		}
		b := grow(out.Cap())
		return copyOut(out, b[:s.WithComponent(t, b)])
	}, bind.Doc("WithComponent writes enabled nodes that have the component into out and returns the count."),
		bind.Params("component", "out"), bind.Example("n := scene.WithComponent(\"Body2D\", bodies)"))
	bind.Func3(m, "InArea", func(c *bind.Ctx, r mathx.Rect, mask int, out bind.Out[scene.NodeHandle]) int {
		s, ok := sceneOf(c)
		if !ok || out.Cap() == 0 {
			return 0
		}
		b := grow(out.Cap())
		if c.World != nil {
			if q, ok := engine.ServiceOf[physics2d.Queries](c.World, physics2d.ServiceID); ok {
				half := mathx.Vec2{X: r.W / 2, Y: r.H / 2}
				f := physics2d.QueryFilter{LayerMask: uint32(mask)}
				return copyOut(out, b[:q.OverlapBox(mathx.Vec2{X: r.X + half.X, Y: r.Y + half.Y}, half, 0, f, b)])
			}
		}
		return copyOut(out, b[:s.InRect(r, b)])
	}, bind.Doc("InArea writes the bodies touching the rectangle on the layers in mask into out when physics runs; without physics, every enabled 2D node whose position lies in the rectangle, ignoring mask."),
		bind.Params("rect", "mask", "out"), bind.Example("n := scene.InArea(Rect{x - 2, y - 2, 4, 4}, -1, near)"))
	bind.Func1(m, "ByID", func(c *bind.Ctx, id string) scene.NodeHandle {
		s, ok := sceneOf(c)
		if !ok {
			return scene.NodeHandle{}
		}
		nid, err := ids.ParseNodeID(id)
		if err != nil {
			return scene.NodeHandle{}
		}
		return orNil(s, s.Handle(nid))
	}, bind.Doc("ByID returns the node with this id, as written in the scene file, or nil."), bind.Params("id"))
}

func installTags(n *bind.Module) {
	bind.Func2(n, "HasTag", func(c *bind.Ctx, h scene.NodeHandle, tag string) bool {
		s, ok := alive(c, h)
		return ok && s.HasTag(h, tag)
	}, bind.Doc("HasTag reports whether the node has the tag or a tag under it: Enemy matches Enemy.Flying."),
		bind.Params("tag"), bind.Example("if other.HasTag(\"Enemy\") {"))
	bind.Func2(n, "AddTag", func(c *bind.Ctx, h scene.NodeHandle, tag string) bool {
		s, ok := alive(c, h)
		return ok && s.AddTag(h, tag)
	}, bind.Doc("AddTag adds a tag and returns false when it is malformed."), bind.Params("tag"),
		bind.Example("node.AddTag(\"Enemy.Stunned\")"))
	bind.Func2(n, "RemoveTag", func(c *bind.Ctx, h scene.NodeHandle, tag string) bool {
		s, ok := alive(c, h)
		return ok && s.RemoveTag(h, tag)
	}, bind.Doc("RemoveTag removes a tag and returns whether the node had it."), bind.Params("tag"))
}
