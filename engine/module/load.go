package module

import (
	"aego/scene"
	"aego/script/bind"
)

func Load(types *scene.Types, script *bind.Registry, mods ...Module) (*Manifest, error) {
	r := New(types, script)
	for _, m := range mods {
		if err := r.Add(m); err != nil {
			return nil, err
		}
	}
	return r.Build()
}
