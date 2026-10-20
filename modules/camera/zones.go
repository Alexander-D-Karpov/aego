package camera

import "aego/scene"

func (s *Service) zoneFor(h scene.NodeHandle) *zone {
	for i := range s.zones {
		if s.zones[i].node == h {
			return &s.zones[i]
		}
	}
	s.zones = append(s.zones, zone{node: h})
	return &s.zones[len(s.zones)-1]
}

func (s *Service) zoneCamera(h scene.NodeHandle, z *CameraZone2D) scene.NodeHandle {
	if vc, ok := z.Camera.Resolve(s.scene); ok && s.scene.IsAlive(vc) && scene.Has[VirtualCamera2D](s.scene, vc) {
		return vc
	}
	if z.Camera.IsZero() && scene.Has[VirtualCamera2D](s.scene, h) {
		return h
	}
	return scene.NodeHandle{}
}

func (s *Service) zoneTarget(z *CameraZone2D, vc scene.NodeHandle) (scene.NodeHandle, bool) {
	if t, ok := z.Target.Resolve(s.scene); ok {
		return t, s.scene.IsAlive(t)
	}
	f, ok := scene.Get[CameraFollow2D](s.scene, vc)
	if !ok {
		return scene.NodeHandle{}, false
	}
	t, ok := f.Target.Resolve(s.scene)
	return t, ok && s.scene.IsAlive(t)
}

func (s *Service) updateZones() {
	for i := range s.zones {
		s.zones[i].seen = false
	}
	scene.Query1[CameraZone2D](s.scene).Each(func(h scene.NodeHandle, z *CameraZone2D) {
		vc := s.zoneCamera(h, z)
		in := false
		if !vc.IsZero() && s.scene.EffectiveEnabled(h) && !empty(z.Area) {
			if t, ok := s.zoneTarget(z, vc); ok {
				in = zoneContains(s.scene.World(h), z.Area, s.scene.WorldPosition(t))
			}
		}
		st := s.zoneFor(h)
		st.seen, st.vc = true, vc
		if in && !st.inside {
			s.seq++
			st.seq = s.seq
		}
		st.inside = in
	})
	n := 0
	for _, z := range s.zones {
		if z.seen {
			s.zones[n] = z
			n++
		}
	}
	s.zones = s.zones[:n]
}
