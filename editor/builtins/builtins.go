package builtins

import (
	"aego/editor/extension"
	"aego/engine/module"
	"aego/render2d"
	render2deditor "aego/render2d/editor"
	"aego/render3d"
	render3deditor "aego/render3d/editor"
)

func Hooks(man *module.Manifest) []module.Hook {
	known := map[string]func(*extension.Registry) error{
		render2d.ID: render2deditor.Hook,
		render3d.ID: render3deditor.Hook,
	}
	var out []module.Hook
	for _, info := range man.Modules {
		if fn, ok := known[info.ID]; ok {
			out = append(out, module.Hook{Owner: info.ID, Fn: fn})
		}
	}
	return append(out, man.Editor...)
}
