package module

import (
	"aego/engine"
	"aego/schema"
	"aego/script/bind"
	"aego/script/runtime"
	"aego/script/stdlib"
)

const (
	SystemSwap       = "script.swap"
	SystemFixed      = "script.fixed"
	SystemUpdate     = "script.update"
	SystemCoroutines = "script.coroutines"
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
	for _, d := range []engine.SystemDesc{
		{ID: SystemSwap, Phase: engine.PhaseFrameBegin, Scope: engine.ScopePlay, System: swapSystem{}},
		{ID: SystemFixed, Phase: engine.PhasePrePhysics, Scope: engine.ScopePlay, System: fixedSystem{}},
		{ID: SystemUpdate, Phase: engine.PhaseUpdate, Scope: engine.ScopePlay, System: updateSystem{}},
		{ID: SystemCoroutines, Phase: engine.PhaseUpdate, Scope: engine.ScopePlay, After: []string{SystemUpdate}, System: coroutineSystem{}},
	} {
		if err := sys.Register(d); err != nil {
			return err
		}
	}
	return nil
}

type swapSystem struct{}

func (swapSystem) Run(ctx *engine.Context) {
	if rt := runtimeOf(ctx); rt != nil {
		rt.RunDeferred()
	}
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

type coroutineSystem struct{}

func (coroutineSystem) Run(ctx *engine.Context) {
	if rt := runtimeOf(ctx); rt != nil {
		rt.Coroutines(ctx)
	}
}
