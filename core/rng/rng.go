package rng

import "time"

type Source struct {
	state uint64
	seed  uint64
}

func New(seed uint64) *Source {
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	return &Source{state: seed, seed: seed}
}

func (s *Source) Seed() uint64 {
	return s.seed
}

func (s *Source) Reset() {
	s.state = s.seed
}

func (s *Source) Uint64() uint64 {
	s.state += 0x9e3779b97f4a7c15
	z := s.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (s *Source) Uint32() uint32 {
	return uint32(s.Uint64() >> 32)
}

func (s *Source) Float32() float32 {
	return float32(s.Uint64()>>40) / float32(1<<24)
}

func (s *Source) Float64() float64 {
	return float64(s.Uint64()>>11) / float64(1<<53)
}

func (s *Source) IntN(n int) int {
	if n <= 0 {
		return 0
	}
	return int(s.Uint64() % uint64(n))
}

func (s *Source) Range(lo, hi float32) float32 {
	return lo + (hi-lo)*s.Float32()
}

func (s *Source) State() uint64 {
	return s.state
}

func (s *Source) SetState(v uint64) {
	s.state = v
}
