package gameplay

import (
	"aego/core/mathx"
	"aego/core/rewind"
	"aego/scene"
)

var _ rewind.Keyframer = (*Service)(nil)

// noKey: the service keeps no state between steps; component state rides in the scene key.
type noKey struct{}

func (noKey) Size() int { return 0 }

func (s *Service) Capture(rewind.Key) rewind.Key {
	return noKey{}
}

func (s *Service) Restore(rewind.Key) {}

type hashFns struct {
	motion func(scene.NodeHandle, *Motion2D)
	health func(scene.NodeHandle, *Health)
	hit    func(scene.NodeHandle, *Hitbox2D)
	kill   func(scene.NodeHandle, *KillZone2D)
	cp     func(scene.NodeHandle, *Checkpoint2D)
	pick   func(scene.NodeHandle, *Pickup2D)
	resp   func(scene.NodeHandle, *Respawn)
	life   func(scene.NodeHandle, *Lifetime)
	fsm    func(scene.NodeHandle, *StateMachine)
}

func newHashFns(s *Service) hashFns {
	return hashFns{
		motion: s.hashMotion,
		health: s.hashHealth,
		hit:    s.hashHit,
		kill:   func(n scene.NodeHandle, c *KillZone2D) { s.hashZone(n, &c.st) },
		cp:     func(n scene.NodeHandle, c *Checkpoint2D) { s.hashZone(n, &c.st) },
		pick:   func(n scene.NodeHandle, c *Pickup2D) { s.hashZone(n, &c.st) },
		resp:   s.hashRespawn,
		life:   s.hashLife,
		fsm:    s.hashFSM,
	}
}

// Hash adds the unexported component state, which the scene hash cannot see.
func (s *Service) Hash(h *rewind.Hasher) {
	h.String(ServiceID)
	s.hh = h
	f := &s.hashFns
	scene.Query1[Motion2D](s.scene).Each(f.motion)
	scene.Query1[Health](s.scene).Each(f.health)
	scene.Query1[Hitbox2D](s.scene).Each(f.hit)
	scene.Query1[KillZone2D](s.scene).Each(f.kill)
	scene.Query1[Checkpoint2D](s.scene).Each(f.cp)
	scene.Query1[Pickup2D](s.scene).Each(f.pick)
	scene.Query1[Respawn](s.scene).Each(f.resp)
	scene.Query1[Lifetime](s.scene).Each(f.life)
	scene.Query1[StateMachine](s.scene).Each(f.fsm)
	s.hh = nil
}

func hashVec(h *rewind.Hasher, v mathx.Vec2) {
	h.F32(v.X)
	h.F32(v.Y)
}

func hashNode(h *rewind.Hasher, n scene.NodeHandle) {
	h.U32(n.Index)
	h.U32(n.Gen)
}

func (s *Service) hashMotion(n scene.NodeHandle, c *Motion2D) {
	h, st := s.hh, &c.st
	hashNode(h, n)
	hashVec(h, st.move)
	hashVec(h, st.dashVel)
	hashVec(h, st.addVel)
	hashVec(h, st.teleportTo)
	hashVec(h, st.floorN)
	hashVec(h, st.platVel)
	hashVec(h, st.wallN)
	h.F32(st.dashLeft)
	h.F32(st.coyote)
	h.F32(st.buffer)
	h.F32(st.landSpeed)
	hashNode(h, st.ground)
	h.U32(uint32(st.events))
	h.Bool(st.jumpReq)
	h.Bool(st.jumpRel)
	h.Bool(st.jumping)
	h.Bool(st.stop)
	h.Bool(st.teleport)
	h.Bool(st.grounded)
}

func (s *Service) hashHealth(n scene.NodeHandle, c *Health) {
	h, st := s.hh, &c.st
	hashNode(h, n)
	h.U32(uint32(st.dmg))
	h.U32(uint32(st.healed))
	hashNode(h, st.dmgSrc)
	hashNode(h, st.diedSrc)
	h.F32(st.invuln)
	h.Bool(st.ready)
	h.Bool(st.died)
	h.Bool(st.revived)
	h.Bool(st.arm)
}

func (s *Service) hashHit(n scene.NodeHandle, c *Hitbox2D) {
	h, st := s.hh, &c.st
	hashNode(h, n)
	h.Bool(st.pulse)
	h.Int(len(st.recs))
	for i := range st.recs {
		r := &st.recs[i]
		hashNode(h, r.node)
		h.F32(r.timer)
		h.Bool(r.inside)
		h.Bool(r.seen)
		h.Bool(r.hit)
	}
}

func (s *Service) hashZone(n scene.NodeHandle, z *zoneState) {
	h := s.hh
	hashNode(h, n)
	h.Bool(z.used)
	h.Int(len(z.inside))
	for _, x := range z.inside {
		hashNode(h, x)
	}
}

func (s *Service) hashRespawn(n scene.NodeHandle, c *Respawn) {
	h, st := s.hh, &c.st
	hashNode(h, n)
	hashVec(h, st.point)
	h.U32(uint32(st.priority))
	h.F32(st.pending)
	h.Bool(st.hasPoint)
	h.Bool(st.hasPri)
	h.Bool(st.waiting)
}

func (s *Service) hashLife(n scene.NodeHandle, c *Lifetime) {
	h := s.hh
	hashNode(h, n)
	h.Bool(c.st.started)
	h.Bool(c.st.done)
}

func (s *Service) hashFSM(n scene.NodeHandle, c *StateMachine) {
	h := s.hh
	hashNode(h, n)
	h.U32(uint32(c.Index))
	h.F32(c.Time)
	h.U32(uint32(c.st.queued))
	h.Bool(c.st.started)
}
