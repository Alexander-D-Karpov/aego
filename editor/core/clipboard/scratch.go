package clipboard

import (
	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/history/commands"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

func loadScratch(sc *scene.Scene, data []byte) (ids.NodeID, error) {
	root := sc.Root()
	created, _, err := sceneio.LoadInto(sc, root, data, "fragment", sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		return ids.NodeID{}, err
	}
	for _, h := range created {
		if sc.Parent(h) == root {
			return sc.ID(h), nil
		}
	}
	return ids.NodeID{}, errs.New(errs.EditorCommand, "the fragment holds no node")
}

func putComponent(sc *scene.Scene, h scene.NodeHandle, td *schema.TypeDescriptor, list []commands.FieldValue) error {
	return commands.NewMutation("", commands.EnvFor(sc), commands.AddRaw(sc, sc.ID(h), td, list)).Do(target{sc})
}
