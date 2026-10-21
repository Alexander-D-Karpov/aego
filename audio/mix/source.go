package mix

import "sync/atomic"

const streamCache = 1<<16 + 16

type Memory struct {
	PCM       []float32
	Rate      int
	Channels  int
	LoopStart int64
	LoopEnd   int64
	Loop      bool
}

func (m *Memory) Frames() int64 {
	if m.Channels <= 0 {
		return 0
	}
	return int64(len(m.PCM) / m.Channels)
}

func (m *Memory) loopRange() (int64, int64) {
	n := m.Frames()
	s, e := m.LoopStart, m.LoopEnd
	if e <= 0 || e > n {
		e = n
	}
	if s < 0 || s >= e {
		s = 0
	}
	return s, e
}

type Stream struct {
	Ring     *Ring
	Rate     int
	Channels int
	eof      atomic.Bool
	cache    []float32
	start    int64
	n        int
	inUse    bool
}

func NewStream(rate, channels, ringFrames int) *Stream {
	if channels != 2 {
		channels = 1
	}
	return &Stream{
		Ring: NewRing(ringFrames, channels), Rate: rate, Channels: channels,
		cache: make([]float32, streamCache*channels),
	}
}

func (s *Stream) SetEOF() {
	s.eof.Store(true)
}

func (s *Stream) EOF() bool {
	return s.eof.Load()
}

func (s *Stream) reset() {
	s.start, s.n = -1, 1
	clear(s.cache[:s.Channels])
}

func (s *Stream) fill(pos, step float64, frames int) {
	ch := s.Channels
	if d := int64(pos) - 1 - s.start; d > 0 {
		k := int(min(d, int64(s.n)))
		copy(s.cache, s.cache[k*ch:s.n*ch])
		s.n -= k
		s.start += int64(k)
	}
	need := min(int(int64(pos+step*float64(frames))+3-s.start), streamCache)
	if s.n < need {
		s.n += s.Ring.Read(s.cache[s.n*ch : need*ch])
	}
}

func (s *Stream) at(k, c int) float32 {
	if k < 0 || k >= s.n {
		return 0
	}
	return s.cache[k*s.Channels+c]
}
