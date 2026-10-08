package native

import (
	"aego/core/mathx"
	"aego/engine"
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

func (Module) Info() engine.ModuleInfo {
	return engine.ModuleInfo{ID: "game"}
}

func (Module) Register(r *sdk.Registry) error {
	if err := scene.RegisterComponent[Rotator](r.Types, schema.Name("game.Rotator"), schema.Space(schema.Space2D)); err != nil {
		return err
	}
	if err := scene.RegisterComponent[Rotator3D](r.Types, schema.Name("game.Rotator3D"), schema.Space(schema.Space3D)); err != nil {
		return err
	}
	if err := scene.RegisterComponent[Health](r.Types, schema.Name("game.Health")); err != nil {
		return err
	}
	sys := r.Engine.Systems()
	if err := sys.Register(engine.SystemDesc{ID: "game.rotate", Phase: engine.PhaseUpdate, System: rotateSystem{}}); err != nil {
		return err
	}
	if err := sys.Register(engine.SystemDesc{ID: "game.rotate3d", Phase: engine.PhaseUpdate, System: rotate3DSystem{}}); err != nil {
		return err
	}
	return sys.Register(engine.SystemDesc{ID: "game.health", Phase: engine.PhaseUpdate, System: healthSystem{}})
}

type rotateSystem struct{}

func (rotateSystem) Run(ctx *engine.Context) {
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

func (rotate3DSystem) Run(ctx *engine.Context) {
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

func (healthSystem) Run(ctx *engine.Context) {
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
