package gameplay

import (
	"math"

	"aego/contracts/physics2d"
	"aego/contracts/scriptcall"
	"aego/core/log"
	"aego/core/mathx"
	"aego/core/rewind"
	"aego/engine"
	"aego/scene"
	"aego/script/vm"
)

const eps2 = 1e-12

type hitKind uint8

const (
	hitFloor hitKind = iota
	hitWall
	hitCeiling
)

type pass struct {
	h      scene.NodeHandle
	c      *Motion2D
	f      physics2d.QueryFilter
	up     mathx.Vec2
	right  mathx.Vec2
	cosMax float32
	margin float32
	floor  bool
	wall   bool
	ceil   bool
	floorN mathx.Vec2
	wallN  mathx.Vec2
	ground scene.NodeHandle
}

type Stats struct {
	Hits     uint64
	Deaths   uint64
	Respawns uint64
	Pickups  uint64
}

type Service struct {
	scene      *scene.Scene
	world      *engine.World
	q          physics2d.Queries
	sig        *signals
	log        *log.ChannelLogger
	dt         float32
	g          float32
	game       float32
	real       float32
	p          pass
	pen        [4]physics2d.CastHit
	hurts      []hurtRec
	cand       []scene.NodeHandle
	entered    []scene.NodeHandle
	stats      Stats
	hh         *rewind.Hasher
	call       scriptcall.Caller
	fdt        float32
	args       [1]vm.Value
	fsmFixFn   func(scene.NodeHandle, *StateMachine)
	fsmUpdFn   func(scene.NodeHandle, *StateMachine)
	stepFn     func(scene.NodeHandle, *Motion2D)
	postFn     func(scene.NodeHandle, *Motion2D)
	hurtFn     func(scene.NodeHandle, *Hurtbox2D)
	hitFn      func(scene.NodeHandle, *Hitbox2D)
	killFn     func(scene.NodeHandle, *KillZone2D)
	cpFn       func(scene.NodeHandle, *Checkpoint2D)
	pickFn     func(scene.NodeHandle, *Pickup2D)
	respFn     func(scene.NodeHandle, *Respawn)
	hpFixFn    func(scene.NodeHandle, *Health)
	hpFn       func(scene.NodeHandle, *Health)
	lifeFn     func(scene.NodeHandle, *Lifetime)
	candHealth func(scene.NodeHandle, *Health)
	candResp   func(scene.NodeHandle, *Respawn)
	candMotion func(scene.NodeHandle, *Motion2D)
	hashFns    hashFns
}

func NewService(s *scene.Scene, w *engine.World, q physics2d.Queries) *Service {
	svc := &Service{scene: s, world: w, q: q}
	svc.stepFn = svc.stepOne
	svc.postFn = svc.postOne
	svc.hurtFn = svc.collectHurt
	svc.hitFn = svc.hitOne
	svc.killFn = svc.killOne
	svc.cpFn = svc.checkpointOne
	svc.pickFn = svc.pickupOne
	svc.respFn = svc.respawnOne
	svc.hpFixFn = svc.healthFixed
	svc.hpFn = svc.flushHealth
	svc.lifeFn = svc.lifeOne
	svc.fsmFixFn = svc.fsmFixed
	svc.fsmUpdFn = svc.fsmUpdate
	svc.candHealth = func(h scene.NodeHandle, _ *Health) { svc.cand = append(svc.cand, h) }
	svc.candResp = func(h scene.NodeHandle, _ *Respawn) {
		if !scene.Has[Health](svc.scene, h) {
			svc.cand = append(svc.cand, h)
		}
	}
	svc.candMotion = func(h scene.NodeHandle, _ *Motion2D) { svc.cand = append(svc.cand, h) }
	svc.hashFns = newHashFns(svc)
	return svc
}

// queries finds the physics queries lazily: physics may attach to the world after this module.
func (s *Service) queries() physics2d.Queries {
	if s.q == nil && s.world != nil {
		s.q, _ = engine.ServiceOf[physics2d.Queries](s.world, physics2d.ServiceID)
	}
	return s.q
}

func (s *Service) Stats() Stats {
	return s.stats
}

func (s *Service) Fixed(dt float32) {
	s.dt = dt
	q := s.queries()
	if q == nil || dt <= 0 {
		return
	}
	s.g = q.Gravity().Len()
	scene.Query1[Motion2D](s.scene).Each(s.stepFn)
}

func (s *Service) Post() {
	scene.Query1[Motion2D](s.scene).Each(s.postFn)
	if s.dt <= 0 {
		return
	}
	s.hurts = s.hurts[:0]
	scene.Query1[Hurtbox2D](s.scene).Each(s.hurtFn)
	scene.Query1[Hitbox2D](s.scene).Each(s.hitFn)
	scene.Query1[KillZone2D](s.scene).Each(s.killFn)
	scene.Query1[Checkpoint2D](s.scene).Each(s.cpFn)
	scene.Query1[Pickup2D](s.scene).Each(s.pickFn)
	scene.Query1[Respawn](s.scene).Each(s.respFn)
	scene.Query1[Health](s.scene).Each(s.hpFixFn)
}

func (s *Service) Frame(game, real float32) {
	s.game, s.real = game, real
	scene.Query1[Health](s.scene).Each(s.hpFn)
	scene.Query1[Lifetime](s.scene).Each(s.lifeFn)
}

func normalize(v mathx.Vec2) (mathx.Vec2, float32) {
	l := v.Len()
	if l < 1e-6 {
		return mathx.Vec2{}, 0
	}
	return v.Scale(1 / l), l
}

func approach(v, target, d float32) float32 {
	if v < target {
		return min(v+d, target)
	}
	return max(v-d, target)
}

func approach2(v, target mathx.Vec2, d float32) mathx.Vec2 {
	diff, l := normalize(target.Sub(v))
	if l <= d {
		return target
	}
	return v.Add(diff.Scale(d))
}

func jumpSpeed(c *Motion2D, g float32) float32 {
	if c.JumpSpeed > 0 {
		return c.JumpSpeed
	}
	return float32(math.Sqrt(float64(2 * g * c.GravityScale * c.JumpHeight)))
}

func (p *pass) kind(n mathx.Vec2) hitKind {
	if p.c.Mode == TopDown {
		return hitWall
	}
	switch d := n.Dot(p.up); {
	case d >= p.cosMax:
		return hitFloor
	case d <= -p.cosMax:
		return hitCeiling
	}
	return hitWall
}

func (p *pass) note(k hitKind, n mathx.Vec2, node scene.NodeHandle) {
	switch k {
	case hitFloor:
		p.floor, p.floorN, p.ground = true, n, node
	case hitCeiling:
		p.ceil = true
	default:
		p.wall, p.wallN = true, n
	}
}

func (s *Service) cast(from, delta mathx.Vec2) (physics2d.CastHit, bool) {
	if delta.LenSq() < eps2 {
		return physics2d.CastHit{}, false
	}
	return s.q.CastBody(s.p.h, from, delta, s.p.f)
}

func (s *Service) warn(h scene.NodeHandle, c *Motion2D, err error) {
	if c.st.warned || s.log == nil {
		return
	}
	c.st.warned = true
	s.log.Warn(CodeNotKinematic+": Motion 2D needs a kinematic Body2D on its node", log.F("node", s.scene.Path(h)), log.F("error", err.Error()))
}

func (s *Service) stepOne(h scene.NodeHandle, c *Motion2D) {
	if !s.scene.EffectiveEnabled(h) {
		return
	}
	pos, ang, ok := s.q.BodyPose(h)
	if !ok {
		return
	}
	st := &c.st
	if st.teleport {
		to := st.teleportTo
		*st = motionState{move: st.move, warned: st.warned}
		c.Velocity, c.Grounded, c.OnWall, c.OnCeiling = mathx.Vec2{}, false, false, false
		c.GroundBody = scene.NodeRef{}
		if err := s.q.Teleport(h, to, ang); err != nil {
			s.warn(h, c, err)
		}
		return
	}
	up, _ := normalize(c.UpDirection)
	if up == (mathx.Vec2{}) {
		up = mathx.Vec2{Y: -1}
	}
	s.p = pass{
		h: h, c: c, up: up, right: mathx.Vec2{X: -up.Y, Y: up.X},
		cosMax: mathx.Cos(mathx.Deg2Rad(c.MaxSlope)), margin: max(c.SafeMargin, 0.001),
		f: physics2d.QueryFilter{LayerMask: uint32(c.CollisionMask), SkipDynamic: c.PushBodies, ExcludeBody: h},
	}
	dt := s.dt
	var carry mathx.Vec2
	if c.MovingPlatforms && st.grounded && !st.ground.IsZero() && s.scene.IsAlive(st.ground) {
		if pv, _, ok := s.q.BodyVelocity(st.ground); ok {
			st.platVel, carry = pv, pv.Scale(dt)
		}
	}
	prevUp := c.Velocity.Dot(up)
	v, jumped := c.Velocity, false
	if c.Mode == TopDown {
		v = s.topDown(c, v, dt)
	} else {
		v, jumped = s.platformer(c, v, dt)
	}
	if st.dashLeft > 0 {
		v = st.dashVel
		if c.Mode == Platformer && c.DashGravity {
			v = v.Sub(up.Scale(s.g * c.GravityScale * dt))
		}
		st.dashLeft -= dt
	}
	v = v.Add(st.addVel)
	st.addVel = mathx.Vec2{}
	if st.stop {
		v, st.stop, st.dashLeft = mathx.Vec2{}, false, 0
	}
	off := s.depenetrate()
	off = s.slide(off, v.Scale(dt).Add(carry), &v)
	off = s.snap(off, jumped)
	s.finish(h, c, pos, ang, off, v, prevUp, jumped)
}

func (s *Service) platformer(c *Motion2D, v mathx.Vec2, dt float32) (mathx.Vec2, bool) {
	p, st := &s.p, &c.st
	grounded := st.grounded
	if grounded {
		st.coyote = c.CoyoteTime
	} else {
		st.coyote -= dt
	}
	if st.jumpReq {
		st.buffer, st.jumpReq = max(c.JumpBuffer, dt*0.5), false
	}
	tangent := p.right
	if grounded && st.floorN != (mathx.Vec2{}) {
		tangent = mathx.Vec2{X: -st.floorN.Y, Y: st.floorN.X}
		if tangent.Dot(p.right) < 0 {
			tangent = tangent.Neg()
		}
	}
	vx, vy := v.Dot(tangent), v.Dot(p.up)
	target := st.move.X * c.MaxSpeed
	rate := c.Decel
	if target != 0 && (vx*target <= 0 || mathx.Abs(target) > mathx.Abs(vx)) {
		rate = c.Accel
	}
	if !grounded {
		rate *= c.AirControl
	}
	vx = approach(vx, target, rate*dt)
	jumped := false
	if st.buffer > 0 && (grounded || st.coyote > 0) {
		vy, jumped, st.jumping = jumpSpeed(c, s.g), true, true
		st.buffer, st.coyote = 0, 0
		st.events |= evJumped
	}
	st.buffer -= dt
	if grounded && !jumped {
		st.jumpRel = false
		return tangent.Scale(vx), false
	}
	if st.dashLeft <= 0 {
		vy -= s.g * c.GravityScale * dt
	}
	if c.MaxFallSpeed > 0 {
		vy = max(vy, -c.MaxFallSpeed)
	}
	if st.jumpRel && st.jumping && vy > 0 {
		vy *= c.JumpCut
		st.jumping = false
	}
	if vy <= 0 {
		st.jumping = false
	}
	st.jumpRel = false
	return p.right.Scale(vx).Add(p.up.Scale(vy)), jumped
}

func (s *Service) topDown(c *Motion2D, v mathx.Vec2, dt float32) mathx.Vec2 {
	dir := c.st.move
	if l := dir.Len(); l > 1 {
		dir = dir.Scale(1 / l)
	}
	target := dir.Scale(c.MaxSpeed)
	rate := c.Decel
	if target.LenSq() > 0 {
		rate = c.Accel
	}
	return approach2(v, target, rate*dt)
}

func (s *Service) depenetrate() mathx.Vec2 {
	p := &s.p
	var off mathx.Vec2
	n := s.q.Penetration(p.h, p.f, s.pen[:])
	for _, hit := range s.pen[:n] {
		if need := hit.Depth + p.margin - off.Dot(hit.Normal); need > 0 {
			off = off.Add(hit.Normal.Scale(need))
		}
	}
	return off
}

func (s *Service) slide(off, move mathx.Vec2, v *mathx.Vec2) mathx.Vec2 {
	p := &s.p
	for i := int32(0); i < p.c.MaxSlides && move.LenSq() > eps2; i++ {
		hit, ok := s.cast(off, move)
		if !ok {
			return off.Add(move)
		}
		n := hit.Normal
		if hit.Initial {
			off = off.Add(n.Scale(hit.Depth + p.margin))
			if d := move.Dot(n); d < 0 {
				move = move.Sub(n.Scale(d))
			}
			continue
		}
		dir, l := normalize(move)
		t := max(hit.Fraction*l-p.margin, 0)
		off = off.Add(dir.Scale(t))
		rest := dir.Scale(l - t)
		k := p.kind(n)
		if k == hitWall && s.stepUp(&off, rest) {
			return off
		}
		p.note(k, n, hit.Node)
		if d := rest.Dot(n); d < 0 {
			rest = rest.Sub(n.Scale(d))
		}
		if d := v.Dot(n); d < 0 {
			*v = v.Sub(n.Scale(d))
		}
		move = rest
	}
	return off
}

func (s *Service) stepUp(off *mathx.Vec2, rest mathx.Vec2) bool {
	p := &s.p
	c := p.c
	if c.Mode != Platformer || c.StepHeight <= 0 || !c.st.grounded {
		return false
	}
	fwd := p.right.Scale(rest.Dot(p.right))
	fl := fwd.Len()
	if fl < 1e-6 {
		return false
	}
	lift := p.up.Scale(c.StepHeight)
	if hit, ok := s.cast(*off, lift); ok {
		lift = lift.Scale(max(hit.Fraction-p.margin/c.StepHeight, 0))
	}
	o := off.Add(lift)
	if hit, ok := s.cast(o, fwd); ok {
		t := max(hit.Fraction*fl-p.margin, 0)
		if hit.Initial || t < p.margin {
			return false
		}
		fwd = fwd.Scale(t / fl)
	}
	o = o.Add(fwd)
	dl := lift.Len() + 2*p.margin
	drop := p.up.Scale(-dl)
	hit, ok := s.cast(o, drop)
	if !ok || hit.Initial || p.kind(hit.Normal) != hitFloor {
		return false
	}
	*off = o.Add(drop.Scale(max(hit.Fraction*dl-p.margin, 0) / dl))
	p.note(hitFloor, hit.Normal, hit.Node)
	return true
}

func (s *Service) snap(off mathx.Vec2, jumped bool) mathx.Vec2 {
	p := &s.p
	c := p.c
	if c.Mode != Platformer || p.floor || jumped || !c.st.grounded {
		return off
	}
	dist := max(c.SnapToGround, 2*p.margin)
	down := p.up.Scale(-dist)
	hit, ok := s.cast(off, down)
	if !ok || hit.Initial || p.kind(hit.Normal) != hitFloor {
		return off
	}
	p.note(hitFloor, hit.Normal, hit.Node)
	return off.Add(down.Scale(max(hit.Fraction*dist-p.margin, 0) / dist))
}

func (s *Service) finish(h scene.NodeHandle, c *Motion2D, pos mathx.Vec2, ang float32, off, v mathx.Vec2, prevUp float32, jumped bool) {
	p, st := &s.p, &c.st
	grounded := p.floor && !jumped
	if grounded && !st.grounded {
		st.events |= evLanded
		st.landSpeed = max(-prevUp, 0)
	}
	if !grounded && st.grounded {
		if !jumped {
			st.events |= evLeft
		}
		switch c.PlatformOnLeave {
		case LeaveKeep:
			v = v.Add(st.platVel)
		case LeaveAdd:
			v = v.Add(p.right.Scale(st.platVel.Dot(p.right)))
		}
		st.platVel = mathx.Vec2{}
	}
	if p.wall && !c.OnWall {
		st.events |= evWall
		st.wallN = p.wallN
	}
	if p.ceil && !c.OnCeiling {
		st.events |= evCeiling
	}
	if grounded {
		if d := v.Dot(p.floorN); d < 0 {
			v = v.Sub(p.floorN.Scale(d))
		}
		st.floorN, st.ground = p.floorN, p.ground
		c.FloorNormal = p.floorN
		c.FloorAngle = mathx.Rad2Deg(float32(math.Acos(float64(mathx.Clamp(p.floorN.Dot(p.up), -1, 1)))))
		c.GroundBody.Set(s.scene, p.ground)
	} else {
		st.floorN, st.ground = mathx.Vec2{}, scene.NodeHandle{}
		c.FloorNormal, c.FloorAngle, c.GroundBody = mathx.Vec2{}, 0, scene.NodeRef{}
	}
	st.grounded = grounded
	c.Velocity, c.Grounded, c.OnWall, c.OnCeiling, c.WallNormal = v, grounded, p.wall, p.ceil, p.wallN
	angle := ang
	if c.Mode == TopDown && c.FaceMovement && v.LenSq() > 1e-4 {
		angle = mathx.Atan2(v.Y, v.X)
	}
	if err := s.q.MoveKinematic(h, pos.Add(off), angle, s.dt); err != nil {
		s.warn(h, c, err)
	}
}

func (s *Service) postOne(h scene.NodeHandle, c *Motion2D) {
	st := &c.st
	ev := st.events
	if ev == 0 {
		return
	}
	st.events = 0
	if s.world == nil || s.sig == nil {
		return
	}
	if ev&evJumped != 0 {
		s.sig.jumped.Emit(s.world, h)
	}
	if ev&evLeft != 0 {
		s.sig.left.Emit(s.world, h)
	}
	if ev&evLanded != 0 {
		s.sig.landed.Emit(s.world, h, st.landSpeed)
	}
	if ev&evWall != 0 {
		s.sig.wall.Emit(s.world, h, st.wallN)
	}
	if ev&evCeiling != 0 {
		s.sig.ceiling.Emit(s.world, h)
	}
}
