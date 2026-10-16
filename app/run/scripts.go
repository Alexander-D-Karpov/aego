package run

import (
	"aego/app"
	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/asset/pipeline"
	scriptsystems "aego/modules/scripting/systems"
	"aego/project"
	"aego/script/bind"
	"aego/script/workspace"
	"aego/sdk"
)

func DefaultImporters() (*pipeline.ImporterRegistry, error) {
	return Importers(nil)
}

func Importers(ws *workspace.Workspace) (*pipeline.ImporterRegistry, error) {
	types, err := assembly.NewTypes()
	if err != nil {
		return nil, err
	}
	man, err := builtin.Default().Load(types, bind.New(), assembly.Options{})
	if err != nil {
		return nil, err
	}
	return app.DevImporters(ws, man)
}

func ScriptBindings(p *project.Project, opt assembly.Options, mods ...sdk.Module) (*bind.Registry, error) {
	types, err := assembly.NewTypes()
	if err != nil {
		return nil, err
	}
	if opt.Input == nil {
		opt.Input = &p.Input
	}
	reg := bind.New()
	if _, err := builtin.Default().Load(types, reg, opt, sdk.AdaptAll(mods)...); err != nil {
		return nil, err
	}
	if err := types.Schema().Freeze(); err != nil {
		return nil, err
	}
	return reg, scriptsystems.Freeze(reg, types.Schema())
}
