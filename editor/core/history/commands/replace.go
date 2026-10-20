package commands

import (
	"aego/core/errs"
	"aego/core/ids"
	"aego/scene"
	"aego/scene/sceneio"
)

func ReplaceNode(s *scene.Scene, node, parent ids.NodeID, index int, data []byte, lib *sceneio.Library) (scene.NodeHandle, error) {
	if h := s.Handle(node); !h.IsZero() && s.IsAlive(h) {
		s.Destroy(h)
	}
	s.Flush()
	p, err := parentOf(s, parent)
	if err != nil {
		return scene.NodeHandle{}, err
	}
	created, _, err := sceneio.LoadInto(s, p, data, "replace", sceneio.LoadOptions{KeepIDs: true, Prefabs: lib})
	if err != nil {
		return scene.NodeHandle{}, err
	}
	for _, h := range created {
		if s.Parent(h) != p {
			continue
		}
		if index >= 0 {
			if err := s.Reparent(h, p, index); err != nil {
				return h, err
			}
			s.Flush()
		}
		return h, nil
	}
	return scene.NodeHandle{}, errs.New(errs.EditorCommand, "replacement has no root node")
}
