//go:build !aego_runtime && !js

package dataasset

import (
	"aego/asset/importers/datafile"
	"aego/asset/pipeline"
	"aego/engine/module"
)

func registerDev(r *module.Registry) {
	r.Dev(func(imps *pipeline.ImporterRegistry, _ *pipeline.DevEnv) error {
		if err := imps.Register(datafile.NewData()); err != nil {
			return err
		}
		return imps.Register(datafile.NewTable())
	})
}
