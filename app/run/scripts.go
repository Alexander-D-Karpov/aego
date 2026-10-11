package run

import (
	"aego/app"
	"aego/asset/pipeline"
	"aego/project"
	"aego/script/bind"
	scriptmodule "aego/script/module"
	"aego/script/stdlib"
	"aego/script/workspace"
	"aego/sdk"
)

func DefaultImporters() (*pipeline.ImporterRegistry, error) {
	return Importers(nil)
}

func Importers(ws *workspace.Workspace) (*pipeline.ImporterRegistry, error) {
	man, err := app.BuildModules(app.NewTypes(), bind.New(), stdlib.Options{}, nil)
	if err != nil {
		return nil, err
	}
	return app.DevImporters(ws, man)
}

func ScriptBindings(p *project.Project, opt stdlib.Options, mods ...sdk.Module) (*bind.Registry, error) {
	types := app.NewTypes()
	if opt.Input == nil {
		opt.Input = &p.Input
	}
	reg := bind.New()
	if _, err := app.BuildModules(types, reg, opt, app.Adapt(mods)); err != nil {
		return nil, err
	}
	if err := types.Schema().Freeze(); err != nil {
		return nil, err
	}
	return reg, scriptmodule.Freeze(reg, types.Schema())
}
