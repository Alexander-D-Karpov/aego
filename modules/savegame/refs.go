package savegame

import (
	"slices"
	"strings"

	"aego/core/ids"
	"aego/format/aetext"
	"aego/scene"
)

func (s *Service) spawnRoot(h scene.NodeHandle) (scene.NodeHandle, ids.NodeID, bool) {
	sp := s.spawner()
	if sp == nil {
		return scene.NodeHandle{}, ids.NodeID{}, false
	}
	root := s.scene.Root()
	for c := h; !c.IsZero() && c != root; c = s.scene.Parent(c) {
		if pid, ok := sp.PersistentID(c); ok {
			return c, pid, true
		}
	}
	return scene.NodeHandle{}, ids.NodeID{}, false
}

func (s *Service) relPath(root, h scene.NodeHandle) string {
	var parts []string
	for c := h; !c.IsZero() && c != root; c = s.scene.Parent(c) {
		parts = append(parts, s.scene.Name(c))
	}
	slices.Reverse(parts)
	return strings.Join(parts, "/")
}

func (s *Service) RefValue(h scene.NodeHandle) (aetext.Value, bool) {
	sc := s.scene
	if h.IsZero() {
		return aetext.NodeRef(ids.NodeID{}.String()), true
	}
	if !sc.IsAlive(h) {
		return aetext.Value{}, false
	}
	if root, pid, ok := s.spawnRoot(h); ok {
		return aetext.Struct(
			aetext.Field{Name: "spawn", Value: aetext.String(pid.String())},
			aetext.Field{Name: "path", Value: aetext.String(s.relPath(root, h))},
		), true
	}
	if id := sc.ID(h); s.base[id] {
		return aetext.NodeRef(id.String()), true
	}
	return aetext.Value{}, false
}

func (s *Service) ResolveRef(v aetext.Value) (scene.NodeHandle, bool) {
	sc := s.scene
	switch v.Kind {
	case aetext.VNode:
		id, err := ids.ParseNodeID(v.Str)
		if err != nil {
			return scene.NodeHandle{}, false
		}
		if id.IsZero() {
			return scene.NodeHandle{}, true
		}
		h := sc.Handle(id)
		return h, !h.IsZero() && sc.IsAlive(h)
	case aetext.VStruct:
		var pidText, path string
		for _, f := range v.Obj {
			switch f.Name {
			case "spawn":
				pidText = f.Value.Str
			case "path":
				path = f.Value.Str
			}
		}
		pid, err := ids.ParseNodeID(pidText)
		sp := s.spawner()
		if err != nil || sp == nil {
			return scene.NodeHandle{}, false
		}
		root, ok := sp.ByPersistentID(pid)
		if !ok {
			return scene.NodeHandle{}, false
		}
		if path == "" {
			return root, true
		}
		h := sc.Find(root, path)
		return h, !h.IsZero() && sc.IsAlive(h)
	}
	return scene.NodeHandle{}, false
}
