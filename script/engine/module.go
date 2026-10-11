package scriptengine

import (
	"aego/engine"
	"aego/engine/module"
	"aego/scene/scriptcomp"
	scriptmodule "aego/script/module"
	"aego/script/stdlib"
)

const ID = "script"

type Module struct {
	opt stdlib.Options
}

func New(opt stdlib.Options) Module {
	return Module{opt: opt}
}

func (Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Builtin: true}
}

func (m Module) Register(r *module.Registry) error {
	if err := module.Component[scriptcomp.ScriptComponent](r, scriptcomp.Options()...).
		Category("Scripts").
		Display("Scripts").
		Icon("cat.scripts").
		ScriptNone().
		Err(); err != nil {
		return err
	}
	r.Types().SetOverlay(scriptcomp.Overlay{})
	if b := r.Bindings(); b != nil {
		stdlib.Install(b, m.opt)
	}
	r.Use(scriptmodule.New())
	r.Finalize(bindMethods)
	registerDev(r)
	return nil
}
