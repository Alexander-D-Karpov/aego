//go:build !aego_runtime && !js

package audio

import (
	audioimp "aego/asset/importers/audio"
	"aego/asset/pipeline"
	"aego/engine/module"
)

func registerDev(r *module.Registry) {
	r.Dev(func(imps *pipeline.ImporterRegistry, _ *pipeline.DevEnv) error {
		return imps.Register(audioimp.New())
	})
}
