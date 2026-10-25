//go:build !aego_runtime && !js

package animation

import (
	"aego/asset/importers/animclip"
	"aego/asset/importers/animctrl"
	"aego/asset/pipeline"
	"aego/engine/module"
)

func registerDev(r *module.Registry) {
	r.Dev(func(imps *pipeline.ImporterRegistry, _ *pipeline.DevEnv) error {
		if err := imps.Register(animclip.New()); err != nil {
			return err
		}
		return imps.Register(animctrl.New())
	})
}
