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
	skipped  atomic.Int64
	hold     atomic.Bool
	wake     chan struct{}
	space    chan struct{}
	waiting  atomic.Bool
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
		cache: make([]float32, streamCache*channels), wake: make(chan struct{}, 1), space: make(chan struct{}, 1),
	}
}

const spaceShare = 4

// WaitSpace waits until the mixer has read a quarter of the ring, a skip or a hold arrives, or cancel closes; false means cancel.
func (s *Stream) WaitSpace(cancel <-chan struct{}) bool {
	s.waiting.Store(true)
	defer s.waiting.Store(false)
	if s.Ring.Free() >= s.Ring.Cap()/spaceShare {
		return true
	}
	select {
	case <-cancel:
		return false
	case <-s.space:
	case <-s.wake:
	}
	return true
}

func (s *Stream) drained() {
	if s.waiting.Load() && s.Ring.Free() >= s.Ring.Cap()/spaceShare {
		select {
		case s.space <- struct{}{}:
		default:
		}
	}
}

// TakeSkip returns the source frames the feeder must skip before it decodes again, and clears them.
func (s *Stream) TakeSkip() int64 {
	return s.skipped.Swap(0)
}

// Held reports that the mixer skips the stream, so the feeder keeps its position and decodes nothing.
func (s *Stream) Held() bool {
	return s.hold.Load()
}

// Wake receives when the mixer adds a skip or releases the hold.
func (s *Stream) Wake() <-chan struct{} {
	return s.wake
}

func (s *Stream) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Stream) release() {
	if s.hold.Swap(false) {
		s.signal()
	}
}

// skip moves a voice to "to" without rendering: decoded frames are dropped from the cache and the ring, the rest is left to the feeder; it reports a stream that ended.
func (s *Stream) skip(to float64) bool {
	ch := s.Channels
	want := int64(to) - 1
	if want <= s.start {
		return false
	}
	if k := int(min(want-s.start, int64(s.n))); k > 0 {
		copy(s.cache, s.cache[k*ch:s.n*ch])
		s.n -= k
		s.start += int64(k)
	}
	if rest := want - s.start; rest > 0 {
		eof := s.eof.Load()
		got := s.Ring.Discard(rest)
		s.start += got
		if rest -= got; rest > 0 {
			if eof && s.Ring.Len() == 0 {
				return true
			}
			s.skipped.Add(rest)
			s.start += rest
		}
	}
	s.hold.Store(true)
	s.signal()
	return false
}

func (s *Stream) SetEOF() {
	s.eof.Store(true)
}

func (s *Stream) EOF() bool {
	return s.eof.Load()
}

func (s *Stream) reset() {
	s.skipped.Store(0)
	s.hold.Store(false)
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
		s.drained()
	}
}

func (s *Stream) at(k, c int) float32 {
	if k < 0 || k >= s.n {
		return 0
	}
	return s.cache[k*s.Channels+c]
}
