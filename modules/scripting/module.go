package scripting

import (
	"aego/contracts/scriptevents"
	"aego/engine"
	"aego/engine/module"
	scriptstdlib "aego/modules/scripting/stdlib"
	scriptsystems "aego/modules/scripting/systems"
	"aego/scene/scriptcomp"
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
		scriptstdlib.Install(b, m.opt)
	}
	r.Use(scriptsystems.New())
	r.Finalize(bindEvents)
	r.Finalize(bindMethods)
	registerDev(r)
	return nil
}

func (Module) AttachWorld(ctx *module.AttachContext) error {
	if !ctx.Play || ctx.World == nil {
		return nil
	}
	ctx.World.SetService(scriptevents.ServiceID, scriptsystems.EventSink(ctx.World))
	return nil
}

func (Module) DetachWorld(ctx *module.AttachContext) {
	if !ctx.Play || ctx.World == nil {
		return
	}
	ctx.World.RemoveService(scriptevents.ServiceID)
}
