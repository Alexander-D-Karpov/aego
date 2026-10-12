package builtin

import (
	"aego/engine/builtin/headless"
	"aego/engine/module"
	"aego/render2d"
	"aego/render3d"
	"aego/scene"
	"aego/script/bind"
	"aego/script/bind/manifest"
	scriptengine "aego/script/engine"
	"aego/script/stdlib"
)

const ID = headless.CoreID

type Core = headless.Core

func Modules(opt stdlib.Options) []module.Module {
	return []module.Module{headless.Core{}, render2d.New(), render3d.New(), scriptengine.New(opt)}
}

func RegisterTypes(t *scene.Types) error {
	return headless.RegisterTypes(t)
}

func NewTypes() (*scene.Types, error) {
	return headless.NewTypes()
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

func AuditIcons(user ...module.Module) (*module.IconReport, error) {
	types, err := NewTypes()
	if err != nil {
		return nil, err
	}
	return module.Audit(types, bind.NewManifest(), append(Modules(stdlib.Options{}), user...)...)
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
