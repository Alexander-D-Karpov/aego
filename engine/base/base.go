package base

import (
	"aego/engine"
	"aego/engine/module"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
	"aego/schema"
)

const CoreID = "engine"

type Core struct{}

func (Core) Info() module.Info {
	return module.Info{ID: CoreID, Version: engine.Version, Builtin: true}
}

func (Core) Register(r *module.Registry) error {
	r.Template(schema.TemplateDesc{ID: CoreID + ".empty", Name: "Empty", Node: "Node", Setup: empty})
	r.Template(schema.TemplateDesc{ID: CoreID + ".empty3d", Name: "Empty", Space: schema.Space3D, Setup: empty})
	return nil
}

func empty(*scene.Scene, scene.NodeHandle) {}

func RegisterTypes(t *scene.Types) error {
	if err := frame.RegisterTypes(t); err != nil {
		return err
	}
	return sceneio.RegisterUnknown(t)
}

func NewTypes() (*scene.Types, error) {
	t := scene.NewTypes(schema.NewRegistry())
	if err := RegisterTypes(t); err != nil {
		return nil, err
	}
	return t, nil
}
