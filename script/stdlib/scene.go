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
	})
	bind.Func2(m, "Instantiate", func(c *bind.Ctx, prefab asset.Ref[asset.Prefab], parent scene.NodeHandle) scene.NodeHandle {
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
		if res == nil || prefab.ID.IsZero() {
			c.Fail("prefab reference is nil")
			return scene.NodeHandle{}
		}
		data, ok := res.Prefab(prefab.ID)
		if !ok {
			c.Failf("prefab %s is not loaded", prefab.ID)
			return scene.NodeHandle{}
		}
		created, _, err := sceneio.LoadInto(s, at, data, "prefab", sceneio.LoadOptions{})
		if err != nil {
			c.Fail(err.Error())
			return scene.NodeHandle{}
		}
		for _, h := range created {
			if s.Parent(h) == at {
				return h
			}
		}
		return scene.NodeHandle{}
	})
	bind.Proc1(m, "Destroy", func(c *bind.Ctx, h scene.NodeHandle) {
		if s, ok := alive(c, h); ok {
			s.Destroy(h)
		}
	})
	installNode(r.Methods(abi.NodeRef))
}

func installNode(n *bind.Module) {
	bind.Func1(n, "IsAlive", func(c *bind.Ctx, h scene.NodeHandle) bool {
		return c.Scene != nil && c.Scene.IsAlive(h)
	})
	bind.Func1(n, "Name", func(c *bind.Ctx, h scene.NodeHandle) string {
		if s, ok := alive(c, h); ok {
			return s.Name(h)
		}
		return ""
	})
	bind.Func1(n, "Parent", func(c *bind.Ctx, h scene.NodeHandle) scene.NodeHandle {
		if s, ok := alive(c, h); ok {
			return orNil(s, s.Parent(h))
		}
		return scene.NodeHandle{}
	})
	bind.Func2(n, "Child", func(c *bind.Ctx, h scene.NodeHandle, name string) scene.NodeHandle {
		if s, ok := alive(c, h); ok {
			return orNil(s, s.Child(h, name))
		}
		return scene.NodeHandle{}
	})
	bind.Func2(n, "Find", func(c *bind.Ctx, h scene.NodeHandle, path string) scene.NodeHandle {
		if s, ok := alive(c, h); ok {
			return orNil(s, s.Find(h, path))
		}
		return scene.NodeHandle{}
	})
	bind.Proc1(n, "Destroy", func(c *bind.Ctx, h scene.NodeHandle) {
		if s, ok := alive(c, h); ok {
			s.Destroy(h)
		}
	})
	bind.Func1(n, "Enabled", func(c *bind.Ctx, h scene.NodeHandle) bool {
		s, ok := alive(c, h)
		return ok && s.Enabled(h)
	})
	bind.Proc2(n, "SetEnabled", func(c *bind.Ctx, h scene.NodeHandle, v bool) {
		if s, ok := alive(c, h); ok {
			s.SetEnabled(h, v)
		}
	})
	bind.Func1(n, "Space", func(c *bind.Ctx, h scene.NodeHandle) int64 {
		if s, ok := alive(c, h); ok {
			return int64(s.Space(h))
		}
		return 0
	})
	bind.Func1(n, "Transform2D", func(c *bind.Ctx, h scene.NodeHandle) scene.Transform2D {
		if s, ok := in2D(c, h); ok {
			return s.Local(h)
		}
		return scene.IdentityTransform()
	})
	bind.Proc2(n, "SetTransform2D", func(c *bind.Ctx, h scene.NodeHandle, t scene.Transform2D) {
		if s, ok := in2D(c, h); ok {
			s.SetLocal(h, t)
		}
	})
	bind.Func1(n, "Position", func(c *bind.Ctx, h scene.NodeHandle) mathx.Vec2 {
		if s, ok := in2D(c, h); ok {
			return s.Local(h).Position
		}
		return mathx.Vec2{}
	})
	bind.Proc2(n, "SetPosition", func(c *bind.Ctx, h scene.NodeHandle, p mathx.Vec2) {
		if s, ok := in2D(c, h); ok {
			s.SetPosition(h, p)
		}
	})
	bind.Func1(n, "WorldPosition", func(c *bind.Ctx, h scene.NodeHandle) mathx.Vec2 {
		if s, ok := in2D(c, h); ok {
			return s.WorldPosition(h)
		}
		return mathx.Vec2{}
	})
	bind.Func1(n, "Rotation", func(c *bind.Ctx, h scene.NodeHandle) float32 {
		if s, ok := in2D(c, h); ok {
			return s.Local(h).Rotation
		}
		return 0
	})
	bind.Proc2(n, "SetRotation", func(c *bind.Ctx, h scene.NodeHandle, r float32) {
		if s, ok := in2D(c, h); ok {
			s.SetRotation(h, r)
		}
	})
	bind.Func1(n, "Scale", func(c *bind.Ctx, h scene.NodeHandle) mathx.Vec2 {
		if s, ok := in2D(c, h); ok {
			return s.Local(h).Scale
		}
		return mathx.Vec2{}
	})
	bind.Proc2(n, "SetScale", func(c *bind.Ctx, h scene.NodeHandle, v mathx.Vec2) {
		if s, ok := in2D(c, h); ok {
			s.SetScale(h, v)
		}
	})
	bind.Func1(n, "Transform3D", func(c *bind.Ctx, h scene.NodeHandle) scene.Transform3D {
		if s, ok := in3D(c, h); ok {
			return s.Local3D(h)
		}
		return scene.IdentityTransform3D()
	})
	bind.Proc2(n, "SetTransform3D", func(c *bind.Ctx, h scene.NodeHandle, t scene.Transform3D) {
		if s, ok := in3D(c, h); ok {
			s.SetLocal3D(h, t)
		}
	})
	bind.Func1(n, "Position3", func(c *bind.Ctx, h scene.NodeHandle) mathx.Vec3 {
		if s, ok := in3D(c, h); ok {
			return s.Local3D(h).Position
		}
		return mathx.Vec3{}
	})
	bind.Proc2(n, "SetPosition3", func(c *bind.Ctx, h scene.NodeHandle, p mathx.Vec3) {
		if s, ok := in3D(c, h); ok {
			s.SetPosition3D(h, p)
		}
	})
	bind.Func1(n, "Rotation3", func(c *bind.Ctx, h scene.NodeHandle) mathx.Quat {
		if s, ok := in3D(c, h); ok {
			return s.Local3D(h).Rotation
		}
		return mathx.QuatIdentity()
	})
	bind.Proc2(n, "SetRotation3", func(c *bind.Ctx, h scene.NodeHandle, q mathx.Quat) {
		if s, ok := in3D(c, h); ok {
			s.SetRotation3D(h, q)
		}
	})
}
