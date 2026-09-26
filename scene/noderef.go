package scene

import "aego/core/ids"

type NodeRef struct {
	ID     ids.NodeID
	handle NodeHandle
	rev    uint64
	ok     bool
}

func NewNodeRef(id ids.NodeID) NodeRef {
	return NodeRef{ID: id}
}

func RefTo(s *Scene, h NodeHandle) NodeRef {
	return NodeRef{ID: s.ID(h), handle: h, rev: s.StructureRevision(), ok: true}
}

func (r *NodeRef) NodeRefID() ids.NodeID {
	return r.ID
}

func (r *NodeRef) SetNodeRefID(id ids.NodeID) {
	if r.ID == id {
		return
	}
	r.ID = id
	r.handle = NodeHandle{}
	r.rev = 0
	r.ok = false
}

func (r *NodeRef) IsZero() bool {
	return r.ID.IsZero()
}

func (r *NodeRef) Clear() {
	*r = NodeRef{}
}

func (r *NodeRef) Resolve(s *Scene) (NodeHandle, bool) {
	if s == nil || r.ID.IsZero() {
		return NodeHandle{}, false
	}
	if rev := s.StructureRevision(); r.ok && r.rev == rev {
		if s.IsAlive(r.handle) {
			return r.handle, true
		}
		r.ok = false
	}
	h := s.Handle(r.ID)
	if h.IsZero() || !s.IsAlive(h) {
		r.ok = false
		r.handle = NodeHandle{}
		return NodeHandle{}, false
	}
	r.handle = h
	r.rev = s.StructureRevision()
	r.ok = true
	return h, true
}

func (r *NodeRef) Set(s *Scene, h NodeHandle) {
	if s == nil || !s.IsAlive(h) {
		r.Clear()
		return
	}
	r.ID = s.ID(h)
	r.handle = h
	r.rev = s.StructureRevision()
	r.ok = true
}
