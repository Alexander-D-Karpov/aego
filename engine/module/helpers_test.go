package module

import "aego/engine"

type nopSystem struct{}

func (nopSystem) Run(*engine.Context) {}

func engineDesc(id string) engine.SystemDesc {
	return engine.SystemDesc{ID: id, Phase: engine.PhaseUpdate, System: nopSystem{}}
}
