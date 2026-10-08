package run

import (
	"sort"

	"aego/asset/importers/atlas"
	"aego/asset/importers/obj"
	"aego/asset/importers/scenefile"
	scriptImporter "aego/asset/importers/script"
	texImporter "aego/asset/importers/texture"
	"aego/asset/pipeline"
	"aego/engine"
	"aego/project"
	"aego/script/abi"
	"aego/script/bind"
	scriptmodule "aego/script/module"
	"aego/script/stdlib"
	"aego/sdk"
)

func DefaultImporters() (*pipeline.ImporterRegistry, error) {
	return Importers("", nil)
}

func Importers(module string, surface func() abi.Surface) (*pipeline.ImporterRegistry, error) {
	reg := pipeline.NewImporterRegistry()
	for _, imp := range []pipeline.Importer{
		atlas.Wrap(texImporter.New()),
		scenefile.NewScene(),
		scenefile.NewPrefab(),
		obj.New(),
		scriptImporter.New(module, surface),
	} {
		if err := reg.Register(imp); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

func ScriptBindings(p *project.Project, opt stdlib.Options, mods ...sdk.Module) (*bind.Registry, error) {
	types := NewTypes()
	if opt.Input == nil {
		opt.Input = &p.Input
	}
	reg := scriptmodule.Bindings(opt)
	if len(mods) > 0 {
		sr := &sdk.Registry{
			Engine:    engine.NewBuilder(engine.Config{Headless: true}).Registry(),
			Types:     types,
			Importers: pipeline.NewImporterRegistry(),
			Script:    reg,
		}
		for _, m := range mods {
			if err := m.Register(sr); err != nil {
				return nil, err
			}
		}
	}
	if err := types.Schema().Freeze(); err != nil {
		return nil, err
	}
	return reg, scriptmodule.Freeze(reg, types.Schema())
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
