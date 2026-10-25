package scene

import "aego/core/mathx"

type presEntry struct {
	owner NodeHandle
	local Transform2D
}

func (s *Scene) SetPresentationLocal(h NodeHandle, t Transform2D) {
	n, ok := s.node(h)
	if !ok || n.space != Space2D {
		return
	}
	if n.pres > 0 {
		s.pres[n.pres-1].local = t
	} else {
		s.pres = append(s.pres, presEntry{owner: h, local: t})
		n.pres = int32(len(s.pres))
	}
	s.presDirty = true
}

func (s *Scene) PresentationLocal(h NodeHandle) (Transform2D, bool) {
	n, ok := s.nodes.Get(h)
	if !ok || n.pres == 0 {
		return Transform2D{}, false
	}
	return s.pres[n.pres-1].local, true
}

func (s *Scene) DropPresentationLocal(h NodeHandle) {
	n, ok := s.nodes.Get(h)
	if !ok || n.pres == 0 {
		return
	}
	i, last := int(n.pres-1), len(s.pres)-1
	if i != last {
		s.pres[i] = s.pres[last]
		if m, ok := s.nodes.Get(s.pres[i].owner); ok && m.pres == int32(last+1) {
			m.pres = int32(i + 1)
		}
	}
	s.pres = s.pres[:last]
	n.pres = 0
	s.presDirty = true
}

func (s *Scene) ClearPresentation() {
	if len(s.pres) == 0 {
		return
	}
	for i := range s.pres {
		if n, ok := s.nodes.Get(s.pres[i].owner); ok && n.pres == int32(i+1) {
			n.pres = 0
		}
	}
	s.pres = s.pres[:0]
	s.presDirty = true
}

func (s *Scene) HasPresentation() bool {
	return len(s.pres) > 0
}

func (s *Scene) RenderWorld(h NodeHandle) mathx.Mat3 {
	n, ok := s.nodes.Get(h)
	if !ok {
		return mathx.Mat3Identity()
	}
	if n.has(flagWorldDirty) || s.presDirty {
		s.UpdateTransforms()
		n, _ = s.nodes.Get(h)
	}
	if n.rslot > 0 {
		return s.render[n.rslot-1]
	}
	return n.world
}

func (s *Scene) renderOf(n, p *nodeData, parent3 bool) {
	if n.space != Space2D || (n.pres == 0 && p.rslot == 0) {
		n.rslot = 0
		return
	}
	local := n.local
	if n.pres > 0 {
		local = s.pres[n.pres-1].local
	}
	m := local.Matrix()
	if !parent3 {
		base := p.world
		if p.rslot > 0 {
			base = s.render[p.rslot-1]
		}
		m = base.Mul(m)
	}
	s.render = append(s.render, m)
	n.rslot = int32(len(s.render))
}
