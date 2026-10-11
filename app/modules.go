package app

import (
	"sort"

	"aego/asset/importers/atlas"
	"aego/asset/importers/scenefile"
	scriptImporter "aego/asset/importers/script"
	texImporter "aego/asset/importers/texture"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/log"
	"aego/engine/builtin"
	"aego/engine/module"
	"aego/scene"
	"aego/script/bind"
	"aego/script/stdlib"
	"aego/script/workspace"
	"aego/sdk"
)

func Adapt(mods []sdk.Module) []module.Module {
	out := make([]module.Module, len(mods))
	for i, m := range mods {
		out[i] = sdk.Adapt(m)
	}
	return out
}

func NewTypes() *scene.Types {
	t, err := builtin.NewTypes()
	if err != nil {
		panic("aego: core types failed to register: " + err.Error())
	}
	return t
}

func BuildModules(types *scene.Types, script *bind.Registry, opt stdlib.Options, mods []module.Module) (*module.Manifest, error) {
	return builtin.Load(types, script, opt, mods...)
}

func CoreImporters() (*pipeline.ImporterRegistry, error) {
	reg := pipeline.NewImporterRegistry()
	for _, imp := range []pipeline.Importer{
		atlas.Wrap(texImporter.New()),
		scenefile.NewScene(),
		scenefile.NewPrefab(),
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
