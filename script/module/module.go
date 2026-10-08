package module

import (
	"aego/engine"
	"aego/schema"
	"aego/script/bind"
	"aego/script/runtime"
	"aego/script/stdlib"
)

const (
	SystemFixed  = "script.fixed"
	SystemUpdate = "script.update"
)

type Module struct{}

func New() *Module {
	return &Module{}
}

func (*Module) Info() engine.ModuleInfo {
	return engine.ModuleInfo{ID: "engine.script", Version: "1"}
}

func (*Module) Register(reg *engine.Registry) error {
	sys := reg.Systems()
	if err := sys.Register(engine.SystemDesc{
		ID:     SystemFixed,
		Phase:  engine.PhasePrePhysics,
		Scope:  engine.ScopePlay,
		System: fixedSystem{},
	}); err != nil {
		return err
	}
	return sys.Register(engine.SystemDesc{
		ID:     SystemUpdate,
		Phase:  engine.PhaseUpdate,
		Scope:  engine.ScopePlay,
		System: updateSystem{},
	})
}

func runtimeOf(ctx *engine.Context) *runtime.Runtime {
	rt, _ := engine.ServiceOf[*runtime.Runtime](ctx.World, runtime.ServiceID)
	return rt
}

type fixedSystem struct{}

func (fixedSystem) Run(ctx *engine.Context) {
	if rt := runtimeOf(ctx); rt != nil {
		rt.FixedUpdate(ctx)
	}
}

type updateSystem struct{}

func (updateSystem) Run(ctx *engine.Context) {
	if rt := runtimeOf(ctx); rt != nil {
		rt.Update(ctx)
	}
}

func Bindings(opt stdlib.Options) *bind.Registry {
	r := bind.New()
	stdlib.Install(r, opt)
	return r
}

func Freeze(r *bind.Registry, sr *schema.Registry) error {
	runtime.BindComponents(r, sr)
	return r.Freeze()
}
