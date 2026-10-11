//go:build !aego_runtime

package render3d

import (
	"aego/asset/importers/obj"
	"aego/asset/pipeline"
	"aego/engine/module"
)

func registerDev(r *module.Registry) {
	r.Dev(func(imps *pipeline.ImporterRegistry, _ *pipeline.DevEnv) error {
		return imps.Register(obj.New())
	})
}
