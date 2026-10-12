//go:build !aego_runtime && !js

package scriptengine

import (
	scriptImporter "aego/asset/importers/script"
	"aego/asset/pipeline"
	"aego/engine/module"
	"aego/script/workspace"
)

const reflectFallback = true

func registerDev(r *module.Registry) {
	registerDocs(r)
	r.Dev(func(imps *pipeline.ImporterRegistry, env *pipeline.DevEnv) error {
		var ws *workspace.Workspace
		if env != nil {
			ws, _ = env.Workspace.(*workspace.Workspace)
		}
		return imps.Register(scriptImporter.New(ws))
	})
}
