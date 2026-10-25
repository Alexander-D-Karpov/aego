package gameplay

import (
	"errors"
	"slices"

	"aego/core/mathx"
	"aego/engine/module"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
)

const (
	NameLifetime   = "gameplay.Lifetime"
	NameKillZone   = "gameplay.KillZone2D"
	NameCheckpoint = "gameplay.Checkpoint2D"
	NameRespawn    = "gameplay.Respawn"
	NamePickup     = "gameplay.Pickup2D"
)

type candKind uint8

const (
	candKill candKind = iota
	candCheckpoint
	candPickup
)

type zoneState struct {
	inside []scene.NodeHandle
	next   []scene.NodeHandle
	used   bool
}

type lifeState struct {
	started bool
	done    bool
}

type Lifetime struct {
	Seconds   float32 `aego:"seconds,min=0"`
	Unscaled  bool    `aego:"unscaled"`
	Despawn   bool    `aego:"despawn"`
	Remaining float32 `aego:"remaining,transient,readonly,category=Runtime"`

	st lifeState
}

type KillZone2D struct {
	Area mathx.Rect `aego:"area"`
	Tag  string     `aego:"tag"`

	st zoneState
}

type Checkpoint2D struct {
	Area       mathx.Rect    `aego:"area"`
	Priority   int32         `aego:"priority"`
	Once       bool          `aego:"once"`
	SpawnPoint scene.NodeRef `aego:"spawnPoint"`

	st zoneState
}

type respawnState struct {
	point    mathx.Vec2
	hasPoint bool
	priority int32
	hasPri   bool
	pending  float32
	waiting  bool
}

type Respawn struct {
	Delay         float32 `aego:"delay,min=0"`
	RestoreHealth bool    `aego:"restoreHealth"`
	ResetVelocity bool    `aego:"resetVelocity"`
	Persist       bool    `aego:"persist"`

	st respawnState
}

type Pickup2D struct {
	Area    mathx.Rect `aego:"area"`
	Tag     string     `aego:"tag"`
	Once    bool       `aego:"once"`
	Despawn bool       `aego:"despawn"`

	st zoneState
}

func DefaultLifetime() Lifetime {
	return Lifetime{Seconds: 2}
}

func DefaultKillZone() KillZone2D {
	return KillZone2D{Area: mathx.Rect{X: -10, Y: -1, W: 20, H: 2}}
}

func DefaultCheckpoint() Checkpoint2D {
	return Checkpoint2D{Area: mathx.Rect{X: -1, Y: -2, W: 2, H: 4}}
}

func DefaultRespawn() Respawn {
	return Respawn{Delay: 0.5, RestoreHealth: true, ResetVelocity: true}
}

func DefaultPickup() Pickup2D {
	return Pickup2D{Area: mathx.Rect{X: -0.5, Y: -0.5, W: 1, H: 1}, Once: true}
}

func (r *Respawn) Trigger() {
	if !r.st.waiting {
		r.st.waiting, r.st.pending = true, r.Delay
	}
}

func (r *Respawn) SetPoint(p mathx.Vec2) {
	r.st.point, r.st.hasPoint = p, true
}

func (s *Service) candidates(tag string, kind candKind) {
	s.cand = s.cand[:0]
	if tag != "" {
		if cap(s.cand) == 0 {
			s.cand = make([]scene.NodeHandle, 0, 16)
		}
		for {
			n := s.scene.WithTag(tag, s.cand[:cap(s.cand)])
			if n < cap(s.cand) {
				s.cand = s.cand[:n]
				return
			}
			s.cand = make([]scene.NodeHandle, 0, 2*cap(s.cand))
		}
	}
	switch kind {
	case candKill:
		scene.Query1[Health](s.scene).Each(s.candHealth)
		scene.Query1[Respawn](s.scene).Each(s.candResp)
	case candCheckpoint:
		scene.Query1[Respawn](s.scene).Each(s.candResp)
		scene.Query1[Health](s.scene).Each(s.candHealth)
	default:
		scene.Query1[Motion2D](s.scene).Each(s.candMotion)
	}
}

func (s *Service) enterZone(z *zoneState, a *area) []scene.NodeHandle {
	z.next = z.next[:0]
	s.entered = s.entered[:0]
	for _, n := range s.cand {
		if !s.scene.IsAlive(n) || !s.scene.EffectiveEnabled(n) || !a.contains(s.scene.WorldPosition(n)) {
			continue
		}
		z.next = append(z.next, n)
		if !slices.Contains(z.inside, n) {
			s.entered = append(s.entered, n)
		}
	}
	z.inside, z.next = z.next, z.inside
	return s.entered
}

func (s *Service) killOne(h scene.NodeHandle, c *KillZone2D) {
	if !s.scene.EffectiveEnabled(h) {
		return
	}
	s.candidates(c.Tag, candKill)
	a := areaOf(s.scene.World(h), c.Area, ShapeBox)
	for _, n := range s.enterZone(&c.st, &a) {
		if hp, ok := scene.Get[Health](s.scene, n); ok {
			hp.Kill(h)
			continue
		}
		if r, ok := scene.Get[Respawn](s.scene, n); ok {
			r.Trigger()
		}
	}
}

func (s *Service) checkpointOne(h scene.NodeHandle, c *Checkpoint2D) {
	if !s.scene.EffectiveEnabled(h) || (c.Once && c.st.used) {
		return
	}
	s.candidates("", candCheckpoint)
	a := areaOf(s.scene.World(h), c.Area, ShapeBox)
	for _, n := range s.enterZone(&c.st, &a) {
		r, ok := scene.Get[Respawn](s.scene, n)
		if !ok || (r.st.hasPri && r.st.priority > c.Priority) {
			continue
		}
		p := s.scene.WorldPosition(h)
		if sp, ok := c.SpawnPoint.Resolve(s.scene); ok {
			p = s.scene.WorldPosition(sp)
		}
		r.st.point, r.st.hasPoint, r.st.priority, r.st.hasPri = p, true, c.Priority, true
		c.st.used = true
		if s.world != nil && s.sig != nil {
			s.sig.reached.Emit(s.world, h, n)
		}
		if c.Once {
			return
		}
	}
}

func (s *Service) pickupOne(h scene.NodeHandle, c *Pickup2D) {
	if !s.scene.EffectiveEnabled(h) || c.st.used {
		return
	}
	s.candidates(c.Tag, candPickup)
	a := areaOf(s.scene.World(h), c.Area, ShapeBox)
	for _, n := range s.enterZone(&c.st, &a) {
		s.stats.Pickups++
		s.record(h, "pickup", "")
		if s.world != nil && s.sig != nil {
			s.sig.collected.Emit(s.world, h, n)
		}
		if c.Once {
			c.st.used = true
			s.remove(h, c.Despawn)
			return
		}
	}
}

func (s *Service) respawnOne(h scene.NodeHandle, c *Respawn) {
	st := &c.st
	if !st.hasPoint {
		st.point, st.hasPoint = s.scene.WorldPosition(h), true
	}
	if !st.waiting {
		return
	}
	st.pending -= s.dt
	if st.pending > 0 {
		return
	}
	st.waiting = false
	p := st.point
	if m, ok := scene.Get[Motion2D](s.scene, h); ok {
		if c.ResetVelocity {
			m.Teleport(p)
		} else {
			m.teleportKeep(p)
		}
	} else if q := s.queries(); q == nil || q.Teleport(h, p, 0) != nil {
		s.scene.SetPosition(h, p)
	}
	if c.RestoreHealth {
		if hp, ok := scene.Get[Health](s.scene, h); ok {
			hp.Revive(hp.Max)
		}
	}
	s.stats.Respawns++
	s.record(h, "respawn", "")
	if s.world != nil && s.sig != nil {
		s.sig.respawned.Emit(s.world, h)
	}
}

func (s *Service) lifeOne(h scene.NodeHandle, c *Lifetime) {
	if c.st.done {
		return
	}
	if !c.st.started {
		c.Remaining, c.st.started = c.Seconds, true
	}
	dt := s.game
	if c.Unscaled {
		dt = s.real
	}
	c.Remaining -= dt
	if c.Remaining <= 0 {
		c.Remaining, c.st.done = 0, true
		s.remove(h, c.Despawn)
	}
}

func registerZones(r *module.Registry, sig *signals) error {
	space := schema.Space(schema.Space2D)
	life := module.Component[Lifetime](r, schema.Name(NameLifetime), space,
		schema.Doc("Removes the node after some seconds: back to its pool or destroyed."),
		schema.FieldDoc("seconds", "Seconds the node lives."),
		schema.FieldDoc("unscaled", "Counts real time, also while paused or slowed down."),
		schema.FieldDoc("despawn", "Returns the node to its pool instead of destroying it."),
		schema.FieldDoc("remaining", "Seconds left."),
		schema.Runtime("remaining"),
	).Category(Category).Display("Lifetime").Icon("gameplay.lifetime").Default(DefaultLifetime)

	kill := module.Component[KillZone2D](r, schema.Name(NameKillZone), space,
		schema.Doc("Kills a Health that enters the area, or respawns a node with Respawn and no Health."),
		schema.FieldDoc("area", "Rectangle in this node's space."),
		schema.FieldDoc("tag", "Only nodes with this tag; empty affects every node with Health or Respawn."),
	).Category(Category).Display("Kill Zone 2D").Icon("gameplay.killzone").Default(DefaultKillZone)

	cp := module.Component[Checkpoint2D](r, schema.Name(NameCheckpoint), space,
		schema.Doc("Sets the respawn point of a node with Respawn that enters the area."),
		schema.FieldDoc("area", "Rectangle in this node's space."),
		schema.FieldDoc("priority", "A checkpoint does not replace one with a higher priority."),
		schema.FieldDoc("once", "Works for the first node only."),
		schema.FieldDoc("spawnPoint", "Node whose position becomes the respawn point. Empty uses this checkpoint's position."),
	).Category(Category).Display("Checkpoint 2D").Icon("gameplay.checkpoint").Default(DefaultCheckpoint)
	sig.reached = cp.Signal1[scene.NodeHandle]("Reached", "Reached fires when a node takes this checkpoint.", "by")

	resp := module.Component[Respawn](r, schema.Name(NameRespawn), space,
		schema.Doc("Brings the node back to its respawn point after death or Trigger. The first point is where the node starts."),
		schema.FieldDoc("delay", "Seconds of game time before the node comes back."),
		schema.FieldDoc("restoreHealth", "Revives Health with full hit points."),
		schema.FieldDoc("resetVelocity", "Stops a Motion 2D on respawn."),
		schema.FieldDoc("persist", "Saves the respawn point with the game."),
	).Category(Category).Display("Respawn").Icon("gameplay.respawn").Default(DefaultRespawn)
	resp.Proc0("Trigger", func(_ *bind.Ctx, r *Respawn) { r.Trigger() },
		bind.Doc("Trigger respawns the node after Delay."))
	resp.Proc1("SetPoint", func(_ *bind.Ctx, r *Respawn, p mathx.Vec2) { r.SetPoint(p) },
		bind.Doc("SetPoint sets the respawn point."), bind.Params("position"))
	sig.respawned = resp.Signal0("Respawned", "Respawned fires when the node is back at its point.")

	pick := module.Component[Pickup2D](r, schema.Name(NamePickup), space,
		schema.Doc("Fires Collected when a node enters the area; with Once the pickup then disappears."),
		schema.FieldDoc("area", "Rectangle in this node's space."),
		schema.FieldDoc("tag", "Only nodes with this tag; empty accepts every node with Motion 2D."),
		schema.FieldDoc("once", "Removes the pickup after the first collection."),
		schema.FieldDoc("despawn", "Returns the pickup to its pool instead of destroying it."),
	).Category(Category).Display("Pickup 2D").Icon("gameplay.pickup").Default(DefaultPickup)
	sig.collected = pick.Signal1[scene.NodeHandle]("Collected", "Collected fires with the node that took the pickup.", "by")

	return errors.Join(life.Err(), kill.Err(), cp.Err(), resp.Err(), pick.Err())
}
