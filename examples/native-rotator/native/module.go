package native

import "aego/sdk"

type Rotator struct {
	Speed float32 `aego:"speed,doc=radians per second"`
}

type rotatorSystem struct{}

func (rotatorSystem) Run(ctx *sdk.Context) {
	s := sdk.SceneOf(ctx)
	if s == nil {
		return
	}
	dt := float32(ctx.Dt)
	sdk.Query1[Rotator](s).Each(func(n sdk.NodeHandle, r *Rotator) {
		local := s.Local(n)
		s.SetRotation(n, local.Rotation+r.Speed*dt)
	})
}

type Module struct{}

func (Module) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "rotator", Version: "0.1.0"}
}

func (Module) Register(r *sdk.Registry) error {
	if err := sdk.Component[Rotator](r, sdk.Name("game.Rotator")).
		Default(func() Rotator { return Rotator{Speed: 1} }).
		Script("speed").
		Err(); err != nil {
		return err
	}
	r.System(sdk.SystemDesc{ID: "rotator.spin", Phase: sdk.PhaseUpdate, System: rotatorSystem{}})
	return registerSpinner(r)
}

func Modules() []sdk.Module {
	return []sdk.Module{Module{}}
}
