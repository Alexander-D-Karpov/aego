package spawn

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/scene"
	"aego/script/bind"
)

func spawner(c *bind.Ctx) *Service {
	s := Of(c.World)
	if s == nil {
		c.Fail("spawning works only during Play")
	}
	return s
}

func installScript(m *bind.Module) {
	bind.Func2(m, "Spawn", func(c *bind.Ctx, ref asset.Ref[asset.Prefab], pos mathx.Vec2) scene.NodeHandle {
		s := spawner(c)
		if s == nil {
			return scene.NodeHandle{}
		}
		h, err := s.Spawn(ref.ID, scene.NodeHandle{}, pos, 0)
		if err != nil {
			c.Fail(err.Error())
		}
		return h
	}, bind.Doc("Spawn places a copy of a prefab at a position under the scene root, taken from its pool when one is free."),
		bind.Params("prefab", "position"), bind.Example("b := scene.Spawn(Bullet, node.WorldPosition())"))
	bind.Func4(m, "SpawnAt", func(c *bind.Ctx, ref asset.Ref[asset.Prefab], parent scene.NodeHandle, pos mathx.Vec2, rot float32) scene.NodeHandle {
		s := spawner(c)
		if s == nil {
			return scene.NodeHandle{}
		}
		h, err := s.Spawn(ref.ID, parent, pos, rot)
		if err != nil {
			c.Fail(err.Error())
		}
		return h
	}, bind.Doc("SpawnAt places a copy of a prefab under parent with a local position and rotation."),
		bind.Params("prefab", "parent", "position", "rotation"), bind.Example("fx := scene.SpawnAt(Spark, node, Vec2{}, 0)"))
	bind.Proc1(m, "Despawn", func(c *bind.Ctx, h scene.NodeHandle) {
		if s := spawner(c); s != nil {
			s.Despawn(h)
		}
	}, bind.Doc("Despawn returns a spawned copy to its pool; it destroys a node that did not come from a pool."),
		bind.Params("node"), bind.Example("scene.Despawn(node)"))
	bind.Proc2(m, "Pool", func(c *bind.Ctx, ref asset.Ref[asset.Prefab], n int64) {
		if s := spawner(c); s != nil {
			if err := s.Pool(ref.ID, int(n)); err != nil {
				c.Fail(err.Error())
			}
		}
	}, bind.Doc("Pool makes n copies of a prefab ahead of time; their Ready runs now and Spawn takes them later."),
		bind.Params("prefab", "count"), bind.Example("scene.Pool(Bullet, 32)"))
}
