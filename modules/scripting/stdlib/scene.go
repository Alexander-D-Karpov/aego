package stdlib

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/script/abi"
	"aego/script/bind"
)

func sceneOf(c *bind.Ctx) (*scene.Scene, bool) {
	if c.Scene == nil {
		c.Fail("no scene is bound to this script")
		return nil, false
	}
	return c.Scene, true
}

func alive(c *bind.Ctx, h scene.NodeHandle) (*scene.Scene, bool) {
	s, ok := sceneOf(c)
	if !ok {
		return nil, false
	}
	if !s.IsAlive(h) {
		c.Fail("node reference is nil or destroyed")
		return nil, false
	}
	return s, true
}

func in2D(c *bind.Ctx, h scene.NodeHandle) (*scene.Scene, bool) {
	s, ok := alive(c, h)
	if ok && s.Space(h) != scene.Space2D {
		c.Fail("node " + s.Name(h) + " is 3D, use the 3D transform methods")
		return nil, false
	}
	return s, ok
}

func in3D(c *bind.Ctx, h scene.NodeHandle) (*scene.Scene, bool) {
	s, ok := alive(c, h)
	if ok && s.Space(h) != scene.Space3D {
		c.Fail("node " + s.Name(h) + " is 2D, use the 2D transform methods")
		return nil, false
	}
	return s, ok
}

func orNil(s *scene.Scene, h scene.NodeHandle) scene.NodeHandle {
	if h == s.Root() || !s.IsAlive(h) {
		return scene.NodeHandle{}
	}
	return h
}

func installScene(r *bind.Registry) {
	m := r.Module(ModScene)
	bind.Func1(m, "Find", func(c *bind.Ctx, path string) scene.NodeHandle {
		s, ok := sceneOf(c)
		if !ok {
			return scene.NodeHandle{}
		}
		return orNil(s, s.Find(s.Root(), path))
	}, bind.Doc("Find returns the node at a path from the scene root, or nil."), bind.Params("path"),
		bind.Example("spawn := scene.Find(\"Level/Spawn\")"))
	bind.Func2(m, "Instantiate", func(c *bind.Ctx, ref asset.Ref[asset.Prefab], parent scene.NodeHandle) scene.NodeHandle {
		s, ok := sceneOf(c)
		if !ok {
			return scene.NodeHandle{}
		}
		at := s.Root()
		if !parent.IsZero() {
			if !s.IsAlive(parent) {
				c.Fail("parent node is destroyed")
				return scene.NodeHandle{}
			}
			at = parent
		}
		res := s.Resolver()
		if res == nil || ref.ID.IsZero() {
			c.Fail("prefab reference is nil")
			return scene.NodeHandle{}
		}
		t, err := sceneio.NewLibrary(s.Registry(), res).Template(ref.ID)
		if err != nil {
			c.Failf("prefab %s is not loaded: %v", ref.ID, err)
			return scene.NodeHandle{}
		}
		h, err := sceneio.Instantiate(s, at, t)
		if err != nil {
			c.Fail(err.Error())
			return scene.NodeHandle{}
		}
		return h
	}, bind.Doc("Instantiate creates a copy of a prefab under parent, or under the root when parent is nil.\nThe new nodes appear at the end of the phase; their scripts get Ready before the next Update."),
		bind.Params("prefab", "parent"), bind.Example("b := scene.Instantiate(Bullet, nil)\nb.SetPosition(node.WorldPosition())"))
	bind.Proc1(m, "Destroy", func(c *bind.Ctx, h scene.NodeHandle) {
		if s, ok := alive(c, h); ok {
			s.Destroy(h)
		}
	}, bind.Doc("Destroy removes a node and its children at the end of the phase."), bind.Params("node"),
		bind.Example("scene.Destroy(Target)"))
	installNode(r.Methods(abi.NodeRef))
	installPrefabNode(r.Methods(abi.NodeRef))
	installQueries(m)
	installTags(r.Methods(abi.NodeRef))
}

func installNode(n *bind.Module) {
	bind.Func1(n, "IsAlive", func(c *bind.Ctx, h scene.NodeHandle) bool {
		return c.Scene != nil && c.Scene.IsAlive(h)
	}, bind.Doc("IsAlive reports whether the node exists. It is false right after Destroy."),
		bind.Example("if Target != nil && Target.IsAlive() {"))
	bind.Func1(n, "Name", func(c *bind.Ctx, h scene.NodeHandle) string {
		if s, ok := alive(c, h); ok {
			return s.Name(h)
		}
		return ""
	}, bind.Doc("Name returns the node name, unique among its siblings."), bind.Example("log.Info(node.Name())"))
	bind.Func1(n, "Parent", func(c *bind.Ctx, h scene.NodeHandle) scene.NodeHandle {
		if s, ok := alive(c, h); ok {
			return orNil(s, s.Parent(h))
		}
		return scene.NodeHandle{}
	}, bind.Doc("Parent returns the parent node, or nil for top-level nodes."), bind.Example("owner := node.Parent()"))
	bind.Func2(n, "Child", func(c *bind.Ctx, h scene.NodeHandle, name string) scene.NodeHandle {
		if s, ok := alive(c, h); ok {
			return orNil(s, s.Child(h, name))
		}
		return scene.NodeHandle{}
	}, bind.Doc("Child returns the direct child with this name, or nil."), bind.Params("name"),
		bind.Example("gun := node.Child(\"Gun\")"))
	bind.Func2(n, "Find", func(c *bind.Ctx, h scene.NodeHandle, path string) scene.NodeHandle {
		if s, ok := alive(c, h); ok {
			return orNil(s, s.Find(h, path))
		}
		return scene.NodeHandle{}
	}, bind.Doc("Find resolves a path relative to the node: Weapon/Gun, ../Sibling or /Root/Player."), bind.Params("path"),
		bind.Example("muzzle := node.Find(\"Weapon/Muzzle\")"))
	bind.Proc1(n, "Destroy", func(c *bind.Ctx, h scene.NodeHandle) {
		if s, ok := alive(c, h); ok {
			s.Destroy(h)
		}
	}, bind.Doc("Destroy removes the node and its children at the end of the phase."), bind.Example("node.Destroy()"))
	bind.Func1(n, "Enabled", func(c *bind.Ctx, h scene.NodeHandle) bool {
		s, ok := alive(c, h)
		return ok && s.Enabled(h)
	}, bind.Doc("Enabled reports the node's own enabled flag, ignoring its parents."), bind.Example("if !Door.Enabled() {"))
	bind.Proc2(n, "SetEnabled", func(c *bind.Ctx, h scene.NodeHandle, v bool) {
		if s, ok := alive(c, h); ok {
			s.SetEnabled(h, v)
		}
	}, bind.Doc("SetEnabled turns the node on or off. Disabled nodes and their children are not drawn."), bind.Params("enabled"),
		bind.Example("Shield.SetEnabled(false)"))
	bind.Func1(n, "Space", func(c *bind.Ctx, h scene.NodeHandle) int64 {
		if s, ok := alive(c, h); ok {
			return int64(s.Space(h))
		}
		return 0
	}, bind.Doc("Space returns 0 for 2D nodes and 1 for 3D nodes."), bind.Example("if node.Space() == 1 {"))
	bind.Func1(n, "Transform2D", func(c *bind.Ctx, h scene.NodeHandle) scene.Transform2D {
		if s, ok := in2D(c, h); ok {
			return s.Local(h)
		}
		return scene.IdentityTransform()
	}, bind.Doc("Transform2D returns a copy of the local 2D transform. Change it and pass it to SetTransform2D."),
		bind.Example("t := node.Transform2D()\nt.Position.X += 1\nnode.SetTransform2D(t)"))
	bind.Proc2(n, "SetTransform2D", func(c *bind.Ctx, h scene.NodeHandle, t scene.Transform2D) {
		if s, ok := in2D(c, h); ok {
			s.SetLocal(h, t)
		}
	}, bind.Doc("SetTransform2D replaces the local 2D transform."), bind.Params("t"), bind.Example("node.SetTransform2D(t)"))
	bind.Func1(n, "Position", func(c *bind.Ctx, h scene.NodeHandle) mathx.Vec2 {
		if s, ok := in2D(c, h); ok {
			return s.Local(h).Position
		}
		return mathx.Vec2{}
	}, bind.Doc("Position returns the local position relative to the parent."), bind.Example("p := node.Position()"))
	bind.Proc2(n, "SetPosition", func(c *bind.Ctx, h scene.NodeHandle, p mathx.Vec2) {
		if s, ok := in2D(c, h); ok {
			s.SetPosition(h, p)
		}
	}, bind.Doc("SetPosition sets the local position."), bind.Params("p"),
		bind.Example("node.SetPosition(node.Position() + velocity * dt)"))
	bind.Func1(n, "WorldPosition", func(c *bind.Ctx, h scene.NodeHandle) mathx.Vec2 {
		if s, ok := in2D(c, h); ok {
			return s.WorldPosition(h)
		}
		return mathx.Vec2{}
	}, bind.Doc("WorldPosition returns the position in world space, as of the last transform pass."),
		bind.Example("d := math.Dist(node.WorldPosition(), Target.WorldPosition())"))
	bind.Func1(n, "Rotation", func(c *bind.Ctx, h scene.NodeHandle) float32 {
		if s, ok := in2D(c, h); ok {
			return s.Local(h).Rotation
		}
		return 0
	}, bind.Doc("Rotation returns the local rotation in radians, clockwise."), bind.Example("a := node.Rotation()"))
	bind.Proc2(n, "SetRotation", func(c *bind.Ctx, h scene.NodeHandle, r float32) {
		if s, ok := in2D(c, h); ok {
			s.SetRotation(h, r)
		}
	}, bind.Doc("SetRotation sets the local rotation in radians."), bind.Params("radians"),
		bind.Example("node.SetRotation(node.Rotation() + Spin * dt)"))
	bind.Func1(n, "Scale", func(c *bind.Ctx, h scene.NodeHandle) mathx.Vec2 {
		if s, ok := in2D(c, h); ok {
			return s.Local(h).Scale
		}
		return mathx.Vec2{}
	}, bind.Doc("Scale returns the local scale."), bind.Example("s := node.Scale()"))
	bind.Proc2(n, "SetScale", func(c *bind.Ctx, h scene.NodeHandle, v mathx.Vec2) {
		if s, ok := in2D(c, h); ok {
			s.SetScale(h, v)
		}
	}, bind.Doc("SetScale sets the local scale. A negative X mirrors the node."), bind.Params("scale"),
		bind.Example("node.SetScale(Vec2{-1, 1})"))
	bind.Func1(n, "Transform3D", func(c *bind.Ctx, h scene.NodeHandle) scene.Transform3D {
		if s, ok := in3D(c, h); ok {
			return s.Local3D(h)
		}
		return scene.IdentityTransform3D()
	}, bind.Doc("Transform3D returns a copy of the local 3D transform."),
		bind.Example("t := node.Transform3D()\nt.Position.Y += 1\nnode.SetTransform3D(t)"))
	bind.Proc2(n, "SetTransform3D", func(c *bind.Ctx, h scene.NodeHandle, t scene.Transform3D) {
		if s, ok := in3D(c, h); ok {
			s.SetLocal3D(h, t)
		}
	}, bind.Doc("SetTransform3D replaces the local 3D transform."), bind.Params("t"), bind.Example("node.SetTransform3D(t)"))
	bind.Func1(n, "Position3", func(c *bind.Ctx, h scene.NodeHandle) mathx.Vec3 {
		if s, ok := in3D(c, h); ok {
			return s.Local3D(h).Position
		}
		return mathx.Vec3{}
	}, bind.Doc("Position3 returns the local 3D position."), bind.Example("p := node.Position3()"))
	bind.Proc2(n, "SetPosition3", func(c *bind.Ctx, h scene.NodeHandle, p mathx.Vec3) {
		if s, ok := in3D(c, h); ok {
			s.SetPosition3D(h, p)
		}
	}, bind.Doc("SetPosition3 sets the local 3D position."), bind.Params("p"),
		bind.Example("node.SetPosition3(node.Position3() + Vec3{0, 0, -Speed * dt})"))
	bind.Func1(n, "Rotation3", func(c *bind.Ctx, h scene.NodeHandle) mathx.Quat {
		if s, ok := in3D(c, h); ok {
			return s.Local3D(h).Rotation
		}
		return mathx.QuatIdentity()
	}, bind.Doc("Rotation3 returns the local 3D rotation."), bind.Example("q := node.Rotation3()"))
	bind.Proc2(n, "SetRotation3", func(c *bind.Ctx, h scene.NodeHandle, q mathx.Quat) {
		if s, ok := in3D(c, h); ok {
			s.SetRotation3D(h, q)
		}
	}, bind.Doc("SetRotation3 sets the local 3D rotation."), bind.Params("q"),
		bind.Example("node.SetRotation3(math.QuatEuler(0, angle, 0))"))
}
