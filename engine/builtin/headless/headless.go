package headless

import (
	"aego/engine"
	"aego/engine/module"
	"aego/render2d"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
	"aego/schema"
	scriptengine "aego/script/engine"
	"aego/script/stdlib"
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

func Modules(opt stdlib.Options) []module.Module {
	return []module.Module{Core{}, render2d.New(), scriptengine.New(opt)}
}

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
