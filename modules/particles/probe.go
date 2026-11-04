package particles

// ProbeCounters samples the current emitters at benchmark boundaries.
func (s *Service) ProbeCounters(dst map[string]uint64) {
	dst[ID+".spawned"] = s.Stats().Spawned
}
