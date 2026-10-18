package dataasset

import (
	"aego/asset"
	"aego/engine"
	"aego/engine/module"
)

const ID = "data"

type Module struct{}

func New() *Module {
	return &Module{}
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Builtin: true}
}

func (*Module) Register(r *module.Registry) error {
	registerDocs(r)
	registerDev(r)
	r.Validator(string(asset.ArtData), checkRecord)
	r.Validator(string(asset.ArtTable), checkTable)
	r.Finalize(module.FinalizeData)
	return nil
}
