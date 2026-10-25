package anim

// RootDelta is the change of a root track between two clip times: a looping clip adds its end minus its start for every cycle crossed, a ping-pong clip only moves back and forth.
func RootDelta(c *Clip, s *Sampler, track int, loop LoopMode, t0, t1 float64) [2]float32 {
	if !(c.Length > 0) || t0 == t1 || track < 0 || track >= len(c.Tracks) {
		return [2]float32{}
	}
	u0, k0 := c.Local(loop, t0)
	u1, k1 := c.Local(loop, t1)
	a := s.Track(c, track, u0).L
	b := s.Track(c, track, u1).L
	d := [2]float32{b[0] - a[0], b[1] - a[1]}
	if loop == LoopRepeat && k1 != k0 {
		start := s.Track(c, track, 0).L
		end := s.Track(c, track, c.Length).L
		n := float32(k1 - k0)
		d[0] += n * (end[0] - start[0])
		d[1] += n * (end[1] - start[1])
	}
	return d
}
