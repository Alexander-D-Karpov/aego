package native

import "aego/sdk"

type Spinner struct {
	Speed float32 `aego:"speed,unit=rad/s"`
	Spins int64   `aego:"spins,readonly"`
}

type spinnerSystem struct{}

func (spinnerSystem) Run(ctx *sdk.Context) {
	s := sdk.SceneOf(ctx)
	if s == nil {
		return
	}
	dt := float32(ctx.Dt)
	sdk.Query1[Spinner](s).Each(func(n sdk.NodeHandle, sp *Spinner) {
		before := s.Local(n).Rotation
		after := before + sp.Speed*dt
		s.SetRotation(n, after)
		if int64(after/6.2831855) != int64(before/6.2831855) {
			sp.Spins++
		}
	})
}

func registerSpinner(r *sdk.Registry) error {
	r.System(sdk.SystemDesc{ID: "rotator.spinner", Phase: sdk.PhaseUpdate, System: spinnerSystem{}})
	return sdk.Component[Spinner](r, sdk.Name("game.Spinner")).
		Category("Game").
		Icon("cat.game").
		Default(func() Spinner { return Spinner{Speed: 2} }).
		Requires("engine.SpriteRenderer").
		Script("Speed").
		Proc0("Reset", func(_ *sdk.ScriptCtx, sp *Spinner) { sp.Spins = 0 },
			sdk.ScriptDoc("Reset clears the spin counter of the node's Spinner.")).
		Template("Spinner", func(s *sdk.Scene, h sdk.NodeHandle) {
			if sr, ok := sdk.Get[sdk.SpriteRenderer](s, h); ok {
				sr.Size = sdk.Vec2{X: 1, Y: 1}
			}
		}).
		Err()
}
