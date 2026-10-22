package gameplay

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/engine/module"
	"aego/scene"
	"aego/schema"
)

const (
	spriteName = "engine.SpriteRenderer"
	boxName    = "physics.BoxCollider2D"
	circleName = "physics.CircleCollider2D"
	dynamic    = 2
)

func ensure(s *scene.Scene, h scene.NodeHandle, name string) *schema.TypeDescriptor {
	td, ok := s.Registry().ByName(name)
	if !ok {
		return nil
	}
	module.Ensure(s, h, td)
	return td
}

func set(s *scene.Scene, h scene.NodeHandle, td *schema.TypeDescriptor, field string, v schema.Value) {
	if td == nil {
		return
	}
	if fd, ok := td.Field(field); ok {
		_ = s.SetProperty(h, td.ID, fd.ID, v)
	}
}

func sprite(s *scene.Scene, h scene.NodeHandle, size mathx.Vec2, c mathx.Color) {
	td := ensure(s, h, spriteName)
	set(s, h, td, "sprite", schema.Asset(asset.BuiltinWhite))
	set(s, h, td, "size", schema.Vec2(size))
	set(s, h, td, "tint", schema.Color(c))
}

func body(s *scene.Scene, h scene.NodeHandle, typ int64) {
	td := ensure(s, h, bodyName)
	set(s, h, td, "type", schema.Enum(typ))
	set(s, h, td, "fixedRotation", schema.Bool(true))
}

func box(s *scene.Scene, h scene.NodeHandle, size mathx.Vec2) {
	set(s, h, ensure(s, h, boxName), "size", schema.Vec2(size))
}

func platformer(s *scene.Scene, h scene.NodeHandle) {
	size := mathx.Vec2{X: 0.6, Y: 1}
	sprite(s, h, size, mathx.Color{R: 0.96, G: 0.62, B: 0.04, A: 1})
	body(s, h, kinematic)
	box(s, h, size)
	*scene.Add[Motion2D](s, h) = DefaultMotion()
	*scene.Add[Health](s, h) = DefaultHealth()
	hb := DefaultHurtbox()
	hb.Area = mathx.Rect{X: -0.3, Y: -0.5, W: 0.6, H: 1}
	*scene.Add[Hurtbox2D](s, h) = hb
	*scene.Add[Respawn](s, h) = DefaultRespawn()
}

func topDown(s *scene.Scene, h scene.NodeHandle) {
	sprite(s, h, mathx.Vec2{X: 0.8, Y: 0.8}, mathx.Color{R: 0.2, G: 0.7, B: 0.95, A: 1})
	body(s, h, kinematic)
	set(s, h, ensure(s, h, circleName), "radius", schema.Float32(0.4))
	m := DefaultMotion()
	m.Mode, m.FaceMovement = TopDown, true
	*scene.Add[Motion2D](s, h) = m
	*scene.Add[Health](s, h) = DefaultHealth()
	*scene.Add[Hurtbox2D](s, h) = DefaultHurtbox()
}

func enemy(s *scene.Scene, h scene.NodeHandle) {
	size := mathx.Vec2{X: 0.8, Y: 0.8}
	sprite(s, h, size, mathx.Color{R: 0.86, G: 0.15, B: 0.15, A: 1})
	body(s, h, dynamic)
	box(s, h, size)
	hp := DefaultHealth()
	hp.Team, hp.DestroyOnDeath = 1, true
	*scene.Add[Health](s, h) = hp
	*scene.Add[Hurtbox2D](s, h) = DefaultHurtbox()
	hit := DefaultHitbox()
	hit.Policy, hit.Period, hit.Knockback, hit.KnockbackFromCenter = Periodic, 0.5, mathx.Vec2{X: 6}, true
	*scene.Add[Hitbox2D](s, h) = hit
}

func platformBody(s *scene.Scene, h scene.NodeHandle) {
	size := mathx.Vec2{X: 3, Y: 0.4}
	sprite(s, h, size, mathx.Color{R: 0.05, G: 0.65, B: 0.91, A: 1})
	body(s, h, kinematic)
	box(s, h, size)
}

func checkpoint(s *scene.Scene, h scene.NodeHandle) {
	*scene.Add[Checkpoint2D](s, h) = DefaultCheckpoint()
}

func killZone(s *scene.Scene, h scene.NodeHandle) {
	*scene.Add[KillZone2D](s, h) = DefaultKillZone()
}

func pickup(s *scene.Scene, h scene.NodeHandle) {
	sprite(s, h, mathx.Vec2{X: 0.5, Y: 0.5}, mathx.Color{R: 0.98, G: 0.8, B: 0.08, A: 1})
	*scene.Add[Pickup2D](s, h) = DefaultPickup()
}

func registerTemplates(r *module.Registry) {
	for _, t := range []struct {
		id, name, icon string
		setup          func(*scene.Scene, scene.NodeHandle)
	}{
		{"platformer", "Platformer Character", "gameplay.motion", platformer},
		{"topdown", "Top-down Character", "gameplay.motion", topDown},
		{"enemy", "Enemy", "gameplay.hitbox", enemy},
		{"platform", "Moving Platform", "gameplay.motion", platformBody},
		{"checkpoint", "Checkpoint", "gameplay.checkpoint", checkpoint},
		{"killzone", "Kill Zone", "gameplay.killzone", killZone},
		{"pickup", "Pickup", "gameplay.pickup", pickup},
	} {
		r.Template(schema.TemplateDesc{ID: ID + "." + t.id, Name: t.name, Icon: t.icon, Menu: Category, Category: Category, Space: schema.Space2D, Setup: t.setup})
	}
}
