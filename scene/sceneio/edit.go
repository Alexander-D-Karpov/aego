package sceneio

import (
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/scene"
	"aego/scene/prefab"
	"aego/schema"
)

func wrapDoc(b *aetext.Node) []byte {
	doc := &aetext.Node{Head: "prefab"}
	doc.Attrs = append(doc.Attrs, aetext.Field{Name: "version", Value: aetext.Int(FormatVersion)})
	if b != nil {
		doc.Blocks = append(doc.Blocks, b)
	}
	return aetext.Write(doc)
}

func EncodePrefab(reg *schema.Registry, root *prefab.Node) []byte {
	w := &writer{reg: reg}
	return wrapDoc(w.modelNode(root, localID, nil))
}

func EncodeInstance(reg *schema.Registry, label ids.NodeID, inst *prefab.Instance) []byte {
	w := &writer{reg: reg}
	return wrapDoc(w.instanceBlock(label, inst))
}

func SaveSubtreeWith(s *scene.Scene, root scene.NodeHandle, lib *Library) []byte {
	return save(s, root, "prefab", true, lib)
}

func SaveFlat(s *scene.Scene, root scene.NodeHandle, deep bool) []byte {
	w := &writer{reg: s.Registry(), lib: NewLibrary(s.Registry(), s.Resolver()), flatten: root, plain: deep}
	return wrapDoc(w.node(s, root))
}

func OwnerRoot(s *scene.Scene, h scene.NodeHandle) (scene.NodeHandle, *PrefabInstance, bool) {
	for c := h; !c.IsZero() && c != s.Root() && s.IsAlive(c); c = s.Parent(c) {
		if pi, ok := InstanceOf(s, c); ok && (pi.Owner || pi.Missing) {
			return c, pi, true
		}
	}
	return scene.NodeHandle{}, nil, false
}

func Overrides(s *scene.Scene, h scene.NodeHandle, lib *Library) (*prefab.Instance, error) {
	pi, ok := InstanceOf(s, h)
	if !ok || !pi.Owner {
		return nil, errs.New(errs.SceneNoComponent, "node is not a prefab instance", log.F("node", s.Name(h)))
	}
	if lib == nil {
		lib = NewLibrary(s.Registry(), s.Resolver())
	}
	inst, ok := (&writer{reg: s.Registry(), lib: lib}).computeInstance(s, h, pi)
	if !ok {
		return nil, errs.New(errs.SceneIOParse, "cannot compute the prefab overrides", log.F("prefab", pi.Prefab.String()))
	}
	return inst, nil
}
