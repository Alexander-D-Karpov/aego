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
		id := strings.ToLower(name)
		r.Template(schema.TemplateDesc{ID: ID + "." + id, Name: name, Menu: "Effects", Category: "Effects", Icon: "particles." + id, Space: schema.Space2D, Setup: func(sc *scene.Scene, h scene.NodeHandle) {
			data, _ := PresetSource(name)
			if c, err := ReadPreset(sc.Types(), sc.Resolver(), data); err == nil {
				*scene.Add[ParticleSystem2D](sc, h) = c
			}
		}})
	}
}
