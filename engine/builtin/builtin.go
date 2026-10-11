package builtin

import (
	"aego/engine"
	"aego/engine/module"
	"aego/render2d"
	"aego/render3d"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/script/bind"
	"aego/script/bind/manifest"
	scriptengine "aego/script/engine"
	"aego/script/stdlib"
)

const ID = "engine"

type Core struct{}

func (Core) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Builtin: true}
}

func (Core) Register(r *module.Registry) error {
	r.Template(schema.TemplateDesc{ID: ID + ".empty", Name: "Empty", Node: "Node", Setup: empty})
	r.Template(schema.TemplateDesc{ID: ID + ".empty3d", Name: "Empty", Space: schema.Space3D, Setup: empty})
	return nil
}

func empty(*scene.Scene, scene.NodeHandle) {}

func Modules(opt stdlib.Options) []module.Module {
	return []module.Module{Core{}, render2d.New(), render3d.New(), scriptengine.New(opt)}
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

func Load(types *scene.Types, script *bind.Registry, opt stdlib.Options, user ...module.Module) (*module.Manifest, error) {
	return module.Load(types, script, append(Modules(opt), user...)...)
}

func Manifest(user ...module.Module) (*manifest.Manifest, error) {
	types, err := NewTypes()
	if err != nil {
		return nil, err
	}
	script := bind.NewManifest()
	if _, err := Load(types, script, stdlib.Options{Seed: 1, Testing: true}, user...); err != nil {
		return nil, err
	}
	return manifest.FromRegistry(script, types.Schema())
}

type iconDir interface {
	IconDir() string
}

func IconSources() []string {
	out := []string{"engine/res/icons/src"}
	for _, m := range Modules(stdlib.Options{}) {
		if d, ok := m.(iconDir); ok {
			out = append(out, d.IconDir())
		}
	}
	return out
}
