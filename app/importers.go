package app

import (
	"aego/asset/importers/preset"
	"sort"

	"aego/asset/importers/artsource"
	"aego/asset/importers/atlas"
	audioImporter "aego/asset/importers/audio"
	matImporter "aego/asset/importers/material"
	"aego/asset/importers/scenefile"
	scriptImporter "aego/asset/importers/script"
	shaderImporter "aego/asset/importers/shader"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/log"
	"aego/engine/module"
	"aego/script/workspace"
)

func CoreImporters() (*pipeline.ImporterRegistry, error) {
	reg := pipeline.NewImporterRegistry()
	for _, imp := range []pipeline.Importer{
		atlas.Wrap(artsource.NewTexture()),
		artsource.New(),
		scenefile.NewScene(),
		scenefile.NewPrefab(),
		preset.New(),
		shaderImporter.New(),
		shaderImporter.NewInclude(),
		matImporter.New(),
	} {
		if err := reg.Register(imp); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

func DevImporters(ws *workspace.Workspace, man *module.Manifest) (*pipeline.ImporterRegistry, error) {
	imps, err := CoreImporters()
	if err != nil {
		return nil, err
	}
	env := &pipeline.DevEnv{Workspace: ws}
	for _, h := range man.Dev {
		fn, ok := h.Fn.(func(*pipeline.ImporterRegistry, *pipeline.DevEnv) error)
		if !ok {
			return nil, errs.New(errs.EngineModule, "dev hook of "+h.Owner+" has an unsupported type", log.F("module", h.Owner))
		}
		if err := fn(imps, env); err != nil {
			return nil, errs.Wrap(err, errs.EngineModule, "dev hook of "+h.Owner+" failed").With(log.F("module", h.Owner))
		}
	}
	return imps, nil
}

// disableExternalImportTools affects the existing importers for the entire
// editor session, including later reimports, not only the initial scan.
func disableExternalImportTools(imps *pipeline.ImporterRegistry) {
	if imp, ok := imps.ByID("shader"); ok {
		if shader, ok := imp.(*shaderImporter.Importer); ok {
			shader.DisableExternal = true
		}
	}
	if imp, ok := imps.ByID("audio"); ok {
		if audio, ok := imp.(*audioImporter.Importer); ok {
			audio.DisableExternal = true
		}
	}
}

func ScriptErrors(pl *pipeline.Pipeline) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range pl.Assets() {
		if a.Importer != scriptImporter.ImporterID || a.Error == "" || seen[a.Error] {
			continue
		}
		seen[a.Error] = true
		out = append(out, a.Error)
	}
	sort.Strings(out)
	return out
}

func ScriptImporter(imps *pipeline.ImporterRegistry) *scriptImporter.Importer {
	imp, ok := imps.ByID(scriptImporter.ImporterID)
	if !ok {
		return nil
	}
	si, _ := imp.(*scriptImporter.Importer)
	return si
}
