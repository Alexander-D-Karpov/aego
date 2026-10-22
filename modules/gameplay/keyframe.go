package gameplay

import (
	"aego/core/mathx"
	"aego/core/rewind"
	"aego/scene"
)

var _ rewind.Keyframer = (*Service)(nil)

// noKey: the service keeps no state between steps; the controllers' state rides in the scene key.
type noKey struct{}

func (noKey) Size() int { return 0 }

func (s *Service) Capture(rewind.Key) rewind.Key {
	return noKey{}
}

func (s *Service) Restore(rewind.Key) {}

// Hash adds the unexported controller state, which the scene hash cannot see.
func (s *Service) Hash(h *rewind.Hasher) {
	h.String(ServiceID)
	s.hh = h
	scene.Query1[Motion2D](s.scene).Each(s.hashFn)
	s.hh = nil
}

func hashVec(h *rewind.Hasher, v mathx.Vec2) {
	h.F32(v.X)
	h.F32(v.Y)
}

func (s *Service) hashOne(n scene.NodeHandle, c *Motion2D) {
	h, st := s.hh, &c.st
	h.U32(n.Index)
	h.U32(n.Gen)
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
	h.U32(st.ground.Index)
	h.U32(st.ground.Gen)
	h.U32(uint32(st.events))
	for _, b := range [...]bool{st.jumpReq, st.jumpRel, st.jumping, st.stop, st.teleport, st.grounded} {
		h.Bool(b)
	}
}
