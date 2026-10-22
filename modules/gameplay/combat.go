package gameplay

import (
	"errors"
	"math"

	"aego/contracts/persistence"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
)

const (
	NameHealth  = "gameplay.Health"
	NameHurtbox = "gameplay.Hurtbox2D"
	NameHitbox  = "gameplay.Hitbox2D"
)

type Shape uint8

const (
	ShapeBox Shape = iota
	ShapeCircle
)

type Policy uint8

const (
	OnEnter Policy = iota
	OncePerActivation
	Periodic
)

type healthState struct {
	ready   bool
	dmg     int32
	dmgSrc  scene.NodeHandle
	healed  int32
	died    bool
	diedSrc scene.NodeHandle
	revived bool
	arm     bool
	invuln  float32
}

type Health struct {
	Max            int32   `aego:"max,min=1"`
	Current        int32   `aego:"current,transient,readonly,category=Runtime"`
	Team           int32   `aego:"team"`
	InvulnTime     float32 `aego:"invulnTime,min=0"`
	DestroyOnDeath bool    `aego:"destroyOnDeath"`
	DespawnOnDeath bool    `aego:"despawnOnDeath"`
	Invulnerable   bool    `aego:"invulnerable,transient,readonly,category=Runtime"`
	Dead           bool    `aego:"dead,transient,readonly,category=Runtime"`

	st healthState
}

func DefaultHealth() Health {
	return Health{Max: 3, InvulnTime: 0.5}
}

func (h *Health) init() {
	if !h.st.ready {
		h.st.ready = true
		if h.Current <= 0 {
			h.Current = h.Max
		}
	}
}

func (h *Health) Damage(amount int32, src scene.NodeHandle) bool {
	h.init()
	if amount <= 0 || h.Dead || h.st.invuln > 0 {
		return false
	}
	h.Current = max(h.Current-amount, 0)
	h.st.dmg += amount
	h.st.dmgSrc = src
	h.st.arm = h.InvulnTime > 0
	if h.Current == 0 {
		h.Dead, h.st.died, h.st.diedSrc = true, true, src
	}
	return true
}

func (h *Health) Heal(amount int32) {
	h.init()
	if amount <= 0 || h.Dead {
		return
	}
	n := min(h.Current+amount, h.Max) - h.Current
	h.Current += n
	h.st.healed += n
}

func (h *Health) Kill(src scene.NodeHandle) {
	h.init()
	if h.Dead {
		return
	}
	h.Current, h.Dead, h.st.died, h.st.diedSrc = 0, true, true, src
}

func (h *Health) Revive(hp int32) {
	h.init()
	if hp <= 0 || hp > h.Max {
		hp = h.Max
	}
	h.Current, h.Dead = hp, false
	h.st.revived, h.st.invuln, h.st.arm = true, 0, false
}

func (h *Health) SetInvulnerable(seconds float32) {
	h.st.invuln = max(h.st.invuln, seconds)
	h.Invulnerable = h.st.invuln > 0
}

type Hurtbox2D struct {
	Area       mathx.Rect    `aego:"area"`
	Shape      Shape         `aego:"shape"`
	Multiplier float32       `aego:"multiplier,min=0"`
	Active     bool          `aego:"active"`
	Health     scene.NodeRef `aego:"health"`
}

const CodeNoHealth = "GMP006"

func validateHurt(ctx module.ValidateContext, c *Hurtbox2D) []problems.Diagnostic {
	if !c.Health.IsZero() {
		return nil
	}
	s := ctx.Scene
	root := s.Root()
	for n := ctx.Node; !n.IsZero(); n = s.Parent(n) {
		if scene.Has[Health](s, n) {
			return nil
		}
		if n == root {
			break
		}
	}
	return []problems.Diagnostic{{Code: CodeNoHealth, Severity: problems.Warning,
		Message: "Hurtbox 2D has no Health on its node or above it, hits on it are ignored"}}
}

func DefaultHurtbox() Hurtbox2D {
	return Hurtbox2D{Area: mathx.Rect{X: -0.5, Y: -0.5, W: 1, H: 1}, Multiplier: 1, Active: true}
}

type hitRec struct {
	node   scene.NodeHandle
	inside bool
	seen   bool
	hit    bool
	timer  float32
}

type hitState struct {
	recs  []hitRec
	pulse bool
}

func (st *hitState) rec(n scene.NodeHandle) *hitRec {
	for i := range st.recs {
		if st.recs[i].node == n {
			return &st.recs[i]
		}
	}
	st.recs = append(st.recs, hitRec{node: n})
	return &st.recs[len(st.recs)-1]
}

type Hitbox2D struct {
	Area                mathx.Rect `aego:"area"`
	Shape               Shape      `aego:"shape"`
	Damage              int32      `aego:"damage,min=0"`
	Knockback           mathx.Vec2 `aego:"knockback"`
	KnockbackFromCenter bool       `aego:"knockbackFromCenter"`
	Team                int32      `aego:"team,min=-1"`
	FriendlyFire        bool       `aego:"friendlyFire"`
	Policy              Policy     `aego:"policy"`
	Period              float32    `aego:"period,min=0"`
	Active              bool       `aego:"active"`

	st hitState
}

func DefaultHitbox() Hitbox2D {
	return Hitbox2D{Area: mathx.Rect{X: -0.5, Y: -0.5, W: 1, H: 1}, Damage: 1, Team: -1, Policy: OncePerActivation, Period: 0.5, Active: true}
}

func (h *Hitbox2D) Activate() {
	h.Active = true
	h.st.recs = h.st.recs[:0]
}

func (h *Hitbox2D) Deactivate() {
	h.Active = false
}

func (h *Hitbox2D) Pulse() {
	h.st.recs = h.st.recs[:0]
	h.st.pulse = true
}

type hurtRec struct {
	node   scene.NodeHandle
	health scene.NodeHandle
	team   int32
	mult   float32
	area   area
}

func (s *Service) healthOwner(h scene.NodeHandle) (scene.NodeHandle, bool) {
	root := s.scene.Root()
	for n := h; !n.IsZero(); n = s.scene.Parent(n) {
		if scene.Has[Health](s.scene, n) {
			return n, true
		}
		if n == root {
			break
		}
	}
	return scene.NodeHandle{}, false
}

func (s *Service) collectHurt(h scene.NodeHandle, c *Hurtbox2D) {
	if !c.Active || !s.scene.EffectiveEnabled(h) {
		return
	}
	hp, ok := c.Health.Resolve(s.scene)
	if !ok || !scene.Has[Health](s.scene, hp) {
		if hp, ok = s.healthOwner(h); !ok {
			return
		}
	}
	hc, _ := scene.Get[Health](s.scene, hp)
	if hc.Dead {
		return
	}
	s.hurts = append(s.hurts, hurtRec{node: h, health: hp, team: hc.Team, mult: c.Multiplier, area: areaOf(s.scene.World(h), c.Area, c.Shape)})
}

func (s *Service) hitOne(h scene.NodeHandle, c *Hitbox2D) {
	st := &c.st
	active := (c.Active || st.pulse) && s.scene.EffectiveEnabled(h)
	st.pulse = false
	n := 0
	for _, r := range st.recs {
		if s.scene.IsAlive(r.node) {
			r.timer -= s.dt
			r.seen = false
			st.recs[n] = r
			n++
		}
	}
	st.recs = st.recs[:n]
	if active {
		owner, hasOwner := s.healthOwner(h)
		team := c.Team
		if team < 0 && hasOwner {
			if hc, ok := scene.Get[Health](s.scene, owner); ok {
				team = hc.Team
			}
		}
		src := h
		if hasOwner {
			src = owner
		}
		a := areaOf(s.scene.World(h), c.Area, c.Shape)
		for i := range s.hurts {
			hr := &s.hurts[i]
			if hr.node == h || (hasOwner && hr.health == owner) {
				continue
			}
			if !c.FriendlyFire && team >= 0 && hr.team == team {
				continue
			}
			if !overlap(&a, &hr.area) {
				continue
			}
			rec := st.rec(hr.health)
			if rec.seen {
				continue
			}
			rec.seen = true
			var fire bool
			switch c.Policy {
			case OnEnter:
				fire = !rec.inside
			case OncePerActivation:
				fire = !rec.hit
			default:
				fire = !rec.inside || rec.timer <= 0
			}
			rec.inside = true
			if fire {
				rec.hit, rec.timer = true, c.Period
				s.strike(h, c, src, a.c, hr)
			}
		}
	}
	for i := range st.recs {
		if !st.recs[i].seen {
			st.recs[i].inside = false
		}
	}
}

func (s *Service) strike(h scene.NodeHandle, c *Hitbox2D, src scene.NodeHandle, from mathx.Vec2, hr *hurtRec) {
	hp, ok := scene.Get[Health](s.scene, hr.health)
	if !ok {
		return
	}
	dmg := int32(math.Round(float64(float32(c.Damage) * hr.mult)))
	if !hp.Damage(dmg, src) {
		return
	}
	s.stats.Hits++
	if s.world != nil && s.sig != nil {
		s.sig.hit.Emit(s.world, h, hr.health, int64(dmg))
	}
	s.knock(h, c, from, hr.health)
}

func (s *Service) knock(h scene.NodeHandle, c *Hitbox2D, from mathx.Vec2, target scene.NodeHandle) {
	k := c.Knockback
	size := k.Len()
	if size == 0 {
		return
	}
	var dir mathx.Vec2
	if c.KnockbackFromCenter {
		dir, _ = normalize(s.scene.WorldPosition(target).Sub(from))
	} else {
		m := s.scene.World(h)
		dir, _ = normalize(mathx.Vec2{X: m[0]*k.X + m[3]*k.Y, Y: m[1]*k.X + m[4]*k.Y})
	}
	if dir == (mathx.Vec2{}) {
		dir = mathx.Vec2{X: 1}
	}
	k = dir.Scale(size)
	if mc, ok := scene.Get[Motion2D](s.scene, target); ok {
		mc.AddVelocity(k)
		return
	}
	if q := s.queries(); q != nil {
		_ = q.ApplyImpulse(target, k)
	}
}

func (s *Service) healthFixed(h scene.NodeHandle, c *Health) {
	c.init()
	if c.st.invuln > 0 {
		c.st.invuln = max(c.st.invuln-s.dt, 0)
	}
	s.flushHealth(h, c)
}

func (s *Service) flushHealth(h scene.NodeHandle, c *Health) {
	c.init()
	st := &c.st
	if st.arm {
		st.invuln, st.arm = max(st.invuln, c.InvulnTime), false
	}
	c.Invulnerable = st.invuln > 0
	emit := s.world != nil && s.sig != nil
	if st.dmg > 0 {
		if emit {
			s.sig.damaged.Emit(s.world, h, int64(st.dmg), st.dmgSrc)
		}
		st.dmg = 0
	}
	if st.healed > 0 {
		if emit {
			s.sig.healed.Emit(s.world, h, int64(st.healed))
		}
		st.healed = 0
	}
	if st.died {
		st.died = false
		s.stats.Deaths++
		if emit {
			s.sig.died.Emit(s.world, h, st.diedSrc)
		}
		s.onDeath(h, c)
	}
	if st.revived {
		st.revived = false
		if emit {
			s.sig.revived.Emit(s.world, h)
		}
	}
}

func (s *Service) onDeath(h scene.NodeHandle, c *Health) {
	if r, ok := scene.Get[Respawn](s.scene, h); ok {
		r.Trigger()
		return
	}
	switch {
	case c.DespawnOnDeath:
		s.remove(h, true)
	case c.DestroyOnDeath:
		s.remove(h, false)
	}
}

type despawner interface {
	Despawn(scene.NodeHandle)
}

func (s *Service) remove(h scene.NodeHandle, pool bool) {
	if pool && s.world != nil {
		if d, ok := engine.ServiceOf[despawner](s.world, persistence.SpawnerID); ok {
			d.Despawn(h)
			return
		}
	}
	s.scene.Destroy(h)
}

func registerCombat(r *module.Registry, sig *signals) error {
	if err := errors.Join(
		module.Enum[Shape](r, "gameplay.Shape",
			schema.EnumValue{Name: "box", Value: int64(ShapeBox)},
			schema.EnumValue{Name: "circle", Value: int64(ShapeCircle)}),
		module.Enum[Policy](r, "gameplay.HitPolicy",
			schema.EnumValue{Name: "onEnter", Value: int64(OnEnter)},
			schema.EnumValue{Name: "oncePerActivation", Value: int64(OncePerActivation)},
			schema.EnumValue{Name: "periodic", Value: int64(Periodic)}),
	); err != nil {
		return err
	}
	space := schema.Space(schema.Space2D)

	hp := module.Component[Health](r, schema.Name(NameHealth), space,
		schema.Doc("Hit points of a node. Hitboxes, kill zones and scripts damage it; it dies once at zero until revived."),
		schema.FieldDoc("max", "Hit points at the start and after a full revive."),
		schema.FieldDoc("current", "Hit points now."),
		schema.FieldDoc("team", "Hitboxes of the same team do not hurt this node unless they allow friendly fire."),
		schema.FieldDoc("invulnTime", "Seconds of game time without damage after a hit. Hits in the same step all count."),
		schema.FieldDoc("destroyOnDeath", "Destroys the node at death, unless it has Respawn."),
		schema.FieldDoc("despawnOnDeath", "Returns the node to its pool at death; a node that was not spawned is destroyed."),
		schema.FieldDoc("invulnerable", "True while damage is ignored."),
		schema.FieldDoc("dead", "True from death until Revive."),
		schema.Runtime("current", "invulnerable", "dead"),
		schema.Persist("current"),
	).Category(Category).Display("Health").Icon("gameplay.health").Default(DefaultHealth)
	hp.Method2("Damage", func(_ *bind.Ctx, h *Health, amount int64, src scene.NodeHandle) bool {
		return h.Damage(int32(amount), src)
	},
		bind.Doc("Damage removes hit points and reports whether they were removed: not while invulnerable, dead or for amounts below 1."), bind.Params("amount", "source"))
	hp.Proc1("Heal", func(_ *bind.Ctx, h *Health, amount int64) { h.Heal(int32(amount)) },
		bind.Doc("Heal adds hit points up to Max; it does nothing while dead."), bind.Params("amount"))
	hp.Proc1("Kill", func(_ *bind.Ctx, h *Health, src scene.NodeHandle) { h.Kill(src) },
		bind.Doc("Kill sets the hit points to zero."), bind.Params("source"))
	hp.Proc1("Revive", func(_ *bind.Ctx, h *Health, amount int64) { h.Revive(int32(amount)) },
		bind.Doc("Revive brings the node back with this many hit points; 0 means Max."), bind.Params("hp"))
	hp.Proc1("SetInvulnerable", func(_ *bind.Ctx, h *Health, seconds float32) { h.SetInvulnerable(seconds) },
		bind.Doc("SetInvulnerable ignores damage for some seconds of game time."), bind.Params("seconds"))
	sig.damaged = hp.Signal2[int64, scene.NodeHandle]("Damaged", "Damaged fires once per step with the damage taken in it.", "amount", "source")
	sig.healed = hp.Signal1[int64]("Healed", "Healed fires with the hit points restored.", "amount")
	sig.died = hp.Signal1[scene.NodeHandle]("Died", "Died fires once when the hit points reach zero.", "source")
	sig.revived = hp.Signal0("Revived", "Revived fires after Revive.")

	hurt := module.Component[Hurtbox2D](r, schema.Name(NameHurtbox), space,
		schema.Doc("Area where hitboxes damage a Health: on this node or on its nearest parent with Health."),
		schema.FieldDoc("area", "Rectangle in this node's space."),
		schema.FieldDoc("shape", "box uses the rectangle; circle uses the circle inside it."),
		schema.FieldDoc("multiplier", "Damage taken here is multiplied by this, for weak spots and armour."),
		schema.FieldDoc("active", "Off ignores hits."),
		schema.FieldDoc("health", "Health that takes the damage. Empty uses this node or its nearest parent with Health."),
	).Category(Category).Display("Hurtbox 2D").Icon("gameplay.hurtbox").Default(DefaultHurtbox).Validate(validateHurt)

	hit := module.Component[Hitbox2D](r, schema.Name(NameHitbox), space,
		schema.Doc("Damages hurtboxes inside its area, once per target per activation, on every entry or periodically."),
		schema.FieldDoc("area", "Rectangle in this node's space."),
		schema.FieldDoc("shape", "box uses the rectangle; circle uses the circle inside it."),
		schema.FieldDoc("damage", "Damage per hit before the hurtbox multiplier."),
		schema.FieldDoc("knockback", "Push on a hit: velocity for a Motion 2D target, impulse for a dynamic body. Turns with the node."),
		schema.FieldDoc("knockbackFromCenter", "Pushes away from the hitbox centre instead, with the length of Knockback."),
		schema.FieldDoc("team", "Team of the attack; -1 takes the team of the nearest Health above."),
		schema.FieldDoc("friendlyFire", "Also hits targets of the same team."),
		schema.FieldDoc("policy", "onEnter hits on every entry, oncePerActivation hits each target once until Activate, periodic hits every Period while overlapping."),
		schema.FieldDoc("period", "Seconds between hits for the periodic policy."),
		schema.FieldDoc("active", "Off deals no damage."),
	).Category(Category).Display("Hitbox 2D").Icon("gameplay.hitbox").Default(DefaultHitbox)
	hit.Proc0("Activate", func(_ *bind.Ctx, h *Hitbox2D) { h.Activate() },
		bind.Doc("Activate starts a new attack: targets hit before can be hit again."))
	hit.Proc0("Deactivate", func(_ *bind.Ctx, h *Hitbox2D) { h.Deactivate() },
		bind.Doc("Deactivate stops dealing damage."))
	hit.Proc0("Pulse", func(_ *bind.Ctx, h *Hitbox2D) { h.Pulse() },
		bind.Doc("Pulse deals one round of damage in the next step, active or not."))
	sig.hit = hit.Signal2[scene.NodeHandle, int64]("Hit", "Hit fires for each target damaged.", "target", "damage")

	return errors.Join(hp.Err(), hurt.Err(), hit.Err())
}
