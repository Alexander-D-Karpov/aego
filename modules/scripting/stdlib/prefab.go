package stdlib

import (
	"aego/scene"
	"aego/scene/sceneio"
	"aego/script/bind"
)

func installPrefabNode(n *bind.Module) {
	bind.Func1(n, "IsPrefabInstance", func(c *bind.Ctx, h scene.NodeHandle) bool {
		s, ok := alive(c, h)
		if !ok {
			return false
		}
		_, in := sceneio.PrefabRoot(s, h)
		return in
	}, bind.Doc("IsPrefabInstance reports whether the node is a prefab copy or lies inside one."),
		bind.Example("if node.IsPrefabInstance() {"))
	bind.Func1(n, "PrefabRoot", func(c *bind.Ctx, h scene.NodeHandle) scene.NodeHandle {
		s, ok := alive(c, h)
		if !ok {
			return scene.NodeHandle{}
		}
		r, _ := sceneio.PrefabRoot(s, h)
		return r
	}, bind.Doc("PrefabRoot returns the root of the nearest prefab copy that holds the node, the node itself for a root, or nil."),
		bind.Example("owner := node.PrefabRoot()\nowner.Destroy()"))
}
