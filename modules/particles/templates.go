//go:build !aego_runtime

package particles

import (
	"aego/engine/module"
	"aego/scene"
	"aego/schema"
	"strings"
)

func registerPresets(r *module.Registry) {
	for _, name := range presetNames {
		r.Template(schema.TemplateDesc{ID: ID + "." + strings.ToLower(name), Name: name, Menu: "Effects", Category: "Effects", Icon: "comp.sprite", Space: schema.Space2D, Setup: func(sc *scene.Scene, h scene.NodeHandle) {
			data, _ := PresetSource(name)
			if c, err := ReadPreset(sc.Types(), sc.Resolver(), data); err == nil {
				*scene.Add[ParticleSystem2D](sc, h) = c
			}
		}})
	}
}
