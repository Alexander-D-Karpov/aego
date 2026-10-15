package native

import (
	"aego/core/mathx"
	"aego/platform/input"
	"aego/scene"
	"aego/schema"
	"aego/sdk"
)

type Rotator struct {
	Speed float32 `aego:"speed,unit=rad/s"`
}

type Rotator3D struct {
	Axis  mathx.Vec3 `aego:"axis"`
	Speed float32    `aego:"speed,unit=rad/s"`
}

type Health struct {
	Max     float32 `aego:"max,min=1,display=Max HP"`
	Current float32 `aego:"current,min=0,display=Current HP"`
	Regen   float32 `aego:"regen,min=0,unit=hp/s,tip=Health restored per second"`
}

type Module struct{}

func Modules() []sdk.Module {
	return []sdk.Module{Module{}}
}

func (Module) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "game"}
}

func (Module) Register(r *sdk.Registry) error {
	if err := sdk.Component[Rotator](r, sdk.Name("game.Rotator"), schema.Space(schema.Space2D)).
		Category("Scripts").Display("Rotator").Icon("cat.scripts").Err(); err != nil {
		return err
	}
	if err := sdk.Component[Rotator3D](r, sdk.Name("game.Rotator3D"), schema.Space(schema.Space3D)).
		Category("Scripts").Display("Rotator 3D").Icon("cat.scripts").Err(); err != nil {
		return err
	}
	if err := sdk.Component[Health](r, sdk.Name("game.Health")).
		Category("Scripts").Display("Health").Icon("cat.scripts").Err(); err != nil {
		return err
	}
	r.Template(sdk.TemplateDesc{ID: "game.enemy", Name: "Enemy", Category: "Actors", Setup: buildEnemy})
	r.System(sdk.SystemDesc{ID: "game.rotate", Phase: sdk.PhaseUpdate, System: rotateSystem{}})
	r.System(sdk.SystemDesc{ID: "game.rotate3d", Phase: sdk.PhaseUpdate, System: rotate3DSystem{}})
	r.System(sdk.SystemDesc{ID: "game.health", Phase: sdk.PhaseUpdate, System: healthSystem{}})
	registerEditor(r)
	return nil
}

func buildEnemy(s *scene.Scene, h scene.NodeHandle) {
	sr := scene.Add[sdk.SpriteRenderer](s, h)
	*sr = sdk.DefaultSpriteRenderer()
	sr.Size = mathx.Vec2{X: 1, Y: 1}
	*scene.Add[Health](s, h) = Health{Max: 50, Current: 50, Regen: 2}
	scene.Add[Rotator](s, h).Speed = 1
}

type rotateSystem struct{}

func (rotateSystem) Run(ctx *sdk.Context) {
	s := sdk.SceneOf(ctx)
	if s == nil {
		return
	}
	dt := float32(ctx.Dt)
	scene.Query1[Rotator](s).Each(func(h scene.NodeHandle, r *Rotator) {
		t := s.Local(h)
		t.Rotation += r.Speed * dt
		s.SetLocal(h, t)
	})
}

type rotate3DSystem struct{}

func (rotate3DSystem) Run(ctx *sdk.Context) {
	s := sdk.SceneOf(ctx)
	if s == nil {
		return
	}
	dt := float32(ctx.Dt)
	scene.Query1[Rotator3D](s).Each(func(h scene.NodeHandle, r *Rotator3D) {
		axis := r.Axis
		if axis.LenSq() < 1e-12 {
			axis = mathx.Vec3{Y: 1}
		}
		q := mathx.QuatAxisAngle(axis, r.Speed*dt).Mul(s.Local3D(h).Rotation)
		s.SetRotation3D(h, q.Normalized())
	})
}

type healthSystem struct{}

func (healthSystem) Run(ctx *sdk.Context) {
	s := sdk.SceneOf(ctx)
	if s == nil {
		return
	}
	hit := sdk.InputOf(ctx).KeyPressed(input.KeySpace)
	dt := float32(ctx.Dt)
	scene.Query1[Health](s).Each(func(_ scene.NodeHandle, hp *Health) {
		if hit {
			hp.Current = max(0, hp.Current-10)
		}
		hp.Current = min(hp.Max, hp.Current+hp.Regen*dt)
	})
}
