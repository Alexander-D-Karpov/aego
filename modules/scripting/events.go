package scripting

import (
	"errors"

	"aego/core/errs"
	"aego/core/log"
	"aego/engine/module"
	"aego/script/abi"
	"aego/script/bind"
)

func bindEvents(r *module.Registry) error {
	b := r.Bindings()
	if b == nil {
		return nil
	}
	var all []error
	for _, e := range r.Events() {
		opts := []bind.Option{bind.Doc(e.Doc)}
		if len(e.Names) > 0 {
			opts = append(opts, bind.Params(e.Names...))
		}
		if err := b.Event(abi.EventDef{Name: e.Name, Params: e.Params}, opts...); err != nil {
			all = append(all, errs.New(errs.EngineModule, "script event "+e.Name+": "+err.Error(), log.F("event", e.Name)))
		}
	}
	return errors.Join(all...)
}
