//go:build !aego_runtime

package sdk

import "aego/asset/pipeline"

type (
	Importer       = pipeline.Importer
	ImportSettings = pipeline.Settings
)

func RegisterImporter(r *Registry, imp Importer) error {
	r.r.Dev(func(imps *pipeline.ImporterRegistry, _ *pipeline.DevEnv) error {
		return imps.Register(imp)
	})
	return nil
}
