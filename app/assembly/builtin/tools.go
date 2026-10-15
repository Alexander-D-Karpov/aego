package builtin

import (
	"aego/app/assembly"
	"aego/engine/module"
	"aego/script/bind"
	"aego/script/bind/manifest"
)

type iconDir interface {
	IconDir() string
}

func Manifest(user ...module.Module) (*manifest.Manifest, error) {
	types, err := assembly.NewTypes()
	if err != nil {
		return nil, err
	}
	script := bind.NewManifest()
	if _, err := Default().Load(types, script, assembly.Options{Seed: 1, Testing: true}, user...); err != nil {
		return nil, err
	}
	return manifest.FromRegistry(script, types.Schema())
}

func AuditIcons(user ...module.Module) (*module.IconReport, error) {
	types, err := assembly.NewTypes()
	if err != nil {
		return nil, err
	}
	mods, err := Default().Modules(assembly.Options{})
	if err != nil {
		return nil, err
	}
	return module.Audit(types, bind.NewManifest(), append(mods, user...)...)
}

func IconSources() []string {
	out := []string{"engine/res/icons/src"}
	mods, _ := Default().Modules(assembly.Options{})
	for _, m := range mods {
		if d, ok := m.(iconDir); ok {
			out = append(out, d.IconDir())
		}
	}
	return out
}
